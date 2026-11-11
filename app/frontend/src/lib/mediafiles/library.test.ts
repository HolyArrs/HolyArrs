import { describe, expect, it } from 'vitest';
import { byName, counts, emptyAdds, slotNotes, loadChoices, matches, matchesTitleQuery, ofKind, pickId, saveChoices, secondVersion, titleQuery, letterOf, firstOfLetter, letters, exceptionChip, missingEpisodes, posterStatus } from './library.ts';
import type { TitleView } from '../core/types.ts';

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
		{ id: 1, name: 'Standard' },
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
		expect(secondVersion([{ id: 1, name: 'Standard' }], [{ id: 10, label: 'Movies', path: '/movies' }], 10)).toEqual({
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
		const have = { kind: 'in_library', label: 'In your Library' } as const;
		expect(exceptionChip('all', have)).toBeNull();
		expect(exceptionChip('have', have)).toBeNull();
		expect(exceptionChip('better', have)).toBeNull();
		expect(exceptionChip('missing', have)).toBe(have);
		expect(exceptionChip('missing', { kind: 'missing', label: 'Missing' })).toBeNull();
		expect(exceptionChip('not_out_yet', { kind: 'coming_soon', label: 'Coming soon' })).toBeNull();
		expect(exceptionChip('missing', { kind: 'missing', label: 'Missing · 3' })).not.toBeNull();
		expect(exceptionChip('all', { kind: 'downloading', label: 'Downloading' })).not.toBeNull();
		expect(exceptionChip('all', null)).toBeNull();
	});

	describe('posterStatus', () => {
		const show = (s: Partial<NonNullable<TitleView['show']>>): TitleView => ({ ...title('Harbour Lights'), kind: 'show', slots: [], show: { details_source: '', episode_order: '', new_seasons: '', status: '', seasons: [{ aired: 6 } as never], episodes: 6, missing: 0, not_out_yet: 0, in_library: 0, better_wanted: 0, ...s } });

		it('gives a movie one status in the shared words', () => {
			expect(posterStatus(title('A', 'in_library', 'wanted'), 'staff')).toEqual({ kind: 'in_library', label: 'In library' });
			expect(posterStatus(title('A', 'in_library'), 'member')).toEqual({ kind: 'in_library', label: 'Ready to watch' });
			expect(posterStatus(title('A', 'wanted'), 'staff')).toEqual({ kind: 'missing', label: 'Missing' });
			expect(posterStatus(title('A', 'not_out_yet'), 'staff')?.kind).toBe('coming_soon');
		});

		it('never shows Failed to members', () => {
			expect(posterStatus(title('A', 'failed'), 'staff')).toEqual({ kind: 'failed', label: 'Failed' });
			expect(posterStatus(title('A', 'failed'), 'member')).toEqual({ kind: 'requested', label: 'Requested' });
			expect(posterStatus({ ...title('A', 'failed'), asks: [{ state: 'approved' } as never] }, 'member')).toEqual({ kind: 'approved', label: 'Approved' });
		});

		it('tells members a missing movie is on its way', () => {
			expect(posterStatus(title('A', 'wanted'), 'member')).toEqual({ kind: 'requested', label: 'Requested' });
		});

		it('puts a show count on the shared word', () => {
			expect(posterStatus(show({ missing: 3, in_library: 2 }), 'staff')).toEqual({ kind: 'missing', label: 'Missing · 3' });
			expect(posterStatus(show({ missing: 1 }), 'staff')?.label).toBe('Missing · 1');
			expect(posterStatus(show({ missing: 3 }), 'member')).toEqual({ kind: 'requested', label: 'Requested · 3 episodes' });
			expect(posterStatus(show({ in_library: 6 }), 'member')).toEqual({ kind: 'in_library', label: 'Ready to watch' });
			expect(posterStatus(show({ not_out_yet: 2 }), 'staff')).toEqual({ kind: 'coming_soon', label: 'Upcoming' });
			expect(posterStatus(show({}), 'staff')).toBeNull();
		});

		it('counts tracks for an artist and says ready to listen', () => {
			const artist: TitleView = { ...title('Tin Lanterns'), kind: 'artist', slots: [], artist: { mbid: '', status: '', new_albums: '', album_types: [], secondary_types: [], release_statuses: [], albums: 1, included: 1, tracks: 4, in_library: 4, missing: 0, not_out_yet: 0, better_wanted: 0 } };
			expect(posterStatus(artist, 'member')).toEqual({ kind: 'in_library', label: 'Ready to listen' });
			expect(posterStatus({ ...artist, artist: { ...artist.artist!, in_library: 2, missing: 2 } }, 'staff')?.label).toBe('Missing · 2');
		});
	});

	it('lists the missing episodes of a show', () => {
		const ep = (id: number, keep: boolean, state: string) => ({ id, keep_up_to_date: keep, slots: [{ state }] });
		expect(missingEpisodes([ep(1, true, 'wanted'), ep(2, true, 'in_library'), ep(3, false, 'wanted'), ep(4, true, 'downloading'), ep(5, true, 'not_out_yet')]).map((e) => e.id)).toEqual([1, 4]);
	});
});

describe('emptyAdds', () => {
	const allOn = () => false;
	it('offers only the page kind on a kind page', () => {
		expect(emptyAdds('movie', allOn)).toEqual([{ kind: 'movie', href: '/search?kind=movie' }]);
		expect(emptyAdds('show', allOn)).toEqual([{ kind: 'show', href: '/search?kind=show' }]);
		expect(emptyAdds('artist', allOn)).toEqual([{ kind: 'artist', href: '/search?kind=music' }]);
	});
	it('offers every part that is on for All, movies first', () => {
		expect(emptyAdds('all', allOn).map((a) => a.kind)).toEqual(['movie', 'show', 'artist']);
		expect(emptyAdds('all', (p) => p === 'music').map((a) => a.kind)).toEqual(['movie', 'show']);
		expect(emptyAdds('all', (p) => p === 'movies').map((a) => a.kind)).toEqual(['show', 'artist']);
	});
});
