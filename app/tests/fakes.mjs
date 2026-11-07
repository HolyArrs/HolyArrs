// Fake Plex and Jellyfin for the browser tests, on 127.0.0.1 only. HolyArrs
// is pointed at the fake Plex with HOLYARRS_PLEX_URL, so the tests never
// reach plex.tv. Test-only names and passwords.
import { createServer } from 'node:http';
import { createHash, generateKeyPairSync, randomBytes, sign } from 'node:crypto';
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
/** The only token the fake Plex takes as a media server (test-only). */
export const plexServerToken = ['stand', 'in', 'plex', 'server'].join('-');

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
	},
	// Only the Requests tests use it: a member asks for one album and follows the one not out yet.
	{
		id: mbid('a4444444', 4), name: 'Wicker Owls', type: 'Group', country: 'CA', ended: false, disambiguation: '',
		albums: [
			[mbid('b4444444', 1), 'Low Hedge', 'Album', [], '2017-03-03', [[mbid('c4444444', 1), '', 'CA', '2017-03-03', 'Digital Media', 2]]],
			[mbid('b4444444', 2), 'Barn Light', 'Album', [], '2020-10-10', [[mbid('c4444444', 2), '', 'CA', '2020-10-10', 'CD', 2]]],
			[mbid('b4444444', 3), 'Night Fields', 'EP', [], '2099-06-01', []]
		]
	},
	// Only the Requests follow test uses it: in the Library, a member follows it (nothing is asked for).
	{
		id: mbid('a6666666', 6), name: 'Quartz Gulls', type: 'Group', country: 'NO', ended: false, disambiguation: '',
		albums: [[mbid('b6666666', 1), 'Tide Clock', 'Album', [], '2019-11-11', [[mbid('c6666666', 1), '', 'NO', '2019-11-11', 'Digital Media', 2]]]]
	},
	// Only the Library folder tests use it (library-folders.spec.ts): adopted from disk, then moved to a second music folder.
	{
		id: mbid('a5555555', 5), name: 'Slate Herons', type: 'Group', country: 'AU', ended: false, disambiguation: '',
		albums: [
			[mbid('b5555555', 1), 'Glass Estuary', 'Album', [], '2017-05-05', [[mbid('c5555555', 1), '', 'AU', '2017-05-05', 'Digital Media', 2]]],
			[mbid('b5555555', 2), 'Kelp Radio', 'Album', [], '2020-09-09', [[mbid('c5555555', 2), '', 'AU', '2020-09-09', 'Digital Media', 1]]]
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

// The fake single sign-on provider's signing key and its open codes.
const oidcKey = generateKeyPairSync('rsa', { modulusLength: 2048 });
/** @type {Map<string, { user: string, nonce: string | null, challenge: string | null }>} */
const oidcCodes = new Map();
const esc = (/** @type {string} */ s) => s.replace(/[&<>"]/g, (ch) => `&#${ch.charCodeAt(0)};`);

/** @param {import('node:http').ServerResponse} res @param {number} code @param {unknown} obj */
function send(res, code, obj, type = 'application/json') {
	res.writeHead(code, { 'Content-Type': type });
	res.end(typeof obj === 'string' ? obj : JSON.stringify(obj));
}

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
	// Only the Requests tests use these: 4K asks, a failed ask, a follow that lands, the owner's own ask.
	{ id: 900030, title: 'Saffron Tramline', release_date: '2019-09-19', imdb_id: 'tt9000300', overview: 'A tram that runs only in harvest week.', dates: [[3, '2019-09-19'], [4, '2019-11-01']] },
	{ id: 900031, title: 'Pewter Kite', release_date: '2018-03-03', imdb_id: 'tt9000310', overview: 'A kite too heavy to fly, until it does.', dates: [[3, '2018-03-03'], [4, '2018-05-01']] },
	{ id: 900032, title: 'Juniper Telegraph', release_date: '2017-10-10', imdb_id: 'tt9000320', overview: 'Messages tapped out through the roots of a hedge.', dates: [[3, '2017-10-10'], [4, '2017-12-01']] },
	{ id: 900033, title: 'Cobalt Orrery', release_date: '2016-05-05', imdb_id: 'tt9000330', overview: 'A model of the planets that keeps better time than the sky.', dates: [[3, '2016-05-05'], [4, '2016-07-01']] },
	{ id: 900034, title: 'Thistle Almanac', release_date: '2015-07-07', imdb_id: 'tt9000340', overview: 'A farm almanac that predicts the wrong year.', dates: [[3, '2015-07-07'], [4, '2015-09-01']] },
	{ id: 900035, title: 'Marigold Bellows', release_date: '2014-04-04', imdb_id: 'tt9000350', overview: 'An accordion repaired one fold at a time.', dates: [[3, '2014-04-04'], [4, '2014-06-01']] },
	// Followed while in the Library without a file (requests.spec): the follower hears once its file is in.
	{ id: 900037, title: 'Heron Clockwork', release_date: '2015-11-11', imdb_id: 'tt9000370', overview: 'A clock that keeps time by the herons on the river.', dates: [[3, '2015-11-11'], [4, '2016-01-01']] },
	// The ask whose download fails (requests.spec): its two good Releases are marked failed.
	{ id: 900036, title: 'Garnet Sextant', release_date: '2013-08-08', imdb_id: 'tt9000360', overview: 'A sextant that steers by stars no one else can see.', dates: [[3, '2013-08-08'], [4, '2013-10-01']] },
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
	{ id: 900022, title: 'Brumwick Gate Again', release_date: '2012-10-10', imdb_id: 'tt9000220', overview: 'The gate opens once more, onto the town it left.', dates: [[3, '2012-10-10'], [4, '2013-01-01']], collection: { id: 950001, name: 'The Brumwick Films' } },
	// Only library-editions.spec.ts uses this one: a Director's Cut named for Plex.
	{ id: 900081, title: 'Zephyr Projectionist', release_date: '2009-11-11', imdb_id: 'tt9000810', overview: 'A projectionist who only screens films that were never made.', dates: [[3, '2009-11-11'], [4, '2010-01-01']] },
	// Import files by hand picks another title (library-import.spec.ts): a file named Tamarind Quay that is really Ochre Bandstand.
	{ id: 900040, title: 'Tamarind Quay', release_date: '2013-06-06', imdb_id: 'tt9000400', overview: 'A quay where the spice ships never quite dock.', dates: [[3, '2013-06-06'], [4, '2013-08-01']] },
	{ id: 900041, title: 'Ochre Bandstand', release_date: '2012-08-08', imdb_id: 'tt9000410', overview: 'A park bandstand that plays the weather.', dates: [[3, '2012-08-08'], [4, '2012-10-01']] },
	// Only the Library select bar tests use these (library-bulk.spec.ts).
	{ id: 900451, title: 'Bramble Foundry', release_date: '2016-02-12', imdb_id: 'tt9004510', overview: 'A foundry that casts bells out of hedgerow thorns.', dates: [[3, '2016-02-12'], [4, '2016-04-01']] },
	{ id: 900452, title: 'Lichen Postcard', release_date: '2013-11-03', imdb_id: 'tt9004520', overview: 'A postcard that grows a new picture every spring.', dates: [[3, '2013-11-03'], [4, '2014-01-10']] },
	// Only library-missing.spec.ts adds these: Search all missing finds them.
	{ id: 900271, title: 'Hollow Compass', release_date: '2015-02-02', imdb_id: 'tt9002710', overview: 'A compass that points at whatever was lost last.', dates: [[3, '2015-02-02'], [4, '2015-04-01']] },
	{ id: 900272, title: 'Brass Meadow', release_date: '2014-06-06', imdb_id: 'tt9002720', overview: 'A meadow of brass flowers that ring in the wind.', dates: [[3, '2014-06-06'], [4, '2014-08-01']] },
	// Only plex-watchlist.spec.ts uses these: asked from the watchlist, already in the Library, added after Movies is off.
	{ id: 900102, title: 'Rowan Semaphore', release_date: '2012-04-04', imdb_id: 'tt9001020', overview: 'Flags on a hilltop that answer before anyone asks.', dates: [[3, '2012-04-04'], [4, '2012-06-01']] },
	{ id: 900103, title: 'Flint Gazebo', release_date: '2011-07-07', imdb_id: 'tt9001030', overview: 'A garden shelter that sparks in every storm.', dates: [[3, '2011-07-07'], [4, '2011-09-01']] },
	{ id: 900104, title: 'Nettle Observatory', release_date: '2010-10-10', imdb_id: 'tt9001040', overview: 'A telescope that only sees the weeds below it.', dates: [[3, '2010-10-10'], [4, '2010-12-01']] },
	// On a member's Plex watchlist only: the first is asked, the second held back by their weekly limit of one.
	{ id: 900105, title: 'Juniper Lantern', release_date: '2013-05-05', imdb_id: 'tt9001050', overview: 'A lantern that lights only the paths nobody took.', dates: [[3, '2013-05-05'], [4, '2013-07-01']] },
	{ id: 900106, title: 'Quill Aqueduct', release_date: '2014-08-08', imdb_id: 'tt9001060', overview: 'Water that writes letters as it runs downhill.', dates: [[3, '2014-08-08'], [4, '2014-10-01']] },
	// Only trailers.spec.ts uses these, the only movies with trailers. videos: [YouTube key, name, official];
	// trailer is the metadata server's pick. How the fake YouTube answers each key is in fakeYouTube.
	{ id: 900501, title: 'Lumen Cistern', release_date: '2016-03-03', imdb_id: 'tt9005010', overview: 'A cistern that fills with light instead of rain.', dates: [[3, '2016-03-03'], [4, '2016-05-01']], trailer: 'lumenCist01', videos: [['lumenCist01', 'Lumen Cistern official trailer', true]] },
	{ id: 900502, title: 'Aster Funicular', release_date: '2017-05-05', imdb_id: 'tt9005020', overview: 'A hillside railway that only climbs for passengers in love.', dates: [[3, '2017-05-05'], [4, '2017-07-01']], trailer: 'asterAge001', videos: [['asterAge001', 'Aster Funicular red band trailer', true], ['asterPlay02', 'Aster Funicular trailer 2', false]] },
	{ id: 900503, title: 'Gossamer Weir', release_date: '2018-07-07', imdb_id: 'tt9005030', overview: 'A river weir spun overnight by spiders.', dates: [[3, '2018-07-07'], [4, '2018-09-01']], trailer: 'weirGone001', videos: [['weirGone001', 'Gossamer Weir official trailer', true], ['weirGone002', 'Gossamer Weir trailer 2', false]] },
	{ id: 900504, title: 'Pumice Belfry', release_date: '2019-09-09', imdb_id: 'tt9005040', overview: 'A bell tower so light it floats off in a storm.', dates: [[3, '2019-09-09'], [4, '2019-11-01']], trailer: 'pumiceBel01', videos: [['pumiceBel01', 'Pumice Belfry official trailer', true]] },
	// Only requests.spec.ts (#136): asked again and declined by three admins in turn.
	{ id: 900601, title: 'Harrow Teaspoon', release_date: '2015-04-04', imdb_id: 'tt9006010', overview: 'A teaspoon that stirs the whole village awake.', dates: [[3, '2015-04-04'], [4, '2015-06-01']] },
	// Only requests.spec.ts (#139): a member's open Requests page follows its decision live.
	{ id: 900602, title: 'Quince Lamplight', release_date: '2016-05-05', imdb_id: 'tt9006020', overview: 'A lamplighter who only lights the lamps no one walks under.', dates: [[3, '2016-05-05'], [4, '2016-07-01']] },
	// Only requests.spec.ts (#139): approved, nothing found for it; its steps show live.
	{ id: 900603, title: 'Rook Lantern', release_date: '2017-02-02', imdb_id: 'tt9006030', overview: 'A lantern a rook carries across the moor each night.', dates: [[3, '2017-02-02'], [4, '2017-04-01']] },
	// Only requests.spec.ts (#139): approved, found and downloaded; the member sees each step.
	{ id: 900604, title: 'Cobalt Weathervane', release_date: '2018-03-03', imdb_id: 'tt9006040', overview: 'A weathervane that points to whoever is lost.', dates: [[3, '2018-03-03'], [4, '2018-05-01']] }
];

/**
 * How the fake YouTube answers each video (trailers.spec.ts): ok gives a
 * muxed MP4 on a made-up googlevideo address, hls an HLS manifest (a music
 * video), age is behind YouTube's age sign-in, gone was removed. Any other
 * key answers as removed too.
 * @type {Record<string, 'ok' | 'hls' | 'age' | 'gone'>}
 */
export const fakeYouTube = { lumenCist01: 'ok', asterAge001: 'age', asterPlay02: 'ok', weirGone001: 'gone', weirGone002: 'gone', pumiceBel01: 'ok', tinLantVid1: 'hls' };

/** An artist only trailers.spec.ts opens (no other spec or pass reaches it first), with one music video. */
export const videoArtist = 'a9999999-0000-4000-8000-000000000191';

/** The music videos the fake metadata server lists, by artist MBID. @returns {Record<string, { youtube_id: string; title: string; duration: number }[]>} */
export const fakeArtistVideos = () => ({ [videoArtist]: [{ youtube_id: 'tinLantVid1', title: 'Lab Artist - Harbour Song (Official Video)', duration: 200 }] });

/** The address the fake YouTube gives for an ok video (expire is a Unix time). */
export const fakeVideoAddress = (/** @type {string} */ key, /** @type {number} */ expire) => `https://rr1---sn-lab.googlevideo.com/videoplayback?expire=${expire}&id=${key}&itag=18`;

/** Where the fake YouTube answers, on the fake metadata server's port. */
export const youtubeURL = () => `http://127.0.0.1:${tmdbPort}/youtube`;

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
	},
	{
		// Only the Requests tests use it: two members ask for different seasons and share one ask.
		tmdb: 800007, tvdb: 810007, tvmaze: 820007, imdb: 'tt8000070', name: 'Kestrel Shoals', first: '2021-04-04', status: 'Returning Series', network: 'Lab Six',
		overview: 'Seabirds keep the charts of a sandbank that moves.',
		episodes: [[1, 1, 'Low Water', '2021-04-04'], [1, 2, 'Spit', '2021-04-11'], [2, 1, 'Bar', '2022-04-03'], [2, 2, 'Gull Light', '2022-04-10'], [3, 1, 'Spring Tide', '2023-04-02']]
	},
	{
		// Only the Library folder tests use it (library-folders.spec.ts): adopted from disk, then moved to a second show folder.
		tmdb: 800008, tvdb: 810008, tvmaze: 820008, imdb: 'tt8000080', name: 'Saltmarsh Signal', first: '2020-03-01', status: 'Ended', network: 'Lab Seven',
		overview: 'A radio mast in the marsh picks up the weather a day early.',
		episodes: [[1, 1, 'Static', '2020-03-01'], [1, 2, 'Carrier', '2020-03-08'], [2, 1, 'Relay', '2021-03-07']]
	},
	{
		// Only Import files by hand uses it (library-import.spec.ts): loose episode files brought in by hand.
		tmdb: 800041, tvdb: 810041, tvmaze: 820041, imdb: 'tt8000410', name: 'Gristmill Hollow', first: '2018-03-04', status: 'Ended', network: 'Lab Seven',
		overview: 'A mill village where the wheel turns only when nobody watches.',
		episodes: [[1, 1, 'Millrace', '2018-03-04'], [1, 2, 'Grain', '2018-03-11'], [2, 1, 'Flood Gate', '2019-03-03']]
	},
	{
		// Only the Library select bar tests use it (library-bulk.spec.ts): one aired season.
		tmdb: 800451, tvdb: 810451, tvmaze: 820451, imdb: 'tt8004510', name: 'Sorrel Junction', first: '2018-03-04', status: 'Ended', network: 'Lab Seven',
		overview: 'A railway junction where the points change with the weather.',
		episodes: [[1, 1, 'Points', '2018-03-04'], [1, 2, 'Signal Box', '2018-03-11']]
	},
	{
		// Only library-missing.spec.ts adds it: Search all missing searches
		// season 1 as a season and S02E01 alone; S02E02 is not out yet.
		tmdb: 800271, tvdb: 810271, tvmaze: 820271, imdb: 'tt8002710', name: 'Gilded Bracken', first: '2022-02-06', status: 'Returning Series', network: 'Lab Seven',
		overview: 'Ferns in a hillside wood turn to gold one leaf at a time.',
		episodes: [[1, 1, 'Fern Gate', '2022-02-06'], [1, 2, 'Root Hall', '2022-02-13'], [2, 1, 'Gold Leaf', '2023-03-05'], [2, 2, 'Late Frost', '2099-03-01']]
	},
	{
		// Anime (library-anime.spec.ts): the Source names its Releases by running
		// number ("[Lab Fansub] Hoshikage Relay - 13 [1080p]"), so 13 is S02E01.
		tmdb: 800032, tvdb: 810032, tvmaze: 820032, imdb: 'tt8000320', name: 'Hoshikage Relay', first: '2023-01-08', status: 'Returning Series', network: 'Lab Seven',
		overview: 'Twelve runners carry a lantern across the night in a relay that never ends.',
		episodes: [
			...Array.from({ length: 12 }, (_, i) => [1, i + 1, `Leg ${i + 1}`, new Date(Date.UTC(2023, 0, 8 + 7 * i)).toISOString().slice(0, 10)]),
			[2, 1, 'Second Wind', '2024-04-07'], [2, 2, 'Anchor Leg', '2024-04-14']
		],
		anime: true
	},
	{
		// Only plex-watchlist.spec.ts uses it: asked whole from the Plex watchlist.
		tmdb: 800102, tvdb: 810102, tvmaze: 820102, imdb: 'tt8001020', name: 'Larch Ferrymen', first: '2019-05-05', status: 'Ended', network: 'Lab Seven',
		overview: 'Two ferrymen row a larch-wood boat across a lake that keeps getting wider.',
		episodes: [[1, 1, 'Oars', '2019-05-05'], [1, 2, 'Far Shore', '2019-05-12']]
	},
	// Only the show list tests use these (library-showlists.spec.ts): TMDB list
	// 104 adds Bellwether Ferry, skips Cinder Lane (on the Never-add list); the
	// Trakt list lab/tv-picks adds Rookery Dial by its TVDB id alone.
	{
		tmdb: 800104, tvdb: 810104, tvmaze: 820104, imdb: 'tt8001040', name: 'Bellwether Ferry', first: '2021-05-02', status: 'Returning Series', network: 'Lab Eight',
		overview: 'A ferry that crosses only when the bell-sheep agree.',
		episodes: [[1, 1, 'Fleece', '2021-05-02'], [1, 2, 'Crossing', '2021-05-09'], [2, 1, 'Second Flock', '2022-05-01'], [3, 1, 'Far Pasture', '2099-05-01']]
	},
	{
		tmdb: 800105, tvdb: 810105, tvmaze: 820105, imdb: 'tt8001050', name: 'Cinder Lane', first: '2020-10-04', status: 'Ended', network: 'Lab Eight',
		overview: 'A street where every chimney tells a different story.',
		episodes: [[1, 1, 'Soot', '2020-10-04'], [1, 2, 'Ember', '2020-10-11']]
	},
	{
		tmdb: 800106, tvdb: 810106, tvmaze: 820106, imdb: 'tt8001060', name: 'Rookery Dial', first: '2019-09-01', status: 'Ended', network: 'Lab Eight',
		overview: 'Crows keep the time on a sundial nobody else can read.',
		episodes: [[1, 1, 'Noon', '2019-09-01'], [1, 2, 'Shadow', '2019-09-08'], [2, 1, 'Dusk', '2020-09-06']]
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
		...('videos' in m && m.videos ? { videos: { results: m.videos.map(([key, name, official]) => ({ key, name, official, site: 'YouTube', type: 'Trailer', iso_639_1: 'en' })) } } : {}),
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
	HOLYARRS_TELEGRAM_URL: `http://127.0.0.1:${discordPort}`,
	// So do Pushover Alerts (POST /1/messages.json).
	HOLYARRS_PUSHOVER_URL: `http://127.0.0.1:${discordPort}`
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
	/** @type {unknown[]} */
	const updates = [];
	const jelly = createServer(async (req, res) => {
		// A single sign-on provider: discovery for the Settings Test button, and a
		// sign-in page where the test types a name, then code, token and keys.
		if (req.url?.startsWith('/oidc/')) {
			const base = `http://127.0.0.1:${jellyfinPort}/oidc`;
			const url = new URL(req.url, base);
			if (req.method === 'GET' && url.pathname === '/oidc/.well-known/openid-configuration')
				return send(res, 200, { issuer: base, authorization_endpoint: `${base}/auth`, token_endpoint: `${base}/token`, jwks_uri: `${base}/keys`, id_token_signing_alg_values_supported: ['RS256'] });
			if (req.method === 'GET' && url.pathname === '/oidc/keys') return send(res, 200, { keys: [{ ...oidcKey.publicKey.export({ format: 'jwk' }), kid: 'e2e', alg: 'RS256', use: 'sig' }] });
			if (req.method === 'GET' && url.pathname === '/oidc/auth') {
				const keep = [...url.searchParams].map(([k, v]) => `<input type="hidden" name="${esc(k)}" value="${esc(v)}">`).join('');
				return send(res, 200, `<!doctype html><title>Fake provider</title><form action="${base}/auth/ok">${keep}<label>Username <input name="user"></label><button>Sign in</button></form>`, 'text/html');
			}
			if (req.method === 'GET' && url.pathname === '/oidc/auth/ok') {
				const q = url.searchParams;
				const user = q.get('user')?.trim();
				const to = q.get('redirect_uri');
				if (!user || !to || q.get('client_id') !== 'holyarrs' || q.get('code_challenge_method') !== 'S256') return send(res, 400, 'bad sign-in request', 'text/plain');
				const code = randomBytes(16).toString('hex');
				oidcCodes.set(code, { user, nonce: q.get('nonce'), challenge: q.get('code_challenge') });
				const back = new URL(to);
				back.searchParams.set('code', code);
				back.searchParams.set('state', q.get('state') ?? '');
				res.writeHead(303, { Location: back.toString() });
				return res.end();
			}
			if (req.method === 'POST' && url.pathname === '/oidc/token') {
				const f = new URLSearchParams(await body(req));
				const c = oidcCodes.get(f.get('code') ?? '');
				oidcCodes.delete(f.get('code') ?? '');
				const pkce = createHash('sha256').update(f.get('code_verifier') ?? '').digest('base64url');
				const why = !c ? 'unknown code' : pkce !== c.challenge ? 'code_verifier does not match' : !String(req.headers.authorization ?? '').startsWith('Basic ') ? 'no client credentials' : '';
				if (why || !c) return send(res, 400, { error: 'invalid_grant', error_description: why });
				const now = Math.floor(Date.now() / 1000);
				const part = (/** @type {object} */ o) => Buffer.from(JSON.stringify(o)).toString('base64url');
				const head = `${part({ alg: 'RS256', kid: 'e2e', typ: 'JWT' })}.${part({ iss: base, aud: 'holyarrs', sub: `sub-${c.user}`, name: c.user, nonce: c.nonce, iat: now, exp: now + 300 })}`;
				const jwt = `${head}.${sign('sha256', Buffer.from(head), oidcKey.privateKey).toString('base64url')}`;
				return send(res, 200, { access_token: randomBytes(16).toString('hex'), token_type: 'Bearer', expires_in: 300, id_token: jwt });
			}
			return send(res, 404, { error: 'no route' });
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
	/** @type {unknown[]} */
	const apps = [];
	const discord = createServer(async (req, res) => {
		// Fake ntfy (POST /<topic>), Gotify (POST /message) and Pushover
		// (POST /1/messages.json); GET /apps reads back what each was sent.
		const appPath = (req.url ?? '').split('?')[0];
		if (req.method === 'POST' && (appPath === '/message' || appPath === '/1/messages.json' || /^\/[A-Za-z0-9_-]{1,64}$/.test(appPath)) && appPath !== '/hook') {
			const app = appPath === '/message' ? 'gotify' : appPath === '/1/messages.json' ? 'pushover' : 'ntfy';
			apps.push({
				app,
				path: appPath,
				title: req.headers.title ?? '',
				click: req.headers.click ?? '',
				auth: Boolean(req.headers.authorization || req.headers['x-gotify-key']),
				body: await body(req)
			});
			return send(res, 200, app === 'pushover' ? { status: 1, request: 'e2e' } : { id: apps.length });
		}
		if (req.method === 'GET' && req.url === '/apps') return send(res, 200, apps);
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
		// A site behind a bot check (#106): a check page, as Cloudflare answers it.
		if (url.pathname.startsWith('/guarded/')) {
			res.writeHead(403, { 'Content-Type': 'text/html', 'Cf-Mitigated': 'challenge', Server: 'cloudflare' });
			res.end('<html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={}</script></body></html>');
			return;
		}
		// A Jackett (#96) at /jackett: its sign-in page, and its list of sites set up there
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
	// Fake qBittorrent: every torrent it is given downloads at 40% for
	// ever (GET /set?progress= moves them on); GET /state shows what was
	// added and deleted.
	/** @type {Map<string, Record<string, unknown>>} */
	const torrents = new Map();
	/** @type {string[]} */
	const deleted = [];
	/** @type {string[]} */
	const added = [];
	/** @type {Record<string, string>[]} */
	const shareLimits = [];
	let mainRid = 0;
	const qbit = createServer(async (req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		const form = new URLSearchParams(req.method === 'POST' ? await body(req) : url.search);
		switch (url.pathname) {
			case '/state':
				return send(res, 200, { added, deleted, shareLimits });
			case '/set': {
				// Test control: move every torrent on (progress, speed, eta),
				// or only the one named by hash; state, save_path and files
				// (names joined by |) finish it with files to import (filesize: their listed size).
				for (const t of torrents.values()) {
					if (form.get('hash') && form.get('hash') !== t.hash) continue;
					for (const k of ['progress', 'dlspeed', 'eta']) if (form.get(k)) t[k] = Number(form.get(k));
					for (const k of ['state', 'save_path']) if (form.get(k)) t[k] = form.get(k);
					if (form.get('files')) t.files = (form.get('files') ?? '').split('|').map((name) => ({ name, size: Number(form.get('filesize') ?? 1000), progress: 1 }));
				}
				return send(res, 200, { ok: true });
			}
			case '/api/v2/app/version':
				return send(res, 200, 'v5.2.4', 'text/plain');
			case '/api/v2/app/webapiVersion':
				return send(res, 200, '2.11.4', 'text/plain');
			case '/api/v2/torrents/setShareLimits':
				shareLimits.push(Object.fromEntries(form));
				return send(res, 200, '', 'text/plain');
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
			case '/api/v2/torrents/properties': {
				const t = torrents.get(form.get('hash') ?? '');
				if (!t) return send(res, 404, 'Not Found', 'text/plain');
				const size = Number(t.size);
				return send(res, 200, { total_size: size, total_downloaded: Math.round(size * Number(t.progress)), total_uploaded: 0, dl_speed: t.dlspeed, up_speed: 0, eta: t.eta, share_ratio: 0, seeds: 3, seeds_total: 10, peers: 1, peers_total: 4, addition_date: 1, completion_date: -1, save_path: t.save_path, is_private: false });
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
