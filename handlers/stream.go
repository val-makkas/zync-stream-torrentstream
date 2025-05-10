package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"

	"github.com/val-makkas/absolute-cinema/torrentstream/models"
)

// HLSJob manages a running FFmpeg HLS process and its temp dir
var hlsJobs = struct {
	sync.Mutex
	jobs map[string]*HLSJob
}{jobs: make(map[string]*HLSJob)}

type HLSJob struct {
	Cmd        *exec.Cmd
	TempDir    string
	LastAccess time.Time
	StopChan   chan struct{}
}

// StreamTorrentHLS handles /hls/:infohash/:file_idx/:filename (playlist.m3u8 or segment.ts)
func StreamTorrentHLS(c *gin.Context, store *models.TorrentStore) {
	infoHash := c.Param("infohash")
	fileIdxStr := c.Param("file_idx")
	filename := c.Param("filename")
	log.Printf("[HLS] Request: infoHash=%s fileIdx=%s filename=%s", infoHash, fileIdxStr, filename)
	fileIdx, err := strconv.Atoi(fileIdxStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file index"})
		return
	}

	// Get torrent and file
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[infoHash]
	store.Mutex.RUnlock()
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}
	if err := waitForTorrentInfo(torrentFile, 5*time.Second); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Timed out waiting for torrent metadata"})
		return
	}
	files := torrentFile.Files()
	if fileIdx < 0 || fileIdx >= len(files) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File index out of range"})
		return
	}
	file := files[fileIdx]

	// Unique job key
	jobKey := infoHash + ":" + fileIdxStr

	hlsJobs.Lock()
	job, exists := hlsJobs.jobs[jobKey]
	hlsJobs.Unlock()

	if !exists {
		log.Printf("[HLS] Starting new HLS job for %s:%s", infoHash, fileIdxStr)
		// Start new HLS job
		tempDir, err := os.MkdirTemp("", "hls-"+infoHash+"-")
		if err != nil {
			log.Printf("[HLS] Failed to create temp dir: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp dir"})
			return
		}
		exePath, _ := os.Executable()
		ffmpegPath := filepath.Join(filepath.Dir(exePath), "ffmpeg_bin", "ffmpeg.exe")
		playlistPath := filepath.Join(tempDir, "playlist.m3u8")
		segmentPattern := filepath.Join(tempDir, "segment%03d.ts")
		cmd := exec.Command(
			ffmpegPath,
			"-hide_banner", "-loglevel", "error",
			"-i", "pipe:0",
			"-c:v", "copy",
			"-c:a", "aac",
			"-b:a", "192k",
			"-ac", "2",
			"-f", "hls",
			"-hls_time", "4",
			"-hls_list_size", "0",
			"-hls_flags", "independent_segments",
			"-hls_segment_filename", segmentPattern,
			playlistPath,
		)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			log.Printf("[HLS] Failed to create FFmpeg stdin pipe: %v", err)
			os.RemoveAll(tempDir)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create FFmpeg stdin pipe"})
			return
		}
		reader := file.NewReader()
		go func() {
			io.Copy(stdin, reader)
			stdin.Close()
			log.Printf("[HLS] Finished piping data to FFmpeg for %s:%s", infoHash, fileIdxStr)
		}()
		if err := cmd.Start(); err != nil {
			log.Printf("[HLS] Failed to start FFmpeg: %v", err)
			os.RemoveAll(tempDir)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start FFmpeg"})
			return
		}
		log.Printf("[HLS] FFmpeg started for %s:%s", infoHash, fileIdxStr)
		job = &HLSJob{Cmd: cmd, TempDir: tempDir, LastAccess: time.Now(), StopChan: make(chan struct{})}
		hlsJobs.Lock()
		hlsJobs.jobs[jobKey] = job
		hlsJobs.Unlock()
		// Cleanup goroutine
		go func(jobKey string, job *HLSJob) {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					hlsJobs.Lock()
					if time.Since(job.LastAccess) > 2*time.Minute {
						job.Cmd.Process.Kill()
						os.RemoveAll(job.TempDir)
						delete(hlsJobs.jobs, jobKey)
						hlsJobs.Unlock()
						return
					}
					hlsJobs.Unlock()
				case <-job.StopChan:
					job.Cmd.Process.Kill()
					os.RemoveAll(job.TempDir)
					hlsJobs.Lock()
					delete(hlsJobs.jobs, jobKey)
					hlsJobs.Unlock()
					return
				}
			}
		}(jobKey, job)
	}

	// Serve requested file (playlist or segment)
	job.LastAccess = time.Now()
	filePath := filepath.Join(job.TempDir, filename)
	for i := 0; i < 60; i++ { // Wait up to 6s for file to appear
		if _, err := os.Stat(filePath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(filePath); err != nil {
		log.Printf("[HLS] HLS file not ready: %s", filePath)
		c.JSON(http.StatusNotFound, gin.H{"error": "HLS file not ready"})
		return
	}

	// --- PATCH: Hardcode duration in playlist.m3u8 and fix last EXTINF ---
	if filename == "playlist.m3u8" {
		store.Mutex.RLock()
		dur := 0.0
		if store.Metadata != nil {
			if m, ok := store.Metadata[infoHash]; ok {
				if d, ok := m[fileIdx]; ok {
					dur = d
				}
			}
		}
		store.Mutex.RUnlock()
		if dur > 0 {
			b, err := os.ReadFile(filePath)
			if err == nil {
				lines := string(b)
				playlistLines := strings.Split(lines, "\n")
				var segIdxs []int
				var segDurSum float64
				for i, l := range playlistLines {
					if strings.HasPrefix(l, "#EXTINF:") {
						segIdxs = append(segIdxs, i)
						var segDur float64
						fmt.Sscanf(l, "#EXTINF:%f", &segDur)
						segDurSum += segDur
					}
				}
				if len(segIdxs) > 0 && segDurSum > 0 && segDurSum != dur {
					lastIdx := segIdxs[len(segIdxs)-1]
					lastDur := dur - (segDurSum - func() float64 { var d float64; fmt.Sscanf(playlistLines[lastIdx], "#EXTINF:%f", &d); return d }())
					if lastDur > 0.1 {
						playlistLines[lastIdx] = fmt.Sprintf("#EXTINF:%.3f,", lastDur)
					}
				}
				// Append custom tag for debugging
				playlistLines = append(playlistLines, fmt.Sprintf("#EXT-X-ABSOLUTE-DURATION:%.3f", dur))

				// --- ATOMIC PATCH: Write to temp file, then rename ---
				log.Printf("[HLS] Patching playlist for duration: %.3f", dur)
				tempPlaylist := filePath + ".tmp"
				err = os.WriteFile(tempPlaylist, []byte(strings.Join(playlistLines, "\n")), 0644)
				if err != nil {
					log.Printf("[HLS] Failed to write temp playlist: %v", err)
				} else {
					err = os.Rename(tempPlaylist, filePath)
					if err != nil {
						log.Printf("[HLS] Failed to rename temp playlist: %v", err)
					} else {
						log.Printf("[HLS] Patched playlist written atomically: %s", filePath)
					}
				}
				// Ensure file is flushed and available before serving (Windows safety)
				time.Sleep(150 * time.Millisecond)

				// Wait for all referenced .ts segments to exist and be non-empty (up to 2s)
				for _, line := range playlistLines {
					if strings.HasSuffix(line, ".ts") {
						segPath := filepath.Join(job.TempDir, strings.TrimSpace(line))
						for i := 0; i < 20; i++ { // up to 2s
							info, err := os.Stat(segPath)
							if err == nil && info.Size() > 0 {
								log.Printf("[HLS] Segment ready: %s (size=%d)", segPath, info.Size())
								break
							}
							if i == 19 {
								log.Printf("[HLS] WARNING: Segment not ready after 2s: %s (err=%v)", segPath, err)
							}
							time.Sleep(100 * time.Millisecond)
						}
					}
				}
			}
		}
	}
	// --- END PATCH ---

	log.Printf("[HLS] Serving file: %s", filePath)
	if filepath.Ext(filename) == ".m3u8" {
		c.Writer.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		c.Writer.Header().Set("Content-Disposition", "inline")
	} else if filepath.Ext(filename) == ".ts" {
		c.Writer.Header().Set("Content-Type", "video/MP2T")
		c.Writer.Header().Set("Content-Disposition", "inline")
	}
	http.ServeFile(c.Writer, c.Request, filePath)
}

// StreamTorrentHLSSeek handles /hls/:infohash/:file_idx/seek/:seconds/:filename for on-demand seeking
func StreamTorrentHLSSeek(c *gin.Context, store *models.TorrentStore) {
	infoHash := c.Param("infohash")
	fileIdxStr := c.Param("file_idx")
	seekStr := c.Param("seconds")
	filename := c.Param("filename")
	log.Printf("[HLS-SEEK] Request: infoHash=%s fileIdx=%s seek=%s filename=%s", infoHash, fileIdxStr, seekStr, filename)
	fileIdx, err := strconv.Atoi(fileIdxStr)
	if err != nil {
		log.Printf("[HLS-SEEK] Invalid file index: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file index"})
		return
	}
	seekSeconds, err := strconv.ParseFloat(seekStr, 64)
	if err != nil {
		log.Printf("[HLS-SEEK] Invalid seek offset: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid seek offset"})
		return
	}
	store.Mutex.RLock()
	torrentFile, exists := store.Torrents[infoHash]
	store.Mutex.RUnlock()
	if !exists {
		log.Printf("[HLS-SEEK] Torrent not found: %s", infoHash)
		c.JSON(http.StatusNotFound, gin.H{"error": "Torrent not found"})
		return
	}
	if err := waitForTorrentInfo(torrentFile, 5*time.Second); err != nil {
		log.Printf("[HLS-SEEK] Timed out waiting for torrent metadata: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Timed out waiting for torrent metadata"})
		return
	}
	files := torrentFile.Files()
	if fileIdx < 0 || fileIdx >= len(files) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File index out of range"})
		return
	}
	file := files[fileIdx]

	// --- GUARD: Prevent seeking to the very end (e.g., >99.5% of duration) ---
	store.Mutex.RLock()
	dur := 0.0
	if store.Metadata != nil {
		if m, ok := store.Metadata[infoHash]; ok {
			if d, ok := m[fileIdx]; ok {
				dur = d
			}
		}
	}
	store.Mutex.RUnlock()
	if dur > 0 && seekSeconds > 0.995*dur {
		log.Printf("[HLS-SEEK] Seek offset %.2f is too close to end (duration %.2f), returning empty playlist.", seekSeconds, dur)
		// Return a minimal valid playlist with ENDLIST
		playlist := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n#EXT-X-ENDLIST\n"
		c.Writer.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		c.Writer.Header().Set("Content-Disposition", "inline")
		c.String(http.StatusOK, playlist)
		return
	}

	// Prioritize pieces for the seeked offset (rough estimate)
	fileLength := file.Length()
	pieceLength := int64(torrentFile.Info().PieceLength)
	seekByte := int64(seekSeconds / float64(store.Metadata[infoHash][fileIdx]) * float64(fileLength))
	startPiece := int(seekByte / pieceLength)
	endPiece := startPiece + 10 // prioritize 10 pieces ahead
	numPieces := int((file.Length() + int64(torrentFile.Info().PieceLength) - 1) / int64(torrentFile.Info().PieceLength))
	for i := startPiece; i < endPiece && i < numPieces; i++ {
		torrentFile.Piece(i).SetPriority(torrent.PiecePriorityNow)
	}
	log.Printf("[HLS-SEEK] Prioritizing pieces for seek offset: %f seconds", seekSeconds)

	tempDir, err := os.MkdirTemp("", "hls-seek-")
	if err != nil {
		log.Printf("[HLS-SEEK] Failed to create temp dir: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp dir"})
		return
	}
	defer os.RemoveAll(tempDir)
	exePath, _ := os.Executable()
	ffmpegPath := filepath.Join(filepath.Dir(exePath), "ffmpeg_bin", "ffmpeg.exe")
	playlistPath := filepath.Join(tempDir, "playlist.m3u8")
	segmentPattern := filepath.Join(tempDir, "segment%03d.ts")
	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%f", seekSeconds),
		"-i", "pipe:0",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-ac", "2",
		"-f", "hls",
		"-hls_time", "4",
		"-hls_list_size", "0",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("[HLS-SEEK] Failed to create FFmpeg stdin pipe: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create FFmpeg stdin pipe"})
		return
	}
	reader := file.NewReader()
	go func() {
		io.Copy(stdin, reader)
		stdin.Close()
		log.Printf("[HLS-SEEK] Finished piping data to FFmpeg for %s:%s seek=%s", infoHash, fileIdxStr, seekStr)
	}()
	if err := cmd.Start(); err != nil {
		log.Printf("[HLS-SEEK] Failed to start FFmpeg: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start FFmpeg"})
		return
	}
	log.Printf("[HLS-SEEK] FFmpeg started for %s:%s seek=%s", infoHash, fileIdxStr, seekStr)
	// Wait for requested file to appear
	filePath := filepath.Join(tempDir, filename)
	for i := 0; i < 60; i++ {
		if _, err := os.Stat(filePath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(filePath); err != nil {
		log.Printf("[HLS-SEEK] HLS file not ready: %s", filePath)
		c.JSON(http.StatusNotFound, gin.H{"error": "HLS file not ready"})
		return
	}
	log.Printf("[HLS-SEEK] Serving file: %s", filePath)
	if filepath.Ext(filename) == ".m3u8" {
		c.Writer.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		c.Writer.Header().Set("Content-Disposition", "inline")
	} else if filepath.Ext(filename) == ".ts" {
		c.Writer.Header().Set("Content-Type", "video/MP2T")
		c.Writer.Header().Set("Content-Disposition", "inline")
	}
	http.ServeFile(c.Writer, c.Request, filePath)
}

// Helper: Wait for torrent info
func waitForTorrentInfo(t *torrent.Torrent, timeout time.Duration) error {
	select {
	case <-t.GotInfo():
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout")
	}
}
