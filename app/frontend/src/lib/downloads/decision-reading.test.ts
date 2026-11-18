import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { lang } from '#lib/core/i18n/i18n.svelte.ts';
import DecisionPanel from './DecisionPanel.svelte';
import { type Decision } from '../core/types.ts';

const card = (reading: boolean): Decision =>
	({ slot_id: 1, title_id: 2, title: 'Dune', version: 'Main', versions: 1, why: 'conflict', reason: 'Reading the download folder took too long; try again.', has_copy: false, since: new Date().toISOString(), kind: 'movie', download_id: 3, reading }) as unknown as Decision;
const fix = (body: string) => body.split('<button').find((b) => b.includes('data-testid="decision-fix"'))?.split('>')[0] ?? '';

describe('a card whose folder Import again reads now', () => {
	it('says the folder is read and Import again waits', () => {
		const { body } = render(DecisionPanel, { props: { items: [card(true)] } });
		expect(body).toContain('Reading the download folder…');
		expect(fix(body)).toContain('disabled');
	});
	it('shows the reason and an active Import again once the read ends', () => {
		const { body } = render(DecisionPanel, { props: { items: [card(false)] } });
		expect(body).not.toContain('Reading the download folder…');
		expect(fix(body)).not.toContain('disabled');
	});
	it('says it in German', () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/de/downloads.json', { eager: true })).map((f) => f.default));
		const start = { value: lang.value, words: lang.words };
		lang.value = 'de';
		lang.words = de;
		try {
			expect(render(DecisionPanel, { props: { items: [card(true)] } }).body).toContain('Download-Ordner wird gelesen…');
		} finally {
			lang.value = start.value;
			lang.words = start.words;
		}
	});
});
