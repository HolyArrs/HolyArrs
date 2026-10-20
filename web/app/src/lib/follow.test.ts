import { describe, expect, it } from 'vitest';
import { followHint, followLabel, setFollow } from './follow.ts';

describe('follow', () => {
	it('labels the bell', () => {
		expect(followLabel(false)).toBe('Follow');
		expect(followLabel(true)).toBe('Following');
	});

	it('says what following tells you', () => {
		expect(followHint('show', false)).toBe('Follow to hear about new episodes and seasons. Nothing is asked for.');
		expect(followHint('artist', false)).toBe('Follow to hear about new albums. Nothing is asked for.');
		expect(followHint('show', true)).toBe('You hear about new episodes and seasons. Tap to stop.');
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
});
