import { createServer } from 'node:http';
import { discordPort, fakeInstallToken, tmdbPort } from './ports.mjs';
import { body, send, squash } from './http.mjs';
import { fakeArtistVideos, fakeVideoAddress, fakeYouTube, tmdbMovies } from './movies.mjs';
import { fakeShows, seasonsOf } from './shows.mjs';

/** The videos the fake YouTube's player call was asked for, in order (GET /youtube/lab/calls reads them). */
const youtubeCalls = /** @type {string[]} */ ([]);

/**
 * The fake YouTube, under /youtube on the fake metadata server: the watch
 * page (a visitor id only) and the player call, answered from fakeYouTube.
 * @param {import('node:http').IncomingMessage} req @param {import('node:http').ServerResponse} res @param {URL} url
 */
async function fakeYouTubeServe(req, res, url) {
	const path = url.pathname.slice('/youtube'.length);
	if (req.method === 'GET' && path === '/lab/calls') return send(res, 200, { player: youtubeCalls });
	if (req.method === 'GET' && path === '/watch') return send(res, 200, '<html><script>ytcfg.set({"VISITOR_DATA":"LAB-VISITOR"});</script></html>', 'text/html');
	if (req.method !== 'POST' || path !== '/youtubei/v1/player') return send(res, 404, {});
	let id = '';
	try {
		id = String(JSON.parse(await body(req)).videoId ?? '');
	} catch {
		return send(res, 400, {});
	}
	youtubeCalls.push(id);
	const mode = fakeYouTube[id] ?? 'gone';
	if (mode === 'age') return send(res, 200, { playabilityStatus: { status: 'LOGIN_REQUIRED', reason: 'Sign in to confirm your age' } });
	if (mode === 'gone') return send(res, 200, { playabilityStatus: { status: 'ERROR', reason: 'Video unavailable' } });
	const expire = Math.floor(Date.now() / 1000) + 6 * 3600;
	// A music video: YouTube's HLS manifest (what an artist page plays, relayed).
	if (mode === 'hls') {
		return send(res, 200, {
			playabilityStatus: { status: 'OK' },
			videoDetails: { title: id, lengthSeconds: '200' },
			streamingData: { hlsManifestUrl: `https://manifest.googlevideo.com/api/manifest/hls_variant/expire/${expire}/id/${id}/file/index.m3u8`, formats: [] }
		});
	}
	send(res, 200, {
		playabilityStatus: { status: 'OK' },
		videoDetails: { title: id, lengthSeconds: '90' },
		streamingData: { formats: [{ itag: 18, url: fakeVideoAddress(id, expire), mimeType: 'video/mp4; codecs="avc1.42001E, mp4a.40.2"', width: 640, height: 360 }] }
	});
}

/** @param {typeof fakeShows[number]} s */
function tmdbShow(s) {
	return {
		id: s.tmdb, name: s.name, first_air_date: s.first, overview: s.overview, original_language: 'en', status: s.status, poster_path: `/s${s.tmdb}.jpg`,
		episode_run_time: [42], networks: [{ name: s.network }], external_ids: { imdb_id: s.imdb, tvdb_id: s.tvdb },
		seasons: seasonsOf(s).map((n) => ({ season_number: n, name: n === 0 ? 'Specials' : `Season ${n}` }))
	};
}

/** @param {typeof tmdbMovies[number]} m */
function tmdbMovie(m) {
	return {
		id: m.id, title: m.title, release_date: m.release_date, overview: m.overview, original_language: 'en', runtime: 101,
		poster_path: `/p${m.id}.jpg`, imdb_id: m.imdb_id,
		...('collection' in m && m.collection ? { belongs_to_collection: { id: m.collection.id, name: m.collection.name } } : {}),
		...('videos' in m && m.videos ? { videos: { results: m.videos.map(([key, name, official]) => ({ key, name, official, site: 'YouTube', type: 'Trailer', iso_639_1: 'en' })) } } : {}),
		release_dates: { results: [{ iso_3166_1: 'US', release_dates: m.dates.map(([type, d]) => ({ type, release_date: `${d}T00:00:00.000Z`, certification: m.cert ?? '' })) }] }
	};
}

