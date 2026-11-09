import { presetOf, presetPerms, presetsFor } from './presets.ts';
import { describe, expect, it } from 'vitest';
import type { Perm } from '../core/session.svelte.ts';

const all: Perm[] = ['ask', 'auto_approve', 'approve', 'manage_library', 'settings'];
const noSettings: Perm[] = ['ask', 'auto_approve', 'approve', 'manage_library'];

describe('role presets', () => {
	it('maps each preset onto permissions', () => {
		expect(presetPerms('admin', all)).toEqual(all);
		expect(presetPerms('manager', all)).toEqual(['ask', 'auto_approve', 'approve', 'manage_library']);
		expect(presetPerms('member', all)).toEqual(['ask']);
	});
	it('makes Kid a real choice: asks only, PG by default', () => {
		const kid = presetsFor(all).find((p) => p.id === 'kid');
		expect(kid?.soon).toBeUndefined();
		expect(kid?.maxRating).toBe('PG');
		expect(presetPerms('kid', all)).toEqual(['ask']);
	});
	it('tells Kid from Member by the age-rating limit', () => {
		expect(presetOf(['ask'], all, 'PG')).toBe('kid');
		expect(presetOf(['ask'], all, '')).toBe('member');
		expect(presetOf(['ask', 'approve'], all, 'PG')).toBe('custom');
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
