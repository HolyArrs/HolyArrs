// Fake Plex and Jellyfin for the browser tests, on 127.0.0.1 only. HolyArrs
// is pointed at the fake Plex with HOLYARRS_PLEX_URL, so the tests never
// reach plex.tv. Test-only names and passwords.
import { createServer } from 'node:http';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

export const plexPort = Number(process.env.HOLYARRS_E2E_PLEX_PORT ?? 18742);
export const jellyfinPort = Number(process.env.HOLYARRS_E2E_JELLYFIN_PORT ?? 18743);
export const tmdbPort = Number(process.env.HOLYARRS_E2E_TMDB_PORT ?? 18744);
export const traktPort = Number(process.env.HOLYARRS_E2E_TRAKT_PORT ?? 18745);
/** A fake TVMaze (the second show details source). */
export const tvmazePort = Number(process.env.HOLYARRS_E2E_TVMAZE_PORT ?? 18740);
/** A fake MusicBrainz (artists, albums, Pressings and tracks). */
export const mbPort = Number(process.env.HOLYARRS_E2E_MB_PORT ?? 18739);
/** A fake Deezer artist search (artist pictures), served by the fake MusicBrainz under /deezer. */
export const deezerURL = `http://127.0.0.1:${mbPort}/deezer`;
// A tiny made-up PNG (2x2, one colour) served as the artist picture.
const artistPicture = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAFklEQVR4nGNk+M/AwMDAxMDAwMDAAAAZ5wEEtUNqgwAAAABJRU5ErkJggg==', 'base64');
/** A fake Discord: it keeps what is posted to it; GET /posts reads them back. */
export const discordPort = Number(process.env.HOLYARRS_E2E_DISCORD_PORT ?? 18746);
/** A fake Torznab Source and a plain rss feed (test-only key). */
export const indexerPort = Number(process.env.HOLYARRS_E2E_INDEXER_PORT ?? 18747);
export const indexerKey = ['stand', 'in', 'source'].join('-');
const allowKeylessIndexer = process.env.HOLYARRS_E2E_INDEXER_NO_KEY === '1';
/** The only Trakt key the fake Trakt takes (test-only). */
export const traktKey = ['stand', 'in', 'trakt'].join('-');
/** A fake qBittorrent and a fake SABnzbd (test-only key) for the Downloader tests. */
export const qbitPort = Number(process.env.HOLYARRS_E2E_QBIT_PORT ?? 18748);
export const sabPort = Number(process.env.HOLYARRS_E2E_SAB_PORT ?? 18749);
export const sabKey = ['stand', 'in', 'sab', 'key'].join('-');
// The lab's fake Subtitle site (cmd/holyarrs-fakesubs), started by server.mjs.
export const subsPort = Number(process.env.HOLYARRS_E2E_SUBS_PORT ?? 18738);
export const mediaUser = { name: 'mum', password: ['e2e', 'media', 'pw'].join('-') };
/** The fake Emby lives under /emby on the fake Jellyfin's port, with its own person. */
export const embyUser = { name: 'gran', password: ['e2e', 'emby', 'pw'].join('-') };
/** The only API key the fake Jellyfin takes for the media server link (test-only). */
export const jellyKey = ['stand', 'in', 'jf'].join('-');
/** A fake old Radarr and a fake old Seerr for the move tests (test-only key). */
export const oldRadarrPort = Number(process.env.HOLYARRS_E2E_OLD_RADARR_PORT ?? 18736);
export const oldSeerrPort = Number(process.env.HOLYARRS_E2E_OLD_SEERR_PORT ?? 18737);
export const oldKey = ['stand', 'in', 'old', 'app'].join('-');

