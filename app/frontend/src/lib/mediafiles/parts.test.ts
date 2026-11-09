import { describe, expect, it } from 'vitest';
import { needsProblem, offPartName, screenOwner, switchPlan, type PartsAnswer, type PartName } from './parts.ts';

const screens = {
	'/': 'core',
	'/library': 'media',
	'/library/[id]': 'title',
	'/library/held': 'media',
	'/library/[id]/albums/[album]': 'music',
	'/library/missing-subtitles': 'subtitles',
	'/requests': 'wishlist',
	'/settings/music': 'music'
};

function answer(off: PartName[]): PartsAnswer {
	const names: Record<PartName, string> = { movies: 'Movies', shows: 'TV shows', music: 'Music', subtitles: 'Subtitles', wishlist: 'Requests' };
	return {
		parts: (Object.keys(names) as PartName[]).map((p) => ({ part: p, name: names[p], on: !off.includes(p), state: off.includes(p) ? 'off' : 'running' })),
		screens,
		safe_mode: false
	};
}

describe('screen owners', () => {
	it('prefers a fixed path over a changing one', () => {
		expect(screenOwner(screens, '/library/held')).toBe('media');
		expect(screenOwner(screens, '/library/12')).toBe('title');
		expect(screenOwner(screens, '/library/12/albums/3')).toBe('music');
		expect(screenOwner(screens, '/')).toBe('core');
		expect(screenOwner(screens, '/nowhere')).toBeNull();
	});
});

describe('pages of parts that are off', () => {
	it('names the part that is off', () => {
		expect(offPartName(answer(['music']), '/settings/music')).toBe('Music');
		expect(offPartName(answer(['wishlist']), '/requests')).toBe('Requests');
		expect(offPartName(answer(['subtitles']), '/library/missing-subtitles')).toBe('Subtitles');
	});
	it('shows pages of parts that are on, the core and single titles', () => {
		expect(offPartName(answer([]), '/settings/music')).toBeNull();
		expect(offPartName(answer(['movies', 'shows', 'music']), '/')).toBeNull();
		expect(offPartName(answer(['movies']), '/library/12')).toBeNull();
		expect(offPartName(null, '/requests')).toBeNull();
	});
	it('hides shared pages only when Movies, TV shows and Music are all off', () => {
		expect(offPartName(answer(['movies', 'shows']), '/library')).toBeNull();
		expect(offPartName(answer(['movies', 'shows', 'music']), '/library')).toBe('Movies, TV shows and Music');
	});
});

describe('safe mode', () => {
	it('keeps the Library pages while every part is off', () => {
		const a = { ...answer(['movies', 'shows', 'music', 'subtitles', 'wishlist']), safe_mode: true, safe_screens: ['/library', '/library/[id]', '/library/[id]/albums/[album]'] };
		expect(offPartName(a, '/library')).toBeNull();
		expect(offPartName(a, '/library/12/albums/3')).toBeNull();
		expect(offPartName(a, '/library/held')).toBe('Movies, TV shows and Music');
		expect(offPartName(a, '/settings/music')).toBe('Music');
		expect(offPartName({ ...a, safe_mode: false }, '/library')).toBe('Movies, TV shows and Music');
	});
});

describe('switching parts', () => {
	const all = { movies: true, shows: true, music: true, subtitles: true, wishlist: true };
	it('turns on before off, and dependents off before what they need', () => {
		expect(switchPlan(all, { movies: true, shows: false, music: false, subtitles: false, wishlist: false })).toEqual([
			{ part: 'wishlist', on: false },
			{ part: 'subtitles', on: false },
			{ part: 'music', on: false },
			{ part: 'shows', on: false }
		]);
		const few = { movies: false, shows: true, music: false, subtitles: false, wishlist: false };
		expect(switchPlan(few, { movies: true, shows: false, music: false, subtitles: true, wishlist: true })).toEqual([
			{ part: 'movies', on: true },
			{ part: 'subtitles', on: true },
			{ part: 'wishlist', on: true },
			{ part: 'shows', on: false }
		]);
		expect(switchPlan(all, all)).toEqual([]);
	});
	it('says what cannot be', () => {
		const none = { movies: false, shows: false, music: false, subtitles: false, wishlist: false };
		expect(needsProblem(none)).toContain('at least one');
		expect(needsProblem({ ...none, music: true, subtitles: true })).toBe('Subtitles need Movies or TV shows.');
		expect(needsProblem({ ...none, music: true, wishlist: true })).toBe('');
	});
});