/** metadataEnv points a HolyArrs at the fake metadata server (no TMDB key). */
export const metadataEnv = {
	HOLYARRS_METADATA_URL: `http://127.0.0.1:${tmdbPort}`,
	HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL: 'true',
	HOLYARRS_TMDB_URL: `http://127.0.0.1:${tmdbPort}`,
	// Telegram Alerts post to the fake Discord server, which also answers sendMessage.
	HOLYARRS_TELEGRAM_URL: `http://127.0.0.1:${discordPort}`,
	// So do Pushover Alerts (POST /1/messages.json).
	HOLYARRS_PUSHOVER_URL: `http://127.0.0.1:${discordPort}`
};

/** Counts of what the fake metadata server answered, for the tests. */
export const metadataCalls = { registered: 0, tmdb: 0 };

/** Starts the fake metadata server, fake TMDB and fake YouTube. */
export function startTmdbFake() {
	// The fake metadata server: registration, /readyz and TMDB reads at
	// /v1/tmdb (answered by the fake TMDB below), like the real one.
	const tmdb = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		// A made-up HolyArrs release feed for the update check (no sign-in).
		if (url.pathname === '/releases/latest') return send(res, 200, { tag_name: 'v9.9.9', html_url: 'https://github.com/HolyArrs/HolyArrs/releases/tag/v9.9.9' });
		if (url.pathname === '/readyz' || url.pathname === '/healthz') return send(res, 200, { ok: true });
		if (url.pathname === '/v1/installations') {
			if (req.method !== 'POST') return send(res, 405, { error: { code: 'method_not_allowed', retry_after_seconds: 0 } });
			metadataCalls.registered++;
			return send(res, 201, { schema_version: 1, token: fakeInstallToken });
		}
		if (url.pathname.startsWith('/youtube/')) return fakeYouTubeServe(req, res, url);
		// An artist's music videos (the hosted MusicBrainz artist, videos only).
		const artistVids = url.pathname.match(/^\/v1\/music\/musicbrainz\/artist\/([0-9a-f-]{36})$/);
		if (artistVids) {
			if (req.headers.authorization !== `Bearer ${fakeInstallToken}`) return send(res, 401, { error: { code: 'unauthorized', retry_after_seconds: 0 } });
			const vids = fakeArtistVideos()[artistVids[1]];
			if (!vids) return send(res, 404, { error: { code: 'not_found', retry_after_seconds: 0 } });
			const fetched = new Date();
			return send(res, 200, { schema_version: 1, source: 'musicbrainz', source_id: artistVids[1], completeness: 'complete', stale: false, fetched_at: fetched.toISOString(), expires_at: new Date(fetched.getTime() + 86_400_000).toISOString(), data: { videos: vids } });
		}
		// The server's trailer pick: only the trailer test movies have one.
		const pick = url.pathname.match(/^\/v1\/trailers\/(movie|tv)\/(\d+)$/);
		if (pick) {
			if (req.headers.authorization !== `Bearer ${fakeInstallToken}`) return send(res, 401, { error: { code: 'unauthorized', retry_after_seconds: 0 } });
			const pm = pick[1] === 'movie' ? tmdbMovies.find((x) => x.id === Number(pick[2])) : undefined;
			if (!pm || !('trailer' in pm) || !pm.trailer) return send(res, 404, { error: { code: 'not_found', retry_after_seconds: 0 } });
			return send(res, 200, { key: pm.trailer, name: `${pm.title} trailer`, language: 'en', checked_at: '2026-11-01T00:00:00Z', status: 'ok' });
		}
		if (!url.pathname.startsWith('/v1/tmdb/')) return send(res, 404, { status_message: 'TMDB reads are at /v1/tmdb' });
		if (req.headers.authorization !== `Bearer ${fakeInstallToken}`) return send(res, 401, { error: { code: 'unauthorized', retry_after_seconds: 0 } });
		metadataCalls.tmdb++;
		url.pathname = url.pathname.slice('/v1/tmdb'.length);
		if (url.pathname === '/search/movie') {
			const q = squash(url.searchParams.get('query') ?? '');
			return send(res, 200, { page: 1, total_pages: 1, results: tmdbMovies.filter((m) => q && squash(m.title).includes(q)).map(tmdbMovie) });
		}
		if (url.pathname === '/search/tv') {
			const q = squash(url.searchParams.get('query') ?? '');
			return send(res, 200, { page: 1, total_pages: 1, results: fakeShows.filter((s) => q && squash(s.name).includes(q)).map(tmdbShow) });
		}
		const findTV = url.pathname.match(/^\/find\/(\w+)$/);
		if (findTV && url.searchParams.get('external_source') === 'tvdb_id') {
			return send(res, 200, { movie_results: [], tv_results: fakeShows.filter((s) => String(s.tvdb) === findTV[1]).map(tmdbShow) });
		}
		const find = url.pathname.match(/^\/find\/(tt\d+)$/);
		if (find) {
			return send(res, 200, { movie_results: tmdbMovies.filter((m) => m.imdb_id === find[1]).map(tmdbMovie), tv_results: fakeShows.filter((s) => s.imdb === find[1]).map(tmdbShow) });
		}
		const tvSeason = url.pathname.match(/^\/tv\/(\d+)\/season\/(\d+)$/);
		const tvShow = fakeShows.find((s) => s.tmdb === Number((tvSeason ?? url.pathname.match(/^\/tv\/(\d+)$/))?.[1]));
		if (tvShow && tvSeason) {
			const n = Number(tvSeason[2]);
			const eps = tvShow.episodes.filter((e) => e[0] === n);
			if (!eps.length) return send(res, 404, { status_message: 'not found' });
			return send(res, 200, {
				season_number: n,
				episodes: eps.map((e) => ({ id: tvShow.tmdb * 1000 + Number(e[0]) * 100 + Number(e[1]), season_number: e[0], episode_number: e[1], name: e[2], air_date: e[3], runtime: 42 }))
			});
		}
		if (tvShow) return send(res, 200, tmdbShow(tvShow));
		// TMDB list 104 (library-showlists.spec.ts): two shows and a movie; a show list takes only the shows.
		if (url.pathname === '/list/104') {
			const shows = fakeShows.filter((s) => s.tmdb === 800104 || s.tmdb === 800105).map((s) => ({ ...tmdbShow(s), media_type: 'tv' }));
			const movies = tmdbMovies.filter((m) => m.id === 900006).map((m) => ({ ...tmdbMovie(m), media_type: 'movie' }));
			return send(res, 200, { id: 104, name: 'Lab shows', page: 1, total_pages: 1, items: [...shows, ...movies] });
		}
		// TMDB list 501: one movie to add and one that is on the Never-add list in the tests.
		if (url.pathname === '/list/501') {
			const items = tmdbMovies.filter((m) => m.id === 900005 || m.id === 900006).map((m) => ({ ...tmdbMovie(m), media_type: 'movie' }));
			return send(res, 200, { id: 501, name: 'Lab list', page: 1, total_pages: 1, items });
		}
		if (url.pathname === '/movie/changes') return send(res, 200, { results: [], page: 1, total_pages: 1 });
		// Genre names, read before a title page's details.
		if (url.pathname === '/genre/movie/list' || url.pathname === '/genre/tv/list') return send(res, 200, { genres: [{ id: 18, name: 'Drama' }, { id: 35, name: 'Comedy' }] });
		// A movie's age ratings alone (the Kid limit check).
		const dates = url.pathname.match(/^\/movie\/(\d+)\/release_dates$/);
		const dm = dates && tmdbMovies.find((x) => x.id === Number(dates[1]));
		if (dm) return send(res, 200, { id: dm.id, results: tmdbMovie(dm).release_dates.results });
		const one = url.pathname.match(/^\/movie\/(\d+)$/);
		const m = one && tmdbMovies.find((x) => x.id === Number(one[1]));
		if (m) return send(res, 200, tmdbMovie(m));
		send(res, 404, { status_message: 'not found' });
	});
	tmdb.listen(tmdbPort, '127.0.0.1');
	return tmdb;
}
