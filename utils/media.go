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

// GetPopularTrackers returns a static list of recommended trackers
func GetPopularTrackers() []string {
	return []string{
		"udp://tracker.opentrackr.org:1337/announce",
		"udp://open.demonii.com:1337/announce",
		"udp://open.stealth.si:80/announce",
		"udp://tracker.torrent.eu.org:451/announce",
		"udp://tracker.skyts.net:6969/announce",
		"udp://explodie.org:6969/announce",
		"udp://exodus.desync.com:6969/announce",
		"udp://wepzone.net:6969/announce",
		"udp://tracker2.dler.org:80/announce",
		"udp://tracker1.myporn.club:9337/announce",
		"udp://tracker.tiny-vps.com:6969/announce",
		"udp://tracker.theoks.net:6969/announce",
		"udp://tracker.dump.cl:6969/announce",
		"udp://tracker.bittor.pw:1337/announce",
		"udp://tracker.0x7c0.com:6969/announce",
		"udp://tracker-udp.gbitt.info:80/announce",
		"udp://retracker01-msk-virt.corbina.net:80/announce",
		"udp://public.tracker.vraphim.com:6969/announce",
		"udp://p4p.arenabg.com:1337/announce",
		"udp://opentracker.io:6969/announce",
	}
}
