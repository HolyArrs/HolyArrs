import { describe, expect, it } from 'vitest';
import { askedBy } from './ask-words.ts';
import { byWhom } from '#lib/downloads/history-kinds.ts';

describe('via a plugin', () => {
	const ask = { people: [], asked_by: { id: 2, name: 'ana' } };
	it('names the plugin on a request it made', () => {
		expect(askedBy({ ...ask, via_plugin: 'discord-bot' })).toContain('discord-bot');
		expect(askedBy(ask)).not.toContain('via');
	});
	it('names the plugin in history', () => {
		const e = { Actor: { kind: 'user', id: 2 }, Data: {}, actor_name: 'ana' };
		expect(byWhom({ ...e, ViaPlugin: 'trakt' })).toContain('trakt');
		expect(byWhom(e)).not.toContain('trakt');
	});
});
