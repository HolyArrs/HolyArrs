import { readdirSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The page's words live in messages/en; put them back so the checks read the page as a person sees it.
const messages = new URL('../core/i18n/messages/en/', import.meta.url);
const words: Record<string, string> = Object.assign({}, ...readdirSync(messages).filter((f) => f.startsWith('settings-')).map((f) => JSON.parse(readFileSync(new URL(f, messages), 'utf8'))));
const english = (page: string) =>
	page
		.replace(/=\{t\('([^']+)'(?:, \{[^}]*\})?\)\}/g, (_, k: string) => `="${words[k] ?? k}"`)
		.replace(/\{t\('([^']+)'(?:, \{[^}]*\})?\)\}/g, (_, k: string) => words[k] ?? k);

const src = english(readFileSync(new URL('../../routes/settings/alerts/+page.svelte', import.meta.url), 'utf8'));

describe('alerts hints', () => {
	it('says what the local-network box turns on', () => {
		const box = src.indexOf('<b>Reach the local network</b>');
		expect(box).toBeGreaterThan(-1);
		const hint = src.slice(box, src.indexOf('</span></span>', box));
		expect(hint).toContain('this machine or your home network');
	});

	it('offers ntfy, Gotify and Pushover, each with its own fields', () => {
		for (const kind of ['ntfy', 'gotify', 'pushover']) expect(src).toContain(`<option value="${kind}">`);
		for (const id of ['al-ntfy-server', 'al-ntfy-topic', 'al-ntfy-token', 'al-gotify-server', 'al-gotify-token', 'al-push-user', 'al-push-token']) {
			expect(src).toContain(`id="${id}"`);
		}
	});

	it('says which letters an ntfy topic may hold, like the server check allows', () => {
		const field = src.indexOf('id="al-ntfy-topic"');
		expect(field).toBeGreaterThan(-1);
		const hint = src.slice(field, src.indexOf('">', field));
		expect(hint).toContain('Letters, digits, - and _');
	});

	it('says a channel can be given by its number, like the server check allows', () => {
		const field = src.indexOf('id="al-tg-chat"');
		expect(field).toBeGreaterThan(-1);
		const hint = src.slice(field, src.indexOf('">', field));
		expect(hint).toContain('A user, group or channel ID');
	});
});
