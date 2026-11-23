import { describe, expect, it } from 'vitest';
import { KEY_LINKS, MASK, keyBody, keyForm, testWords } from './mode.ts';

describe('metadata keys form', () => {
	it('shows saved keys as the mask and others empty', () => {
		const f = keyForm({ tmdb_token: true, tvdb_key: false });
		expect(f.tmdb_token).toBe(MASK);
		expect(f.tvdb_key).toBe('');
		expect(f.fanart_key).toBe('');
	});
	it('sends the mask back unchanged, trims new keys and sends empty to remove', () => {
		const b = keyBody({ tmdb_token: MASK, tvdb_key: '  abc ', tvdb_pin: '', mdblist_key: '', fanart_key: 'x', omdb_key: ' omdb ' });
		expect(b).toEqual({ tmdb_token: MASK, tvdb_key: 'abc', tvdb_pin: '', mdblist_key: '', fanart_key: 'x', omdb_key: 'omdb' });
	});
	it('keeps an unknown test line as the server sent it', () => {
		expect(testWords('Something new')).toBe('Something new');
		expect(testWords('Key works')).not.toBe('');
	});
	it('turns each test code into its own fix, never the raw code', () => {
		const lines = ['works', 'no_key', 'refused', 'tvdb_refused', 'tvdb_pin_refused', 'unreachable'].map((c) => testWords('x', c));
		expect(new Set(lines).size).toBe(lines.length);
		for (const l of lines) expect(l).not.toBe('x');
		expect(testWords('Enter a key first')).toBe(testWords('', 'no_key'));
	});
	it('links every key a person types to where it is made, over https', () => {
		for (const k of ['tmdb_token', 'tvdb_key', 'mdblist_key', 'fanart_key'] as const) expect(KEY_LINKS[k]).toMatch(/^https:\/\//);
	});
});
