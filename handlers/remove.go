package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/models"
)

// RemoveTorrent handles DELETE /remove/:infohash requests
func RemoveTorrent(c *gin.Context, store *models.TorrentStore) {
	infoHash := c.Param("infohash")
	fmt.Printf("Received /remove request for infohash: %s\n", infoHash)

	// Get the torrent from our map
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[infoHash]
	store.Mutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}

	// Drop the torrent
	torrentFile.Drop()
	fmt.Printf("Dropped torrent %s\n", infoHash)

	// Remove from our map
	store.Mutex.Lock()
	delete(store.Torrents, infoHash)
	store.Mutex.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "Torrent removed successfully"})
}
