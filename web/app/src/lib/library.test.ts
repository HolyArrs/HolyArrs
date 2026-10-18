import { describe, expect, it } from 'vitest';
import { byName, counts, slotNotes, loadChoices, matches, matchesTitleQuery, ofKind, pickId, saveChoices, secondVersion, titleQuery, inLibraryWord, letterOf, firstOfLetter, letters, exceptionChip, missingEpisodes } from './library.ts';
import type { TitleView } from './types.ts';

function title(name: string, ...states: string[]): TitleView {
	return {
		id: 1, kind: 'movie', name, year: 2024, ids: null, keep_up_to_date: true, why_added: 'by_hand', release_gate: 'released',
		gate_delay_days: 0, artwork: [], stage: 'released', gate_open: true, gate_reason: '', versions: [],
		slots: states.map((s, i) => ({ id: i + 1, version: '', version_id: 1, state: s, state_label: s }))
	};
}

describe('library filters', () => {
	const ts = [title('Dune', 'wanted'), title('Lantern', 'not_out_yet'), title('Harbour', 'better_wanted'), title('Kept', 'in_library'), title('Busy', 'downloading')];
	it('sorts titles into filters', () => {
		expect(ts.filter((t) => matches(t, 'missing')).map((t) => t.name)).toEqual(['Dune', 'Busy']);
		expect(ts.filter((t) => matches(t, 'not_out_yet')).map((t) => t.name)).toEqual(['Lantern']);
		expect(ts.filter((t) => matches(t, 'better')).map((t) => t.name)).toEqual(['Harbour']);
		expect(ts.filter((t) => matches(t, 'have')).map((t) => t.name)).toEqual(['Harbour', 'Kept']);
	});
	it('counts each filter', () => {
		expect(counts(ts)).toEqual({ all: 5, missing: 2, not_out_yet: 1, better: 1, have: 2 });
	});
	it('sorts artists by their track counts', () => {
		const artist = (name: string, a: Partial<NonNullable<TitleView['artist']>>): TitleView => ({
			...title(name),
			kind: 'artist',
			artist: { mbid: 'x', status: 'running', new_albums: 'all', album_types: [], secondary_types: [], release_statuses: [], albums: 2, included: 2, tracks: 4, in_library: 0, missing: 0, not_out_yet: 0, better_wanted: 0, ...a }
		});
		const music = [artist('Tin Lanterns', { missing: 2, in_library: 2 }), artist('Moss Choir', { not_out_yet: 4 })];
		expect(music.filter((t) => matches(t, 'missing')).map((t) => t.name)).toEqual(['Tin Lanterns']);
		expect(music.filter((t) => matches(t, 'have')).map((t) => t.name)).toEqual(['Tin Lanterns']);
		expect(music.filter((t) => matches(t, 'not_out_yet')).map((t) => t.name)).toEqual(['Moss Choir']);
		expect(ofKind(music[0], 'artist')).toBe(true);
		expect(ofKind(music[0], 'show')).toBe(false);
	});
	it('finds by name words and year', () => {
		expect(byName(title('Dune: Part Two'), 'dune two')).toBe(true);
		expect(byName(title('Dune: Part Two'), '2024')).toBe(true);
		expect(byName(title('Dune: Part Two'), 'three')).toBe(false);
	});
	it('bounds title queries and keeps local name and year matches beside indexed ids', () => {
		const words = Array.from({ length: 18 }, (_, i) => `word${i + 1}`).join(' ');
		expect(titleQuery(`  ${words}  `).split(' ')).toHaveLength(16);
		expect(titleQuery('x'.repeat(300))).toHaveLength(256);
		expect(matchesTitleQuery(title('Dune: Part Two'), 'dune two', new Set())).toBe(true);
		expect(matchesTitleQuery(title('Dune: Part Two'), '2024', new Set())).toBe(true);
		expect(matchesTitleQuery(title('Dune: Part Two'), 'alias', new Set([1]))).toBe(true);
		expect(matchesTitleQuery(title('Dune: Part Two'), 'alias', new Set())).toBe(false);
	});
});

