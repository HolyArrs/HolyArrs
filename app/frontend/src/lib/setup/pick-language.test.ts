import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';

// The language store is unreachable: every call fails, as when the server is offline or the pack list is missing.
vi.mock('#lib/core/api.ts', async (orig) => {
	const real = await orig<typeof import('#lib/core/api.ts')>();
	const down = () => Promise.reject(new Error('offline'));
	return { ...real, api: { ...real.api, get: vi.fn(down), post: vi.fn(down), put: vi.fn(down) } };
});

import PickLanguage, { installPack, packState } from './PickLanguage.svelte';
import { listInstalledPublic, packList } from '#lib/core/i18n/language-packs.svelte.ts';

describe('setup language step without the language store', () => {
	beforeEach(() => {
		for (const k of Object.keys(packState)) if (k !== 'en') delete packState[k];
		packList.installed = null;
	});

	it('marks the pick as failed instead of waiting forever', async () => {
		expect(await installPack('de')).toBe(false);
		expect(packState.de).toBe('failed');
	});

	it('a second ask waits for the same download', async () => {
		const a = installPack('fr');
		const b = installPack('fr');
		expect(b).toBe(a);
		expect(await b).toBe(false);
	});

	it('keeps English working with no list', async () => {
		await listInstalledPublic();
		expect(packList.installed).toBeNull();
		expect(await installPack('en')).toBe(true);
	});

	it('never disables Continue', () => {
		const { body } = render(PickLanguage, { props: { ondone: () => {} } });
		expect(body).toContain('data-testid="setup-lang-go"');
		expect(body).not.toMatch(/data-testid="setup-lang-go"[^>]*disabled|disabled[^>]*data-testid="setup-lang-go"/);
	});
});
