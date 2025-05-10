package utils

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
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

// FFProbeFormat represents the format information from ffprobe
type FFProbeFormat struct {
	Format struct {
		FormatName string `json:"format_name"`
	} `json:"format"`
}

// FFProbeStream represents a single stream's information from ffprobe
type FFProbeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
}

// FFProbeResult represents the complete result from ffprobe
type FFProbeResult struct {
	Streams []FFProbeStream `json:"streams"`
	Format  struct {
		FormatName string `json:"format_name"`
	} `json:"format"`
}

// IsBrowserCompatibleVideo returns true if the file is a browser-compatible MP4 (H264/AAC)
// Uses extension for fast check, and ffprobe for codec check if available
func IsBrowserCompatibleVideo(ffprobePath, filePath string) (bool, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".mp4" {
		// Fast path: check codecs with ffprobe
		cmd := exec.Command(ffprobePath, "-v", "error", "-select_streams", "v:0,a:0", "-show_entries", "stream=codec_type,codec_name", "-show_format", "-of", "json", filePath)
		out, err := cmd.Output()
		if err != nil {
			return false, err
		}
		var result FFProbeResult
		if err := json.Unmarshal(out, &result); err != nil {
			return false, err
		}
		videoOk := false
		audioOk := false
		for _, s := range result.Streams {
			if s.CodecType == "video" && (s.CodecName == "h264" || s.CodecName == "avc1") {
				videoOk = true
			}
			if s.CodecType == "audio" && (s.CodecName == "aac" || s.CodecName == "mp4a") {
				audioOk = true
			}
		}
		return videoOk && audioOk, nil
	}
	return false, nil // Only .mp4 is considered browser compatible for direct serve
}
