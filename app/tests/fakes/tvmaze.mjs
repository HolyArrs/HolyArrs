import { createServer } from 'node:http';
import { tvmazePort } from './ports.mjs';
import { send, squash } from './http.mjs';
import { fakeShows } from './shows.mjs';

/** @param {typeof fakeShows[number]} s */
function mazeShow(s) {
	return {
		id: s.tvmaze, name: s.name, premiered: s.first, summary: `<p>${s.overview}</p>`, language: 'English', status: s.status === 'Ended' ? 'Ended' : 'Running',
		averageRuntime: 42, schedule: { time: '21:00' }, network: { name: s.network, country: { timezone: 'America/New_York' } },
		externals: { thetvdb: s.tvdb, imdb: s.imdb }, image: null
	};
}

/** Starts the fake TVMaze. */
export function startTvmazeFake() {
	// Fake TVMaze: the same shows by their TVMaze, TVDB and IMDb ids.
	const tvmaze = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (url.pathname === '/search/shows') {
			const q = squash(url.searchParams.get('q') ?? '');
			return send(res, 200, fakeShows.filter((s) => q && squash(s.name).includes(q)).map((s) => ({ score: 1, show: mazeShow(s) })));
		}
		if (url.pathname === '/lookup/shows') {
			const s = fakeShows.find((x) => String(x.tvdb) === url.searchParams.get('thetvdb') || x.imdb === url.searchParams.get('imdb'));
			return s ? send(res, 200, mazeShow(s)) : send(res, 404, { name: 'Not Found' });
		}
		const eps = url.pathname.match(/^\/shows\/(\d+)\/episodes$/);
		const one = fakeShows.find((x) => x.tvmaze === Number((eps ?? url.pathname.match(/^\/shows\/(\d+)$/))?.[1]));
		if (one && eps) {
			return send(
				res,
				200,
				one.episodes.map((e) => ({
					id: one.tvmaze * 1000 + Number(e[0]) * 100 + Number(e[1]), season: e[0] === 0 ? 1 : e[0], number: e[0] === 0 ? null : e[1], name: e[2],
					airdate: e[3], airstamp: `${e[3]}T01:00:00+00:00`, runtime: 42
				}))
			);
		}
		if (one) return send(res, 200, mazeShow(one));
		send(res, 404, { name: 'Not Found' });
	});
	tvmaze.listen(tvmazePort, '127.0.0.1');
	return tvmaze;
}