// Made-up artists for the fake MusicBrainz. Albums: [id, name, main kind,
// extra kinds, first date, Pressings]; a Pressing is [id, name note,
// country, date, format, track count]. 2099 is not out yet.
const mbid = (/** @type {string} */ head, /** @type {number} */ n) => `${head}-0000-4000-8000-${String(n).padStart(12, '0')}`;
/** @type {{ id: string, name: string, type: string, country: string, ended: boolean, disambiguation: string, albums: any[][] }[]} */
export const fakeArtists = [
	{
		id: mbid('a1111111', 1), name: 'Tin Lanterns', type: 'Group', country: 'GB', ended: false, disambiguation: 'lab folk band',
		albums: [
			[mbid('b1111111', 1), 'Harbour Songs', 'Album', [], '2019-04-12', [[mbid('c1111111', 1), '', 'XW', '2019-04-12', 'Digital Media', 3], [mbid('c1111111', 2), 'deluxe', 'GB', '2019-05-01', 'CD', 4], [mbid('c1111111', 3), '', 'US', '2019-06-01', 'CD', 3]]],
			[mbid('b1111111', 2), 'Night Ferry', 'Album', [], '2022-09-30', [[mbid('c1111111', 4), '', 'XW', '2022-09-30', 'Digital Media', 2]]],
			[mbid('b1111111', 3), 'Live at the Pier', 'Album', ['Live'], '2021-07-01', [[mbid('c1111111', 5), '', 'GB', '2021-07-01', 'Digital Media', 2]]],
			[mbid('b1111111', 4), 'Paper Boats', 'EP', [], '2099-05-01', []]
		]
	},
	{
		id: mbid('a2222222', 2), name: 'Moss Choir', type: 'Choir', country: 'IE', ended: true, disambiguation: '',
		albums: [
			[mbid('b2222222', 1), 'Slow Green', 'Album', [], '2020-01-01', [[mbid('c2222222', 1), '', 'IE', '2020-01-01', 'CD', 2]]],
			[mbid('b2222222', 2), 'Fern Hymns', 'Single', [], '2021-03-03', [[mbid('c2222222', 2), '', 'IE', '2021-03-03', 'Digital Media', 1]]]
		]
	},
	// On disk before HolyArrs knows it: the music import test adopts it.
	{
		id: mbid('a3333333', 3), name: 'Copper Wren', type: 'Person', country: 'NZ', ended: false, disambiguation: '',
		albums: [
			[mbid('b3333333', 1), 'Low Orbit', 'Album', [], '2018-08-08', [[mbid('c3333333', 1), '', 'NZ', '2018-08-08', 'Digital Media', 2]]],
			[mbid('b3333333', 2), 'Wire', 'Single', [], '2019-02-02', [[mbid('c3333333', 2), '', 'NZ', '2019-02-02', 'Digital Media', 1]]]
		]
	}
];

/** @param {any} a */
function mbArtist(a, score = 100) {
	return { id: a.id, name: a.name, 'sort-name': a.name, type: a.type, country: a.country, disambiguation: a.disambiguation, score, 'life-span': { ended: a.ended } };
}

/** A release's tracks get ids of their own; recordings are shared by every Pressing of an album. @param {any} p @param {number} albumN @param {string} albumId @param {string} albumName */
function mbRelease(p, albumN, albumId, albumName) {
	const [id, note, country, date, format, count] = p;
	const n = Number(id.slice(-12));
	const tracks = Array.from({ length: count }, (_, i) => ({
		id: mbid(`d${id.slice(1, 8)}`, n * 100 + i + 1),
		position: i + 1,
		title: `Song ${albumN}.${i + 1}`,
		length: 180000 + i * 15000,
		recording: { id: mbid(`e${albumId.slice(1, 8)}`, albumN * 100 + i + 1), length: 180000 + i * 15000 }
	}));
	return { id, title: albumName, status: 'Official', country, date, disambiguation: note, media: [{ position: 1, format, tracks }] };
}

/** @param {import('node:http').IncomingMessage} req @returns {Promise<string>} */
function body(req) {
	return new Promise((resolve) => {
		let b = '';
		req.on('data', (/** @type {Buffer} */ c) => (b += c));
		req.on('end', () => resolve(b));
	});
}

/** @param {import('node:http').ServerResponse} res @param {number} code @param {unknown} obj */
function send(res, code, obj, type = 'application/json') {
	res.writeHead(code, { 'Content-Type': type });
	res.end(typeof obj === 'string' ? obj : JSON.stringify(obj));
}

