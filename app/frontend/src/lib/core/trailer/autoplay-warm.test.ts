import { afterEach, describe, expect, it, vi } from 'vitest';

// A hero only looks the trailer up; a hover card also warms its first bytes
// in a hidden video, which its player takes over.
const get = vi.fn();
vi.mock('../api.ts', async (orig) => ({ ...(await orig<typeof import('../api.ts')>()), api: { get: (...a: unknown[]) => get(...a), post: vi.fn(), warm: vi.fn(), onSignedOut: vi.fn(), setCsrf: vi.fn() } }));
vi.mock('./trailer.ts', async (orig) => ({ ...(await orig<typeof import('./trailer.ts')>()), mayAutoplay: () => true, browserConditions: () => ({}) }));

const flush = () => new Promise((r) => setTimeout(r, 0));
function fakeVideo() {
	return { muted: false, playsInline: false, loop: false, preload: '', style: {}, src: '', setAttribute: vi.fn(), getAttribute: vi.fn(), removeAttribute: vi.fn(), addEventListener: vi.fn(), play: () => Promise.resolve(), pause: vi.fn(), remove: vi.fn(), load: vi.fn() };
}

describe('prefetchTrailer', () => {
	afterEach(() => vi.unstubAllGlobals());
	it('looks the trailer up for a hero without warming a hidden video, and warms one for a card', async () => {
		get.mockImplementation((p: string) =>
			String(p).startsWith('/trailer/')
				? Promise.resolve({ type: 'mp4', url: `https://x/${p.split('/').pop()}.mp4`, audio: true, key: 'k', expires_at: new Date(Date.now() + 3600e3).toISOString() })
				: Promise.resolve({ autoplay: true })
		);
		const createElement = vi.fn(() => fakeVideo());
		const appendChild = vi.fn();
		vi.stubGlobal('document', { createElement, body: { appendChild } });
		const { prefetchTrailer } = await import('./autoplay.svelte.ts');

		prefetchTrailer('movie', '1', false);
		for (let i = 0; i < 5; i++) await flush();
		expect(get.mock.calls.filter((c) => String(c[0]).startsWith('/trailer/movie/1'))).toHaveLength(1);
		expect(createElement).not.toHaveBeenCalled();

		prefetchTrailer('movie', '2');
		for (let i = 0; i < 5; i++) await flush();
		expect(createElement).toHaveBeenCalledWith('video');
		expect(appendChild).toHaveBeenCalledTimes(1);
	});
});
