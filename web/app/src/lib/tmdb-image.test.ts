import { describe, expect, it } from 'vitest';
import { sized } from './tmdb-image.ts';

describe('sized', () => {
	it('asks TMDB for a w342 poster', () => {
		expect(sized('https://image.tmdb.org/t/p/original/a.jpg', 'poster')).toBe('https://image.tmdb.org/t/p/w342/a.jpg');
		expect(sized('https://image.tmdb.org/t/p/w500/a.jpg', 'poster')).toBe('https://image.tmdb.org/t/p/w342/a.jpg');
	});
	it('asks for a w780 backdrop', () => {
		expect(sized('https://image.tmdb.org/t/p/w1280/b.jpg', 'backdrop')).toBe('https://image.tmdb.org/t/p/w780/b.jpg');
	});
	it('leaves other pictures alone', () => {
		expect(sized('/api/v1/artwork/x.jpg', 'poster')).toBe('/api/v1/artwork/x.jpg');
		expect(sized(null, 'poster')).toBe('');
	});
});
