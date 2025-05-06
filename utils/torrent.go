package utils

import (
	"fmt"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// GetTorrentState returns a string representation of a torrent's state
func GetTorrentState(t *torrent.Torrent) string {
	if t.Info() == nil {
		return "getting metadata"
	}

	if t.BytesCompleted() >= t.Length() {
		return "completed"
	}

	if !t.Seeding() {
		return "downloading"
	}

	return "seeding"
}

// GetLargestFileIndex returns the index of the largest file in a torrent
func GetLargestFileIndex(t *torrent.Torrent) (int, error) {
	if t.Info() == nil {
		return -1, fmt.Errorf("torrent metadata not available")
	}

	files := t.Files()
	if len(files) == 0 {
		return -1, fmt.Errorf("torrent has no files")
	}

	largestIdx := 0
	largestSize := int64(0)

	for i, file := range files {
		if file.Length() > largestSize {
			largestSize = file.Length()
			largestIdx = i
		}
	}

	return largestIdx, nil
}

// WaitForMetadata waits for torrent metadata to become available
func WaitForMetadata(t *torrent.Torrent, timeout time.Duration) error {
	if t.Info() != nil {
		return nil // already have metadata
	}

	select {
	case <-t.GotInfo():
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for metadata")
	}
}

// ParseMagnetURI creates a torrent from a magnet URI
func ParseMagnetURI(magnetURI string) (*metainfo.Magnet, error) {
	magnet, err := metainfo.ParseMagnetUri(magnetURI)
	if err != nil {
		return nil, err
	}
	return &magnet, nil
}

// AddTrackersToTorrent adds trackers to a torrent
func AddTrackersToTorrent(t *torrent.Torrent) {
	if t == nil {
		return
	}

	trackers := GetPopularTrackers()
	for _, tracker := range trackers {
		t.AddTrackers([][]string{{tracker}})
	}
}
