import { createServer } from 'node:http';
import { oldKey, oldSeerrPort } from './ports.mjs';
import { send } from './http.mjs';

/** Starts the fake old Seerr for the move tests. */
export function startOldSeerrFake() {
	const oldSeerr = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (req.headers['x-api-key'] !== oldKey) return send(res, 401, {});
		const page = (/** @type {unknown[]} */ all) => ({ pageInfo: { pages: 1, page: 1, results: all.length }, results: all });
		switch (url.pathname) {
			case '/api/v1/status':
				return send(res, 200, { version: '2.7.0', commitTag: 'lab' });
			case '/api/v1/user':
				return send(res, 200, page([
					{ id: 1, email: 'luis@example.com', plexUsername: 'luis', userType: 1, permissions: 2 },
					{ id: 2, email: 'pia@example.com', displayName: 'Pia', userType: 2, permissions: 32, movieQuotaLimit: 4, movieQuotaDays: 7 }
				]));
			case '/api/v1/request':
				return send(res, 200, page([{ id: 5, status: 1, is4k: false, createdAt: '2026-10-01T00:00:00.000Z', media: { mediaType: 'movie', tmdbId: 900007 }, requestedBy: { id: 2 } }]));
			case '/api/v1/settings/main':
				return send(res, 200, { applicationTitle: 'Lab asks', defaultPermissions: 32 });
		}
		return send(res, 404, {});
	});
	oldSeerr.listen(oldSeerrPort, '127.0.0.1');
	return oldSeerr;
}
