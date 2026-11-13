import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const board = readFileSync(new URL('./RequestsBoard.svelte', import.meta.url), 'utf8');
const page = readFileSync(new URL('../../routes/requests/+page.svelte', import.meta.url), 'utf8');

// The line under a card's title (kind and year), on the approvers' cards and the members' cards.
function line(src: string): string {
	const at = src.indexOf('<span class="ban-f">');
	return at < 0 ? '' : src.slice(at, src.indexOf('\n', at));
}

describe('4K mark on request cards', () => {
	const mark = `{#if a.four_k}<span class="ap4k-tag" title={t('requests.board.fourKTitle')}>4K</span>{/if}`;

	it('shows on the approvers cards', () => {
		expect(line(board)).toContain(mark);
	});

	it('shows on the members own cards the same way', () => {
		expect(line(page)).toContain(mark);
		expect(page).toMatch(/\.ap4k-tag \{/);
	});
});
