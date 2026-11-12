import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// The sign-in corner (light/dark and language): 44px taps on touch screens, the desktop look kept for a mouse.
function styles(path: string): string {
	const src = readFileSync(new URL(path, import.meta.url), 'utf8');
	return compile(src, { filename: path, css: 'external' }).css?.code.replace(/:where\(\.svelte-[\w-]+\)|\.svelte-[\w-]+/g, '') ?? '';
}
/** The rules inside one @media block (its braces matched). */
function media(css: string, query: string): string {
	const at = css.indexOf(`@media ${query}`);
	if (at < 0) return '';
	let depth = 0;
	for (let i = css.indexOf('{', at); i < css.length; i++) {
		if (css[i] === '{') depth++;
		else if (css[i] === '}' && --depth === 0) return css.slice(at, i + 1);
	}
	return '';
}
const corner = styles('./SigninTheme.svelte');

describe('sign-in corner buttons', () => {
	it('keeps the quiet 36px language button for a mouse', () => {
		expect(corner).toMatch(/\.mode \.trigger\.button\s*\{[^}]*min-height:\s*36px/);
	});
	it('makes the language button a 44px tap on touch screens', () => {
		expect(media(corner, '(pointer: coarse)')).toMatch(/\.mode \.trigger\.button\s*\{[^}]*min-height:\s*44px/);
	});
	it('leaves the light/dark button its 44px touch size from IconButton', () => {
		expect(corner).not.toMatch(/\.ui-ibtn[^{]*\{[^}]*\b(width|height|min-height):/);
		expect(media(styles('../ui/IconButton.svelte'), '(pointer: coarse)')).toMatch(/\.md\s*\{[^}]*width:\s*44px;\s*height:\s*44px/);
	});
});
