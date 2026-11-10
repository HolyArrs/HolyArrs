import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserLocale } from './i18n.svelte.ts';

const here = fileURLToPath(new URL('.', import.meta.url));
const src = join(here, '..', '..', '..');
const messages = join(here, 'messages');

type Words = Record<string, string>;
const read = (dir: string): Words =>
	Object.assign({}, ...readdirSync(dir).filter((f) => f.endsWith('.json')).map((f) => JSON.parse(readFileSync(join(dir, f), 'utf8')) as Words));
const en = read(join(messages, 'en'));

function files(dir: string, out: string[] = []): string[] {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) files(p, out);
		else if (/\.(svelte|ts)$/.test(name) && !name.endsWith('.test.ts')) out.push(p);
	}
	return out;
}

const placeholders = (line: string) => [...line.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
const markers = (line: string) => [...line.matchAll(/<\/?(b|strong|em|code|mono|a\d?)>/g)].map((m) => m[0]).sort();

describe('message keys', () => {
	it('every literal key in the code has an English line', () => {
		const missing: string[] = [];
		for (const f of files(src)) {
			const text = readFileSync(f, 'utf8');
			for (const m of text.matchAll(/\b(?:t|say|tr)\('([a-z][\w-]*(?:\.[\w-]+)+)'\s*[,)]/g)) if (!(m[1] in en)) missing.push(`${m[1]} (${f.slice(src.length + 1)})`);
			for (const m of text.matchAll(/<Words\s+key="([^"]+)"/g)) if (!(m[1] in en)) missing.push(`${m[1]} (${f.slice(src.length + 1)})`);
		}
		expect(missing).toEqual([]);
	});

	it('other languages only use English keys, with the same placeholders and markers', () => {
		const wrong: string[] = [];
		for (const l of readdirSync(messages).filter((d) => d !== 'en')) {
			const words = read(join(messages, l));
			for (const [k, line] of Object.entries(words)) {
				if (!(k in en)) wrong.push(`${l}: ${k} is not an English key`);
				else if (placeholders(line).join() !== placeholders(en[k]).join()) wrong.push(`${l}: ${k} placeholders differ`);
				else if (markers(line).join() !== markers(en[k]).join()) wrong.push(`${l}: ${k} markers differ`);
			}
		}
		expect(wrong).toEqual([]);
	});
});

describe('browserLocale', () => {
	it('maps browser languages to the closest one the app has', () => {
		expect(browserLocale(['pt-PT'])).toBe('pt-br');
		expect(browserLocale(['zh-TW'])).toBe('zh-cn');
		expect(browserLocale(['ar-EG'])).toBe('ar');
		expect(browserLocale(['uk-UA'])).toBe('uk');
		expect(browserLocale(['it-IT'])).toBe('it');
		expect(browserLocale(['nl-BE'])).toBe('nl');
		expect(browserLocale(['pl-PL'])).toBe('pl');
		expect(browserLocale(['tr-TR'])).toBe('tr');
		expect(browserLocale(['xx'])).toBe('en');
		expect(browserLocale(['xx', 'de-AT'])).toBe('de');
	});
});
