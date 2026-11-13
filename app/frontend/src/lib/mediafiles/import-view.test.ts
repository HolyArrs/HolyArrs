import { describe, expect, it } from 'vitest';
import { bannerTitle, canTick, findFolders, matchOptions, pickedOf, readImportView, sortFolders, statusCounts, tickCounts, tickState } from './import-view.ts';
import type { AdoptCandidate } from './organise.ts';

const f = (folder: string, over: Partial<AdoptCandidate> = {}): AdoptCandidate => ({
	folder,
	parsed_name: folder.replace(/ \(\d+\)$/, ''),
	videos: [],
	others: [],
	status: 'matched',
	why: '',
	...over
});
const heat = f('Heat (1995)', { match: { tmdb_id: '949', name: 'Heat', year: 1995, score: 1, poster: '/heat.jpg' } });
const amelie = f('Amelie', { status: 'unsure', match: { tmdb_id: '194', name: 'Amélie', year: 2001, score: 0.85 }, others: [{ tmdb_id: '7', name: 'Amelia', year: 2009, score: 0.5 }] });
const alien = f('Alien (1979)', { status: 'unsure', match: { tmdb_id: '348', name: 'Alien', year: 1979, score: 1 } });
const notes = f('Notes', { status: 'unmatched' });
const owned = f('Coco (2017)', { status: 'in_library', title_id: 4, match: { tmdb_id: '354912', name: 'Coco', year: 2017, score: 1 } });

describe('readImportView', () => {
	it('keeps a known view and opens on Rows otherwise', () => {
		expect(readImportView('posters')).toBe('posters');
		expect(readImportView('banners')).toBe('banners');
		expect(readImportView('overview')).toBe('overview');
		expect(readImportView('table')).toBe('rows');
		expect(readImportView(undefined)).toBe('rows');
		expect(readImportView(3)).toBe('rows');
	});
});

describe('sortFolders', () => {
	it('puts the folders to check first, then by title, and the ones left alone last', () => {
		expect(sortFolders([owned, heat, notes, amelie, alien]).map((x) => x.folder)).toEqual(['Alien (1979)', 'Amelie', 'Notes', 'Heat (1995)', 'Coco (2017)']);
	});
	it('does not change the list it is given', () => {
		const list = [heat, alien];
		sortFolders(list);
		expect(list[0]).toBe(heat);
	});
});

describe('findFolders', () => {
	const all = [heat, amelie, alien, notes, owned];
	it('finds by title or folder name, ignoring case and accents', () => {
		expect(findFolders(all, 'amelie').map((x) => x.folder)).toEqual(['Amelie']);
		expect(findFolders(all, 'AMÉLIE 2001').map((x) => x.folder)).toEqual(['Amelie']);
		expect(findFolders(all, '1979').map((x) => x.folder)).toEqual(['Alien (1979)']);
		expect(findFolders(all, 'not').map((x) => x.folder)).toEqual(['Notes']);
	});
	it('keeps every folder with no words, and narrows by status', () => {
		expect(findFolders(all, '  ')).toHaveLength(5);
		expect(findFolders(all, '', 'unsure').map((x) => x.folder)).toEqual(['Amelie', 'Alien (1979)']);
		expect(findFolders(all, 'heat', 'unsure')).toEqual([]);
	});
});

describe('statusCounts', () => {
	it('counts each status found, in the filter order', () => {
		expect(statusCounts([amelie, heat, alien, owned])).toEqual([
			{ status: 'matched', n: 1 },
			{ status: 'unsure', n: 2 },
			{ status: 'in_library', n: 1 }
		]);
	});
});

describe('pickedOf and matchOptions', () => {
	it('shows the pick from the scan, a looked-up title, or the folder name', () => {
		expect(pickedOf(amelie, '7')).toEqual({ name: 'Amelia', year: 2009, score: 0.5, poster: '' });
		expect(pickedOf(amelie, '99', { '99': { name: 'Amélie Two', year: 2020, poster: '/a2.jpg' } })).toEqual({ name: 'Amélie Two', year: 2020, poster: '/a2.jpg' });
		expect(pickedOf(notes, undefined)).toEqual({ name: 'Notes', year: undefined, poster: '' });
	});
	it('lists the match and its others once, plus a looked-up pick', () => {
		const dup = f('X', { match: { tmdb_id: '1', name: 'X', year: 2000, score: 1 }, others: [{ tmdb_id: '1', name: 'X', year: 2000, score: 1 }] });
		expect(matchOptions(dup, '1')).toEqual([{ value: '1', label: 'X (2000) · 100%' }]);
		expect(matchOptions(amelie, '99', { '99': { name: 'Amélie Two', year: 2020 } }).map((o) => o.label)).toEqual(['Amélie (2001) · 85%', 'Amelia (2009) · 50%', 'Amélie Two (2020)']);
		expect(matchOptions(notes, undefined)).toEqual([]);
	});
});

describe('ticks', () => {
	it('ticks only folders that can be adopted and have a pick', () => {
		expect(canTick(heat, { 'Heat (1995)': '949' })).toBe(true);
		expect(canTick(heat, {})).toBe(false);
		expect(canTick(owned, { 'Coco (2017)': '354912' })).toBe(false);
	});
	it('says whether all, some or none of the shown folders are ticked', () => {
		const sel = { 'Heat (1995)': '949', 'Alien (1979)': '348' };
		expect(tickState([heat, alien, notes], sel, {})).toBe('none');
		expect(tickState([heat, alien, notes], sel, { 'Heat (1995)': true })).toBe('some');
		expect(tickState([heat, alien, notes], sel, { 'Heat (1995)': true, 'Alien (1979)': true })).toBe('all');
	});
	it('counts the shown folders that can be ticked and how many of them are', () => {
		const sel = { 'Heat (1995)': '949', 'Alien (1979)': '348' };
		expect(tickCounts([heat, alien, notes, owned], sel, { 'Heat (1995)': true, 'Notes': true })).toEqual({ can: 2, on: 1 });
		expect(tickCounts([], sel, {})).toEqual({ can: 0, on: 0 });
	});
});

describe('bannerTitle', () => {
	const p = { name: 'Heat', year: 1995, poster: '/heat.jpg' };
	it('asks the extras by TMDB id for movies and shows, with the poster as the fallback', () => {
		expect(bannerTitle('movie', '949', p)).toMatchObject({ kind: 'movie', ids: { tmdb: '949' }, artwork: ['/heat.jpg'] });
		expect(bannerTitle('show', 'tmdb:1396', p)).toMatchObject({ kind: 'show', ids: { tmdb: '1396' } });
		expect(bannerTitle('show', 'tvdb:81189', p).ids).toEqual({});
	});
	it('uses the MusicBrainz id for artists', () => {
		expect(bannerTitle('music', 'mbid:abc', p)).toMatchObject({ kind: 'artist', artist: { mbid: 'abc' } });
	});
});
