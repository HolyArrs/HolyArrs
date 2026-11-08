import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../api.ts';

const get = vi.fn();
vi.mock('../api.ts', async (orig) => ({ ...(await orig<typeof import('../api.ts')>()), api: { get: (...a: unknown[]) => get(...a), post: vi.fn(), warm: vi.fn(), onSignedOut: vi.fn(), setCsrf: vi.fn() } }));

describe('trailer lookups', () => {
	beforeEach(() => get.mockReset());
	it('asks once for a title with no trailer (404), however many players ask', async () => {
		get.mockImplementation((p: string) => (String(p).startsWith('/trailer/') ? Promise.reject(new ApiError('none', 404)) : Promise.resolve({})));
		const { trailerSources } = await import('./autoplay.svelte.ts');
		await trailerSources('movie', '101', undefined);
		await trailerSources('movie', '101', undefined);
		expect(get.mock.calls.filter((c) => String(c[0]).startsWith('/trailer/movie/101'))).toHaveLength(1);
	});
	it('embeds the key a 404 names, and nothing when no trailer plays here', async () => {
		get.mockImplementation((p: string) => {
			const body = String(p).startsWith('/trailer/movie/303') ? { error: 'no direct stream', reason: 'age_restricted', key: 'nextKey0001' } : { error: 'no direct stream', reason: 'age_restricted', key: '' };
			return String(p).startsWith('/trailer/') ? Promise.reject(new ApiError('no direct stream', 404, body)) : Promise.resolve({});
		});
		const { missReason, trailerSources } = await import('./autoplay.svelte.ts');
		const videos = [{ key: 'ownPick0001', type: 'Trailer', official: true }];
		// The reason rides on the embed, for the line shown if YouTube's player refuses it too.
		expect(await trailerSources('movie', '303', videos)).toEqual([{ kind: 'embed', key: 'nextKey0001', why: 'age_restricted' }]);
		expect(await trailerSources('movie', '304', videos)).toEqual([]);
		expect(missReason('movie', '304')).toBe('age_restricted');
		expect(missReason('movie', '999')).toBe('');
		// Remembered: a second player neither asks again nor embeds.
		expect(await trailerSources('movie', '304', videos)).toEqual([]);
		expect(get.mock.calls.filter((c) => String(c[0]).startsWith('/trailer/movie/304'))).toHaveLength(1);
	});
	it('embeds the key that comes alone with a title not looked up yet, and its own pick when the key is empty', async () => {
		get.mockImplementation((p: string) => {
			const body = String(p).startsWith('/trailer/movie/404') ? { error: 'no direct stream', key: 'keptKey0001' } : { error: 'no direct stream', key: '' };
			return String(p).startsWith('/trailer/') ? Promise.reject(new ApiError('no direct stream', 404, body)) : Promise.resolve({});
		});
		const { missReason, trailerSources } = await import('./autoplay.svelte.ts');
		expect(await trailerSources('movie', '404', undefined)).toStrictEqual([{ kind: 'embed', key: 'keptKey0001' }]);
		const videos = [{ key: 'ownPick0001', type: 'Trailer', official: true }];
		expect(await trailerSources('movie', '405', videos)).toStrictEqual([{ kind: 'embed', key: 'ownPick0001' }]);
		// Not looked up yet: no reason to show.
		expect(missReason('movie', '404')).toBe('');
	});
	it('asks again after a failure that is not a 404', async () => {
		get.mockImplementation((p: string) => (String(p).startsWith('/trailer/') ? Promise.reject(new ApiError('down', 0)) : Promise.resolve({})));
		const { trailerSources } = await import('./autoplay.svelte.ts');
		await trailerSources('movie', '202', undefined);
		await new Promise((r) => setTimeout(r, 0));
		await trailerSources('movie', '202', undefined);
		expect(get.mock.calls.filter((c) => String(c[0]).startsWith('/trailer/movie/202'))).toHaveLength(2);
	});
});
