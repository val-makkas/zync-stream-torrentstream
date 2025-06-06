import express from 'express';
import cors from 'cors';
import dotenv from 'dotenv';
import { addTorrent, removeTorrent, cleanupDownloads } from './torrentManager.js';
import { getTorrentStream, downloadTorrentFile } from './videoServer.js';

dotenv.config();

const PORT = process.env.PORT || 8888;

const app = express();
app.use(cors());
app.use(express.json());

cleanupDownloads();

app.post('/add', async (req, res) => {
  const { magnet, fileIdx } = req.body;
  if (!magnet) return res.status(400).json({ error: 'Missing magnet URI' });
  if (fileIdx === undefined || fileIdx === null) return res.status(400).json({ error: 'Missing fileIdx' });
  try {
    const info = await addTorrent(magnet, fileIdx);
    res.json(info);
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.get('/stream/:infoHash/:fileIdx', (req, res) => {
  const { infoHash, fileIdx } = req.params;
  return getTorrentStream(infoHash, Number(fileIdx), req, res);
});

app.get('/download/:infoHash/:fileIdx', (req, res) => {
  const { infoHash, fileIdx } = req.params;
  return downloadTorrentFile(infoHash, Number(fileIdx), req, res);
});

app.delete('/remove/:infoHash', async (req, res) => {
  const { infoHash } = req.params;
  try {
    await removeTorrent(infoHash);
    res.json({ removed: true });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.listen(PORT);
