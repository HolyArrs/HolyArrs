import { createServer } from 'node:http';
import { mdblistKey, mdblistPort } from './ports.mjs';
import { send } from './http.mjs';

/** What reached the fake MDBList: reads of the public link and of the API. */
const hits = { site: 0, api: 0 };

/** Starts the fake MDBList. lab/mdb-picks holds Hollow Barometer (lists.spec.ts). */
export function startMdblistFake() {
	const item = { id: 900090, imdb_id: 'tt9000900', mediatype: 'movie', title: 'Hollow Barometer', release_year: 2012 };
	const mdblist = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (url.pathname === '/hits') return send(res, 200, hits);
		// The public link (HOLYARRS_MDBLIST_SITE_URL), read without a key.
		if (url.pathname === '/lists/lab/mdb-picks/json') {
			hits.site++;
			if (url.searchParams.has('apikey')) return send(res, 400, { error: 'no key here' });
			return send(res, 200, [{ ...item, tvdbid: 0 }]);
		}
		// The API (HOLYARRS_MDBLIST_URL), only with the test key.
		if (url.pathname === '/api/lists/lab/mdb-picks/items') {
			hits.api++;
			if (url.searchParams.get('apikey') !== mdblistKey) return send(res, 401, { error: 'key' });
			return send(res, 200, { movies: [{ ...item, tvdb_id: 0 }], shows: [], pagination: { limit: 1000, offset: 0, total: 1, has_more: false } });
		}
		send(res, 404, { error: 'no list' });
	});
	mdblist.listen(mdblistPort, '127.0.0.1');
	return mdblist;
}
