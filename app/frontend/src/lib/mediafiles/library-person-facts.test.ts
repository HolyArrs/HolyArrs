import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('./LibraryList.svelte', import.meta.url), 'utf8');

describe('phone table facts', () => {
	it('shows the facts line through PersonText so deleted people get the deleted look', () => {
		const line = src.split('\n').find((l) => l.includes('class="facts"'));
		expect(line).toBeTruthy();
		expect(line).toContain('<PersonText');
	});
});