// A few made-up movies for the fake TMDB. Dates use TMDB release types:
// 3 cinema, 4 digital, 5 disc.
export const tmdbMovies = [
	{ id: 900017, title: 'Driftwood Moon', release_date: '2024-01-01', imdb_id: 'tt9000170', overview: 'A moon carved from the wood that washes ashore.', dates: [[3, '2024-01-01'], [4, '2024-03-01']] },
	{ id: 900001, title: 'The Lantern Keepers', release_date: '2031-02-01', imdb_id: 'tt9000010', overview: 'Keepers of a lighthouse that is not on any map.', dates: [[3, '2031-02-01']] },
	{ id: 900002, title: 'Paper Harbour', release_date: '2023-05-05', imdb_id: 'tt9000020', overview: 'A harbour town folds itself away each winter.', cert: 'PG', dates: [[3, '2023-05-05'], [4, '2023-07-01']] },
	{ id: 900003, title: 'Quiet Orchard', release_date: '2022-09-09', imdb_id: 'tt9000030', overview: 'An orchard that only grows at night.', dates: [[3, '2022-09-09'], [5, '2022-12-01']] },
	{ id: 900005, title: 'Salt Choir', release_date: '2020-01-01', imdb_id: 'tt9000050', overview: 'A choir that sings the tide in.', dates: [[3, '2020-01-01'], [4, '2020-03-01']] },
	{ id: 900006, title: 'Ember Road', release_date: '2019-06-06', imdb_id: 'tt9000060', overview: 'A road that is only there while it burns.', cert: 'R', dates: [[3, '2019-06-06'], [4, '2019-08-01']] },
	{ id: 900007, title: 'Northern Kettle', release_date: '2018-02-02', imdb_id: 'tt9000070', overview: 'A kettle that boils only for strangers.', dates: [[3, '2018-02-02'], [4, '2018-04-01']] },
	// Movies Requests tests ask for.
	{ id: 900008, title: 'Copper Lighthouse', release_date: '2017-04-04', imdb_id: 'tt9000080', overview: 'A lighthouse that guides ships into the past.', dates: [[3, '2017-04-04'], [4, '2017-06-01']] },
	{ id: 900009, title: 'Velvet Tide', release_date: '2016-08-08', imdb_id: 'tt9000090', overview: 'The sea comes in soft one night a year.', dates: [[3, '2016-08-08'], [4, '2016-10-01']] },
	// The Downloader tests add this one and mark its download failed.
	{ id: 900011, title: 'Iron Lullaby', release_date: '2015-03-03', imdb_id: 'tt9000110', overview: 'A lullaby forged for a sleeping city.', dates: [[3, '2015-03-03'], [4, '2015-05-01']] },
	// The movies in the old library the import tests adopt (makeOldLibrary).
	{ id: 900012, title: 'Marble Comet', release_date: '2014-02-02', imdb_id: 'tt9000120', overview: 'A comet carved from a quarry wall.', dates: [[3, '2014-02-02'], [4, '2014-04-01']] },
	{ id: 900013, title: 'Willow Signal', release_date: '2013-03-03', imdb_id: 'tt9000130', overview: 'A willow tree that blinks in code.', dates: [[3, '2013-03-03'], [4, '2013-05-01']] },
	{ id: 900014, title: 'Cinder Waltz', release_date: '2012-04-04', imdb_id: 'tt9000140', overview: 'A dance that leaves only ash behind.', dates: [[3, '2012-04-04'], [4, '2012-06-01']] },
	// The subtitle tests adopt this one from disk (subtitles.spec.ts).
	{ id: 900015, title: 'Quill Atlas', release_date: '2011-05-05', imdb_id: 'tt9000150', overview: 'An atlas that redraws itself with a quill.', dates: [[3, '2011-05-05'], [4, '2011-07-01']] },
	// The old Radarr's one movie the move test adopts in place (move.spec.ts).
	{ id: 900016, title: 'Lodestone Ferry', release_date: '2010-06-06', imdb_id: 'tt9000160', overview: 'A ferry that only sails toward north.', dates: [[3, '2010-06-06'], [4, '2010-08-01']] },
	{ id: 900004, title: 'Glass Meridian', release_date: '2021-03-03', imdb_id: 'tt9000040', overview: 'A line of glass towers marks noon across a desert.', dates: [[3, '2021-03-03'], [4, '2021-05-01']] },
	// One movie series (library-tools.spec.ts removes it): the only movies with belongs_to_collection.
	{ id: 900021, title: 'Brumwick Gate', release_date: '2008-09-09', imdb_id: 'tt9000210', overview: 'A town gate that opens onto a different town each night.', dates: [[3, '2008-09-09'], [4, '2008-12-01']], collection: { id: 950001, name: 'The Brumwick Films' } },
	{ id: 900022, title: 'Brumwick Gate Again', release_date: '2012-10-10', imdb_id: 'tt9000220', overview: 'The gate opens once more, onto the town it left.', dates: [[3, '2012-10-10'], [4, '2013-01-01']], collection: { id: 950001, name: 'The Brumwick Films' } }
];

