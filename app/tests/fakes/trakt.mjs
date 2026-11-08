import { createServer } from 'node:http';
import { traktKey, traktPort } from './ports.mjs';
import { send } from './http.mjs';

/** Starts the fake Trakt. */
export function startTraktFake() {
	// Fake Trakt: lab/favourites holds Northern Kettle (by IMDb id only).
	const trakt = createServer((req, res) => {
		if (req.headers['trakt-api-key'] !== traktKey || req.headers['trakt-api-version'] !== '2') return send(res, 403, { error: 'key' });
		const url = new URL(req.url ?? '/', 'http://x');
		// lab/tv-picks holds Rookery Dial (by TVDB id only) and Bellwether Ferry (library-showlists.spec.ts).
		if (url.pathname === '/users/lab/lists/tv-picks/items/show') {
			res.setHeader('X-Pagination-Page-Count', '1');
			return send(res, 200, [
				{ type: 'show', show: { title: 'Rookery Dial', year: 2019, ids: { tvdb: 810106 } } },
				{ type: 'show', show: { title: 'Bellwether Ferry', year: 2021, ids: { tmdb: 800104, tvdb: 810104, imdb: 'tt8001040' } } }
			]);
		}
		if (url.pathname !== '/users/lab/lists/favourites/items/movie') return send(res, 404, { error: 'no list' });
		res.setHeader('X-Pagination-Page-Count', '1');
		send(res, 200, [{ type: 'movie', movie: { title: 'Northern Kettle', year: 2018, ids: { imdb: 'tt9000070' } } }]);
	});
	trakt.listen(traktPort, '127.0.0.1');
	return trakt;
}
