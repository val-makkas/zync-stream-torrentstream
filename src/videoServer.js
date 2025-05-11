import { getTorrentInfo } from './torrentManager.js';

function getTorrentStream(infoHash, fileIdx, req, res) {
  const torrent = getTorrentInfo(infoHash);
  if (!torrent) {
    return res.status(404).send('Torrent not found');
  }
  const file = torrent.files[fileIdx];
  if (!file) {
    return res.status(404).send('File not found in torrent');
  }
  const total = file.length;
  const range = req.headers.range;
  // Detect mime type
  let mimeType = 'application/octet-stream';
  if (file.name && file.name.toLowerCase().endsWith('.mp4')) {
    mimeType = 'video/mp4';
  } else if (file.name && file.name.toLowerCase().endsWith('.mkv')) {
    mimeType = 'video/x-matroska';
  }
  let stream;
  if (range) {
    const parts = range.replace(/bytes=/, '').split('-');
    const start = parseInt(parts[0], 10);
    const end = parts[1] ? parseInt(parts[1], 10) : total - 1;
    if (start >= total || end >= total) {
      res.status(416).send('Requested range not satisfiable');
      return;
    }
    const chunkSize = (end - start) + 1;
    res.writeHead(206, {
      'Content-Range': `bytes ${start}-${end}/${total}`,
      'Accept-Ranges': 'bytes',
      'Content-Length': chunkSize,
      'Content-Type': mimeType,
    });
    stream = file.createReadStream({ start, end });
  } else {
    res.writeHead(200, {
      'Content-Length': total,
      'Content-Type': mimeType,
    });
    stream = file.createReadStream();
  }
  // Cleanup and error handling
  const cleanup = () => {
    if (stream && !stream.destroyed) {
      stream.destroy();
    }
  };
  res.on('close', cleanup);
  res.on('finish', cleanup);
  stream.on('error', err => {
    if (res.headersSent) {
      res.end();
    }
    if (err && err.message && err.message.includes('Writable stream closed prematurely')) {
      return;
    }
    console.error('[getTorrentStream] Stream error:', err);
  });
  stream.pipe(res);
}

export { getTorrentStream };
