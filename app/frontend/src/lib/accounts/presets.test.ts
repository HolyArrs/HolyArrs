import { choosePreset, presetOf, presetOptions, presetPerms, presetsFor } from './presets.ts';
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

describe('choosing a preset after customising', () => {
	const custom = { perms: ['ask', 'approve'] as Perm[], max: '' };
	it('keeps the custom switches so choosing Custom again brings them back', () => {
		const a = choosePreset('member', all, custom, null);
		expect(a.mix).toEqual({ perms: ['ask'], max: '' });
		expect(a.saved).toEqual(custom);
		const b = choosePreset('custom', all, a.mix!, a.saved);
		expect(b.mix).toEqual(custom);
	});
	it('saves nothing when the switches already match a preset', () => {
		const a = choosePreset('manager', all, { perms: ['ask'], max: '' }, null);
		expect(a.saved).toBeNull();
	});
	it('does nothing when Custom is chosen with nothing saved', () => {
		expect(choosePreset('custom', all, custom, null).mix).toBeNull();
	});
	it('ignores a preset the person cannot hand out', () => {
		expect(choosePreset('admin', noSettings, custom, null).mix).toBeNull();
	});
});

describe('preset bar segments', () => {
	it('keeps the same segments whether or not a mix exists, so the bar never changes shape', () => {
		const none = presetOptions(all, 'Custom', false);
		const some = presetOptions(all, 'Custom', true);
		expect(none.map((o) => o.value)).toEqual(some.map((o) => o.value));
		expect(none[none.length - 1].value).toBe('custom');
	});
	it('only lets Custom be picked while there is something to show', () => {
		expect(presetOptions(all, 'Custom', false).at(-1)?.disabled).toBe(true);
		expect(presetOptions(all, 'Custom', true).at(-1)?.disabled).toBe(false);
	});
});
