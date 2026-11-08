import { catalogParts, changeWords, fullCount, listening, smallReleasesWord } from './music-pages.ts';
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
		expect(catalogParts({ albums: 16, eps: 5, singles: 89, since: '2005', span: '2005–2026' })).toEqual(['16 albums', '5 EPs', '89 singles', 'since 2005']);
		expect(catalogParts({ albums: 1, eps: 0, singles: 1, since: '', span: '' })).toEqual(['1 album', '1 single']);
	});

	it('names singles and EPs by what there is', () => {
		expect(smallReleasesWord({ eps: 5, singles: 89 })).toBe('Singles & EPs');
		expect(smallReleasesWord({ eps: 0, singles: 2 })).toBe('Singles');
		expect(smallReleasesWord({ eps: 1, singles: 0 })).toBe('EP');
	});
});
