import { endedWords, order, others, type SessionRow } from './session-list.ts';
import { describe, expect, it } from 'vitest';

const row = (id: string, last: string, current = false): SessionRow => ({
	id,
	device: 'Firefox on Windows',
	created_at: '2026-10-01T00:00:00Z',
	last_used_at: last,
	expires_at: '2026-11-01T00:00:00Z',
	current
});

describe('sessions', () => {
	it('puts this device first, then newest use', () => {
		const got = order([row('a', '2026-10-02T00:00:00Z'), row('b', '2026-10-03T00:00:00Z'), row('c', '2026-10-01T00:00:00Z', true)]);
		expect(got.map((r) => r.id)).toEqual(['c', 'b', 'a']);
	});
	it('counts the other devices', () => {
		expect(others([row('a', 'x', true), row('b', 'y')])).toBe(1);
		expect(others([])).toBe(0);
	});
	it('says what a sign-out did', () => {
		expect(endedWords(0)).toBe('No other devices were signed in.');
		expect(endedWords(1)).toBe('Signed out 1 device.');
		expect(endedWords(3)).toBe('Signed out 3 devices.');
	});
});
