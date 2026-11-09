// Made-up artists for the fake MusicBrainz. Albums: [id, name, main kind,
// extra kinds, first date, Pressings]; a Pressing is [id, name note,
// country, date, format, track count]. 2099 is not out yet.
export const mbid = (/** @type {string} */ head, /** @type {number} */ n) => `${head}-0000-4000-8000-${String(n).padStart(12, '0')}`;
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
