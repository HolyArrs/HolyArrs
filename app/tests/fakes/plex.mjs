import { createServer } from 'node:http';
import { plexPort, plexServerToken } from './ports.mjs';
import { body, send } from './http.mjs';

/** Starts the fake Plex (plex.tv sign-in, discover and the media server). */
export function startPlexFake() {
	let nextPin = 100;
	const confirmed = new Set();
	// PUT /lab/plex-user names who the next pin signs in as (default plexkid),
	// so two Plex accounts can be told apart.
	/** @type {Map<string, { id: number, username: string, email?: string }>} */
	const plexUsers = new Map();
	/** @type {{ id: number, username: string, email?: string } | null} */
	let nextPlexUser = null;
	// Plex's discover service (/discover/...): each token's watchlist, set by
	// PUT /lab/watchlist {token, items}; token "*" is every known token's.
	/** @type {Map<string, unknown[]>} */
	const plexWatchlists = new Map();
	let plexMovieRoot = '';
	/** @type {{ section: string, path: string }[]} */
	const plexScans = [];
	const plex = createServer(async (req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (req.method === 'POST' && url.pathname === '/api/v2/pins') {
			++nextPin;
			if (nextPlexUser) plexUsers.set(`ptok${nextPin}`, nextPlexUser);
			nextPlexUser = null;
			return send(res, 201, { id: nextPin, code: `CODE${nextPin}` });
		}
		if (req.method === 'PUT' && url.pathname === '/lab/plex-user') {
			nextPlexUser = JSON.parse((await body(req)) || 'null');
			return send(res, 204, '');
		}
		if (req.method === 'PUT' && url.pathname === '/lab/watchlist') {
			const b = JSON.parse((await body(req)) || '{}');
			plexWatchlists.set(String(b.token ?? '*'), Array.isArray(b.items) ? b.items : []);
			return send(res, 204, '');
		}
		if (url.pathname.startsWith('/discover/')) {
			const tok = String(req.headers['x-plex-token'] ?? '');
			if (!/^ptok\d+$/.test(tok) || req.headers.accept !== 'application/json') return send(res, 401, '', 'text/plain');
			const items = /** @type {{ ratingKey?: unknown }[]} */ (plexWatchlists.get(tok) ?? plexWatchlists.get('*') ?? []);
			if (req.method === 'GET' && url.pathname === '/discover/library/sections/watchlist/all') {
				const list = items.slice(Number(url.searchParams.get('X-Plex-Container-Start') ?? 0), Number(url.searchParams.get('X-Plex-Container-Size') ?? 20));
				return send(res, 200, { MediaContainer: { size: list.length, totalSize: items.length, Metadata: list } });
			}
			const one = url.pathname.match(/^\/discover\/library\/metadata\/([^/]+)$/);
			if (req.method === 'GET' && one) {
				const list = items.filter((m) => String(m.ratingKey) === decodeURIComponent(one[1]));
				return send(res, 200, { MediaContainer: { size: list.length, Metadata: list } });
			}
		}
		const pin = url.pathname.match(/^\/api\/v2\/pins\/(\d+)$/);
		if (req.method === 'GET' && pin) return send(res, 200, { id: Number(pin[1]), authToken: confirmed.has(pin[1]) ? `ptok${pin[1]}` : null });
		if (req.method === 'GET' && url.pathname === '/api/v2/user') {
			const u = plexUsers.get(String(req.headers['x-plex-token'] ?? '')) ?? { id: 9001, username: 'plexkid', email: 'kid@example.test' };
			return send(res, 200, { uuid: 'u', email: '', ...u });
		}
		if (req.method === 'GET' && url.pathname === '/api/v2/resources') return send(res, 200, [{ clientIdentifier: 'fake-server', provides: 'server' }]);
		// The page the person sees on "Plex": opening it confirms the pin.
		if (req.method === 'GET' && url.pathname === '/app/auth') {
			const code = new URLSearchParams(url.hash || url.search).get('code') ?? '';
			for (let i = 100; i <= nextPin; i++) confirmed.add(String(i));
			return send(res, 200, `<!doctype html><title>Fake Plex</title><p>Signed in ${code}</p>`, 'text/html');
		}
		// Plex as a media server. The test names the movie library's folder
		// (PUT /lab/movie-library), HolyArrs lists the libraries and asks for
		// scans, and GET /lab/scans reads back each scan asked for.
		if (req.method === 'PUT' && url.pathname === '/lab/movie-library') {
			plexMovieRoot = String(JSON.parse((await body(req)) || '{}').path ?? '');
			return send(res, 204, '');
		}
		if (req.method === 'GET' && url.pathname === '/lab/scans') return send(res, 200, plexScans);
		if (url.pathname.startsWith('/library/sections')) {
			if (req.headers['x-plex-token'] !== plexServerToken) return send(res, 401, '', 'text/plain');
			if (req.method === 'GET' && url.pathname === '/library/sections') {
				const path = plexMovieRoot.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;');
				return send(res, 200, `<MediaContainer size="1"><Directory key="7" type="movie" title="Lab Movies"><Location id="7" path="${path}"/></Directory></MediaContainer>`, 'application/xml');
			}
			const scan = url.pathname.match(/^\/library\/sections\/([^/]+)\/refresh$/);
			if (req.method === 'GET' && scan) {
				plexScans.push({ section: scan[1], path: url.searchParams.get('path') ?? '' });
				return send(res, 200, '', 'application/xml');
			}
		}
		send(res, 404, { error: 'no route' });
	});
	plex.listen(plexPort, '127.0.0.1');
	return plex;
}
