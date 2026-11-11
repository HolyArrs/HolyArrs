import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

const src = readFileSync(new URL('../../routes/settings/downloaders/+page.svelte', import.meta.url), 'utf8');

describe('Settings > Downloaders: other apps Usenet jobs', () => {
	it('has a switch for downloads_sab_outside, on until the server says', () => {
		expect(src).toContain('bind:checked={settings.downloads_sab_outside}');
		expect(src).toMatch(/downloads_sab_outside: true/);
		expect(t('settings.downloaders.sabOutside')).toBe('Show Usenet jobs other apps sent');
		expect(t('settings.downloaders.sabOutsideHint')).not.toBe('settings.downloaders.sabOutsideHint');
	});
});
