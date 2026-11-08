import { describe, expect, it } from 'vitest';
import { cardTmdb, followsByTmdb } from './title-cards.ts';

const card = (c: Record<string, unknown>) => c as Parameters<typeof followsByTmdb>[0];
const year = new Date().getFullYear();

describe('followsByTmdb', () => {
	it('follows a show by TMDB id until it is in the library', () => {
		expect(followsByTmdb(card({ kind: 'show' }))).toBe(true);
		expect(followsByTmdb(card({ kind: 'show', titleId: 5 }))).toBe(false);
	});
	it('follows an unreleased movie by TMDB id', () => {
		expect(followsByTmdb(card({ kind: 'movie', year: year + 1 }))).toBe(true);
		expect(followsByTmdb(card({ kind: 'movie', titleId: 5, year: year + 1 }))).toBe(false);
		expect(followsByTmdb(card({ kind: 'movie', year: year - 1 }))).toBe(false);
		expect(followsByTmdb(card({ kind: 'movie', year }))).toBe(false);
		expect(followsByTmdb(card({ kind: 'movie' }))).toBe(false);
	});
	it('follows a coming card by TMDB id whatever its year', () => {
		expect(followsByTmdb(card({ kind: 'movie', fresh: { kind: 'coming' } }))).toBe(true);
		expect(followsByTmdb(card({ kind: 'movie', fresh: { kind: 'new' } }))).toBe(false);
	});
});

describe('cardTmdb', () => {
	it('reads the id from a TMDB ref of a movie or show', () => {
		expect(cardTmdb({ kind: 'movie', ref: 'tmdb:603' })).toBe('603');
		expect(cardTmdb({ kind: 'show', ref: 'tmdb:1396' })).toBe('1396');
	});
	it('gives nothing for other refs or kinds', () => {
		expect(cardTmdb({ kind: 'artist', ref: 'tmdb:1' })).toBe('');
		expect(cardTmdb({ kind: 'show', ref: 'mb:1' })).toBe('');
		expect(cardTmdb({ kind: 'movie' })).toBe('');
		expect(cardTmdb({ kind: 'movie', ref: '' })).toBe('');
	});
});
