package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/models"
	"github.com/val-makkas/absolute-cinema/torrentstream/utils"
)

// AddTorrent handles POST /add requests to add a new torrent
func AddTorrent(c *gin.Context, client *torrent.Client, store *models.TorrentStore) {
	var req models.TorrentRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request format: %v", err)})
		return
	}

	fmt.Printf("Received /add request\n")

	// Validate the infohash
	if req.InfoHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "InfoHash is required"})
		return
	}

	// Create magnet URI if not provided
	magnetURI := req.MagnetURI
	if magnetURI == "" {
		magnetURI = fmt.Sprintf("magnet:?xt=urn:btih:%s", req.InfoHash)
	}

	fmt.Printf("Adding magnet URI: %s\n", magnetURI)

	// Check if we already have this torrent
	store.Mutex.RLock()
	_, exists := store.Torrents[req.InfoHash]
	store.Mutex.RUnlock()

	if exists {
		c.JSON(http.StatusOK, gin.H{"message": "Torrent already exists", "infoHash": req.InfoHash})
		return
	}

	// Parse the magnet URI
	_, err := utils.ParseMagnetURI(magnetURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid magnet URI: %v", err)})
		return
	}

	// Add the torrent
	t, err := client.AddMagnet(magnetURI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to add torrent: %v", err)})
		return
	}

	// Store the torrent
	store.Mutex.Lock()
	store.Torrents[req.InfoHash] = t
	store.Mutex.Unlock()

	fmt.Printf("Torrent %s added, waiting for metadata...\n", req.InfoHash)

	// Wait for metadata in a separate goroutine
	go func() {
		// Wait for metadata
		select {
		case <-t.GotInfo():
			fmt.Printf("Metadata received for torrent: %s, Name: %s\n", req.InfoHash, t.Name())

			// Add popular trackers
			utils.AddTrackersToTorrent(t)
			fmt.Printf("Added popular trackers for: %s\n", t.Name())

			// Prioritize largest file if this is a multi-file torrent
			if len(t.Info().Files) > 0 {
				largestFileIdx, err := utils.GetLargestFileIndex(t)
				if err == nil {
					prioFile := t.Files()[largestFileIdx]
					pieceLength := t.Info().PieceLength

					// Calculate first and last pieces for this file
					firstPiece := int(prioFile.Offset() / pieceLength)
					lastPiece := int((prioFile.Offset() + prioFile.Length() - 1) / pieceLength)

					// Prioritize beginning pieces (first 1MB) to get metadata quickly
					startBytes := int64(1048576) // 1MB
					endFirstPrioPiece := firstPiece + int(startBytes/pieceLength)
					if endFirstPrioPiece > lastPiece {
						endFirstPrioPiece = lastPiece
					}

					fmt.Printf("Prioritizing start pieces %d-%d (covering up to %d bytes) for largest file %s\n",
						firstPiece, endFirstPrioPiece, startBytes, prioFile.Path())

					for i := firstPiece; i <= endFirstPrioPiece; i++ {
						if i >= 0 && i < t.NumPieces() {
							t.Piece(i).SetPriority(torrent.PiecePriorityNow)
						}
					}

					// Also prioritize end pieces (last 1MB) to get metadata
					endBytes := prioFile.Length() - startBytes
					if endBytes < 0 {
						endBytes = 0
					}
					startLastPrioPiece := int((prioFile.Offset() + endBytes) / pieceLength)
					if startLastPrioPiece < firstPiece {
						startLastPrioPiece = firstPiece
					}

					fmt.Printf("Prioritizing end pieces %d-%d (covering from byte %d) for largest file %s\n",
						startLastPrioPiece, lastPiece, endBytes, prioFile.Path())

					for i := startLastPrioPiece; i <= lastPiece; i++ {
						if i >= 0 && i < t.NumPieces() {
							t.Piece(i).SetPriority(torrent.PiecePriorityNow)
						}
					}

					// Set normal priority for all other pieces
					fmt.Printf("Setting normal priority for all other pieces of largest file from %d to %d\n",
						firstPiece, lastPiece)
					for i := firstPiece; i <= lastPiece; i++ {
						if i >= 0 && i < t.NumPieces() && i < startLastPrioPiece && i > endFirstPrioPiece {
							t.Piece(i).SetPriority(torrent.PiecePriorityNormal)
						}
					}

					// Start sliding window manager for this file
					go manageSlidingWindow(t, prioFile, pieceLength, client)
				}
			}
		case <-time.After(60 * time.Second):
			fmt.Printf("Timed out waiting for metadata for torrent: %s\n", req.InfoHash)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message":  "Torrent added successfully",
		"infoHash": req.InfoHash,
	})
}

