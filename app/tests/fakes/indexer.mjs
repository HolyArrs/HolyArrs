import { createServer } from 'node:http';
import { indexerKey, indexerPort } from './ports.mjs';
import { squash } from './http.mjs';
import { fakeArtists } from './artists.mjs';
import { tmdbMovies } from './movies.mjs';
import { fakeShows } from './shows.mjs';

const allowKeylessIndexer = process.env.HOLYARRS_E2E_INDEXER_NO_KEY === '1';

/** How long the fake Source takes to answer a movie search. */
export const searchDelayMs = 2500;

/** @param {number} tmdbId @param {number} releaseNumber */
export const movieReleaseHash = (tmdbId, releaseNumber) => `${tmdbId}`.padStart(8, '0') + `${releaseNumber}`.repeat(32);

/** @param {typeof tmdbMovies[number]} m */
function releasesFor(m) {
	const dotted = `${m.title.replace(/[^A-Za-z0-9]+/g, '.')}.${m.release_date.slice(0, 4)}`;
	const one = (/** @type {string} */ name, /** @type {number} */ size, /** @type {number} */ seeders, /** @type {string} */ hash) =>
		`<item><title>${name}</title><guid>${name}</guid><pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>` +
		`<enclosure url="http://127.0.0.1:${indexerPort}/dl/${hash}.torrent" length="${size}" type="application/x-bittorrent"/>` +
		`<torznab:attr name="seeders" value="${seeders}"/><torznab:attr name="peers" value="${seeders + 5}"/><torznab:attr name="infohash" value="${hash}"/></item>`;
	return one(`${dotted}.1080p.WEB-DL.H264-HOLYARRS`, 4_000_000_000, 50, movieReleaseHash(m.id, 1)) +
		one(`${dotted}.720p.WEB.H264-HOLYARRS`, 1_500_000_000, 20, movieReleaseHash(m.id, 2)) +
		one(`${dotted}.HDCAM.x264-NOPE`, 1_000_000_000, 5, movieReleaseHash(m.id, 3));
}

/**
 * albumReleasesFor answers an album search (t=music, or a text search that
 * names an artist and one of its albums): a FLAC copy, an MP3 320 copy and
 * a 128 kbps copy the music Pick rules refuse. Sizes suit a 10-minute album.
 * @param {string} artist
 * @param {string} album
 * @param {string} year
 */
export function albumReleasesFor(artist, album, year) {
	const one = (/** @type {string} */ name, /** @type {number} */ size, /** @type {number} */ seeders) => {
		const hash = [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 11).toString(16).padStart(8, '0').repeat(5);
		return `<item><title>${name}</title><guid>${name}</guid><pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>` +
			`<enclosure url="http://127.0.0.1:${indexerPort}/dl/${hash}.torrent" length="${size}" type="application/x-bittorrent"/>` +
			`<torznab:attr name="seeders" value="${seeders}"/><torznab:attr name="peers" value="${seeders + 5}"/><torznab:attr name="infohash" value="${hash}"/></item>`;
	};
	const base = `${artist} - ${album} (${year})`;
	return one(`${base} [FLAC]`, 60_000_000, 30) + one(`${base} [MP3 320]`, 20_000_000, 40) + one(`${base} [128kbps]`, 9_000_000, 50);
}

/**
 * showReleasesFor answers a show search (t=tvsearch): for an episode a
 * 1080p and a 720p web copy; for a whole season a 1080p season pack beside
 * the 1080p copy of each aired episode of that season.
 * @param {typeof fakeShows[number]} s
 * @param {number} season
 * @param {number | null} ep
 */
