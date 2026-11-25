import { describe, expect, it } from 'vitest';
import { nameCheckTags, showNumbers, type NameCheckResult } from './name-check.ts';

const result = (parsed: NameCheckResult['parsed']): NameCheckResult => ({ name: 'x', parsed, title: null, copies: [] });

describe('showNumbers', () => {
	it('writes episodes, packs and runs the way names do', () => {
		expect(showNumbers({ seasons: [1], episodes: [2] })).toBe('S01E02');
		expect(showNumbers({ seasons: [1], episodes: [2, 3, 4] })).toBe('S01E02-E04');
		expect(showNumbers({ seasons: [3] })).toBe('S03');
		expect(showNumbers({ seasons: [1, 2, 3] })).toBe('S01-S03');
		expect(showNumbers({ air_date: '2026-03-14' })).toBe('2026-03-14');
		expect(showNumbers({ absolute: [112] })).toBe('#112');
		expect(showNumbers(undefined)).toBe('');
	});
});

describe('nameCheckTags', () => {
	it('adds numbers, edition, marks and the group, leaving out what the badges show', () => {
		const tags = nameCheckTags(result({ show: { seasons: [1], episodes: [2] }, edition: ['directors_cut'], flags: ['multi', 'proper'], group: { value: 'GRP', conf: 'sure' } }));
		expect(tags).toEqual(['S01E02', 'directors cut', 'multi', '-GRP']);
	});
	it('is empty for a bare movie name', () => {
		expect(nameCheckTags(result({ title: { value: 'Film', conf: 'sure' } }))).toEqual([]);
	});
});