// A few made-up shows, known to the fake TMDB and the fake TVMaze by the
// same ids. Episodes: [season, number, name, air day]; 2099 is not out yet.
export const fakeShows = [
	{
		tmdb: 800001, tvdb: 810001, tvmaze: 820001, imdb: 'tt8000010', name: 'Harbour Lights', first: '2024-01-07', status: 'Returning Series', network: 'Lab TV',
		overview: 'A harbour town where the lights tell the tides what to do.',
		episodes: [[0, 1, 'Lights Out Special', '2024-03-01'], [1, 1, 'First Light', '2024-01-07'], [1, 2, 'Low Tide', '2024-01-14'], [2, 1, 'Return of the Fog', '2025-02-02'], [2, 2, 'Far Beacon', '2099-01-04']]
	},
	{
		tmdb: 800002, tvdb: 810002, tvmaze: 820002, imdb: 'tt8000020', name: 'Lantern Street', first: '2023-03-05', status: 'Returning Series', network: 'Lab TV',
		overview: 'Every house on the street keeps one lantern lit for someone.',
		episodes: [[1, 1, 'The First Lantern', '2023-03-05'], [1, 2, 'Wick', '2023-03-12'], [2, 1, 'Oil and Glass', '2024-04-07'], [2, 2, 'Draught', '2024-04-14'], [3, 1, 'Dawn Shift', '2099-02-01']]
	},
	{
		tmdb: 800003, tvdb: 810003, tvmaze: 820003, imdb: 'tt8000030', name: 'Moss Parade', first: '2022-05-01', status: 'Ended', network: 'Lab Two',
		overview: 'A slow parade of moss across a sleeping city.',
		episodes: [[1, 1, 'Green Start', '2022-05-01'], [1, 2, 'Soft March', '2022-05-08']]
	},
	{
		// Kept on disk before HolyArrs (Import a library, 5.2); its second
		// season airs this week, so the Calendar shows it.
		tmdb: 800004, tvdb: 810004, tvmaze: 820004, imdb: 'tt8000040', name: 'Quarry Bells', first: '2021-09-05', status: 'Returning Series', network: 'Lab Three',
		overview: 'Bells ring in an old quarry whenever someone lies.',
		episodes: [[1, 1, 'Echo', '2021-09-05'], [1, 2, 'Dust', '2021-09-12'], [1, 3, 'Granite', '2021-09-19'], [2, 1, 'New Bell', soon(1)], [2, 2, 'Second Ring', soon(8)]]
	},
	{
		// The subtitle tests adopt it from disk (subtitles.spec.ts).
		tmdb: 800005, tvdb: 810005, tvmaze: 820005, imdb: 'tt8000050', name: 'Fennel Clock', first: '2020-02-02', status: 'Ended', network: 'Lab Four',
		overview: 'A clock tower that keeps time by the smell of fennel.',
		episodes: [[1, 1, 'Tick', '2020-02-02'], [1, 2, 'Tock', '2020-02-09']]
	},
	{
		// Kept in HD and 4K (showversions.spec.ts): its searches also answer a 2160p copy.
		tmdb: 800006, tvdb: 810006, tvmaze: 820006, imdb: 'tt8000060', name: 'Tidal Atlas', first: '2019-06-02', status: 'Ended', network: 'Lab Five',
		overview: 'A map of the sea that redraws itself with every tide.',
		episodes: [[1, 1, 'Ebb', '2019-06-02'], [1, 2, 'Flow', '2019-06-09']],
		uhd: true
	}
];

