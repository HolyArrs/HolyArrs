import { genreTintToken } from './genre-tint.ts';
import { describe, expect, it } from 'vitest';

describe('genreTintToken', () => {
	it('gives a listed genre the same token whatever its hue', () => {
		expect(genreTintToken('rock', 3)).toBe('--mg-wine');
		expect(genreTintToken('rock', 19)).toBe('--mg-wine');
	});
	it('gives neighbouring genres different tokens', () => {
		const order = ['hip-hop-rap', 'country', 'pop', 'randb-soul', 'alternative', 'latin', 'rock', 'soundtrack', 'k-pop', 'children-s-music', 'dance', 'electronic', 'metal'];
		const t = order.map((s) => genreTintToken(s));
		expect(new Set(t).size).toBe(order.length);
	});
	it('colours an unlisted genre from its hue', () => {
		expect(genreTintToken('polka', 2)).toMatch(/^--mg-[a-z]+$/);
		expect(genreTintToken('polka', -3)).toMatch(/^--mg-[a-z]+$/);
	});
});
