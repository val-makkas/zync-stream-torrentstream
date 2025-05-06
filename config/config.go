package config

import (
	"time"

	"github.com/anacrolix/torrent"
)

// Config holds global application configuration
type Config struct {
	ClientConfig    *torrent.ClientConfig
	Port            string
	MaxRuntimeHours int
	WindowSize      int
	SleepInterval   time.Duration
}

// Default configuration values
var (
	// AppConfig is the global application configuration
	AppConfig = &Config{
		ClientConfig:    torrent.NewDefaultClientConfig(),
		Port:            ":5050",
		MaxRuntimeHours: 3,
		WindowSize:      50,
		SleepInterval:   1 * time.Second,
	}
)

// ConfigureTorrentClient sets up the torrent client configuration
func ConfigureTorrentClient() {
	// Configure the torrent client
	AppConfig.ClientConfig.Seed = true
	AppConfig.ClientConfig.NoUpload = false
	AppConfig.ClientConfig.DisableTrackers = false
	AppConfig.ClientConfig.DisableWebseeds = false
	AppConfig.ClientConfig.NoDHT = false

	// Optimize for streaming
	AppConfig.ClientConfig.DisableTCP = false
	AppConfig.ClientConfig.DisableUTP = false
	AppConfig.ClientConfig.DisableIPv6 = false

	// Set download/upload limits if needed
	// AppConfig.ClientConfig.DownloadRateLimiter = rate.NewLimiter(rate.Limit(5*1024*1024), 5*1024*1024)

	// Set connection limits
	AppConfig.ClientConfig.EstablishedConnsPerTorrent = 50
	AppConfig.ClientConfig.HalfOpenConnsPerTorrent = 25
}
