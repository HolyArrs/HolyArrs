import { afterEach, describe, expect, it, vi } from 'vitest';
import { appLanguage, setLang } from './i18n.svelte.ts';

const storage = (saved: string | null) => ({ getItem: () => saved, setItem() {}, removeItem() {} });

describe('appLanguage', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('is the saved language, else the browser one, before the app starts', () => {
		vi.stubGlobal('navigator', { languages: ['de-DE'], language: 'de-DE' });
		vi.stubGlobal('localStorage', storage('pt-br'));
		expect(appLanguage()).toBe('pt-br');
		vi.stubGlobal('localStorage', storage(null));
		expect(appLanguage()).toBe('de');
	});

	it('is the language picked last, while its words still load', () => {
		vi.stubGlobal('navigator', { languages: ['de-DE'], language: 'de-DE' });
		vi.stubGlobal('localStorage', storage(null));
		vi.stubGlobal('fetch', () => new Promise(() => {}));
		void setLang('es');
		expect(appLanguage()).toBe('es');
		void setLang('auto');
		expect(appLanguage()).toBe('de');
	});
});
