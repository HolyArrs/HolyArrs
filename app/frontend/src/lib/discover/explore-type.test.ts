import { type ExploreCard, type ExploreRow } from '#lib/ui/title-cards.ts';
import { filterRows, parseType, typeHref, typeKindOf } from './explore-type.ts';
import { describe, expect, it } from 'vitest';

const card = (key: string, kind: ExploreCard['kind']): ExploreCard => ({ key, kind, name: key, badges: [] });
const rows: ExploreRow[] = [
	{ id: 'recent', title: 'Recently added', cards: [card('m1', 'movie'), card('s1', 'show'), card('a1', 'artist')] },
	{ id: 'trending', title: 'Trending', cards: [card('m2', 'movie'), card('s2', 'show')] },
	{ id: 'movies', title: 'Movies', cards: [card('m3', 'movie')] },
	{ id: 'shows', title: 'TV shows', cards: [card('s3', 'show')] },
	{ id: 'music', title: 'Music', cards: [card('a3', 'artist')] }
];

describe('filterRows', () => {
	it('keeps everything for All', () => {
		expect(filterRows(rows, 'all')).toBe(rows);
	});
	it('keeps one kind of card and only its own kind shelf', () => {
		const got = filterRows(rows, 'movies');
		expect(got.map((r) => r.id)).toEqual(['recent', 'trending', 'movies']);
		expect(got.flatMap((r) => r.cards.map((c) => c.kind))).toEqual(['movie', 'movie', 'movie']);
	});
	it('hides rows left empty', () => {
		expect(filterRows(rows, 'music').map((r) => r.id)).toEqual(['recent', 'music']);
	});
	it('never repeats a row and does not change the input', () => {
		const ids = filterRows(rows, 'shows').map((r) => r.id);
		expect(new Set(ids).size).toBe(ids.length);
		expect(rows[0].cards).toHaveLength(3);
	});
});

describe('parseType', () => {
	it('reads known types and falls back to All', () => {
		expect(parseType('movies')).toBe('movies');
		expect(parseType('shows')).toBe('shows');
		expect(parseType(null)).toBe('all');
		expect(parseType('films')).toBe('all');
	});
	it('treats music as All while the music part is off', () => {
		expect(parseType('music', true)).toBe('music');
		expect(parseType('music', false)).toBe('all');
	});
	it('maps types to card kinds', () => {
		expect([typeKindOf('all'), typeKindOf('movies'), typeKindOf('shows'), typeKindOf('music')]).toEqual([null, 'movie', 'show', 'artist']);
	});
});

describe('typeHref', () => {
	it('leaves links alone for All', () => {
		expect(typeHref('/library', 'all')).toBe('/library');
		expect(typeHref(undefined, 'movies')).toBeUndefined();
	});
	it('sets the Library kind', () => {
		expect(typeHref('/library', 'music')).toBe('/library?kind=artist');
		expect(typeHref('/base/library?show=missing', 'shows')).toBe('/base/library?show=missing&kind=show');
	});
	it('keeps a Library link that already names its kind', () => {
		expect(typeHref('/library?kind=show&show=missing', 'shows')).toBe('/library?kind=show&show=missing');
	});
	it('adds type= to other pages', () => {
		expect(typeHref('/calendar', 'movies')).toBe('/calendar?type=movies');
		expect(typeHref('/requests', 'shows')).toBe('/requests?type=shows');
	});
});
