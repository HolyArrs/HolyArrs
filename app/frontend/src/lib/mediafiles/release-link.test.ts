import { readFileSync, readdirSync, statSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { releasePage } from './release-link.ts';

describe('releasePage', () => {
	it('keeps web links to the release page', () => {
		expect(releasePage('https://site.example/details/1')).toBe('https://site.example/details/1');
		expect(releasePage('http://site.example/t?id=2')).toBe('http://site.example/t?id=2');
	});

	it('drops anything that is not a web link', () => {
		for (const bad of [undefined, '', 'javascript:alert(1)', ' JavaScript:alert(1)', 'data:text/html,x', '//site.example/x', 'ftp://site.example/x', 'not a link']) {
			expect(releasePage(bad)).toBeUndefined();
		}
	});
});

describe('release name link in the release sheet', () => {
	const src = readFileSync(new URL('./ReleaseSheet.svelte', import.meta.url), 'utf8');
	const name = src.slice(src.indexOf('class="rs-name"'), src.indexOf('</p>', src.indexOf('class="rs-name"')));

	it('opens only checked web links in a new tab without handing over the page', () => {
		expect(src).toContain('releasePage(r.info_url)');
		expect(name).toMatch(/\{#if page\}<a [^>]*href=\{page\}/);
		expect(name).toContain('target="_blank"');
		expect(name).toContain('rel="noopener noreferrer"');
	});

	it('keeps the plain name when there is no page', () => {
		expect(name).toMatch(/\{:else\}\{r\.name\}\{\/if\}/);
	});

	it('names the link for screen readers in every language', () => {
		const dir = new URL('../core/i18n/messages/', import.meta.url);
		const langs = readdirSync(dir).filter((d) => statSync(new URL(d, dir)).isDirectory());
		expect(langs.length).toBe(44);
		for (const l of langs) {
			const m = JSON.parse(readFileSync(new URL(`${l}/mediafiles.json`, dir), 'utf8'));
			expect(m['mediafiles.releaseSheet.openPage'], l).toContain('{source}');
			expect(m['mediafiles.releaseSheet.openPage'], l).toContain('{name}');
		}
	});
});
