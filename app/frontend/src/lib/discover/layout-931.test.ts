import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// small layout fixes (top bar title pill, Add a language spacing, show Library row hints).
function styles(path: string): string {
	const src = readFileSync(new URL(path, import.meta.url), 'utf8');
	return compile(src, { filename: path, css: 'external' }).css?.code.replace(/:where\(\.svelte-[\w-]+\)|\.svelte-[\w-]+/g, '') ?? '';
}
function rule(css: string, selector: string): string {
	// The rule whose whole selector is this one (not a shared list like ".glass, .crumbs").
	const body = css.replace(/\/\*[\s\S]*?\*\//g, '').split('}').find((r) => r.trim().startsWith(`${selector} {`));
	return body ? `${body.trim()}}` : '';
}

describe('#931 layout', () => {
	it('lets the top bar title pill shrink so a long title ends before the search', () => {
		const css = styles('../../routes/(frame)/TopBar.svelte');
		const crumbs = rule(css, '.crumbs');
		expect(crumbs).not.toMatch(/flex:\s*none/);
		expect(crumbs).toMatch(/flex:\s*0 1 auto/);
		expect(crumbs).toMatch(/min-width:\s*0/);
	});

	it('keeps Add a language with the language rows: no line above, room below', () => {
		const css = styles('../../routes/settings/subtitles/+page.svelte');
		const add = rule(css, '.settings-content #lang-card > .body > .add');
		expect(add).toMatch(/box-shadow:\s*none/);
		expect(add).toMatch(/padding-bottom:\s*14px/);
	});

	it('styles the Season packs row (a child component) like the other rows, and its hint wraps', () => {
		const css = styles('./TitleLibrary.svelte');
		expect(rule(css, '.lib-rows .lib-row')).toMatch(/display:\s*flex/);
		expect(rule(css, '.lib-rows .lib-row > span > small.wrap')).toMatch(/white-space:\s*normal/);
		expect(rule(css, '.lib-rows .lib-pick')).toMatch(/max-width:\s*55%/);
		const src = readFileSync(new URL('./TitleLibrary.svelte', import.meta.url), 'utf8');
		expect(src).toMatch(/episodeOrder'\)\}<\/b>\{#if eo\.hint\}<small class="wrap">/);
		expect(readFileSync(new URL('./SeasonPacksRow.svelte', import.meta.url), 'utf8')).toMatch(/<small class="wrap">/);
	});

	it('lines Size and Added up on the left like the other Downloads columns (as Sonarr does)', () => {
		for (const f of ['../downloads/DlRow.svelte', '../downloads/DlSections.svelte']) expect(styles(f)).not.toMatch(/\.num[^{]*\{[^}]*text-align:\s*right/);
		expect(readFileSync(new URL('../downloads/DlSections.svelte', import.meta.url), 'utf8')).not.toMatch(/class:end=\{c\.num\}/);
	});
});
