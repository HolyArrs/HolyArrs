import { beforeEach, describe, expect, it, vi } from 'vitest';

const calls: string[] = [];
vi.mock('../core/api.ts', () => ({
	api: {
		get: vi.fn(async () => ({ display_name: 'Ana', language: 'es' })),
		put: vi.fn(async () => {
			calls.push('put');
			return {};
		}),
		forget: vi.fn(() => calls.push('forget'))
	}
}));
vi.mock('../core/i18n/i18n.svelte.ts', () => ({
	lang: { choice: 'auto' },
	isLocaleChoice: () => true,
	setLang: vi.fn(async () => {
		calls.push('setLang');
	})
}));

const { loadProfile, saveLanguage, forgetProfile } = await import('./profile.svelte.ts');

describe('language switch drops answers kept in the old language', () => {
	beforeEach(() => {
		calls.length = 0;
		forgetProfile();
		try {
			sessionStorage.clear();
			localStorage.clear();
		} catch {
			/* no storage */
		}
	});

	it('forgets kept answers when the account language replaces the browser one', async () => {
		await loadProfile();
		await Promise.resolve();
		expect(calls).toContain('setLang');
		expect(calls.indexOf('forget')).toBeGreaterThan(calls.indexOf('setLang'));
	});

	it('forgets kept answers when a person picks a language', async () => {
		await saveLanguage('de');
		expect(calls.slice(0, 2)).toEqual(['setLang', 'forget']);
	});
});
