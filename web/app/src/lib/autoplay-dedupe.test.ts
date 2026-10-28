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
	it('asks again after a failure that is not a 404', async () => {
		get.mockImplementation((p: string) => (String(p).startsWith('/trailer/') ? Promise.reject(new ApiError('down', 0)) : Promise.resolve({})));
		const { trailerSources } = await import('./autoplay.svelte.ts');
		await trailerSources('movie', '202', undefined);
		await new Promise((r) => setTimeout(r, 0));
		await trailerSources('movie', '202', undefined);
		expect(get.mock.calls.filter((c) => String(c[0]).startsWith('/trailer/movie/202'))).toHaveLength(2);
	});
});
