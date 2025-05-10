package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/val-makkas/absolute-cinema/torrentstream/models"
)

// Global map to track download stats for speed calculation
var downloadStats = make(map[string]models.ProgressStats)

// GetFileProgress handles GET /progress/:infohash/:file_idx requests
func GetFileProgress(c *gin.Context, store *models.TorrentStore) {
	infoHash := c.Param("infohash")
	fileIdxStr := c.Param("file_idx")
	// Progress endpoint can be very chatty, keep logs here concise
	// fmt.Printf("Received /progress request for infohash: %s, file index: %s\n", infoHash, fileIdxStr)

	fileIdx, err := strconv.Atoi(fileIdxStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file index"})
		return
	}

	// Get torrent
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[infoHash]
	store.Mutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}

	if torrentFile.Info() == nil {
		select {
		case <-torrentFile.GotInfo():
			// Metadata ready
		case <-time.After(5 * time.Second):
			c.JSON(http.StatusOK, gin.H{"ready": false, "status": "metadata not yet available"})
			return
		}
	}

	torrentInfo := torrentFile.Info()
	if torrentInfo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get torrent info for progress"})
		return
	}
	pieceLength := torrentInfo.PieceLength
	if pieceLength == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Torrent piece length is zero for progress calculation"})
		return
	}

	files := torrentFile.Files()
	if fileIdx < 0 || fileIdx >= len(files) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File index out of range"})
		return
	}

	file := files[fileIdx]
	fileLength := file.Length()

	var percentByBytes float64
	var calculatedBytesCompleted int64

	var startPieceIdx, endPieceIdx int
	totalPiecesInFileRange := 0
	completedPiecesInFileRange := 0

	if fileLength > 0 {
		startPieceIdx = int((file.Offset()) / pieceLength)
		endPieceIdx = int((file.Offset() + file.Length() - 1) / pieceLength)

		totalPiecesInFileRange = endPieceIdx - startPieceIdx + 1
		if totalPiecesInFileRange <= 0 {
			totalPiecesInFileRange = 0
		}

		// Keep track of pieces completed *within* this file's range
		completedPiecesIndices := []int{}

		for i := startPieceIdx; i <= endPieceIdx; i++ {
			if i >= 0 && i < torrentFile.NumPieces() {
				piece := torrentFile.Piece(i)
				if piece.State().Complete {
					completedPiecesInFileRange++
					completedPiecesIndices = append(completedPiecesIndices, i)
					// Note: calculatedBytesCompleted is a simplified estimate
				}
			}
		}

		// Recalculate estimated bytes based on completed pieces count * average piece size for the file range
		if totalPiecesInFileRange > 0 && fileLength > 0 {
			estimatedBytesPerPiece := float64(fileLength) / float64(totalPiecesInFileRange)
			calculatedBytesCompleted = int64(float64(completedPiecesInFileRange) * estimatedBytesPerPiece)
			if calculatedBytesCompleted > fileLength { // Cap at file length
				calculatedBytesCompleted = fileLength
			}
		} else {
			calculatedBytesCompleted = 0
		}

	} else { // fileLength is 0
		percentByBytes = 100.0
		calculatedBytesCompleted = 0
	}

	if fileLength > 0 {
		percentByBytes = (float64(calculatedBytesCompleted) / float64(fileLength)) * 100
	} else {
		percentByBytes = 100.0 // 0-byte file is 100% complete
	}

	var percentByPieces float64
	if totalPiecesInFileRange > 0 {
		percentByPieces = (float64(completedPiecesInFileRange) / float64(totalPiecesInFileRange)) * 100
	} else if fileLength == 0 {
		percentByPieces = 100.0
	}

	// Calculate download speed by tracking bytes over time
	downloadStatsKey := fmt.Sprintf("%s_%d", infoHash, fileIdx)
	currentTime := time.Now()

	var downloadSpeedBytesPerSec int64 = 0

	// Get the previous stats if they exist
	if prevStats, exists := downloadStats[downloadStatsKey]; exists {
		timeDiff := currentTime.Sub(prevStats.Timestamp).Seconds()
		if timeDiff > 0 {
			bytesDiff := calculatedBytesCompleted - prevStats.BytesCompleted
			if bytesDiff > 0 {
				downloadSpeedBytesPerSec = int64(float64(bytesDiff) / timeDiff)
			}
		}
	}

	// Update stats for next calculation
	downloadStats[downloadStatsKey] = models.ProgressStats{
		BytesCompleted: calculatedBytesCompleted,
		Timestamp:      currentTime,
	}

	// Determine readiness based on actual downloaded bytes for non-empty files
	isReady := (calculatedBytesCompleted > 0 && fileLength > 0) || fileLength == 0

	var duration float64
	store.Mutex.RLock()
	if store.Metadata != nil {
		if m, ok := store.Metadata[infoHash]; ok {
			if d, ok := m[fileIdx]; ok {
				duration = d
			}
		}
	}
	store.Mutex.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"ready":                          isReady,
		"file_path":                      file.Path(),
		"completed_bytes_estimated":      calculatedBytesCompleted, // This is an estimation
		"length_bytes":                   fileLength,
		"percent_by_bytes_estimated":     percentByBytes,
		"completed_pieces_in_file_range": completedPiecesInFileRange, // <-- How many pieces for this file are complete?
		"total_pieces_in_file_range":     totalPiecesInFileRange,     // <-- How many pieces total for this file?
		"percent_by_pieces":              percentByPieces,
		"download_speed_bytes_per_sec":   downloadSpeedBytesPerSec, // <-- Download speed in bytes per second
		"duration":                       duration,
	})
}
