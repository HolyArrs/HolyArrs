import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const src = readFileSync(new URL('./DlPane.svelte', import.meta.url), 'utf8');
const [markup, css = ''] = src.split('<style>');
const btn = markup.match(/<button[^>]*data-testid="dl-pane-close"[^>]*>[\s\S]*?<\/button>/)?.[0] ?? '';

// An X next to the collapse arrow closes the pane fully; clicking a row opens it again.
describe('download pane close button', () => {
	it('sits right after the collapse arrow', () => {
		expect(markup).toMatch(/data-testid="dl-pane-toggle"[\s\S]{0,120}<\/button>\s*<button[^>]*data-testid="dl-pane-close"/);
	});
	it('closes the pane, has a translated label and the shared x icon', () => {
		expect(btn).toMatch(/aria-label=\{t\('downloads\.pane\.close'\)\}/);
		expect(btn).toMatch(/onclick=\{closePane\}/);
		expect(btn).toMatch(/<Icon name="x" size=\{14\}/);
		expect(markup).toMatch(/function closePane\(\)[\s\S]*?openId = 0[\s\S]*?\.focus\(\)/);
	});
	it('has a 44px touch target', () => {
		expect(css).toMatch(/\.pcx::after \{[^}]*inset: -8px/);
		expect(css).toMatch(/\.pcx \{[^}]*width: 28px; height: 28px/);
	});
});
