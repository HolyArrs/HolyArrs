import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { pluralKey } from '#lib/core/i18n/i18n.svelte.ts';

describe('pluralKey', () => {
	it('picks the form the language uses for the number', () => {
		expect(pluralKey('x', 1, 'en')).toBe('x.one');
		expect(pluralKey('x', 3, 'en')).toBe('x.other');
		expect(pluralKey('x', 3, 'ru')).toBe('x.few');
		expect(pluralKey('x', 5, 'ru')).toBe('x.many');
		expect(pluralKey('x', 2, 'ar')).toBe('x.two');
		expect(pluralKey('x', 3, 'ja')).toBe('x.other');
	});
});

describe('the count heading', () => {
	const dir = 'src/lib/core/i18n/messages';
	const forms = ['zero', 'one', 'two', 'few', 'many', 'other'];
	it('has every plural form, each with the number, in all 44 languages', () => {
		const langs = readdirSync(dir);
		expect(langs.length).toBe(44);
		for (const l of langs) {
			const m = JSON.parse(readFileSync(`${dir}/${l}/alerts.json`, 'utf8'));
			for (const f of forms) expect(m[`alerts.dot.count.${f}`], `${l} ${f}`).toContain('{n}');
		}
	});
});