describe('add choices', () => {
	it('round-trips and survives bad storage', () => {
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		saveChoices({ release_gate: 'in_cinemas' }, store);
		expect(loadChoices(store)).toEqual({ release_gate: 'in_cinemas' });
		m.set('holyarrs.add-choices', '{nope');
		expect(loadChoices(store)).toEqual({});
		expect(loadChoices(undefined)).toEqual({});
	});
	it('remembers only the Release gate; folder, Pick rules and the switches come from Movies and TV defaults', () => {
		const m = new Map<string, string>([['holyarrs.add-choices', JSON.stringify({ library_folder_id: 2, pick_rules_id: 3, keep_up_to_date: false, release_gate: 'announced', search_on_add: false })]]);
		const store = { getItem: (k: string) => m.get(k) ?? null };
		expect(loadChoices(store)).toEqual({ release_gate: 'announced' });
		m.set('holyarrs.add-choices', JSON.stringify({ library_folder_id: 2 }));
		expect(loadChoices(store)).toEqual({});
	});
	it('keeps a remembered id only when offered', () => {
		expect(pickId(2, [{ id: 1 }, { id: 2 }], 1)).toBe(2);
		expect(pickId(9, [{ id: 1 }], 1)).toBe(1);
		expect(pickId(undefined, [], 0)).toBe(0);
	});
});

describe('secondVersion', () => {
	const rules = [
		{ id: 1, name: 'Good everyday' },
		{ id: 2, name: '4K' }
	];
	it('suggests the 4K Pick rules and a 4K folder', () => {
		const folders = [
			{ id: 10, label: 'Movies', path: '/movies' },
			{ id: 11, label: 'Movies UHD', path: '/movies-uhd' }
		];
		expect(secondVersion(rules, folders, 10)).toEqual({ label: '4K', pick_rules_id: 2, library_folder_id: 11 });
	});
	it('falls back to the main folder and the first Pick rules', () => {
		expect(secondVersion([{ id: 1, name: 'Good everyday' }], [{ id: 10, label: 'Movies', path: '/movies' }], 10)).toEqual({
			label: '4K',
			pick_rules_id: 1,
			library_folder_id: 10
		});
	});
});

describe('slotNotes', () => {
	it('drops gate notes (the Why line says it) and repeats across Versions', () => {
		const slots = [{ reason: 'release gate: searching from 2023-11-21 (digital date)' }, { reason: 'release gate: searching from 2023-11-21 (digital date)' }, { reason: 'searching' }, { reason: 'searching' }, { reason: '' }];
		expect(slotNotes(slots)).toEqual(['searching']);
	});
});

describe('inLibraryWord', () => {
	it('names the Versions in the Library when a show has several', () => {
		expect(inLibraryWord('in_library')).toBe(' · in the Library');
		expect(inLibraryWord('wanted')).toBe('');
		const vs = [
			{ label: 'Main', state: 'in_library' },
			{ label: '4K', state: 'wanted' }
		];
		expect(inLibraryWord('in_library', vs)).toBe(' · in the Library: Main');
		expect(inLibraryWord('wanted', [{ label: 'Main', state: 'wanted' }, { label: '4K', state: 'in_library' }])).toBe(' · in the Library: 4K');
		expect(inLibraryWord('wanted', [{ label: 'Main', state: 'wanted' }, { label: '4K', state: 'wanted' }])).toBe('');
	});
});

describe('library letters and quiet states', () => {
	it('files names under a letter, accents and numbers included', () => {
		expect(letterOf('Élan')).toBe('E');
		expect(letterOf(' quiet Orchard')).toBe('Q');
		expect(letterOf('2 Guns')).toBe('#');
		expect(letterOf('')).toBe('#');
		const m = firstOfLetter([{ id: 3, name: 'Alpha' }, { id: 1, name: 'Avon' }, { id: 2, name: 'Cinder' }]);
		expect([...m]).toEqual([['A', 3], ['C', 2]]);
		expect(letters).toHaveLength(27);
	});

	it('drops the state a view already says, keeps counts and other states', () => {
		const have = { state: 'in_library', label: 'In library' };
		expect(exceptionChip('all', have)).toBeNull();
		expect(exceptionChip('have', have)).toBeNull();
		expect(exceptionChip('missing', have)).toBe(have);
		expect(exceptionChip('all', { state: 'in_library', label: '4 in library' })).not.toBeNull();
		expect(exceptionChip('missing', { state: 'wanted', label: 'Wanted' })).toBeNull();
		expect(exceptionChip('missing', { state: 'wanted', label: '3 missing' })).not.toBeNull();
		expect(exceptionChip('all', { state: 'downloading', label: 'Downloading' })).not.toBeNull();
		expect(exceptionChip('all', null)).toBeNull();
	});

	it('lists the missing episodes of a show', () => {
		const ep = (id: number, keep: boolean, state: string) => ({ id, keep_up_to_date: keep, slots: [{ state }] });
		expect(missingEpisodes([ep(1, true, 'wanted'), ep(2, true, 'in_library'), ep(3, false, 'wanted'), ep(4, true, 'downloading'), ep(5, true, 'not_out_yet')]).map((e) => e.id)).toEqual([1, 4]);
	});
});