// manageSlidingWindow manages a sliding window of prioritized pieces for efficient streaming
func manageSlidingWindow(torrentFile *torrent.Torrent, file *torrent.File, pieceLength int64, client *torrent.Client) {
	// Early bail-out conditions
	if file == nil || file.Length() == 0 || pieceLength == 0 {
		return
	}

	// Calculate piece range for the file
	firstPieceOfFile := int(file.Offset() / pieceLength)
	lastPieceOfFile := int((file.Offset() + file.Length() - 1) / pieceLength)
	totalPieces := lastPieceOfFile - firstPieceOfFile + 1

	if totalPieces <= 0 {
		return
	}

	fmt.Printf("Starting sliding window manager for %s (%d pieces total)\n", file.Path(), totalPieces)

	const windowSize = 50             // Number of pieces ahead to prioritize
	const sleepInterval = 1           // Check every 1 second
	const maxIterations = 3 * 60 * 60 // Run for max 3 hours

	for iter := 0; iter < maxIterations; iter++ {
		// Check if torrent is closed/dropped
		select {
		case <-torrentFile.Closed():
			fmt.Printf("Sliding window manager: Torrent closed for %s\n", file.Path())
			return
		default:
			// Continue processing
		}

		// Find the first incomplete piece in the file
		firstIncompletePiece := -1
		for i := firstPieceOfFile; i <= lastPieceOfFile; i++ {
			if i >= 0 && i < torrentFile.NumPieces() {
				if !torrentFile.Piece(i).State().Complete {
					firstIncompletePiece = i
					break
				}
			}
		}

		// If no incomplete pieces found, exit
		if firstIncompletePiece == -1 {
			fmt.Printf("Sliding window manager: All pieces complete for %s\n", file.Path())
			return
		}

		// Calculate the window end (limited by file's last piece)
		windowEnd := firstIncompletePiece + windowSize
		if windowEnd > lastPieceOfFile {
			windowEnd = lastPieceOfFile
		}

		// Prioritize the window of pieces
		for i := firstIncompletePiece; i <= windowEnd; i++ {
			if i >= 0 && i < torrentFile.NumPieces() && !torrentFile.Piece(i).State().Complete {
				// Only set priority if piece is not already complete
				torrentFile.Piece(i).SetPriority(torrent.PiecePriorityNow)
			}
		}

		// Log progress every 12 iterations (about every 12 seconds now with reduced interval)
		if iter%12 == 0 {
			completedInFile := 0
			for i := firstPieceOfFile; i <= lastPieceOfFile; i++ {
				if i >= 0 && i < torrentFile.NumPieces() && torrentFile.Piece(i).State().Complete {
					completedInFile++
				}
			}

			progress := float64(completedInFile) / float64(totalPieces) * 100
			fmt.Printf("Sliding window progress for %s: %.2f%% (%d/%d pieces), current window: %d-%d\n",
				file.Path(), progress, completedInFile, totalPieces, firstIncompletePiece, windowEnd)
		}

		// Sleep before next check
		time.Sleep(sleepInterval * time.Second)

		// If torrent is completely downloaded, exit
		if torrentFile.BytesCompleted() >= torrentFile.Length() {
			fmt.Printf("Sliding window manager: Download complete for %s\n", file.Path())
			return
		}
	}

	fmt.Printf("Sliding window manager: Reached max runtime for %s\n", file.Path())
}
