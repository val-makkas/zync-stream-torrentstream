package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/models"
)

// StreamTorrentFile handles GET /stream/:infohash/:file_idx requests
func StreamTorrentFile(c *gin.Context, store *models.TorrentStore) {
	fmt.Println("STREAM_HANDLER_VERSION_3 - Entering StreamTorrentFile") // New distinct log message
	infoHash := c.Param("infohash")
	fileIdxStr := c.Param("file_idx")
	fmt.Printf("Received /stream request for torrent: %s, file index: %s\n", infoHash, fileIdxStr)

	// Convert fileIdx to int
	fileIdx, err := strconv.Atoi(fileIdxStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file index"})
		return
	}

	// Get the torrent from our map
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[infoHash]
	store.Mutex.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}

	// Wait for torrent metadata if not yet available
	metadataTimeout := time.After(5 * time.Second)
	select {
	case <-torrentFile.GotInfo():
		// Got metadata, proceed
	case <-metadataTimeout:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Timed out waiting for torrent metadata"})
		return
	}

	// Make sure info is available
	if torrentFile.Info() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Torrent info not available"})
		return
	}

	// Check if files exist in the torrent
	files := torrentFile.Files()
	if len(files) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Torrent does not have any files to stream"})
		return
	}

	// Validate file index
	if fileIdx < 0 || fileIdx >= len(files) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File index out of range"})
		return
	}

	file := files[fileIdx]
	fileLength := file.Length()

	fmt.Printf("Starting stream for %s, file index %d: %s (size: %d bytes)\n", infoHash, fileIdx, file.Path(), fileLength)

	// --- Remux to fragmented MP4 using FFmpeg ---
	// Set up HTTP response headers for MP4
	c.Writer.Header().Set("Content-Type", "video/mp4")
	c.Writer.Header().Set("Accept-Ranges", "none") // Range requests not supported in this mode
	c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
	c.Writer.Header().Set("X-Frame-Options", "SAMEORIGIN")
	c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
	c.Writer.WriteHeader(http.StatusOK)

	reader := file.NewReader()
	defer reader.Close()
	reader.SetReadahead(10 * 1024 * 1024)

	// Dynamically resolve ffmpeg path relative to the Go binary
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("[ERROR] Failed to resolve service executable path: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve service executable path"})
		return
	}
	ffmpegPath := filepath.Join(filepath.Dir(exePath), "ffmpeg_bin", "ffmpeg.exe")
	fmt.Printf("[DEBUG] Resolved ffmpegPath: %s\n", ffmpegPath)

	// Prepare FFmpeg command
	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "384k",
		"-ac", "2",
		"-profile:a", "aac_low",
		"-f", "mp4",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"pipe:1",
	)

	// Print FFmpeg stderr in real time for debugging
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("[ERROR] Failed to create FFmpeg stderr pipe: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create FFmpeg stderr pipe"})
		return
	}
	go func() {
		io.Copy(os.Stderr, stderr)
	}()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("[ERROR] Failed to create FFmpeg stdin pipe: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create FFmpeg stdin pipe"})
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("[ERROR] Failed to create FFmpeg stdout pipe: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create FFmpeg stdout pipe"})
		return
	}
	if err := cmd.Start(); err != nil {
		// Print FFmpeg stderr if start fails
		errMsg, _ := io.ReadAll(stderr)
		fmt.Printf("[ERROR] Failed to start FFmpeg process: %v\nFFmpeg stderr: %s\n", err, string(errMsg))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start FFmpeg process", "ffmpeg_stderr": string(errMsg)})
		return
	}

	// Copy file data to FFmpeg stdin in a goroutine
	copyErrChan := make(chan error, 1)
	go func() {
		_, err := io.Copy(stdin, reader)
		stdin.Close()
		copyErrChan <- err
	}()

	// Copy FFmpeg stdout to HTTP response
	if _, err = io.Copy(c.Writer, stdout); err != nil {
		fmt.Printf("Error while copying FFmpeg output to HTTP response: %v\n", err)
		stdout.Close()
		return
	}
	stdout.Close()

	// Wait for FFmpeg and file copy to finish
	<-copyErrChan
	cmd.Wait()

	fmt.Printf("Finished streaming (remuxed) file %s\n", file.Path())
}
