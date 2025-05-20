# zync-stream-torrentstream

TorrentStream is a Node.js microservice for streaming torrents as video over HTTP, used in Absolute Cinema.

---

## Features

- **Add torrents via Magnet URI or InfoHash**
- **Direct video streaming endpoint**
- **In-memory torrent management**
- **Automatic download directory management**
- **Range requests for efficient seeking**
- **CORS support for web clients**

---

## How It Works

- Torrents are managed in-memory and can be added via HTTP API.
- Video files are streamed directly from torrents using range requests.
- The service uses [WebTorrent](https://webtorrent.io/) for torrenting and Express for the HTTP API.
- FFmpeg is included as a dependency for future HLS or transcoding features (not yet implemented).
- Torrents can be removed via the API.

---

## Running

**Install dependencies:**
```sh
npm install
```

**Start the service:**
```sh
npm start
```
The server will start on port `8888` by default (configurable via `.env`).

---

## Example Usage

**Add a torrent:**
```sh
curl -X POST http://localhost:8888/add -H "Content-Type: application/json" -d '{"magnet":"<MAGNET_URI>", "fileIdx":0}'
```

**Stream a video file:**
```sh
curl -H "Range: bytes=0-" http://localhost:8888/stream/<INFO_HASH>/<FILE_IDX> --output video.mp4
```

**Remove a torrent:**
```sh
curl -X DELETE http://localhost:8888/remove/<INFO_HASH>
```

---

## Architecture

- **src/index.js:** Entry point, sets up the Express server and routes.
- **src/torrentManager.js:** Torrent management logic (add, remove, get info).
- **src/videoServer.js:** Handles video streaming from torrent files.
- **src/utils.js:** Utility functions (currently empty).
- **downloads/:** Directory for storing downloaded torrent data.
- **public/:** Static files (currently unused).

---

## License

MIT

---

## Credits

- [WebTorrent](https://webtorrent.io/)
- [Express](https://expressjs.com/)
- [FFmpeg](https://ffmpeg.org/)

---

*For more details, see the source code and comments in each file.*