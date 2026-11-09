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
		// Kept in HD and 4K (show-versions.spec.ts): its searches also answer a 2160p copy.
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
export function soon(n) {
	return new Date(Date.now() + n * 86400000).toISOString().slice(0, 10);
}

/** @param {typeof fakeShows[number]} s */
export function seasonsOf(s) {
	return [...new Set(s.episodes.map((e) => Number(e[0])))].sort((a, b) => a - b);
}
