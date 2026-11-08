import { describe, expect, it } from 'vitest';
import { builtInDefaults, changedParts, defaultsLoader, foldersFor, rulesFor, startChoices } from './title-defaults.ts';
import type { LibraryFolder, PickRulesView } from '../core/types.ts';
import type { TitleDefaults } from './types.ts';

const folder = (id: number, kind: string, over: Partial<LibraryFolder> = {}): LibraryFolder => ({
	id,
	path: `/f${id}`,
	kind,
	label: '',
	default_pick_rules_id: null,
	free_space: 0,
	health: 'ok',
	health_reason: '',
	...over
});
const rule = (id: number, kinds: string[]): PickRulesView => ({ id, name: `R${id}`, kinds, preset: false });

const folders = [
	folder(1, 'movie', { health: 'missing' }),
	folder(2, 'movie', { default_pick_rules_id: 11 }),
	folder(3, 'movie', { default_pick_rules_id: 12 }),
	folder(4, 'show', { default_pick_rules_id: 13 }),
	folder(5, 'music')
];
const rules = [rule(10, ['movie', 'show']), rule(11, ['movie']), rule(12, ['movie', 'show']), rule(13, ['show']), rule(14, ['music'])];
const saved = (movie: Partial<TitleDefaults['movie']>, show: Partial<TitleDefaults['show']> = {}): TitleDefaults => ({
	movie: { ...builtInDefaults.movie, ...movie },
	show: { ...builtInDefaults.show, ...show }
});

describe('builtInDefaults', () => {
	it('keeps both up to date, searches when a movie is added and not when a show is', () => {
		expect(builtInDefaults).toEqual({
			movie: { library_folder_id: 0, pick_rules_id: 0, keep_up_to_date: true, search_on_add: true },
			show: { library_folder_id: 0, pick_rules_id: 0, keep_up_to_date: true, search_on_add: false }
		});
	});
});

describe('foldersFor and rulesFor', () => {
	it('offers only the folders and Pick rules of that kind; rules without kinds fit every kind', () => {
		expect(foldersFor(folders, 'show').map((f) => f.id)).toEqual([4]);
		expect(rulesFor(rules, 'movie').map((r) => r.id)).toEqual([10, 11, 12]);
		expect(rulesFor(rules, 'show').map((r) => r.id)).toEqual([10, 12, 13]);
		expect(rulesFor([rule(20, [])], 'show').map((r) => r.id)).toEqual([20]);
	});
});

describe('startChoices', () => {
	it('with nothing saved: the first healthy folder, its own Pick rules, keep on, search for movies', () => {
		expect(startChoices(null, folders, rules, 'movie')).toEqual({ folderId: 2, rulesId: 11, keep: true, searchOnAdd: true });
		expect(startChoices(builtInDefaults, folders, rules, 'show')).toEqual({ folderId: 4, rulesId: 13, keep: true, searchOnAdd: false });
	});

	it('uses the saved folder, rules and switches', () => {
		const d = saved({ library_folder_id: 3, pick_rules_id: 10, keep_up_to_date: false, search_on_add: false });
		expect(startChoices(d, folders, rules, 'movie')).toEqual({ folderId: 3, rulesId: 10, keep: false, searchOnAdd: false });
	});

	it("a saved folder without saved rules takes the folder's own", () => {
		expect(startChoices(saved({ library_folder_id: 3 }), folders, rules, 'movie').rulesId).toBe(12);
	});

	it('a saved folder or rule that is gone, or of another kind, falls back', () => {
		const d = saved({ library_folder_id: 4, pick_rules_id: 14 });
		expect(startChoices(d, folders, rules, 'movie')).toMatchObject({ folderId: 2, rulesId: 11 });
		expect(startChoices(saved({ library_folder_id: 99, pick_rules_id: 99 }), folders, rules, 'movie')).toMatchObject({ folderId: 2, rulesId: 11 });
	});

	it("a folder whose own rules are not offered takes the first rule for the kind", () => {
		const f = [folder(7, 'movie', { default_pick_rules_id: 14 })];
		expect(startChoices(null, f, rules, 'movie')).toMatchObject({ folderId: 7, rulesId: 10 });
	});

	it('no healthy folder: the first one; no folder or rule at all: 0', () => {
		expect(startChoices(null, [folder(8, 'movie', { health: 'missing' })], rules, 'movie').folderId).toBe(8);
		expect(startChoices(null, [], [], 'show')).toEqual({ folderId: 0, rulesId: 0, keep: true, searchOnAdd: false });
	});
});

describe('changedParts', () => {
	it('names the routes whose part changed', () => {
		const a = { defaults: builtInDefaults, gate: { release_gate: 'released', gate_delay_days: 0, bulk_search_respects_gate: true }, shows: { season_choice: 'all' } };
		const same = JSON.parse(JSON.stringify(a));
		expect(changedParts(a, same)).toEqual([]);
		const b = { ...same, gate: { ...same.gate, gate_delay_days: 3 } };
		expect(changedParts(a, b)).toEqual(['gate']);
		const c = { ...b, defaults: saved({ search_on_add: false }), shows: { season_choice: 'latest' } };
		expect(changedParts(a, c)).toEqual(['defaults', 'gate', 'shows']);
	});
});

describe('defaultsLoader', () => {
	const saved: TitleDefaults = { movie: { library_folder_id: 4, pick_rules_id: 2, keep_up_to_date: true, search_on_add: false }, show: { ...builtInDefaults.show } };

	it('asks the server once per visit and shares the answer', async () => {
		let calls = 0;
		const l = defaultsLoader(async (p) => (calls++, expect(p).toBe('/settings/title-defaults'), saved));
		const [a, b] = await Promise.all([l.load(), l.load()]);
		expect(a).toEqual(saved);
		expect(b).toBe(a);
		expect(calls).toBe(1);
		l.forget();
		await l.load();
		expect(calls).toBe(2);
	});

	it('starts from the built-in defaults when the server fails, and tries again next time', async () => {
		let calls = 0;
		const l = defaultsLoader(async () => {
			calls++;
			if (calls === 1) throw new Error('down');
			return saved;
		});
		expect(await l.load()).toBeNull();
		expect(await l.load()).toEqual(saved);
	});
});
