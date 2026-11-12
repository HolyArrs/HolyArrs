import { describe, expect, it } from 'vitest';
import { resultKey, searchSettings, settingIndex, type SettingEntry } from './settings-search.ts';
import { sections } from '#lib/core/settings.ts';

// The Settings home puts each page in front of the hand-made entries (see +page.svelte), so the same list is searched here.
const index: SettingEntry[] = [...sections.map((s) => ({ label: s.label, href: s.href, anchor: '', words: s.about })), ...settingIndex];

describe('Settings search result keys (#268)', () => {
	it('gives every entry its own key', () => {
		const keys = index.map(resultKey);
		const twice = keys.filter((k, i) => keys.indexOf(k) !== i);
		expect(twice).toEqual([]);
	});

	it('typing one letter returns rows with no repeated key', () => {
		for (const q of ['d', 'p', 's', 'pick', 'schedules']) {
			const keys = searchSettings(q, () => true, () => '', index).map(resultKey);
			expect(new Set(keys).size, q).toBe(keys.length);
		}
	});
});
