import { describe, expect, it } from 'vitest';
import { needsConfirm } from './sections.ts';
import { MENUS, menuView, GROUP_DOT } from '../ui/settings-looks.ts';

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

describe('settings menu views', () => {
	it('offers five views', () => {
		expect(MENUS.map((m) => m.value)).toEqual([1, 2, 3, 4, 5]);
	});
	it('shows view 1 when nothing valid is picked', () => {
		expect(menuView(3)).toBe(3);
		expect(menuView(0)).toBe(1);
		expect(menuView(9)).toBe(1);
	});
	it('gives every group a dot colour', () => {
		for (const g of ['general', 'library', 'copies', 'people', 'linked', 'system']) expect(GROUP_DOT[g]).toMatch(/^#/);
	});
});
