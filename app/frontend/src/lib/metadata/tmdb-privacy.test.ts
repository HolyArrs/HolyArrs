import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The Metadata page and the Metadata doc say the same thing about what the metadata server gets; the headers named here are the ones the backend privacy test allows.
const page = readFileSync(new URL('../../routes/settings/tmdb/+page.svelte', import.meta.url), 'utf8');
const doc = readFileSync(new URL('../../../../../docs/library/metadata.md', import.meta.url), 'utf8');

describe('what the metadata server gets', () => {
	for (const word of ['install token', 'Cache-Control', 'If-None-Match', 'X-Fetch: ahead', 'X-Country-Count: off', 'Count this install', 'public IP address']) {
		it(`page and doc both name ${word}`, () => {
			expect(page).toContain(word);
			expect(doc).toContain(word);
		});
	}
	it('never says IP addresses are not stored', () => {
		expect(page).not.toMatch(/IP[^.]*not (stored|kept)/i);
		expect(doc).not.toMatch(/IP[^.]*not (stored|kept)/i);
	});
});
