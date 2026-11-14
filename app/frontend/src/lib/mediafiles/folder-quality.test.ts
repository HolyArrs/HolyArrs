import { describe, expect, it } from 'vitest';
import { defaultChoice, qualityBody, qualityChoices, qualityFor, qualityValue } from './folder-quality.ts';
import type { PickRulesView } from '../core/types.ts';
import type { TitleDefaults } from './types.ts';

const rule = (id: number, kinds: string[]): PickRulesView => ({ id, name: `R${id}`, kinds, preset: false });
const rules = [rule(1, ['movie', 'show']), rule(2, ['movie']), rule(3, ['show']), rule(4, ['music']), rule(5, ['artist'])];
const ids = (l: PickRulesView[]) => l.map((r) => r.id);

describe('qualityFor', () => {
	it('lists only the sets that fit the folder kind', () => {
		expect(ids(qualityFor(rules, 'movie'))).toEqual([1, 2]);
		expect(ids(qualityFor(rules, 'show'))).toEqual([1, 3]);
		expect(ids(qualityFor(rules, 'music'))).toEqual([4, 5]);
	});
	it('keeps a set the folder already has, even of another kind', () => {
		expect(ids(qualityFor(rules, 'movie', 4))).toEqual([1, 2, 4]);
	});
});

describe('qualityBody', () => {
	it('sends the id as a number', () => {
		expect(qualityBody('7')).toEqual({ default_pick_rules_id: 7 });
	});
	it('sends 0 for the first option, which clears the folder quality', () => {
		expect(qualityBody('')).toEqual({ default_pick_rules_id: 0 });
		expect(qualityBody(null)).toEqual({ default_pick_rules_id: 0 });
	});
});

// The settings row.
const set = (id: number, name: string, kinds: string[]): PickRulesView => ({ id, name, kinds, preset: true });
const sets = [set(1, 'Standard', ['movie', 'show']), set(2, 'Best', ['movie', 'show']), set(3, 'Lossless first', ['music']), set(4, 'Anime', ['show'])];
const saved = (movie: number, show = 0): TitleDefaults => ({
	movie: { library_folder_id: 0, pick_rules_id: movie, keep_up_to_date: true, search_on_add: true },
	show: { library_folder_id: 0, pick_rules_id: show, keep_up_to_date: true, search_on_add: false }
});

describe('qualityValue', () => {
	it('shows the folder own set by name', () => {
		expect(qualityValue({ kind: 'movie', default_pick_rules_id: 2 }, sets)).toEqual({ text: 'Best', own: true });
	});
	it('names the built-in default per kind', () => {
		expect(qualityValue({ kind: 'movie', default_pick_rules_id: null }, sets)).toEqual({ text: 'Standard · default', own: false });
		expect(qualityValue({ kind: 'music', default_pick_rules_id: null }, sets).text).toBe('Lossless first · default');
	});
	it('follows the saved Movies and TV default', () => {
		expect(qualityValue({ kind: 'show', default_pick_rules_id: null }, sets, saved(0, 4)).text).toBe('Anime · default');
		expect(qualityValue({ kind: 'movie', default_pick_rules_id: null }, sets, saved(2)).text).toBe('Best · default');
	});
	it('says just Default when the default is unknown', () => {
		expect(qualityValue({ kind: 'movie', default_pick_rules_id: null }, [set(9, 'Mine', ['movie'])])).toEqual({ text: 'Default', own: false });
		expect(defaultChoice([], 'music')).toBe('Default');
	});
});

describe('qualityChoices', () => {
	it('lists the default first, then the sets that fit each kind', () => {
		expect(qualityChoices(sets, 'movie', null).map((c) => c.label)).toEqual(['Default (Standard)', 'Standard', 'Best']);
		expect(qualityChoices(sets, 'show', null).map((c) => c.label)).toEqual(['Default (Standard)', 'Standard', 'Best', 'Anime']);
		expect(qualityChoices(sets, 'music', null).map((c) => c.label)).toEqual(['Default (Lossless first)', 'Lossless first']);
	});
	it('ticks the current one', () => {
		expect(qualityChoices(sets, 'movie', null).filter((c) => c.checked).map((c) => c.id)).toEqual([0]);
		expect(qualityChoices(sets, 'movie', 2).filter((c) => c.checked).map((c) => c.id)).toEqual([2]);
	});
});
