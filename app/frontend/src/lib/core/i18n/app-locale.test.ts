import { afterEach, describe, expect, it } from 'vitest';
import { appLocale, lang } from './i18n.svelte.ts';

describe('appLocale', () => {
	afterEach(() => {
		lang.value = 'en';
	});

	it('gives Thai dates the Gregorian year, not the Buddhist one', () => {
		lang.value = 'th' as typeof lang.value;
		const tag = appLocale();
		expect(tag).toBe('th-TH-u-ca-gregory');
		const shown = new Date(Date.UTC(2026, 10, 12)).toLocaleDateString(tag, { year: 'numeric', timeZone: 'UTC' });
		expect(shown).toContain('2026');
		expect(shown).not.toContain('2569');
	});

	it('leaves other languages as plain tags', () => {
		lang.value = 'de' as typeof lang.value;
		expect(appLocale()).toBe('de');
	});
});
