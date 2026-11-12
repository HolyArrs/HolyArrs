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
	it('offers none for a 4K copy, which always uses the 4K quality from the request settings', () => {
		const c = createAskChoices({ rules, folders: [] });
		expect(c.rulesOf({ kind: 'show', four_k: true } as Ask)).toEqual([]);
		expect(c.rulesOf({ kind: 'movie', four_k: true } as Ask)).toEqual([]);
	});
});
