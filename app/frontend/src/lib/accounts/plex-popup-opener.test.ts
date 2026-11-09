import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('../../routes/settings/sign-in/+page.svelte', import.meta.url), 'utf8');

describe('settings Plex link popup', () => {
	it('cuts window.opener before sending the popup to Plex', () => {
		const open = src.indexOf("window.open('about:blank', 'plex-link'");
		const nav = src.indexOf('win.location.href = r.auth_url');
		const cut = src.indexOf('win.opener = null');
		expect(open).toBeGreaterThan(-1);
		expect(cut).toBeGreaterThan(open);
		expect(cut).toBeLessThan(nav);
	});
});
