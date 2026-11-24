import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('./ReleaseSheet.svelte', import.meta.url), 'utf8');
const en = JSON.parse(readFileSync(new URL('../core/i18n/messages/en/mediafiles.json', import.meta.url), 'utf8'));

describe('release list with nothing to show', () => {
	const empty = src.slice(src.indexOf('class="rs-none'), src.indexOf('</li>', src.indexOf('class="rs-none')));

	it('says no good matches and offers the hidden ones only on the Good tab', () => {
		expect(empty).toMatch(/filter === 'good'[\s\S]*releaseSheet\.noMatches[\s\S]*releaseSheet\.showHidden/);
	});

	it('says the filters match nothing on the All tab', () => {
		expect(empty).toContain("t('mediafiles.releaseSheet.noFilterMatches')");
		expect(en['mediafiles.releaseSheet.noFilterMatches']).toBe('Nothing matches these filters.');
	});

	it('keeps a space between the sentence and the link', () => {
		expect(empty).toMatch(/releaseSheet\.noMatches'\)\}\{' '\}/);
	});
});
