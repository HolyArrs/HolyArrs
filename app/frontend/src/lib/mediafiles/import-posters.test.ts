import { describe, expect, it } from 'vitest';
import { pickedPoster, type AdoptCandidate } from './index.ts';

const f = (over: Partial<AdoptCandidate> = {}): AdoptCandidate => ({
	folder: 'Heat (1995)', parsed_name: 'Heat', videos: [], others: [], status: 'matched', why: '',
	match: { tmdb_id: '949', name: 'Heat', year: 1995, score: 1, poster: 'https://image.tmdb.org/t/p/w500/h.jpg' },
	...over
});

describe('pickedPoster (#305)', () => {
	it('gives the poster of the picked match', () => {
		expect(pickedPoster(f(), '949')).toBe('https://image.tmdb.org/t/p/w500/h.jpg');
	});
	it('follows another pick among the others', () => {
		const c = f({ others: [{ tmdb_id: '1', name: 'Other', year: 2000, score: 0.5, poster: '/o.jpg' }] });
		expect(pickedPoster(c, '1')).toBe('/o.jpg');
	});
	it('gives nothing when nothing is picked or the pick has no poster', () => {
		expect(pickedPoster(f(), undefined)).toBe('');
		expect(pickedPoster(f({ match: { tmdb_id: '949', name: 'Heat', year: 1995, score: 1 } }), '949')).toBe('');
		expect(pickedPoster(f({ status: 'unmatched', match: undefined }), '949')).toBe('');
	});
});
