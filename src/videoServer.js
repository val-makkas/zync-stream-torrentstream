import { getTorrentInfo } from './torrentManager.js';
import path from 'path'

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

function downloadTorrentFile(infoHash, fileIdx, req, res) {
  const torrent = getTorrentInfo(infoHash);
  if (!torrent) {
    return res.status(404).json({ success: false, error: 'Torrent not found' });
  }
  
  const file = torrent.files[fileIdx];
  if (!file) {
    return res.status(404).json({ success: false, error: 'File not found in torrent' });
  }

  try {
    // Since WebTorrent is already downloading the file, just get its local path
    // WebTorrent stores files in the torrent's download directory
    const torrentDownloadPath = torrent.path; // Base download directory
    const localFilePath = path.join(torrentDownloadPath, file.path);
    
    // Check if file has started downloading and has some data
    const downloadProgress = (file.downloaded / file.length) * 100;
    
    if (file.downloaded === 0) {
      return res.status(202).json({ 
        success: false, 
        error: 'File is still starting download. Please wait...',
        downloading: true,
        progress: downloadProgress
      });
    }

    // Return the local file path where WebTorrent is already downloading/storing the file
    res.json({
      success: true,
      localPath: localFilePath,
      hash: infoHash,
      segment: fileIdx.toString(),
      downloaded: file.downloaded,
      total: file.length,
      progress: downloadProgress,
      isComplete: file.downloaded === file.length
    });

  } catch (error) {
    console.error('Error in downloadTorrentFile:', error);
    res.status(500).json({ 
      success: false, 
      error: error.message 
    });
  }
}

export { getTorrentStream, downloadTorrentFile };