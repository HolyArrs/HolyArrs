import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const post = vi.fn();
const get = vi.fn();
vi.mock('#lib/core/api.ts', () => ({ api: { post: (...a: unknown[]) => post(...a), get: (...a: unknown[]) => get(...a), del: vi.fn() } }));

const { lang, langPack } = await import('./i18n.svelte.ts');
const { packs, fetchMissing, answerOffer, installLanguage } = await import('./language-packs.svelte.ts');

describe('language packs', () => {
	const kept = new Map<string, string>();
	beforeEach(() => {
		kept.clear();
		post.mockReset().mockResolvedValue({});
		get.mockReset();
		vi.stubGlobal('localStorage', { getItem: (k: string) => kept.get(k) ?? null, setItem: (k: string, v: string) => void kept.set(k, v), removeItem: (k: string) => void kept.delete(k) });
		// The pack stays missing after the install in these tests: no server answers the pack itself.
		vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 404 })));
		packs.offer = packs.failed = packs.busy = null;
	});
	afterEach(() => vi.unstubAllGlobals());

	it('installs a language someone picked without asking', async () => {
		lang.choice = 'de';
		langPack.missing = 'de';
		await fetchMissing();
		expect(post).toHaveBeenCalledWith('/languages/de');
		expect(get).not.toHaveBeenCalled();
		expect(packs.offer).toBeNull();
	});

	it('installs the browser language without asking on an install that had it built in', async () => {
		lang.choice = 'auto';
		langPack.missing = 'fr';
		get.mockResolvedValue({ languages: [], auto_browser: true });
		await fetchMissing();
		expect(post).toHaveBeenCalledWith('/languages/fr');
	});

	it('asks once on a new install, and keeps English when told to', async () => {
		lang.choice = 'auto';
		langPack.missing = 'ja';
		get.mockResolvedValue({ languages: [], auto_browser: false });
		await fetchMissing();
		expect(post).not.toHaveBeenCalled();
		expect(packs.offer).toBe('ja');
		await answerOffer(false);
		expect(packs.offer).toBeNull();
		expect(post).not.toHaveBeenCalled();
		await fetchMissing();
		expect(packs.offer).toBeNull();
	});

	it('installs when the offer is taken', async () => {
		lang.choice = 'auto';
		langPack.missing = 'ko';
		get.mockResolvedValue({ languages: [], auto_browser: false });
		await fetchMissing();
		await answerOffer(true);
		expect(post).toHaveBeenCalledWith('/languages/ko');
	});

	it('marks a language that could not be downloaded, until a try works', async () => {
		post.mockRejectedValueOnce(new Error('offline'));
		expect(await installLanguage('it')).toBe(false);
		expect(packs.failed).toBe('it');
		expect(await installLanguage('it')).toBe(true);
		expect(packs.failed).toBeNull();
	});

	it('tries a failed download once per page load, not again each time it ends', async () => {
		lang.choice = 'pl';
		langPack.missing = 'pl';
		post.mockRejectedValue(new Error('offline'));
		await fetchMissing();
		await fetchMissing();
		await fetchMissing();
		expect(post).toHaveBeenCalledTimes(1);
		expect(packs.failed).toBe('pl');
		// Try again still works.
		await installLanguage('pl');
		expect(post).toHaveBeenCalledTimes(2);
	});
});
