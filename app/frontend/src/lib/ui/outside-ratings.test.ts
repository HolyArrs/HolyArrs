import { describe, expect, it } from 'vitest';
import { cleanExtras, loadExtras, scoreText } from './outside-ratings.ts';

describe('outside ratings', () => {
	it('keeps artwork cache pictures and known scores in strip order', () => {
		const e = cleanExtras({
			logo: '/api/v1/artwork/img/title-logo/movie-603/1200.png',
			thumb: 'https://evil.test/t.jpg',
			backgrounds: ['/api/v1/artwork/img/title-wide/movie-603/1600.webp?v=a1', 'https://assets.fanart.tv/x.jpg', '/api/v1/artwork/img/../x.jpg'],
			scores: [
				{ source: 'metacritic', value: 73, max: 100, votes: 40 },
				{ source: 'imdb', value: 8.7, max: 10, votes: 2000000 },
				{ source: 'mystery', value: 5, max: 10, votes: 9 },
				{ source: 'rt_critics', value: 120, max: 100, votes: 50 }
			]
		});
		expect(e.logo).toBe('/api/v1/artwork/img/title-logo/movie-603/1200.png');
		expect(e.thumb).toBeUndefined();
		expect(e.backgrounds).toEqual(['/api/v1/artwork/img/title-wide/movie-603/1600.webp?v=a1']);
		expect(e.scores.map((s) => s.source)).toEqual(['imdb', 'metacritic']);
	});
	it('reads each score its own way', () => {
		expect(scoreText({ source: 'imdb', value: 8.66, max: 10, votes: 1 })).toBe('8.7');
		expect(scoreText({ source: 'rt_critics', value: 83, max: 100, votes: 1 })).toBe('83%');
		expect(scoreText({ source: 'metacritic', value: 73, max: 100, votes: 1 })).toBe('73');
		expect(scoreText({ source: 'letterboxd', value: 4.2, max: 5, votes: 1 })).toBe('4.2');
	});
	it('answers empty when there is nothing', async () => {
		expect(cleanExtras(null)).toEqual({ backgrounds: [], scores: [] });
		const e = await loadExtras(() => Promise.reject(new Error('down')), 'movie', '1');
		expect(e.scores).toEqual([]);
		expect((await loadExtras(() => Promise.reject(new Error('x')), 'movie', null)).scores).toEqual([]);
	});
});