/** soon is the day n days from today (UTC), so a fake episode airs this week.
 * @param {number} n */
function soon(n) {
	return new Date(Date.now() + n * 86400000).toISOString().slice(0, 10);
}

/** @param {typeof fakeShows[number]} s */
function seasonsOf(s) {
	return [...new Set(s.episodes.map((e) => Number(e[0])))].sort((a, b) => a - b);
}

/** @param {typeof fakeShows[number]} s */
function tmdbShow(s) {
	return {
		id: s.tmdb, name: s.name, first_air_date: s.first, overview: s.overview, original_language: 'en', status: s.status, poster_path: `/s${s.tmdb}.jpg`,
		episode_run_time: [42], networks: [{ name: s.network }], external_ids: { imdb_id: s.imdb, tvdb_id: s.tvdb },
		seasons: seasonsOf(s).map((n) => ({ season_number: n, name: n === 0 ? 'Specials' : `Season ${n}` }))
	};
}

/** @param {typeof fakeShows[number]} s */
function mazeShow(s) {
	return {
		id: s.tvmaze, name: s.name, premiered: s.first, summary: `<p>${s.overview}</p>`, language: 'English', status: s.status === 'Ended' ? 'Ended' : 'Running',
		averageRuntime: 42, schedule: { time: '21:00' }, network: { name: s.network, country: { timezone: 'America/New_York' } },
		externals: { thetvdb: s.tvdb, imdb: s.imdb }, image: null
	};
}

/** @param {typeof tmdbMovies[number]} m */
function tmdbMovie(m) {
	return {
		id: m.id, title: m.title, release_date: m.release_date, overview: m.overview, original_language: 'en', runtime: 101,
		poster_path: `/p${m.id}.jpg`, imdb_id: m.imdb_id,
		...('collection' in m && m.collection ? { belongs_to_collection: { id: m.collection.id, name: m.collection.name } } : {}),
		release_dates: { results: [{ iso_3166_1: 'US', release_dates: m.dates.map(([type, d]) => ({ type, release_date: `${d}T00:00:00.000Z`, certification: m.cert ?? '' })) }] }
	};
}

const squash = (/** @type {string} */ s) => s.toLowerCase().replace(/[^a-z0-9]+/g, '');

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

/**
 * makeOldLibrary writes a small movie library as someone might already
 * have it, for the import tests: a folder named "Name (Year)" (matched), one
 * with a tmdb tag (matched), one with half a name (unsure), home videos
 * (unmatched) and a folder with no video. The videos are tiny stand-ins.
 * @param {string} dir
 */
export function makeOldLibrary(dir) {
	/** @type {[string, string][]} */
	const files = [
		['Marble Comet (2014)', 'Marble.Comet.2014.1080p.WEB-DL.H264-HOLYARRS.mkv'],
		['Willow Signal {tmdb-900013}', 'willow signal.mkv'],
		['Cinder (2012)', 'Cinder.2012.720p.BluRay.x264-OLD.mkv'],
		['Home Videos', 'beach day.mp4'],
		['Notes', 'readme.txt']
	];
	for (const [folder, name] of files) {
		mkdirSync(join(dir, folder), { recursive: true });
		writeFileSync(join(dir, folder, name), 'stand-in video '.repeat(64));
	}
}

// The fake metadata server's one installation credential.
export const fakeInstallToken = ['hai', 'stand', 'in', 'install'].join('_');

/** metadataEnv points a HolyArrs at the fake metadata server (no TMDB key). */
export const metadataEnv = {
	HOLYARRS_METADATA_URL: `http://127.0.0.1:${tmdbPort}`,
	HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL: 'true',
	HOLYARRS_TMDB_URL: `http://127.0.0.1:${tmdbPort}`,
	// Telegram Alerts post to the fake Discord server, which also answers sendMessage.
	HOLYARRS_TELEGRAM_URL: `http://127.0.0.1:${discordPort}`
};

/** Counts of what the fake metadata server answered, for the tests. */
export const metadataCalls = { registered: 0, tmdb: 0 };

