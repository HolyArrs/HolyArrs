import { describe, expect, it } from 'vitest';
import { createAskChoices } from './ask-choices.svelte.ts';
import type { Ask, PickRulesView } from '#lib/core/types.ts';

const rules = [
	{ id: 1, name: 'Standard', kinds: ['movie', 'show'] },
	{ id: 2, name: '4K', kinds: ['movie', 'show'] }
] as PickRulesView[];

describe('quality offered when approving', () => {
	it('offers the quality choices for a plain request', () => {
		const c = createAskChoices({ rules, folders: [] });
		expect(c.rulesOf({ kind: 'show' } as Ask).map((r) => r.id)).toEqual([1, 2]);
	});
	it('offers the lines with the other rules between them for a 4K copy too, the 4K line picked', () => {
		const c = createAskChoices({ rules, folders: [] });
		c.lines = { movie: [{ id: 'standard', name: '', extra: false, pick_rules_id: 1 }, { id: '4k', name: '', extra: true, pick_rules_id: 2 }], show: [] };
		const row = c.qualityRowOf({ kind: 'movie', quality: '4k', four_k: true } as Ask);
		expect(row.pills.map((p) => p.label)).toEqual(['Standard', '4K']);
		expect(row.asked).toBe('4k');
		expect(c.qualityRowOf({ kind: 'artist' } as Ask).pills).toEqual([]);
	});
});
