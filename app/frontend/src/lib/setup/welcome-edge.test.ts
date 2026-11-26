import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// The first-run steps sit over the sign-in backdrop: the page behind them must not show a white scrollbar gutter down the right edge.
const src = readFileSync(new URL('../../routes/welcome/+page.svelte', import.meta.url), 'utf8');
const css = compile(src, { filename: '+page.svelte', css: 'external' }).css?.code ?? '';

describe('first-run welcome edge', () => {
	it('drops the stable gutter and paints the page dark while the backdrop shows', () => {
		const rule = css.match(/html:has\(\.welcome\.over-art[^)]*\)\s*\{([^}]*)\}/)?.[1] ?? '';
		expect(rule).toMatch(/scrollbar-gutter:\s*auto/);
		expect(rule).toMatch(/background:\s*#050507/);
		expect(rule).toMatch(/color-scheme:\s*dark/);
	});
});
