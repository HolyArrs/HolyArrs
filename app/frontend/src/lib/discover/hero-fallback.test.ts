import { describe, expect, it } from 'vitest';
import { heroFallback } from './hero-fallback.ts';
import type { ExploreCard, ExploreRow } from '../ui/title-cards.ts';

const card = (key: string, kind: ExploreCard['kind'], image: string | null): ExploreCard => ({ key, kind, name: key, image, badges: [] });
const row = (id: string, cards: ExploreCard[]): ExploreRow => ({ id, title: id, cards });

describe('heroFallback', () => {
	it('takes the first movies and shows that have a picture', () => {
		const got = heroFallback([row('a', [card('m1', 'movie', null), card('p1', 'artist', 'x.jpg'), card('s1', 'show', 's.jpg')]), row('b', [card('s1', 'show', 's.jpg'), card('m2', 'movie', 'm.jpg')])]);
		expect(got.map((c) => c.key)).toEqual(['s1', 'm2']);
	});
	it('stops at max and is empty with no rows', () => {
		expect(heroFallback([row('a', [card('1', 'movie', 'a'), card('2', 'movie', 'b')])], 1)).toHaveLength(1);
		expect(heroFallback([])).toEqual([]);
	});
});
