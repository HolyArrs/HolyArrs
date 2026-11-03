import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './api.ts';

const get = vi.fn();
vi.mock('./api.ts', async (orig) => ({ ...(await orig<typeof import('./api.ts')>()), api: { get: (...a: unknown[]) => get(...a), post: vi.fn(), warm: vi.fn(), onSignedOut: vi.fn(), setCsrf: vi.fn() } }));

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
		const { trailerSources } = await import('./autoplay.svelte.ts');
		const videos = [{ key: 'ownPick0001', type: 'Trailer', official: true }];
		expect(await trailerSources('movie', '303', videos)).toEqual([{ kind: 'embed', key: 'nextKey0001' }]);
		expect(await trailerSources('movie', '304', videos)).toEqual([]);
		// Remembered: a second player neither asks again nor embeds.
		expect(await trailerSources('movie', '304', videos)).toEqual([]);
		expect(get.mock.calls.filter((c) => String(c[0]).startsWith('/trailer/movie/304'))).toHaveLength(1);
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
