import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('../../routes/settings/subtitles/+page.svelte', import.meta.url), 'utf8');

// The card's action row, from its opening div to the closing one.
const actions = (() => {
	const start = src.indexOf('<div class="row actions">', src.indexOf('data-testid="subtitle-site"'));
	return src.slice(start, src.indexOf('</div>', start));
})();

describe('Settings > Subtitles: a saved site whose kind is not offered', () => {
	it('shows Turn on, Pause and Change only when the kind is offered', () => {
		const guard = actions.indexOf('{#if k}');
		const close = actions.indexOf('{/if}', actions.lastIndexOf('changeButton'));
		expect(guard).toBeGreaterThan(-1);
		for (const key of ['turnOn', 'pause', 'changeButton']) {
			const at = actions.indexOf(`t('settings.subtitles.${key}')`);
			expect(at, key).toBeGreaterThan(guard);
			expect(at, key).toBeLessThan(close);
		}
	});

	it('keeps Test and Remove outside that check', () => {
		const guard = actions.indexOf('{#if k}');
		expect(actions.indexOf("t('settings.subtitles.test')")).toBeLessThan(guard);
		expect(actions.indexOf("t('settings.subtitles.remove')")).toBeGreaterThan(actions.lastIndexOf('{/if}'));
	});
});
