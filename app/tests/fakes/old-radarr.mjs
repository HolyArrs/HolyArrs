import { createServer } from 'node:http';
import { oldKey, oldRadarrPort } from './ports.mjs';
import { send } from './http.mjs';

/** Starts the fake old Radarr for the move tests. */
export function startOldRadarrFake() {
	// The old apps, written from the move readers' fixtures (the manuals'
	// API paths and fields): a Radarr with one movie, and a Seerr with two
	// people and one open ask. Secrets are masked as the apps mask them.
	const oldRoutes = {
		'/api/v3/system/status': { appName: 'Radarr', version: '6.4.4', migrationVersion: 242 },
		'/api/v3/movie': [{ id: 1, tmdbId: 900016, imdbId: 'tt9000160', title: 'Lodestone Ferry', year: 2010, path: '/old/movies/Lodestone Ferry (2010)', rootFolderPath: '/old/movies', monitored: true, qualityProfileId: 1, tags: [1] }],
		'/api/v3/tag': [{ id: 1, label: 'family' }],
		'/api/v3/rootfolder': [{ id: 1, path: '/old/movies', accessible: true }],
		'/api/v3/qualityprofile': [{ id: 1, name: 'HD-1080p', upgradeAllowed: true, cutoff: 7, items: [{ quality: { id: 7, name: 'Bluray-1080p' }, items: [], allowed: true }], formatItems: [] }],
		'/api/v3/config/naming': { id: 1, renameMovies: true, standardMovieFormat: '{Movie Title} ({Release Year}) {Quality Full}' },
		'/api/v3/indexer': [{ id: 1, name: 'Old Torznab', implementation: 'Torznab', protocol: 'torrent', priority: 25, enableRss: true, fields: [{ name: 'baseUrl', value: 'http://127.0.0.1:9/old/' }, { name: 'apiKey', value: '********', privacy: 'apiKey' }] }],
		'/api/v3/downloadclient': [{ id: 1, name: 'Old qBit', implementation: 'QBittorrent', enable: true, priority: 1, protocol: 'torrent', fields: [{ name: 'host', value: 'old-qbit' }, { name: 'port', value: 8080 }, { name: 'username', value: 'admin' }, { name: 'password', value: '********', privacy: 'password' }] }],
		'/api/v3/notification': [{ id: 1, name: 'Old Discord', implementation: 'Discord', onGrab: true, fields: [{ name: 'webHookUrl', value: '********', privacy: 'apiKey' }] }, { id: 2, name: 'Old Pushover', implementation: 'Pushover', fields: [] }]
	};
	const oldRadarr = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (req.headers['x-api-key'] !== oldKey) return send(res, 401, {});
		const ans = /** @type {Record<string, unknown>} */ (oldRoutes)[url.pathname];
		if (ans !== undefined) return send(res, 200, ans);
		return send(res, 200, url.pathname.endsWith('/naming') ? {} : []);
	});
	oldRadarr.listen(oldRadarrPort, '127.0.0.1');
	return oldRadarr;
}
