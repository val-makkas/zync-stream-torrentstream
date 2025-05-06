package models

import (
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

// TorrentStore manages the in-memory storage of torrent instances
type TorrentStore struct {
	Torrents   map[string]*torrent.Torrent
	Mutex      *sync.RWMutex
}

// NewTorrentStore creates a new torrent store
func NewTorrentStore() *TorrentStore {
	return &TorrentStore{
		Torrents:   make(map[string]*torrent.Torrent),
		Mutex:      &sync.RWMutex{},
	}
}

// ProgressStats tracks download progress for a file
type ProgressStats struct {
	BytesCompleted int64
	Timestamp      time.Time
}

// RequestedPieces tracks which pieces are requested by users through seeking
type RequestedPieces struct {
	Pieces map[string]map[int]bool
	Mutex  *sync.RWMutex
}

// NewRequestedPieces creates a new requested pieces tracker
func NewRequestedPieces() *RequestedPieces {
	return &RequestedPieces{
		Pieces: make(map[string]map[int]bool),
		Mutex:  &sync.RWMutex{},
	}
}

// TorrentRequest represents a request to add a new torrent
type TorrentRequest struct {
	InfoHash string `json:"infoHash"`
	MagnetURI string `json:"magnetURI,omitempty"`
}

// ProgressResponse represents the response to a progress request
type ProgressResponse struct {
	Ready                    bool    `json:"ready"`
	FilePath                 string  `json:"file_path,omitempty"`
	CompletedBytesEstimated  int64   `json:"completed_bytes_estimated"`
	LengthBytes              int64   `json:"length_bytes"`
	PercentByBytesEstimated  float64 `json:"percent_by_bytes_estimated"`
	CompletedPiecesInFileRange int   `json:"completed_pieces_in_file_range"`
	TotalPiecesInFileRange   int     `json:"total_pieces_in_file_range"`
	PercentByPieces          float64 `json:"percent_by_pieces"`
	DownloadSpeedBytesPerSec int64   `json:"download_speed_bytes_per_sec"`
	Status                   string  `json:"status,omitempty"`
}

// PrioritizeRequest represents a request to prioritize pieces for seeking
type PrioritizeRequest struct {
	InfoHash   string  `json:"infoHash"`
	FileIdx    int     `json:"fileIdx"`
	Percentage float64 `json:"percentage"`
}
