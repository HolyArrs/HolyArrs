import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('./+page.svelte', import.meta.url), 'utf8');

describe('alerts hints', () => {
	it('names Telegram among the places reached with the local-network box off', () => {
		const box = src.indexOf('<b>May reach the local network</b>');
		expect(box).toBeGreaterThan(-1);
		const hint = src.slice(box, src.indexOf('</span></span>', box));
		expect(hint).toContain('api.telegram.org');
		expect(hint).toContain('discord.com');
		expect(hint).toContain('api.pushover.net');
		expect(hint).toContain('ntfy and Gotify servers');
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
		expect(hint).toContain('Letters, digits, - and _ only');
	});

	it('says a channel can be given by its number, like the server check allows', () => {
		const field = src.indexOf('id="al-tg-chat"');
		expect(field).toBeGreaterThan(-1);
		const hint = src.slice(field, src.indexOf('">', field));
		expect(hint).toContain('a number for a person, group or channel');
		expect(hint).toContain('groups and channels start with -');
	});
});
