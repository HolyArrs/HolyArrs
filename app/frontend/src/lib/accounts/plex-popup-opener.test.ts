import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8');
const places = {
	settings: read('../../routes/settings/sign-in/+page.svelte'),
	account: read('./AccountSecurity.svelte'),
	invite: read('../../routes/invite/[token]/+page.svelte'),
	signin: read('./signin-plex.ts'),
};
const helper = read('./plex-window.ts');

describe('Plex sign-in windows', () => {
	it('every Plex button opens, sends and closes through the shared helper', () => {
		for (const [name, src] of Object.entries(places)) {
			expect(src, name).toContain('openPlexWindow(');
			expect(src, name).toContain('sendToPlex(win');
			expect(src, name).toContain('closePlexWindow(win)');
			expect(src, name).not.toContain('auth_url;');
		}
	});

	it('keeps window.opener so this tab can close the window', () => {
		expect(helper).not.toContain('win.opener = null');
	});
});