function showReleasesFor(s, season, ep) {
	const dotted = s.name.replace(/[^A-Za-z0-9]+/g, '.');
	const two = (/** @type {number} */ n) => String(n).padStart(2, '0');
	const one = (/** @type {string} */ name, /** @type {number} */ size, /** @type {number} */ seeders) => {
		const hash = [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7).toString(16).padStart(8, '0').repeat(5);
		return `<item><title>${name}</title><guid>${name}</guid><pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>` +
			`<enclosure url="http://127.0.0.1:${indexerPort}/dl/${hash}.torrent" length="${size}" type="application/x-bittorrent"/>` +
			`<torznab:attr name="seeders" value="${seeders}"/><torznab:attr name="peers" value="${seeders + 5}"/><torznab:attr name="infohash" value="${hash}"/></item>`;
	};
	const aired = s.episodes.filter((e) => Number(e[0]) === season && String(e[3]) < '2090');
	if (ep !== null) {
		const code = `S${two(season)}E${two(ep)}`;
		return one(`${dotted}.${code}.1080p.WEB-DL.H264-HOLYARRS`, 1_200_000_000, 40) + one(`${dotted}.${code}.720p.WEB.H264-HOLYARRS`, 600_000_000, 30) +
			('uhd' in s ? one(`${dotted}.${code}.2160p.WEB-DL.DV.H265-HOLYARRS`, 6_000_000_000, 35) : '');
	}
	return one(`${dotted}.S${two(season)}.1080p.WEB-DL.H264-HOLYARRS`, aired.length * 1_200_000_000, 60) +
		('uhd' in s ? one(`${dotted}.S${two(season)}.2160p.WEB-DL.DV.H265-HOLYARRS`, aired.length * 6_000_000_000, 45) : '') +
		aired.map((e) => one(`${dotted}.S${two(season)}E${two(Number(e[1]))}.1080p.WEB-DL.H264-HOLYARRS`, 1_200_000_000, 40)).join('');
}

/** The fake indexer's info hash for an anime Release name. @param {string} name */
export const animeReleaseHash = (name) => [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 13).toString(16).padStart(8, '0').repeat(5);

/**
 * animeReleasesFor answers an anime show's episode search the way fansub
 * groups name things: by running number, never by season. season 0 means
 * ep is already a running number (a show set to absolute order); otherwise
 * the running number is counted over the regular episodes in aired order.
 * It gives a 1080p and a 720p copy of the episode asked for and a 1080p
 * copy of the next one, as a text search on a real site also brings back.
 * @param {typeof fakeShows[number]} s
 * @param {number} season
 * @param {number | null} ep
 */
function animeReleasesFor(s, season, ep) {
	if (ep === null) return '';
	const regular = s.episodes.filter((e) => Number(e[0]) > 0);
	const abs = season === 0 ? ep : regular.findIndex((e) => Number(e[0]) === season && Number(e[1]) === ep) + 1;
	if (abs < 1) return '';
	const one = (/** @type {string} */ name, /** @type {number} */ size, /** @type {number} */ seeders) => {
		const hash = animeReleaseHash(name);
		return `<item><title>${name}</title><guid>${name}</guid><pubDate>Mon, 01 Jan 2024 00:00:00 +0000</pubDate>` +
			`<enclosure url="http://127.0.0.1:${indexerPort}/dl/${hash}.torrent" length="${size}" type="application/x-bittorrent"/>` +
			`<torznab:attr name="seeders" value="${seeders}"/><torznab:attr name="peers" value="${seeders + 5}"/><torznab:attr name="infohash" value="${hash}"/></item>`;
	};
	const two = (/** @type {number} */ n) => String(n).padStart(2, '0');
	return one(`[Lab Fansub] ${s.name} - ${two(abs)} [1080p]`, 1_300_000_000, 60) + one(`[Lab Fansub] ${s.name} - ${two(abs)} [720p]`, 650_000_000, 45) +
		(abs < regular.length ? one(`[Lab Fansub] ${s.name} - ${two(abs + 1)} [1080p]`, 1_300_000_000, 70) : '');
}

