import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The page's words live in messages/en; put them back so the checks read the page as a person sees it.
const messages = new URL('../core/i18n/messages/en/', import.meta.url);
const words: Record<string, string> = Object.assign({}, ...readdirSync(messages).filter((f) => f.startsWith('settings-')).map((f) => JSON.parse(readFileSync(new URL(f, messages), 'utf8'))));
const english = (page: string) =>
	page
		.replace(/=\{t\('([^']+)'(?:, \{[^}]*\})?\)\}/g, (_, k: string) => `="${words[k] ?? k}"`)
		.replace(/\{t\('([^']+)'(?:, \{[^}]*\})?\)\}/g, (_, k: string) => words[k] ?? k)
		.replace(/<Words key="([^"]+)"[^>]*\/>/g, (_, k: string) => words[k] ?? k);

// The Metadata page and the Metadata doc say the same thing about what the metadata server gets; the headers named here are the ones the backend privacy test allows.
const page = english(readFileSync(new URL('../../routes/settings/tmdb/+page.svelte', import.meta.url), 'utf8'));
const doc = readFileSync(new URL('../../../../../docs/library/metadata.md', import.meta.url), 'utf8');

describe('what the metadata server gets', () => {
	for (const word of ['install token', 'Cache-Control', 'If-None-Match', 'X-Fetch: ahead', 'X-Country-Count: off', 'Count this install', 'public IP address']) {
		it(`page and doc both name ${word}`, () => {
			expect(page).toContain(word);
			expect(doc).toContain(word);
		});
	}
	it('never says IP addresses are not stored', () => {
		expect(page).not.toMatch(/IP[^.]*not (stored|kept)/i);
		expect(doc).not.toMatch(/IP[^.]*not (stored|kept)/i);
	});
});
