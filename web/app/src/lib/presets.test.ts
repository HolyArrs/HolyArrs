import { describe, expect, it } from 'vitest';
import { presetOf, presetPerms, presetsFor } from './presets.ts';
import type { Perm } from './session.svelte.ts';

const all: Perm[] = ['ask', 'auto_approve', 'approve', 'manage_library', 'settings'];
const noSettings: Perm[] = ['ask', 'auto_approve', 'approve', 'manage_library'];

describe('role presets', () => {
	it('maps each preset onto permissions', () => {
		expect(presetPerms('admin', all)).toEqual(all);
		expect(presetPerms('manager', all)).toEqual(['ask', 'auto_approve', 'approve', 'manage_library']);
		expect(presetPerms('member', all)).toEqual(['ask']);
	});
	it('keeps Kid shown but not choosable', () => {
		expect(presetsFor(all).find((p) => p.id === 'kid')?.soon).toBeTruthy();
		expect(presetPerms('kid', all)).toBeNull();
	});
	it('hides presets with permissions this person cannot grant', () => {
		expect(presetsFor(noSettings).map((p) => p.id)).toEqual(['manager', 'member', 'kid']);
		expect(presetPerms('admin', noSettings)).toBeNull();
	});
	it('names the matching preset in any order, else Custom', () => {
		expect(presetOf(['manage_library', 'ask', 'approve', 'auto_approve'], all)).toBe('manager');
		expect(presetOf(['ask'], all)).toBe('member');
		expect(presetOf(all, all)).toBe('admin');
		expect(presetOf(['ask', 'approve'], all)).toBe('custom');
		expect(presetOf([], all)).toBe('custom');
	});
});
