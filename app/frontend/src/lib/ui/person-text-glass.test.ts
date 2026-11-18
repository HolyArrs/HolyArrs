import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// Glass panels redefine --text-muted to near-full ink, so a deleted name
// takes its own token, drawn from the ink around it, to stay grey there.
describe('PersonText on glass', () => {
	const src = readFileSync(new URL('./PersonText.svelte', import.meta.url), 'utf8');
	it('the deleted name uses its own token, not --text-muted', () => {
		const name = /\.name\s*\{([^}]*)\}/.exec(src)?.[1] ?? '';
		expect(name).toContain('var(--person-gone');
		expect(name).not.toContain('--text-muted');
	});
});
