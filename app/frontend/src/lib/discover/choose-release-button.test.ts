import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('./TitlePage.svelte', import.meta.url), 'utf8');
const sheet = readFileSync(new URL('./CheckReleasesSheet.svelte', import.meta.url), 'utf8');

describe('Check releases on the title page', () => {
	it('shows its words on phones, where a round icon has no hover label', () => {
		const line = src.split('\n').find((l) => l.includes('data-testid="hero-choose-release"')) ?? '';
		expect(line).toContain("label={t('discover.titleOwned.checkReleases')}");
		// Desktop keeps the round icon with its hover label; phones (no hover) get the words.
		expect(line).toContain('pill={phone}');
		// The full words do not fit next to Follow at 390 px, so phones read the short one.
		expect(line).toContain("caption={phone ? t('library.title.releases')");
	});
	it('opens the check sheet and adds nothing until a release is picked', () => {
		const line = src.split('\n').find((l) => l.includes('data-testid="hero-choose-release"')) ?? '';
		expect(line).toContain('checkOpen = true');
		expect(line).not.toContain('addMovie');
	});
	it('adds with the Pick rules the check used, then sends the picked release', () => {
		expect(src).toContain("addMovie?.open(details?.movie as LookupMovie, { choose: true, rulesId })");
		expect(src).toMatch(/\/releases\/check\/\$\{pick\.checkId\}\/pick/);
	});
	it('searches only when asked and judges by the chosen Pick rules', () => {
		expect(sheet).toContain("api.post<{ job_ids: number[] }>('/releases/check'");
		expect(sheet).toContain('pick_rules_id: rulesId');
		// Opening loads nothing from the Sources: with no check yet the list is empty.
		expect(sheet).toContain('checkId ? api.get<ReleasePage>');
	});
	it('keeps the Pick rules picked in the sheet (#714)', () => {
		// The open-reset must not read rulesId, or picking 4K re-runs it and puts the saved default back.
		const reset = sheet.slice(sheet.indexOf('$effect(() => {'), sheet.indexOf('let sheetKey'));
		expect(reset).toContain('untrack(');
		expect(reset.slice(0, reset.indexOf('untrack('))).not.toContain('rulesId');
	});
});
