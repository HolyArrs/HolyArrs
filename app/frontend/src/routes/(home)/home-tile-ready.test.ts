import { describe, expect, it } from 'vitest';
import { tileWord } from './home.ts';

describe('tileWord', () => {
	it('says Getting ready while the first list loads and no picture is in', () => {
		expect(tileWord(true, false, "What's trending")).toBe('Getting ready…');
	});
	it('keeps its own words once a picture is in or the list is done', () => {
		expect(tileWord(true, true, "What's trending")).toBe("What's trending");
		expect(tileWord(false, false, "What's trending")).toBe("What's trending");
	});
});

describe('tilesSettled', () => {
	it('stops Getting ready once asking stops, even with no answer or a last building answer', async () => {
		const { tilesSettled } = await import('./home.ts');
		const art = { pop: [], soon: [], week: { count: 0, posters: [] }, building: true };
		const out = tilesSettled(art);
		expect(out.building).toBe(false);
		expect(tileWord(out.building, false, "What's next")).toBe("What's next");
	});
});
