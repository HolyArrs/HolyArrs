import { describe, expect, it } from 'vitest';
import { qualityBody, qualityFor } from './folder-quality.ts';
import type { PickRulesView } from '../core/types.ts';

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
