import { afterEach, describe, expect, it } from 'vitest';
import { sized } from './tmdb-image.ts';
import { setBase } from './paths.ts';

describe('sized', () => {
	afterEach(() => setBase(''));
	it('asks the artwork cache for a w342 poster', () => {
		expect(sized('https://image.tmdb.org/t/p/original/a.jpg', 'poster')).toBe('/api/v1/artwork/tmdb/w342/a.jpg');
		expect(sized('https://image.tmdb.org/t/p/w500/a.jpg', 'poster')).toBe('/api/v1/artwork/tmdb/w342/a.jpg');
	});
	it('asks for a w780 backdrop', () => {
		expect(sized('https://image.tmdb.org/t/p/w1280/b.png', 'backdrop')).toBe('/api/v1/artwork/tmdb/w780/b.png');
	});
	it('keeps the URL base', () => {
		setBase('/holyarrs');
		expect(sized('https://image.tmdb.org/t/p/w500/a.jpg', 'poster')).toBe('/holyarrs/api/v1/artwork/tmdb/w342/a.jpg');
	});
	it('leaves other pictures alone', () => {
		expect(sized('/api/v1/artwork/x.jpg', 'poster')).toBe('/api/v1/artwork/x.jpg');
		expect(sized('https://image.tmdb.org/t/p/w500/../x.jpg', 'poster')).toBe('https://image.tmdb.org/t/p/w500/../x.jpg');
		expect(sized(null, 'poster')).toBe('');
	});
});
