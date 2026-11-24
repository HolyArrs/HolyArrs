import { describe, expect, it } from 'vitest';
import { englishName, installedCodes, languageAction, languageCard, sortLanguageRows, LANGUAGE_PACKS_VERSION } from './languages.ts';
import type { LanguageRow } from '#lib/core/i18n/language-packs.svelte.ts';

const rows: LanguageRow[] = [
	{ code: 'en', state: 'built_in' },
	{ code: 'es', state: 'available' },
	{ code: 'fr', state: 'failed', error: 'x' },
	{ code: 'de', state: 'installed', version: '1.2.1', size: 9000 },
	{ code: 'ja', state: 'installing' }
];

describe('language pack cards', () => {
	it('puts English, then installed packs, then the rest in the app order', () => {
		expect(sortLanguageRows(rows).map((r) => r.code)).toEqual(['en', 'de', 'es', 'fr', 'ja']);
	});
	it('names a language in English, regions included', () => {
		expect(englishName('de')).toBe('German');
		expect(englishName('pt-br')).toContain('Portuguese');
	});
	it('counts English and installed packs as installed for the picker', () => {
		expect([...installedCodes(rows)].sort()).toEqual(['de', 'en']);
	});
});

describe('language packs card and focus', () => {
	it('is one plugin card: counts packs and installed ones, English left out', () => {
		expect(languageCard(new Set(['en', 'de', 'fr']), '', 'all', 'all')).toEqual({ n: 43, installed: 2 });
		expect(languageCard(null, '', 'all', 'all')).toEqual({ n: 43, installed: 0 });
	});
	it('shows the card when the search names a language or language packs, on the All tab only', () => {
		expect(languageCard(null, 'GERM', 'all', 'all')).not.toBeNull();
		expect(languageCard(null, 'deu', 'all', 'all')).not.toBeNull();
		expect(languageCard(null, 'language', 'all', 'all', 'Language packs')).not.toBeNull();
		expect(languageCard(null, 'tailscale', 'all', 'all', 'Language packs')).toBeNull();
		expect(languageCard(null, '', 'cat:network', 'all')).toBeNull();
		expect(languageCard(null, '', 'all', 'updates')).toBeNull();
		expect(languageCard(null, '', 'all', 'installed')).toBeNull();
		expect(languageCard(new Set(['en', 'de']), '', 'all', 'installed')).not.toBeNull();
	});
	it('finds the button in the card of the language just changed', () => {
		const seen: string[] = [];
		const root = { querySelector: (sel: string) => (seen.push(sel), null) } as unknown as ParentNode;
		expect(languageAction(root, 'pt-br')).toBeNull();
		expect(seen[0]).toContain('[data-code="pt-br"]');
		expect(seen[0]).toContain('button');
	});
});

describe('Language Packs version', () => {
	it('is its own plugin version, never the app build the packs fit', () => {
		expect(LANGUAGE_PACKS_VERSION).toBe('1.0.0');
	});
});
