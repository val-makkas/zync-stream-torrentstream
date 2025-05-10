package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/models"
)

// PrioritizePieces handles POST /prioritize requests to prioritize pieces at a specific position
func PrioritizePieces(c *gin.Context, store *models.TorrentStore, requestedPieces *models.RequestedPieces) {
	// Parse request body
	var req models.PrioritizeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Get torrent
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[req.InfoHash]
	store.Mutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}

	// Wait for torrent info
	if torrentFile.Info() == nil {
		select {
		case <-torrentFile.GotInfo():
			// Got info, proceed
		case <-time.After(3 * time.Second):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Waiting for torrent metadata"})
			return
		}
	}

	files := torrentFile.Files()
	if req.FileIdx < 0 || req.FileIdx >= len(files) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File index out of range"})
		return
	}

	file := files[req.FileIdx]
	fileLength := file.Length()

	if fileLength == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "File has zero length, nothing to prioritize"})
		return
	}

	// Calculate the pieces to prioritize based on percentage
	pieceLength := torrentFile.Info().PieceLength
	firstPieceOfFile := int(file.Offset() / pieceLength)
	lastPieceOfFile := int((file.Offset() + file.Length() - 1) / pieceLength)
	totalPieces := lastPieceOfFile - firstPieceOfFile + 1

	// Determine the piece at seek position
	seekPositionBytes := int64(float64(fileLength) * req.Percentage / 100.0)
	seekPositionPiece := int((file.Offset() + seekPositionBytes) / pieceLength)

	// Prevent prioritizing if seeking to the very end (e.g., >99.5%)
	if req.Percentage > 99.5 {
		c.JSON(http.StatusOK, gin.H{
			"message":           "Seek percentage too close to end; ignoring auto-seek to end.",
			"ignored":           true,
			"seekPositionPiece": seekPositionPiece,
			"percentage":        req.Percentage,
		})
		return
	}

	// Create a window of pieces to prioritize around seek position
	const priorityWindowSize = 20 // Prioritize 20 pieces ahead of seek position
	startPriority := seekPositionPiece
	endPriority := seekPositionPiece + priorityWindowSize
	if endPriority > lastPieceOfFile {
		endPriority = lastPieceOfFile
	}

	fmt.Printf("Prioritizing pieces %d to %d for seek position at %.2f%% in file %s (total pieces: %d)\n",
		startPriority, endPriority, req.Percentage, file.Path(), totalPieces)

	// Set high priority for these pieces
	requestedPieces.Mutex.Lock()
	if requestedPieces.Pieces[req.InfoHash] == nil {
		requestedPieces.Pieces[req.InfoHash] = make(map[int]bool)
	}
	requestedPieces.Mutex.Unlock()

	for i := startPriority; i <= endPriority; i++ {
		if i >= 0 && i < torrentFile.NumPieces() {
			torrentFile.Piece(i).SetPriority(torrent.PiecePriorityNow)

			requestedPieces.Mutex.Lock()
			requestedPieces.Pieces[req.InfoHash][i] = true
			requestedPieces.Mutex.Unlock()
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Prioritized pieces %d to %d for seek position at %.2f%%",
			startPriority, endPriority, req.Percentage),
		"seekPositionBytes": seekPositionBytes,
		"seekPositionPiece": seekPositionPiece,
		"startPriority":     startPriority,
		"endPriority":       endPriority,
	})
}
