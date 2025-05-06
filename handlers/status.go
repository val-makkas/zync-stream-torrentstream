package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/models"
	"github.com/val-makkas/absolute-cinema/torrentstream/utils"
)

// GetTorrentStatus handles GET /status/:infohash requests
func GetTorrentStatus(c *gin.Context, store *models.TorrentStore) {
	infoHash := c.Param("infohash")
	fmt.Printf("Received /status request for infohash: %s\n", infoHash)

	// Get the torrent from our map
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[infoHash]
	store.Mutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}

	// Basic torrent info
	status := gin.H{
		"infoHash": infoHash,
		"state":    utils.GetTorrentState(torrentFile),
	}

	// If we have metadata, add more details
	if torrentFile.Info() != nil {
		files := torrentFile.Files()
		fileDetails := make([]gin.H, len(files))

		for i, file := range files {
			fileDetails[i] = gin.H{
				"path":   file.Path(),
				"length": file.Length(),
				"offset": file.Offset(),
			}
		}

		status["name"] = torrentFile.Name()
		status["length"] = torrentFile.Length()
		status["files"] = fileDetails
		status["bytesCompleted"] = torrentFile.BytesCompleted()
		status["percentCompleted"] = float64(torrentFile.BytesCompleted()) / float64(torrentFile.Length()) * 100
	} else {
		status["metadataCompleted"] = false
	}

	c.JSON(http.StatusOK, status)
}
