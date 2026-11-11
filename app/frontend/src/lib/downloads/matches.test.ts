import { bannerWords, candidateLine } from './matches.ts';
import { describe, expect, it } from 'vitest';

describe('Confirm match words', () => {
	it('counts the downloads waiting, torrent or Usenet', () => {
		expect(bannerWords(0)).toBe('');
		expect(bannerWords(1)).toBe('1 download needs a match');
		expect(bannerWords(4)).toBe('4 downloads need a match');
	});
	it('names a choice by kind and year', () => {
		expect(candidateLine({ kind: 'movie', year: 2023 })).toBe('Movie · 2023');
		expect(candidateLine({ kind: 'show' })).toBe('TV show');
	});
});
