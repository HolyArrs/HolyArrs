import { describe, expect, it } from 'vitest';
import { MASK, keyBody, keyForm, testWords } from './mode.ts';

describe('metadata keys form', () => {
	it('shows saved keys as the mask and others empty', () => {
		const f = keyForm({ tmdb_token: true, tvdb_key: false });
		expect(f.tmdb_token).toBe(MASK);
		expect(f.tvdb_key).toBe('');
		expect(f.fanart_key).toBe('');
	});
	it('sends the mask back unchanged, trims new keys and sends empty to remove', () => {
		const b = keyBody({ tmdb_token: MASK, tvdb_key: '  abc ', tvdb_pin: '', mdblist_key: '', fanart_key: 'x' });
		expect(b).toEqual({ tmdb_token: MASK, tvdb_key: 'abc', tvdb_pin: '', mdblist_key: '', fanart_key: 'x' });
	});
	it('keeps an unknown test line as the server sent it', () => {
		expect(testWords('Something new')).toBe('Something new');
		expect(testWords('Key works')).not.toBe('');
	});
});