/** Starts the fake Torznab Source and rss feed. */
export function startIndexerFake() {
	const indexer = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		res.setHeader('Content-Type', 'application/xml');
		// A site behind a bot check: a check page, as Cloudflare answers it.
		if (url.pathname.startsWith('/guarded/')) {
			res.writeHead(403, { 'Content-Type': 'text/html', 'Cf-Mitigated': 'challenge', Server: 'cloudflare' });
			res.end('<html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={}</script></body></html>');
			return;
		}
		// A Jackett at /jackett: its sign-in page, and its list of sites set up there
		// (one, whose feed is this fake indexer). A wrong key gets 200 with an error, as Jackett answers.
		if (url.pathname === '/jackett/UI/Login') {
			res.setHeader('Content-Type', 'text/html');
			res.end('<html><head><title>Jackett</title></head><body>Jackett</body></html>');
			return;
		}
		if (url.pathname === '/jackett/api/v2.0/indexers/all/results/torznab/api') {
			if (url.searchParams.get('apikey') !== indexerKey) {
				res.end('<?xml version="1.0" encoding="UTF-8"?><error code="100" description="Invalid API Key" />');
				return;
			}
			res.end('<?xml version="1.0" encoding="UTF-8"?><indexers><indexer id="labsite" configured="true"><title>Lab Jackett Site</title><type>public</type>' +
				'<caps><categories><category id="2000" name="Movies"><subcat id="2040" name="Movies/HD"/></category><category id="5000" name="TV"/></categories></caps></indexer></indexers>');
			return;
		}
		if (url.pathname === '/rss') {
			res.end('<?xml version="1.0"?><rss version="2.0"><channel><title>lab</title>' +
				'<item><title>Lab.Movie.2024.1080p.WEB.H264-HOLYARRS</title><enclosure url="http://127.0.0.1/a.torrent" length="1000" type="application/x-bittorrent"/></item>' +
				'<item><title>Lab.Movie.2024.720p.WEB.H264-HOLYARRS</title><enclosure url="http://127.0.0.1/b.torrent" length="900" type="application/x-bittorrent"/></item>' +
				'</channel></rss>');
			return;
		}
		if (!allowKeylessIndexer && url.searchParams.get('apikey') !== indexerKey) {
			res.statusCode = 401;
			res.end('<error code="100" description="wrong key"/>');
			return;
		}
		// A movie search by IMDb id answers after a short wait (so Home can
		// show it running) with three made-up Releases: a 1080p and a 720p
		// web copy, and a cam copy the default Pick rules refuse. A search
		// with no words (the feed check) finds nothing.
		const t = url.searchParams.get('t');
		if (t === 'tvsearch') {
			// A show by its TVDB id (or name), a season and maybe an episode.
			const tvdb = Number(url.searchParams.get('tvdbid'));
			const q = squash(url.searchParams.get('q') ?? '');
			const s = fakeShows.find((x) => x.tvdb === tvdb) ?? fakeShows.find((x) => q && q.startsWith(squash(x.name)));
			const season = Number(url.searchParams.get('season') ?? '0');
			const ep = url.searchParams.get('ep');
			const items = !s ? '' : 'anime' in s ? animeReleasesFor(s, season, ep ? Number(ep) : null) : showReleasesFor(s, season, ep ? Number(ep) : null);
			setTimeout(() => res.end(`<?xml version="1.0"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><title>lab</title>${items}</channel></rss>`), s ? 1500 : 0);
			return;
		}
		// An album search: by artist and album, or words naming both.
		const words = squash(`${url.searchParams.get('artist') ?? ''}${url.searchParams.get('album') ?? ''}${url.searchParams.get('q') ?? ''}`);
		const artist = (t === 'music' || t === 'search') && words ? fakeArtists.find((a) => words.startsWith(squash(a.name))) : undefined;
		const album = artist?.albums.find((x) => words === squash(artist.name + x[1]));
		if (artist && album) {
			const items = albumReleasesFor(artist.name, album[1], String(album[4]).slice(0, 4));
			setTimeout(() => res.end(`<?xml version="1.0"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><title>lab</title>${items}</channel></rss>`), 1500);
			return;
		}
		if (t === 'movie' || t === 'search') {
			const m = tmdbMovies.find((x) => x.imdb_id === `tt${url.searchParams.get('imdbid')}`) ??
				tmdbMovies.find((x) => url.searchParams.get('q') && squash(url.searchParams.get('q') ?? '').startsWith(squash(x.title)));
			const items = m ? releasesFor(m) : '';
			setTimeout(() => res.end(`<?xml version="1.0"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><title>lab</title>${items}</channel></rss>`), m ? searchDelayMs : 0);
			return;
		}
		res.end('<?xml version="1.0"?><caps><limits max="50"/><searching><search available="yes" supportedParams="q"/>' +
			'<movie-search available="yes" supportedParams="q,imdbid"/><tv-search available="yes" supportedParams="q,season,ep,tvdbid"/>' +
			'<music-search available="yes" supportedParams="q,artist,album"/></searching>' +
			'<categories><category id="2000"/><category id="3000"/><category id="5000"/></categories></caps>');
	});
	indexer.listen(indexerPort, '127.0.0.1');
	return indexer;
}
