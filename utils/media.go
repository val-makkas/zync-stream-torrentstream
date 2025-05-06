package utils

import (
	"path/filepath"
)

// GetContentType returns the appropriate Content-Type header value based on file extension
func GetContentType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".aac":
		return "audio/aac"
	default:
		return "application/octet-stream"
	}
}

// GetPopularTrackers returns a list of popular BitTorrent trackers
func GetPopularTrackers() []string {
	return []string{
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://open.stealth.si:80/announce",
		"udp://tracker.openbittorrent.com:80/announce",
		"udp://exodus.desync.com:6969/announce",
		"udp://tracker.torrent.eu.org:451/announce",
		"udp://tracker.moeking.me:6969/announce",
		"udp://explodie.org:6969/announce",
		"udp://tracker1.bt.moack.co.kr:80/announce",
		"udp://tracker.tiny-vps.com:6969/announce",
		"udp://tracker.altrosky.nl:6969/announce",
		"udp://tracker.dler.org:6969/announce",
		"udp://p4p.arenabg.com:1337/announce",
		"udp://open.demonii.com:1337/announce",
	}
}
