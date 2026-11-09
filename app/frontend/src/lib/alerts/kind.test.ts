import { switchKind } from './kind.ts';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const src = readFileSync(new URL('../../routes/settings/alerts/+page.svelte', import.meta.url), 'utf8');

describe('switchKind', () => {
	const form = { kind: 'gotify', name: 'Phone', topic: 'holyarrs', app_token: ['AStandIn', 'GotifyKey'].join('.'), server_url: 'https://gotify.example.com', clear_app_token: false };

	it('empties the shared token and server when the Kind changes', () => {
		const next = switchKind(form, 'ntfy');
		expect(next.kind).toBe('ntfy');
		expect(next.app_token).toBe('');
		expect(next.server_url).toBe('');
		expect(next.name).toBe('Phone');
		expect(next.topic).toBe('holyarrs');
	});

	it('keeps everything when the same Kind is picked again', () => {
		expect(switchKind(form, 'gotify')).toBe(form);
	});

	it('is what the Kind picker uses', () => {
		const field = src.indexOf('<select id="al-kind"');
		expect(field).toBeGreaterThan(-1);
		expect(src.slice(field, src.indexOf('\n', field))).toContain('switchKind(');
	});
});

describe('ntfy access token', () => {
	it('offers a box to remove a stored token', () => {
		expect(src).toContain('id="al-ntfy-clear"');
		expect(src).toContain('bind:checked={form.clear_app_token}');
	});

	it('only talks about a stored token when there is one', () => {
		const field = src.indexOf('id="al-ntfy-token"');
		const hint = src.slice(field, src.indexOf('">', field));
		expect(hint).toContain('isNew || !editing?.app_token');
	});
});
