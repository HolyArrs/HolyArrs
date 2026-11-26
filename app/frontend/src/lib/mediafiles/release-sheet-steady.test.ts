import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// On desktop the release sheet keeps one height while filtering, so a chip that
// shortens the list cannot move the sheet out from under the next click.
const sheet = readFileSync(new URL('../ui/Sheet.svelte', import.meta.url), 'utf8');
const releases = readFileSync(new URL('./ReleaseSheet.svelte', import.meta.url), 'utf8');

describe('release sheet keeps its size', () => {
	it('asks the sheet for a steady height', () => {
		expect(releases).toMatch(/<Sheet[^>]*\bsteady\b/);
	});
	it('gives steady sheets a fixed desktop height and leaves phones alone', () => {
		expect(sheet).toMatch(/class:steady/);
		const desk = sheet.match(/@media \(min-width: 768px\) \{[^}]*\.steady[^}]*height: min\(86dvh, 820px\)/);
		expect(desk).not.toBeNull();
	});
});
