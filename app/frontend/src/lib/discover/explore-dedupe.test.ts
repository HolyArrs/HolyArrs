import { cardIdentity, type ExploreCard, type ExploreRow } from '#lib/ui/title-cards.ts';
import { dedupeTop } from './genres.ts';
import { describe, expect, it } from 'vitest';

const card = (row: string, id: number): ExploreCard => ({ key: `${row}-movie-${id}`, kind: 'movie', name: `T${id}`, ref: `tmdb:${id}`, badges: [] });
const row = (id: string, ids: number[]): ExploreRow => ({ id, title: id, cards: ids.map((n) => card(id, n)) });

describe('dedupeTop', () => {
	it('drops titles shown in the hero and rows above', () => {
		const out = dedupeTop([row('a', [1, 2, 3, 4, 5, 6, 7, 8]), row('b', [1, 9, 10, 11, 12, 13, 14, 15])], [cardIdentity(card('hero', 8))]);
		expect(out[0].cards.map((c) => c.name)).not.toContain('T8');
		expect(out[1].cards.map((c) => c.name)).not.toContain('T1');
	});
	it('keeps repeats rather than shrink a row under its minimum', () => {
		const out = dedupeTop([row('a', [1, 2, 3, 4, 5, 6]), row('b', [1, 2, 3, 4, 5, 9])]);
		expect(out[1].cards).toHaveLength(6);
	});
	it('leaves rows past the first n alone', () => {
		const rows = Array.from({ length: 10 }, (_, i) => row(`r${i}`, [1, 2, 3, 4, 5, 6, 7, 100 + i]));
		const out = dedupeTop(rows, [], 8, 1);
		expect(out[1].cards).toHaveLength(1);
		expect(out[9].cards).toHaveLength(8);
	});
});
