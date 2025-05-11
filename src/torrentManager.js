import WebTorrent from 'webtorrent';
const path = require('path');
const fs = require('fs');

const torrents = {};

const client = new WebTorrent();

function parseInfoHash(magnet) {
  // Extract infoHash from magnet URI
  const match = magnet.match(/btih:([a-fA-F0-9]{40,})/);
  return match ? match[1].toLowerCase() : null;
}

async function addTorrent(magnet, fileIdx) {
  let infoHash;
  if (magnet.startsWith('magnet:')) {
    infoHash = parseInfoHash(magnet);
  } else {
    infoHash = magnet;
  }
  console.log('[addTorrent] magnet:', magnet);
  console.log('[addTorrent] parsed infoHash:', infoHash);
  if (fileIdx !== undefined) {
    console.log('[addTorrent] fileIdx:', fileIdx);
  }
  let torrent = undefined;
  try {
    const maybePromise = client.get(infoHash);
    if (maybePromise && typeof maybePromise.then === 'function') {
      torrent = await maybePromise;
      console.log('[addTorrent] awaited client.get(infoHash):', torrent);
    } else {
      torrent = maybePromise;
      console.log('[addTorrent] sync client.get(infoHash):', torrent);
    }
  } catch (err) {
    console.error('[addTorrent] error in client.get:', err);
    torrent = undefined;
  }
  if (torrent) {
    if (torrent.ready) {
      console.log('[addTorrent] torrent is ready:', torrent.infoHash);
      return { infoHash: torrent.infoHash, fileIdx };
    } else if (typeof torrent.on === 'function') {
      console.log('[addTorrent] torrent found but not ready, attaching listeners');
      return await new Promise((resolve, reject) => {
        torrent.on('ready', () => {
          torrents[torrent.infoHash] = torrent;
          console.log('[addTorrent] torrent ready event fired:', torrent.infoHash);
          resolve({ infoHash: torrent.infoHash, fileIdx });
        });
        torrent.on('error', (err) => {
          console.error('[addTorrent] torrent error:', err);
          reject(err);
        });
        setTimeout(() => {
          console.error('[addTorrent] timeout waiting for torrent metadata');
          reject(new Error('Torrent metadata timeout (no seeds?)'));
        }, 30000);
      });
    } else {
      console.error('[addTorrent] Invalid torrent instance:', torrent);
      throw new Error('Torrent instance is invalid');
    }
  }
  console.log('[addTorrent] Adding new torrent to client');
  return await new Promise((resolve, reject) => {
    const DOWNLOAD_DIR = path.resolve(__dirname, '../downloads');
    if (!fs.existsSync(DOWNLOAD_DIR)) {
      fs.mkdirSync(DOWNLOAD_DIR, { recursive: true });
    }

    const torrent = client.add(magnet, { path: DOWNLOAD_DIR, destroyStoreOnDestroy: true });
    torrent.on('ready', () => {
      torrents[torrent.infoHash] = torrent;
      console.log('[addTorrent] new torrent ready event fired:', torrent.infoHash);
      resolve({ infoHash: torrent.infoHash, fileIdx });
    });
    torrent.on('error', (err) => {
      console.error('[addTorrent] new torrent error:', err);
      reject(err);
    });
    setTimeout(() => {
      console.error('[addTorrent] timeout waiting for new torrent metadata');
      reject(new Error('Torrent metadata timeout (no seeds?)'));
    }, 30000);
  });
}

function removeTorrent(infoHash) {
  return new Promise((resolve, reject) => {
    const torrent = torrents[infoHash] || client.get(infoHash);
    if (!torrent) return resolve();
    torrent.destroy(err => {
      if (err) return reject(err);
      delete torrents[infoHash];
      resolve();
    });
  });
}

function getTorrentInfo(infoHash) {
  return torrents[infoHash] || client.get(infoHash);
}

export { addTorrent, removeTorrent, getTorrentInfo };
