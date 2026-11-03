import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// Home's phone and High Contrast rules, read from the compiled page styles.
const css = compile(readFileSync(new URL('./+page.svelte', import.meta.url), 'utf8'), { filename: '+page.svelte', css: 'external' }).css?.code.replace(/:where\(\.svelte-[\w-]+\)|\.svelte-[\w-]+/g, '') ?? '';
/** The rules inside one @media block (its braces matched). */
function media(query: string): string {
	const at = css.indexOf(`@media ${query}`);
	if (at < 0) return '';
	let depth = 0;
	for (let i = css.indexOf('{', at); i < css.length; i++) {
		if (css[i] === '{') depth++;
		else if (css[i] === '}' && --depth === 0) return css.slice(at, i + 1);
	}
	return '';
}

describe('Home page styles', () => {
	it('draws a system outline for focus in forced colors', () => {
		expect(media('(forced-colors: active)')).toMatch(/:focus-visible\s*\{[^}]*outline:\s*2px solid Highlight !important/);
	});
	it('keeps the header buttons on their own row on phones', () => {
		expect(media('(max-width: 767px)')).toMatch(/\.ui-head \.actions\s*\{[^}]*flex-basis:\s*100%/);
	});
	it('gives phone taps on the legend links and member Details their full size', () => {
		const phone = media('(max-width: 599px)');
		expect(phone).toMatch(/a\.leg\s*\{[^}]*min-height:\s*36px/);
		// As specific as the desktop .members .link rule, so the phone size wins.
		expect(phone).toMatch(/\.members \.link\s*\{[^}]*min-height:\s*36px/);
	});
	it('makes every control a 44px tap on touch phones only', () => {
		const touch = media('(max-width: 767px) and (pointer: coarse)');
		expect(touch).toMatch(/\.col-link::after\s*\{[^}]*inset:\s*-6px 0/);
		for (const sel of ['a\\.leg', '\\.lead', '\\.acts \\.link', '\\.members li']) expect(touch).toMatch(new RegExp(`${sel}[^{]*\\{[^}]*min-height:\\s*44px`));
	});
});
