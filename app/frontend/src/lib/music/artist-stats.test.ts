import { changeWords, fullCount, listening } from './music-pages.ts';
import { describe, expect, it } from 'vitest';

describe('artist stats band helpers', () => {
	it('writes today’s change up or down, and nothing for zero', () => {
		expect(changeWords(387_000)).toBe('▲ 387K today');
		expect(changeWords(-12_400)).toBe('▼ 12.4K today');
		expect(changeWords(-12_000, '')).toBe('▼ 12K');
		expect(changeWords(0)).toBe('');
		expect(changeWords(undefined)).toBe('');
	});

	it('picks listeners first, then total plays, else nothing', () => {
		expect(listening({ monthly_listeners: 72_785_924, world_rank: 22, listeners_change_today: -5 })).toEqual({ n: 72_785_924, unit: 'monthly listeners', rank: 22, today: -5 });
		expect(listening({ total_plays: 412_903_551 })).toEqual({ n: 412_903_551, unit: 'total plays' });
		expect(listening({})).toBeNull();
		expect(listening(undefined)).toBeNull();
	});

	it('writes full counts and the catalog in words', () => {
		expect(fullCount(72_785_924)).toBe('72,785,924');
	});

});
