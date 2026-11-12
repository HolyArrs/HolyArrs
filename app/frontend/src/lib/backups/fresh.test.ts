import { describe, expect, it } from 'vitest';
import { freshAsk } from './fresh.ts';

describe('freshAsk', () => {
	it('lets a fresh or non-session sign-in through', () => {
		expect(freshAsk({ via: 'session', localRecent: true, hasPassword: true })).toBe(null);
		expect(freshAsk({ via: 'key', localRecent: false, hasPassword: false })).toBe(null);
	});
	it('asks to set a password when there is none', () => {
		expect(freshAsk({ via: 'session', localRecent: false, hasPassword: false })).toBe('set');
	});
	it('asks to confirm the password otherwise', () => {
		expect(freshAsk({ via: 'session', localRecent: false, hasPassword: true })).toBe('confirm');
	});
});
