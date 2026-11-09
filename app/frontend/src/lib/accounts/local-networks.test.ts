import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('../../routes/settings/sign-in/+page.svelte', import.meta.url), 'utf8');

describe('settings local networks', () => {
	it('lists the networks under the skip switch and saves them', () => {
		const skip = src.indexOf('Skip sign-in on my local network</span>');
		const box = src.indexOf('<textarea id="local-nets"');
		expect(skip).toBeGreaterThan(-1);
		expect(box).toBeGreaterThan(skip);
		expect(src).toContain("api.put<SignInSettings>('/auth/settings', { local_networks: netList() })");
	});

	it('sends the typed networks when the skip is turned on', () => {
		expect(src).toContain('v ? { skip_login_local_network: v, local_networks: netList() }');
	});
});
