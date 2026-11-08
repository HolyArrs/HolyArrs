import { describe, expect, it } from 'vitest';
import { addTags, cleanTag } from './tags.ts';

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
});
