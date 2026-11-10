import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The page's words live in messages/en; put them back so the checks read the page as a person sees it.
const messages = new URL('../core/i18n/messages/en/', import.meta.url);
const words: Record<string, string> = Object.assign({}, ...readdirSync(messages).filter((f) => f.startsWith('settings-')).map((f) => JSON.parse(readFileSync(new URL(f, messages), 'utf8'))));
const english = (page: string) =>
	page
		.replace(/=\{t\('([^']+)'(?:, \{[^}]*\})?\)\}/g, (_, k: string) => `="${words[k] ?? k}"`)
		.replace(/\{t\('([^']+)'(?:, \{[^}]*\})?\)\}/g, (_, k: string) => words[k] ?? k);

const src = english(readFileSync(new URL('../../routes/settings/sign-in/+page.svelte', import.meta.url), 'utf8'));

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
