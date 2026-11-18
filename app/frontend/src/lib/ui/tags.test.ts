import { describe, expect, it } from 'vitest';
import { addTags, cleanTag, switchDropped } from './tags.ts';

describe('tags', () => {
	it('cleans a label as the server does', () => {
		expect(cleanTag('  Kids ')).toBe('kids');
		expect(cleanTag('4K  Only')).toBe('4k-only');
		expect(cleanTag(',anime,')).toBe('anime');
		expect(cleanTag('x'.repeat(50))).toHaveLength(40);
	});
	it('adds comma-separated labels without repeats', () => {
		expect(addTags(['kids'], 'Kids, 4k , ,anime')).toEqual(['kids', '4k', 'anime']);
		expect(addTags(Array.from({ length: 20 }, (_, i) => `t${i}`), 'one-more')).toHaveLength(20);
	});
	it('knows when taking the last tag away switched "only these tags" off', () => {
		expect(switchDropped({ tags: ['anime'], only: true }, [])).toBe(true);
		expect(switchDropped({ tags: ['anime', 'subs'], only: true }, ['subs'])).toBe(false);
		expect(switchDropped({ tags: ['anime'], only: false }, [])).toBe(false);
		expect(switchDropped({ tags: [], only: true }, [])).toBe(false);
	});
});
