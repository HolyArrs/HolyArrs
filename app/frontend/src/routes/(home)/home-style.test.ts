import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// Home's phone and High Contrast rules, read from the compiled styles of the page and its overview.
const css = ['+page.svelte', 'HomeOverview.svelte']
	.map((file) => compile(readFileSync(new URL(`./${file}`, import.meta.url), 'utf8'), { filename: file, css: 'external' }).css?.code.replace(/:where\(\.svelte-[\w-]+\)|\.svelte-[\w-]+/g, '') ?? '')
	.join('\n');
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
	it('puts Search and Explore as round buttons beside the switch on phones, the note under them', () => {
		const phone = media('(max-width: 767px)');
		expect(phone).toMatch(/\.wide-only\s*\{[^}]*display:\s*none/);
		expect(phone).toMatch(/\.phone-only\s*\{[^}]*display:\s*flex/);
		expect(phone).toMatch(/\.note\s*\{[^}]*flex-basis:\s*100%/);
	});
	it('makes the Home | System switch a 44px tap on phones', () => {
		expect(media('(max-width: 767px)')).toMatch(/.top-row .ui-seg button\s*\{[^}]*min-height:\s*44px/);
	});
	it('lifts the hidden grid out of the flow, so the page is only as tall as the grid on screen', () => {
		expect(css).toMatch(/\.bento\.gone\s*\{[^}]*position:\s*absolute/);
	});
});
