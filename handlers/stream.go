package handlers

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/models"
	"github.com/val-makkas/absolute-cinema/torrentstream/utils"
)

// StreamTorrentFile handles GET /stream/:infohash/:file_idx requests
func StreamTorrentFile(c *gin.Context, store *models.TorrentStore) {
	infoHash := c.Param("infohash")
	fileIdxStr := c.Param("file_idx")
	fmt.Printf("Received /stream request for torrent: %s, file index: %s\n", infoHash, fileIdxStr)

	// Convert fileIdx to int
	fileIdx, err := strconv.Atoi(fileIdxStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file index"})
		return
	}

	// Get the torrent from our map
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[infoHash]
	store.Mutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}

	// Wait for torrent metadata if not yet available
	metadataTimeout := time.After(10 * time.Second)
	select {
	case <-torrentFile.GotInfo():
		// Got metadata, proceed
	case <-metadataTimeout:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Timed out waiting for torrent metadata"})
		return
	}

	// Make sure info is available
	if torrentFile.Info() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Torrent info not available"})
		return
	}

	// Check if files exist in the torrent
	files := torrentFile.Files()
	if len(files) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Torrent does not have any files to stream"})
		return
	}

	// Validate file index
	if fileIdx < 0 || fileIdx >= len(files) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File index out of range"})
		return
	}

	file := files[fileIdx]
	fileLength := file.Length()

	// Log stream initialization
	fmt.Printf("Starting stream for %s, file index %d: %s (size: %d bytes)\n", infoHash, fileIdx, file.Path(), fileLength)

	// Increase the read-ahead window for this specific file to improve streaming
	file.SetPriority(torrent.PiecePriorityNow)

	// Set a larger buffer for better streaming (reduced buffering issues)
	const bufferSize = 2 * 1024 * 1024 // 2MB buffer for streaming

	// Calculate piece range for the file for prioritization
	pieceLength := torrentFile.Info().PieceLength
	firstPieceOfFile := int(file.Offset() / pieceLength)
	lastPieceOfFile := int((file.Offset() + file.Length() - 1) / pieceLength)

	// Prioritize the first several pieces more aggressively for fast start
	// The number of pieces to prioritize upfront is calculated based on buffer size
	piecesToPrioritize := int(bufferSize / pieceLength)
	if piecesToPrioritize < 1 {
		piecesToPrioritize = 1
	}

	endPieceToHighPrioritize := firstPieceOfFile + piecesToPrioritize
	if endPieceToHighPrioritize > lastPieceOfFile {
		endPieceToHighPrioritize = lastPieceOfFile
	}

	fmt.Printf("High prioritizing first %d pieces for file %s\n", piecesToPrioritize, file.Path())
	for i := firstPieceOfFile; i <= endPieceToHighPrioritize; i++ {
		if i >= 0 && i < torrentFile.NumPieces() {
			torrentFile.Piece(i).SetPriority(torrent.PiecePriorityNow)
		}
	}

	// Set up HTTP response
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(file.Path())))
	c.Writer.Header().Set("Content-Type", utils.GetContentType(file.Path()))
	c.Writer.Header().Set("Content-Length", strconv.FormatInt(fileLength, 10))
	c.Writer.Header().Set("Accept-Ranges", "bytes")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Add useful headers for video streaming
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
	c.Writer.Header().Set("X-Frame-Options", "SAMEORIGIN")
	c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")

	// Handle range requests
	rangeHeader := c.GetHeader("Range")
	var start, end int64 = 0, fileLength - 1

	if rangeHeader != "" {
		fmt.Printf("Range header received: %s\n", rangeHeader)
		// Parse Range: bytes=start-end
		if strings.HasPrefix(rangeHeader, "bytes=") {
			rangeStr := strings.TrimPrefix(rangeHeader, "bytes=")
			rangeParts := strings.Split(rangeStr, "-")

			if len(rangeParts) == 2 {
				if rangeParts[0] != "" {
					start, _ = strconv.ParseInt(rangeParts[0], 10, 64)
				}

				if rangeParts[1] != "" {
					end, _ = strconv.ParseInt(rangeParts[1], 10, 64)
				}
			}
		}

		// Validate range
		if start < 0 {
			start = 0
		}

		if end >= fileLength {
			end = fileLength - 1
		}

		if start > end {
			c.JSON(http.StatusRequestedRangeNotSatisfiable, gin.H{"error": "Invalid range"})
			return
		}

		// Set headers for range response
		c.Writer.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileLength))
		c.Writer.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		c.Writer.WriteHeader(http.StatusPartialContent)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	// Create reader for the file
	reader := file.NewReader()
	defer reader.Close()
	fmt.Printf("Created reader for file %s\n", file.Path())

	var seekErr error
	_, seekErr = reader.Seek(start, io.SeekStart)
	if seekErr != nil {
		fmt.Printf("Error seeking in file %s reader to %d: %v\n", file.Path(), start, seekErr)
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to seek file: %w", seekErr))
		return
	}
	fmt.Printf("Seek successful to byte %d\n", start)

	// Copy the file to the response
	bytesToStream := end - start + 1
	bytesStreamed := int64(0)

	const copyBufferSize = 32 * 1024 // 32KB copy buffer
	buffer := make([]byte, copyBufferSize)

	for bytesStreamed < bytesToStream {
		// Safety check
		if bytesStreamed >= bytesToStream {
			break
		}

		// Calculate how many bytes to read in this iteration
		bytesLeft := bytesToStream - bytesStreamed
		bytesThisTime := bytesLeft
		if bytesThisTime > int64(len(buffer)) {
			bytesThisTime = int64(len(buffer))
		}

		// Read data
		readBytes, readErr := reader.Read(buffer[:bytesThisTime])
		if readErr != nil && readErr != io.EOF {
			fmt.Printf("Error reading from file %s reader at position %d: %v\n", file.Path(), start+bytesStreamed, readErr)
			// Don't abort - we might have already sent some data
			break
		}

		// Write data to response
		if readBytes > 0 {
			_, writeErr := c.Writer.Write(buffer[:readBytes])
			if writeErr != nil {
				fmt.Printf("Error writing to response for file %s at position %d: %v\n", file.Path(), start+bytesStreamed, writeErr)
				// Client probably disconnected
				break
			}
			bytesStreamed += int64(readBytes)

			// Flush the writer to ensure data is sent immediately
			c.Writer.Flush()
		}

		// Check if we're done
		if readBytes == 0 || readErr == io.EOF {
			break
		}
	}

	fmt.Printf("Finished streaming file %s, streamed %d/%d bytes\n", file.Path(), bytesStreamed, bytesToStream)
}
