import { createServer } from 'node:http';
import { qbitPort } from './ports.mjs';
import { body, send } from './http.mjs';

/** Starts the fake qBittorrent. */
export function startQbittorrentFake() {
	// Fake qBittorrent: every torrent it is given downloads at 40% for
	// ever (GET /set?progress= moves them on); GET /state shows what was
	// added and deleted.
	/** @type {Map<string, Record<string, unknown>>} */
	const torrents = new Map();
	/** @type {string[]} */
	const deleted = [];
	/** @type {string[]} */
	const added = [];
	/** @type {Record<string, string>[]} */
	const shareLimits = [];
	let mainRid = 0;
	const qbit = createServer(async (req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		const form = new URLSearchParams(req.method === 'POST' ? await body(req) : url.search);
		switch (url.pathname) {
			case '/state':
				return send(res, 200, { added, deleted, shareLimits });
			case '/set': {
				// Test control: move every torrent on (progress, speed, eta),
				// or only the one named by hash; state, save_path and files
				// (names joined by |) finish it with files to import (filesize: their listed size).
				for (const t of torrents.values()) {
					if (form.get('hash') && form.get('hash') !== t.hash) continue;
					for (const k of ['progress', 'dlspeed', 'eta']) if (form.get(k)) t[k] = Number(form.get(k));
					for (const k of ['state', 'save_path']) if (form.get(k)) t[k] = form.get(k);
					if (form.get('files')) t.files = (form.get('files') ?? '').split('|').map((name) => ({ name, size: Number(form.get('filesize') ?? 1000), progress: 1 }));
				}
				return send(res, 200, { ok: true });
			}
			case '/api/v2/app/version':
				return send(res, 200, 'v5.2.4', 'text/plain');
			case '/api/v2/app/webapiVersion':
				return send(res, 200, '2.11.4', 'text/plain');
			case '/api/v2/torrents/setShareLimits':
				shareLimits.push(Object.fromEntries(form));
				return send(res, 200, '', 'text/plain');
			case '/api/v2/torrents/createCategory':
				return send(res, 200, '', 'text/plain');
			case '/api/v2/torrents/add': {
				const link = form.get('urls') ?? '';
				const hash = (link.match(/\/dl\/([0-9a-f]+)\.torrent/) ?? [])[1];
				if (!hash) return send(res, 200, 'Fails.', 'text/plain');
				added.push(hash);
				torrents.set(hash, { hash, name: `torrent-${hash.slice(-6)}`, category: form.get('category') ?? '', progress: 0.4, dlspeed: 1048576, eta: 600,
					state: 'downloading', save_path: form.get('savepath') ?? '/downloads', size: 4000000000 });
				return send(res, 200, 'Ok.', 'text/plain');
			}
			case '/api/v2/torrents/info': {
				const hashes = form.get('hashes');
				const cat = form.get('category');
				return send(res, 200, [...torrents.values()].filter((t) => (!hashes || hashes.includes(String(t.hash))) && (!cat || t.category === cat)).map(({ files, ...t }) => t));
			}
			case '/api/v2/sync/maindata': {
				// HolyArrs follows qBittorrent through sync/maindata: always a full answer here.
				mainRid++;
				const list = Object.fromEntries([...torrents.values()].map(({ files, ...t }) => [t.hash, t]));
				return send(res, 200, { rid: mainRid, full_update: true, torrents: list, categories: {}, server_state: { dl_info_speed: 0, up_info_speed: 0, connection_status: 'connected', free_space_on_disk: 1_000_000_000_000 } });
			}
			case '/api/v2/torrents/properties': {
				const t = torrents.get(form.get('hash') ?? '');
				if (!t) return send(res, 404, 'Not Found', 'text/plain');
				const size = Number(t.size);
				return send(res, 200, { total_size: size, total_downloaded: Math.round(size * Number(t.progress)), total_uploaded: 0, dl_speed: t.dlspeed, up_speed: 0, eta: t.eta, share_ratio: 0, seeds: 3, seeds_total: 10, peers: 1, peers_total: 4, addition_date: 1, completion_date: -1, save_path: t.save_path, is_private: false });
			}
			case '/api/v2/torrents/files': {
				const t = torrents.get(form.get('hash') ?? '');
				return send(res, 200, t?.files ?? []);
			}
			case '/api/v2/torrents/delete':
				for (const h of (form.get('hashes') ?? '').split('|')) {
					torrents.delete(h);
					deleted.push(h);
				}
				return send(res, 200, '', 'text/plain');
		}
		send(res, 404, 'Not Found', 'text/plain');
	});
	qbit.listen(qbitPort, '127.0.0.1');
	return qbit;
}