export function startFakes() {
	// The fake metadata server: registration, /readyz and TMDB reads at
	// /v1/tmdb (answered by the fake TMDB below), like the real one.
	const tmdb = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		// A made-up HolyArrs release feed for the update check (no sign-in).
		if (url.pathname === '/releases/latest') return send(res, 200, { tag_name: 'v9.9.9', html_url: 'https://github.com/CoderLuii/holyarrs/releases/tag/v9.9.9' });
		if (url.pathname === '/readyz' || url.pathname === '/healthz') return send(res, 200, { ok: true });
		if (url.pathname === '/v1/installations') {
			if (req.method !== 'POST') return send(res, 405, { error: { code: 'method_not_allowed', retry_after_seconds: 0 } });
			metadataCalls.registered++;
			return send(res, 201, { schema_version: 1, token: fakeInstallToken });
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
	// Fake MusicBrainz: artist search, an artist, its release groups and
	// each release group's releases with media and tracks.
	const mb = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		// Fake Deezer: the first made-up artist has a picture, the others
		// have none (an empty picture id, as Deezer answers).
		if (url.pathname === '/deezer/search/artist') {
			const q = squash(url.searchParams.get('q') ?? '');
			const data = fakeArtists
				.map((a, i) => ({ a, i }))
				.filter(({ a }) => q && squash(a.name).includes(q))
				.map(({ a, i }) => {
					const pic = i === 0 ? `${deezerURL}/images/artist/${i + 1}/1000x1000.png` : `${deezerURL}/images/artist//1000x1000.png`;
					return { id: i + 1, name: a.name, picture_xl: pic, picture_big: pic, picture_medium: pic };
				});
			return send(res, 200, { data, total: data.length });
		}
		if (/^\/deezer\/images\/artist\/\d+\/1000x1000\.png$/.test(url.pathname)) {
			res.writeHead(200, { 'content-type': 'image/png', 'content-length': artistPicture.length });
			return res.end(artistPicture);
		}
		if (url.pathname === '/ws/2/artist') {
			const q = squash(url.searchParams.get('query') ?? '');
			return send(res, 200, { artists: fakeArtists.filter((a) => q && squash(a.name).includes(q)).map((a) => mbArtist(a)) });
		}
		const one = url.pathname.match(/^\/ws\/2\/artist\/([0-9a-f-]+)$/);
		if (one) {
			const a = fakeArtists.find((x) => x.id === one[1]);
			return a ? send(res, 200, mbArtist(a)) : send(res, 404, { error: 'Not Found' });
		}
		if (url.pathname === '/ws/2/release-group') {
			const a = fakeArtists.find((x) => x.id === url.searchParams.get('artist'));
			const groups = (a?.albums ?? []).map((al) => ({ id: al[0], title: al[1], 'primary-type': al[2], 'secondary-types': al[3], 'first-release-date': al[4], disambiguation: '' }));
			return send(res, 200, { 'release-group-count': groups.length, 'release-groups': groups });
		}
		if (url.pathname === '/ws/2/release') {
			const rg = url.searchParams.get('release-group');
			for (const a of fakeArtists) {
				const i = a.albums.findIndex((al) => al[0] === rg);
				if (i >= 0) {
					const al = a.albums[i];
					const releases = /** @type {any[]} */ (al[5]).map((p) => mbRelease(p, i + 1, al[0], al[1]));
					return send(res, 200, { 'release-count': releases.length, releases });
				}
			}
			return send(res, 200, { 'release-count': 0, releases: [] });
		}
		send(res, 404, { error: 'Not Found' });
	});
	mb.listen(mbPort, '127.0.0.1');
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
	// Fake Trakt: lab/favourites holds Northern Kettle (by IMDb id only).
	const trakt = createServer((req, res) => {
		if (req.headers['trakt-api-key'] !== traktKey || req.headers['trakt-api-version'] !== '2') return send(res, 403, { error: 'key' });
		const url = new URL(req.url ?? '/', 'http://x');
		if (url.pathname !== '/users/lab/lists/favourites/items/movie') return send(res, 404, { error: 'no list' });
		res.setHeader('X-Pagination-Page-Count', '1');
		send(res, 200, [{ type: 'movie', movie: { title: 'Northern Kettle', year: 2018, ids: { imdb: 'tt9000070' } } }]);
	});
	trakt.listen(traktPort, '127.0.0.1');
	let nextPin = 100;
	const confirmed = new Set();
	const plex = createServer(async (req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (req.method === 'POST' && url.pathname === '/api/v2/pins') return send(res, 201, { id: ++nextPin, code: `CODE${nextPin}` });
		const pin = url.pathname.match(/^\/api\/v2\/pins\/(\d+)$/);
		if (req.method === 'GET' && pin) return send(res, 200, { id: Number(pin[1]), authToken: confirmed.has(pin[1]) ? `ptok${pin[1]}` : null });
		if (req.method === 'GET' && url.pathname === '/api/v2/user') return send(res, 200, { id: 9001, uuid: 'u', username: 'plexkid', email: 'kid@example.test' });
		if (req.method === 'GET' && url.pathname === '/api/v2/resources') return send(res, 200, [{ clientIdentifier: 'fake-server', provides: 'server' }]);
		// The page the person sees on "Plex": opening it confirms the pin.
		if (req.method === 'GET' && url.pathname === '/app/auth') {
			const code = new URLSearchParams(url.hash || url.search).get('code') ?? '';
			for (let i = 100; i <= nextPin; i++) confirmed.add(String(i));
			return send(res, 200, `<!doctype html><title>Fake Plex</title><p>Signed in ${code}</p>`, 'text/html');
		}
		send(res, 404, { error: 'no route' });
	});
	/** @type {unknown[]} */
	const updates = [];
	const jelly = createServer(async (req, res) => {
		// A single sign-on provider's discovery document, for the Settings Test button.
		if (req.method === 'GET' && req.url === '/oidc/.well-known/openid-configuration') {
			const base = `http://127.0.0.1:${jellyfinPort}/oidc`;
			return send(res, 200, { issuer: base, authorization_endpoint: `${base}/auth`, token_endpoint: `${base}/token`, jwks_uri: `${base}/keys`, id_token_signing_alg_values_supported: ['RS256'] });
		}
		// Emby, as it often sits at http://host/emby. It wants the X-Emby-Authorization header.
		if (req.url?.startsWith('/emby/')) {
			if (req.method === 'POST' && req.url === '/emby/Users/AuthenticateByName') {
				if (!String(req.headers['x-emby-authorization'] ?? '').startsWith('MediaBrowser ')) return send(res, 400, 'no client header', 'text/plain');
				const b = JSON.parse((await body(req)) || '{}');
				if (b.Username === embyUser.name && b.Pw === embyUser.password)
					return send(res, 200, { User: { Id: 'em-1', Name: 'Gran', Policy: { IsDisabled: false } }, AccessToken: 'etok' });
				return send(res, 401, 'Error processing request.', 'text/plain');
			}
			if (req.method === 'POST' && req.url === '/emby/Sessions/Logout') return send(res, 204, '');
			if (req.method === 'GET' && req.url === '/emby/System/Info/Public') return send(res, 200, { ServerName: 'Fake Emby', Version: '4.8.0', ProductName: 'Emby Server', Id: 'fake-emby' });
			return send(res, 404, { error: 'no route' });
		}
		if (req.method === 'POST' && req.url === '/Users/AuthenticateByName') {
			const b = JSON.parse((await body(req)) || '{}');
			if (b.Username === mediaUser.name && b.Pw === mediaUser.password)
				return send(res, 200, { User: { Id: 'jf-1', Name: 'Mum', Policy: { IsDisabled: false } }, AccessToken: 'jtok' });
			return send(res, 401, 'Error processing request.', 'text/plain');
		}
		if (req.method === 'POST' && req.url === '/Sessions/Logout') return send(res, 204, '');
		// The address test in Settings asks the open server info, as a real Jellyfin answers it.
		if (req.method === 'GET' && req.url === '/System/Info/Public') return send(res, 200, { ServerName: 'Fake Jellyfin', Version: '10.9.0', ProductName: 'Jellyfin Server', Id: 'fake-jellyfin' });
		// Current Jellyfin uses the MediaBrowser Authorization token for server calls.
		const serverCall = ['/System/Info', '/Library/Media/Updated', '/Library/Refresh'].includes(req.url ?? '');
		if (serverCall && (!req.headers.authorization?.startsWith('MediaBrowser ') || !req.headers.authorization.includes(`Token="${jellyKey}"`))) return send(res, 401, '');
		if (req.method === 'GET' && req.url === '/System/Info') return send(res, 200, { ServerName: 'Fake Jellyfin', Version: '10.9.0' });
		if (req.method === 'POST' && req.url === '/Library/Media/Updated') {
			updates.push(JSON.parse((await body(req)) || '{}'));
			return send(res, 204, '');
		}
		if (req.method === 'POST' && req.url === '/Library/Refresh') return send(res, 204, '');
		if (req.method === 'GET' && req.url === '/updates') return send(res, 200, updates);
		send(res, 404, { error: 'no route' });
	});
	plex.listen(plexPort, '127.0.0.1');
	jelly.listen(jellyfinPort, '127.0.0.1');
	/** @type {unknown[]} */
	const posts = [];
	/** @type {unknown[]} */
	const hooks = [];
	/** @type {unknown[]} */
	const telegram = [];
	const discord = createServer(async (req, res) => {
		// A fake Telegram Bot API: POST /bot<token>/sendMessage; GET /telegram reads the bodies back.
		if (req.method === 'POST' && /^\/bot[^/]+\/sendMessage$/.test(req.url ?? '')) {
			telegram.push(JSON.parse((await body(req)) || '{}'));
			return send(res, 200, { ok: true, result: {} });
		}
		if (req.method === 'GET' && req.url === '/telegram') return send(res, 200, telegram);
		if (req.method === 'POST' && req.url?.startsWith('/api/webhooks/')) {
			posts.push(JSON.parse((await body(req)) || '{}'));
			return send(res, 204, '');
		}
		if (req.method === 'GET' && req.url === '/posts') return send(res, 200, posts);
		// A webhook receiver: keeps the body and the HolyArrs headers.
		if (req.method === 'POST' && req.url === '/hook') {
			hooks.push({ body: JSON.parse((await body(req)) || '{}'), event: req.headers['x-holyarrs-event'], signature: req.headers['x-holyarrs-signature'] ?? '' });
			return send(res, 204, '');
		}
		if (req.method === 'GET' && req.url === '/hooks') return send(res, 200, hooks);
		send(res, 404, { error: 'no route' });
	});
	discord.listen(discordPort, '127.0.0.1');
	const indexer = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		res.setHeader('Content-Type', 'application/xml');
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
			const items = s ? showReleasesFor(s, season, ep ? Number(ep) : null) : '';
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
	// Fake qBittorrent: every torrent it is given downloads at 40% for
	// ever (GET /set?progress= moves them on); GET /state shows what was
	// added and deleted.
	/** @type {Map<string, Record<string, unknown>>} */
	const torrents = new Map();
	/** @type {string[]} */
	const deleted = [];
	/** @type {string[]} */
	const added = [];
	let mainRid = 0;
	const qbit = createServer(async (req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		const form = new URLSearchParams(req.method === 'POST' ? await body(req) : url.search);
		switch (url.pathname) {
			case '/state':
				return send(res, 200, { added, deleted });
			case '/set': {
				// Test control: move every torrent on (progress, speed, eta),
				// or only the one named by hash; state, save_path and files
				// (names joined by |) finish it with files to import.
				for (const t of torrents.values()) {
					if (form.get('hash') && form.get('hash') !== t.hash) continue;
					for (const k of ['progress', 'dlspeed', 'eta']) if (form.get(k)) t[k] = Number(form.get(k));
					for (const k of ['state', 'save_path']) if (form.get(k)) t[k] = form.get(k);
					if (form.get('files')) t.files = (form.get('files') ?? '').split('|').map((name) => ({ name, size: 1000, progress: 1 }));
				}
				return send(res, 200, { ok: true });
			}
			case '/api/v2/app/version':
				return send(res, 200, 'v5.2.4', 'text/plain');
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
	return () => {
		oldRadarr.close();
		oldSeerr.close();
		qbit.close();
		sab.close();
		indexer.close();
		discord.close();
		tmdb.close();
		mb.close();
		tvmaze.close();
		trakt.close();
		plex.close();
		jelly.close();
	};
}
