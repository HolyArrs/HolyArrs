import { describe, expect, it } from 'vitest';
import { needsConfirm } from './sections.ts';
import { LOOKS, NO_TILE, lookFrom } from '../ui/settings-looks.ts';

describe('needsConfirm', () => {
	it('asks before a password change or a link without a recent local sign-in', () => {
		expect(needsConfirm('password', { localRecent: false, via: 'session' })).toBe(true);
		expect(needsConfirm('link', { localRecent: false, via: 'session' })).toBe(true);
	});
	it('goes straight on after a recent local sign-in', () => {
		expect(needsConfirm('password', { localRecent: true, via: 'session' })).toBe(false);
		expect(needsConfirm('link', { localRecent: true, via: 'session' })).toBe(false);
	});
	it('asks before two-factor changes only on a local-network visit', () => {
		expect(needsConfirm('two-factor', { localRecent: false, via: 'session' })).toBe(false);
		expect(needsConfirm('two-factor', { localRecent: false, via: 'local_network' })).toBe(true);
	});
});

describe('settings looks', () => {
	it('offers today plus five looks', () => {
		expect(LOOKS.map((l) => l.value)).toEqual([0, 1, 2, 3, 4, 5]);
	});
	it('reads ?look= safely', () => {
		expect(lookFrom('3')).toBe(3);
		expect(lookFrom('9')).toBe(0);
		expect(lookFrom(null)).toBe(0);
		expect(lookFrom('x')).toBe(0);
	});
	it('drops the icon tile in at least three looks', () => {
		expect(NO_TILE.size).toBeGreaterThanOrEqual(3);
	});
});
