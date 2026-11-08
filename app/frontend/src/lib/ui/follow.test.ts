import { describe, expect, it } from 'vitest';
import { followHint, followLabel, followPath, setFollow } from './follow.ts';

describe('follow', () => {
	it('labels the bell', () => {
		expect(followLabel(false)).toBe('Follow');
		expect(followLabel(true)).toBe('Following');
	});

	it('says what following tells you', () => {
		expect(followHint('show', false)).toBe('Follow to see it on your calendar and hear about new episodes and seasons. Nothing is asked for.');
		expect(followHint('artist', false)).toBe('Follow to see it on your calendar and hear about new albums. Nothing is asked for.');
		expect(followHint('show', true)).toBe('On your calendar; you hear about new episodes and seasons. Tap to stop.');
		expect(followHint('movie', false)).toBe('Follow to see its dates on your calendar. Nothing is asked for.');
		expect(followHint('movie', false, true)).toBe('Follow to see it on your calendar and hear when it is in the Library. Nothing is asked for.');
		expect(followHint('movie', true)).toBe('On your calendar; you hear about it. Tap to stop.');
	});

	it('follows with PUT and unfollows with DELETE', async () => {
		const calls: string[] = [];
		const client = {
			put: async <T>(p: string) => {
				calls.push('PUT ' + p);
				return { title_id: 7, following: true } as T;
			},
			del: async <T>(p: string) => {
				calls.push('DELETE ' + p);
				return { title_id: 7, following: false } as T;
			}
		};
		expect(await setFollow(client, 7, true)).toBe(true);
		expect(await setFollow(client, 7, false)).toBe(false);
		expect(calls).toEqual(['PUT /titles/7/follow', 'DELETE /titles/7/follow']);
	});

	it('follows a show not in the Library by its TMDB id', () => {
		expect(followPath({ tmdb: '203857' })).toBe('/metadata/show/203857/follow');
		expect(followPath({ titleId: 3 })).toBe('/titles/3/follow');
		expect(followPath({ tmdb: '1061474', kind: 'movie' })).toBe('/metadata/movie/1061474/follow');
		expect(followHint('show', false, true)).toBe('Follow to see it on your calendar and hear about it when it lands and about new seasons. Nothing is asked for.');
	});
});
