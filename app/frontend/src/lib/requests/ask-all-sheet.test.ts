import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const sheet = readFileSync(new URL('./AskAllSheet.svelte', import.meta.url), 'utf8');

describe('Request all sheet', () => {
	it('waits for the fresh read before anything can be sent', () => {
		// A request someone else made after the page loaded only shows up in the fresh read; sending before it answers skips it instead of joining.
		const button = sheet.match(/<Button variant="primary" \{busy\}[^>]*>/)?.[0] ?? '';
		expect(button).toMatch(/disabled=\{[^}]*reading/);
		expect(sheet).toMatch(/async function go\(\) \{\s*if \([^)]*reading/);
		expect(sheet).toMatch(/finally\(\(\) => \{ if \(mine === opened\) reading = false; \}\)/);
	});
});
