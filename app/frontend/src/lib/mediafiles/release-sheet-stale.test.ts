import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('./ReleaseSheet.svelte', import.meta.url), 'utf8');
const en = JSON.parse(readFileSync(new URL('../core/i18n/messages/en/mediafiles.json', import.meta.url), 'utf8'));

describe('release list after the Pick rules change', () => {
	it('reads the out-of-date mark from the list', () => {
		expect(src).toContain('rulesChanged = !!p.rules_changed;');
	});

	it('says the verdicts are out of date while no search runs', () => {
		expect(src).toMatch(/\{#if rulesChanged && releases\.length && !running\.length\}[^\n]*releaseSheet\.rulesChanged/);
		expect(en['mediafiles.releaseSheet.rulesChanged']).toMatch(/^Pick rules changed since this search/);
	});

	it('looks again when the tab comes back into view', () => {
		expect(src).toMatch(/addEventListener\('visibilitychange', back\)/);
		expect(src).toMatch(/removeEventListener\('visibilitychange', back\)/);
	});
});
