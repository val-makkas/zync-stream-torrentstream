# absolute-cinema-torrentstream

TorrentStream is a torrent client as a HTTP service for video streaming used in Absolute Cinema.

---

## Features

- **Add torrents via Magnet URI or InfoHash**
- **On-the-fly HLS streaming**
- **Progress and status endpoints**
- **Piece prioritization for fast seeking**
- **Automatic tracker injection**

---

## How It Works

- Torrents are managed in-memory and can be added via HTTP API.
- Metadata is extracted and video duration is determined using ffprobe.
- HLS streaming is provided via endpoints for playlists and segments, using FFmpeg for on-the-fly conversion.
- Piece prioritization enables smooth seeking and playback.
- Progress and status endpoints provide real-time information.
- Torrents can be removed via the API.

---

## Running

**Build the service:**
```sh
go build -o torrentstream main.go
```

**Place FFmpeg and ffprobe binaries:**
- `ffmpeg.exe` in `ffmpeg_bin/`
- `ffprobe.exe` in `ffprobe_bin/`

**Run the service:**
```sh
./torrentstream
```
The server will start on port `5050` by default.

---

## Example Usage

**Add a torrent:**
```sh
curl -X POST http://localhost:5050/add -H "Content-Type: application/json" -d '{"infoHash":"<INFO_HASH>"}'
```

**Get status:**
```sh
curl http://localhost:5050/status/<INFO_HASH>
```

**Stream HLS:**
- Playlist: `http://localhost:5050/hls/<INFO_HASH>/<FILE_IDX>/playlist.m3u8`
- Segment:  `http://localhost:5050/hls/<INFO_HASH>/<FILE_IDX>/segment001.ts`

---

## Architecture

- **main.go:** Entry point, sets up the HTTP server and routes.
- **handlers:** HTTP handlers for all endpoints.
- **models:** Data models and in-memory stores.
- **utils:** Utility functions for media and torrent operations.
- **config:** Application configuration.

---

## License

MIT

---

## Credits

- [anacrolix/torrent](https://github.com/anacrolix/torrent)
- [gin-gonic/gin](https://github.com/gin-gonic/gin)
- [FFmpeg](https://ffmpeg.org/)

---

*For more details, see the source code and comments in each file.*
