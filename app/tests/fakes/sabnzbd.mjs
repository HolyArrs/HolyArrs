import { createServer } from 'node:http';
import { sabKey, sabPort } from './ports.mjs';
import { send } from './http.mjs';

/** Starts the fake SABnzbd. */
export function startSabnzbdFake() {
	// Fake SABnzbd: answers the version, the key check and the categories.
	const sab = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (url.pathname !== '/api') return send(res, 404, 'Not Found', 'text/plain');
		const mode = url.searchParams.get('mode');
		if (mode === 'version') return send(res, 200, { version: '5.1.3' });
		if (url.searchParams.get('apikey') !== sabKey) return send(res, 403, 'API Key Incorrect', 'text/plain');
		if (mode === 'get_cats') return send(res, 200, { categories: ['*', 'movies', 'tv'] });
		if (mode === 'queue') return send(res, 200, { queue: { status: 'Idle', kbpersec: '0.00', slots: [] } });
		if (mode === 'history') return send(res, 200, { history: { slots: [] } });
		send(res, 200, { status: false, error: 'not implemented' });
	});
	sab.listen(sabPort, '127.0.0.1');
	return sab;
}
