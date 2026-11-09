import { tmdbPort } from './ports.mjs';


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
	// Followed while in the Library without a file (decisions.spec): the follower hears once its file is in.
	{ id: 900037, title: 'Heron Clockwork', release_date: '2015-11-11', imdb_id: 'tt9000370', overview: 'A clock that keeps time by the herons on the river.', dates: [[3, '2015-11-11'], [4, '2016-01-01']] },
	// The ask whose download fails (decisions.spec): its two good Releases are marked failed.
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
	// Only decisions.spec.ts: asked again and declined by three admins in turn.
	{ id: 900601, title: 'Harrow Teaspoon', release_date: '2015-04-04', imdb_id: 'tt9006010', overview: 'A teaspoon that stirs the whole village awake.', dates: [[3, '2015-04-04'], [4, '2015-06-01']] },
	// Only decisions.spec.ts: a member's open Requests page follows its decision live.
	{ id: 900602, title: 'Quince Lamplight', release_date: '2016-05-05', imdb_id: 'tt9006020', overview: 'A lamplighter who only lights the lamps no one walks under.', dates: [[3, '2016-05-05'], [4, '2016-07-01']] },
	// Only decisions.spec.ts: approved, nothing found for it; its steps show live.
	{ id: 900603, title: 'Rook Lantern', release_date: '2017-02-02', imdb_id: 'tt9006030', overview: 'A lantern a rook carries across the moor each night.', dates: [[3, '2017-02-02'], [4, '2017-04-01']] },
	// Only decisions.spec.ts: approved, found and downloaded; the member sees each step.
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
