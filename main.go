package main

import (
	"fmt"
	"log"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/config"
	"github.com/val-makkas/absolute-cinema/torrentstream/handlers"
	"github.com/val-makkas/absolute-cinema/torrentstream/models"
)

func main() {
	// Initialize the torrent client with our configuration
	config.ConfigureTorrentClient()
	client, err := torrent.NewClient(config.AppConfig.ClientConfig)
	if err != nil {
		log.Fatalf("Failed to create torrent client: %v", err)
	}
	defer client.Close()

	// Initialize data stores
	torrentStore := models.NewTorrentStore()
	requestedPieces := models.NewRequestedPieces()

	// Create router
	router := gin.Default()

	// Configure CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// API routes
	router.POST("/add", func(c *gin.Context) {
		handlers.AddTorrent(c, client, torrentStore)
	})

	router.GET("/status/:infohash", func(c *gin.Context) {
		handlers.GetTorrentStatus(c, torrentStore)
	})

	router.GET("/hls/:infohash/:file_idx/:filename", func(c *gin.Context) {
		handlers.StreamTorrentHLS(c, torrentStore)
	})

	router.GET("/hls/:infohash/:file_idx/seek/:seconds/:filename", func(c *gin.Context) {
		handlers.StreamTorrentHLSSeek(c, torrentStore)
	})

	router.GET("/progress/:infohash/:file_idx", func(c *gin.Context) {
		handlers.GetFileProgress(c, torrentStore)
	})

	router.DELETE("/remove/:infohash", func(c *gin.Context) {
		handlers.RemoveTorrent(c, torrentStore)
	})

	router.POST("/prioritize", func(c *gin.Context) {
		handlers.PrioritizePieces(c, torrentStore, requestedPieces)
	})

	// Start the server
	fmt.Println("Server starting on port 5050...")
	err = router.Run(config.AppConfig.Port)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
