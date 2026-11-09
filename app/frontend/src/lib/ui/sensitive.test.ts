import { describe, expect, it } from 'vitest';
import { needsConfirm } from './sensitive.ts';

describe('needsConfirm', () => {
	it('asks before a password change or a link without a recent local sign-in', () => {
		expect(needsConfirm('password', { localRecent: false, via: 'session' })).toBe(true);
		expect(needsConfirm('link', { localRecent: false, via: 'session' })).toBe(true);
	});
	it('goes straight on after a recent local sign-in', () => {
		expect(needsConfirm('password', { localRecent: true, via: 'session' })).toBe(false);
		expect(needsConfirm('link', { localRecent: true, via: 'session' })).toBe(false);
	});
	it('has nothing to confirm for a first password', () => {
		expect(needsConfirm('password', { localRecent: false, via: 'session', hasPassword: false })).toBe(false);
		expect(needsConfirm('link', { localRecent: false, via: 'session', hasPassword: false })).toBe(true);
	});
	it('asks before two-factor changes without a recent local sign-in', () => {
		expect(needsConfirm('two-factor', { localRecent: false, via: 'session' })).toBe(true);
		expect(needsConfirm('two-factor', { localRecent: true, via: 'session' })).toBe(false);
		expect(needsConfirm('two-factor', { localRecent: false, via: 'local_network' })).toBe(true);
	});
	it('lets an administrator who must enroll set up two-factor straight away', () => {
		expect(needsConfirm('two-factor', { localRecent: false, via: 'session', mustTwoFactor: true })).toBe(false);
	});
});

