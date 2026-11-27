import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import SeasonToolsSheet from './SeasonToolsSheet.svelte';

// The class alone is not the fix: the row is a grid whose first column is the
// 28px episode number, so a bare text item there wraps one word per line. Pin
// the two rules that decide it, or the note silently folds back into a column.
const css = readFileSync(new URL('./SeasonToolsSheet.svelte', import.meta.url), 'utf8');
const rule = (selector: string) => {
	const at = css.indexOf(`${selector} {`);
	expect(at, `no ${selector} rule`).toBeGreaterThan(-1);
	return css.slice(at, css.indexOf('}', at));
};

// a season with nothing on disk offers no Delete Season.
const ep = (id: number, file: Record<string, unknown> | null) => ({ id, episode: id, slots: [{ file }] }) as never;

describe('Season tools for a season with no files', () => {
	for (const mode of ['files', 'rename'] as const) {
		it(`hides Delete Season on the ${mode} tab when nothing is on disk`, () => {
			const { body } = render(SeasonToolsSheet, { props: { open: true, mode, titleId: 1, season: 1, episodes: [ep(1, null), ep(2, null)], ondelete: () => {} } });
			expect(body).toContain('data-testid="season-tools"');
			expect(body).not.toMatch(/class="st-del[ "]/);
		});
	}
	it('still offers Delete Season once a file is on disk', () => {
		const { body } = render(SeasonToolsSheet, { props: { open: true, mode: 'files', titleId: 1, season: 1, episodes: [ep(1, { RelativePath: 'a.mkv', Size: 1 }), ep(2, null)], ondelete: () => {} } });
		expect(body).toMatch(/class="st-del[ "]/);
	});
	it('counts a file in a second Version slot, and offers nothing for a season with no episodes', () => {
		const two = { id: 1, episode: 1, slots: [{ file: null }, { file: { RelativePath: 'b.mkv' } }] } as never;
		expect(render(SeasonToolsSheet, { props: { open: true, titleId: 1, season: 1, episodes: [two], ondelete: () => {} } }).body).toMatch(/class="st-del[ "]/);
		expect(render(SeasonToolsSheet, { props: { open: true, titleId: 1, season: 1, episodes: [], ondelete: () => {} } }).body).not.toMatch(/class="st-del[ "]/);
	});
	it('puts the empty note on one full-width line', () => {
		const { body } = render(SeasonToolsSheet, { props: { open: true, mode: 'files', titleId: 1, season: 1, episodes: [ep(1, null)] } });
		expect(body).toMatch(/<li class="st-empty[^"]*"/);
		expect(rule('.st-files li')).toContain('grid-template-columns: 28px');
		expect(rule('.st-files li.st-empty')).toContain('display: block');
	});
});
