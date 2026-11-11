import { describe, expect, it } from 'vitest';
import { readFile, readdir } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserLocale } from './i18n.svelte.ts';

const here = fileURLToPath(new URL('.', import.meta.url));
const src = join(here, '..', '..', '..');
const messages = join(here, 'messages');

// Every message file and source file is read once, in parallel, before the tests
// start; the tests then only compare in memory. Reading them one by one inside the
// tests took seconds under a full parallel run and hit the 5 s test limit.
type Words = Record<string, string>;
async function readWords(dir: string): Promise<Words> {
	const names = (await readdir(dir)).filter((f) => f.endsWith('.json'));
	return Object.assign({}, ...(await Promise.all(names.map(async (f) => JSON.parse(await readFile(join(dir, f), 'utf8')) as Words))));
}
const langs = new Map(await Promise.all((await readdir(messages)).map(async (l) => [l, await readWords(join(messages, l))] as const)));
const en = langs.get('en') ?? {};

async function sourceFiles(dir: string): Promise<string[]> {
	const found = await Promise.all(
		(await readdir(dir, { withFileTypes: true })).map(async (e) => {
			const p = join(dir, e.name);
			if (e.isDirectory()) return sourceFiles(p);
			return /\.(svelte|ts)$/.test(e.name) && !e.name.endsWith('.test.ts') ? [p] : [];
		})
	);
	return found.flat();
}
const sources = await Promise.all((await sourceFiles(src)).map(async (f) => ({ f, text: await readFile(f, 'utf8') })));

// A line without "{" or "<" can hold no placeholder or marker, so most lines skip the regex.
const placeholders = (line: string) => (line.includes('{') ? [...line.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join() : '');
const markers = (line: string) => (line.includes('<') ? [...line.matchAll(/<\/?(b|strong|em|code|mono|a\d?)>/g)].map((m) => m[0]).sort().join() : '');
const shape = new Map(Object.entries(en).map(([k, line]) => [k, { holes: placeholders(line), marks: markers(line) }]));

describe('message keys', () => {
	it('every literal key in the code has an English line', () => {
		expect(Object.keys(en).length).toBeGreaterThan(0);
		const missing: string[] = [];
		for (const { f, text } of sources) {
			for (const m of text.matchAll(/\b(?:t|say|tr)\('([a-z][\w-]*(?:\.[\w-]+)+)'\s*[,)]/g)) if (!(m[1] in en)) missing.push(`${m[1]} (${f.slice(src.length + 1)})`);
			for (const m of text.matchAll(/<Words\s+key="([^"]+)"/g)) if (!(m[1] in en)) missing.push(`${m[1]} (${f.slice(src.length + 1)})`);
		}
		expect(missing).toEqual([]);
	});

	it('other languages only use English keys, with the same placeholders and markers', () => {
		expect(langs.size).toBeGreaterThan(1);
		const wrong: string[] = [];
		for (const [l, words] of langs) {
			if (l === 'en') continue;
			for (const [k, line] of Object.entries(words)) {
				const want = shape.get(k);
				if (!want) wrong.push(`${l}: ${k} is not an English key`);
				else if (placeholders(line) !== want.holes) wrong.push(`${l}: ${k} placeholders differ`);
				else if (markers(line) !== want.marks) wrong.push(`${l}: ${k} markers differ`);
			}
		}
		expect(wrong).toEqual([]);
	});
});

describe('browserLocale', () => {
	it('maps browser languages to the closest one the app has', () => {
		expect(browserLocale(['pt-PT'])).toBe('pt-pt');
		expect(browserLocale(['pt-BR'])).toBe('pt-br');
		expect(browserLocale(['pt'])).toBe('pt-br');
		expect(browserLocale(['zh-TW'])).toBe('zh-tw');
		expect(browserLocale(['zh-HK'])).toBe('zh-tw');
		expect(browserLocale(['zh-Hant-TW'])).toBe('zh-tw');
		expect(browserLocale(['zh-CN'])).toBe('zh-cn');
		expect(browserLocale(['zh'])).toBe('zh-cn');
		expect(browserLocale(['no'])).toBe('nb');
		expect(browserLocale(['nn-NO'])).toBe('nb');
		expect(browserLocale(['nb-NO'])).toBe('nb');
		expect(browserLocale(['he-IL'])).toBe('he');
		expect(browserLocale(['iw'])).toBe('he');
		expect(browserLocale(['sv-SE'])).toBe('sv');
		expect(browserLocale(['th-TH'])).toBe('th');
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
