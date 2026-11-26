import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const css = readFileSync(new URL('./DlRow.svelte', import.meta.url), 'utf8').split('<style>')[1] ?? '';

// A long failure reason ("ERROR: Bad archive /data/...rar") must stay inside its row: two lines at most, the rest cut, the full text in the cell's title.
describe('status reason in a download row', () => {
	it('is clamped to two lines on wide screens', () => {
		const rule = css.match(/\.st \.why \{[^}]*-webkit-line-clamp[^}]*\}/)?.[0] ?? '';
		expect(rule).toMatch(/-webkit-line-clamp: 2/);
		expect(rule).toMatch(/overflow: hidden/);
	});
	it('stays on one cut line on phones', () => {
		expect(css).toMatch(/\.dline > \[data-col='status'\] \.why \{[^}]*white-space: nowrap;[^}]*text-overflow: ellipsis/);
	});
});

describe('status cell height', () => {
	it('never draws past its row', () => {
		expect(css).toMatch(/\t\.st \{[^}]*max-height: 100%;[^}]*overflow: hidden;/);
	});
});
