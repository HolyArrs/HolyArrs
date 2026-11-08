import { initials, pickPosters, wall } from './pick.ts';
import { describe, expect, it } from 'vitest';
import { flip, shift } from '#lib/core/motion.ts';

describe('Settings home pictures', () => {
	it('keeps movie and show posters, newest first, no repeats, at most the limit', () => {
		const t = [
			{ id: 1, kind: 'movie', artwork: ['/a'] },
			{ id: 3, kind: 'show', artwork: ['/b'] },
			{ id: 2, kind: 'artist', artwork: ['/c'] },
			{ id: 4, kind: 'movie', artwork: [] },
			{ id: 5, kind: 'movie', artwork: ['/a'] }
		];
		expect(pickPosters(t)).toEqual(['/a', '/b']);
		expect(pickPosters(t, 1)).toEqual(['/a']);
	});
	it('makes initials and fills the wall only when there are posters', () => {
		expect(initials('Maya Lopez')).toBe('ML');
		expect(initials('luis')).toBe('LU');
		expect(initials('  ')).toBe('?');
		expect(initials('Luis (old)')).toBe('LO');
		expect(wall([], 4)).toEqual([]);
		expect(wall(['x', 'y'], 5)).toEqual(['x', 'y', 'x', 'y', 'x']);
	});
});

describe('Settings motion maths', () => {
	it('flip maps the page box onto the card box from the top-left corner', () => {
		const card = { left: 100, top: 200, width: 300, height: 150 };
		const page = { left: 0, top: 56, width: 1200, height: 600 };
		expect(flip(card, page)).toBe('translate(100px, 144px) scale(0.25, 0.25)');
		expect(flip(page, page)).toBe('translate(0px, 0px) scale(1, 1)');
		expect(shift(card, page)).toBe('translate(100px, 144px)');
	});
	it('never divides by zero', () => {
		expect(flip({ left: 0, top: 0, width: 10, height: 10 }, { left: 0, top: 0, width: 0, height: 0 })).toContain('scale(1, 1)');
	});
});
