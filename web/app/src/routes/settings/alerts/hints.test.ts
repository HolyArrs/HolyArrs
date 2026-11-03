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
	});

	it('says a channel can be given by its number, like the server check allows', () => {
		const field = src.indexOf('id="al-tg-chat"');
		expect(field).toBeGreaterThan(-1);
		const hint = src.slice(field, src.indexOf('">', field));
		expect(hint).toContain('a number for a person, group or channel');
		expect(hint).toContain('groups and channels start with -');
	});
});
