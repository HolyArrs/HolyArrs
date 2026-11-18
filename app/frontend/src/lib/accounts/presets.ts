// Role presets: display-only shortcuts onto the existing permission checkboxes. Saving still sends the plain permission list.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Perm } from '../core/session.svelte.ts';

export type PresetId = 'admin' | 'manager' | 'member' | 'kid';

export interface Preset {
	id: PresetId;
	label: string;
	perms: Perm[];
	/** The age-rating limit this preset sets ('' or absent: none). */
	maxRating?: string;
	/** Shown but not choosable yet; the reason is one line under it. */
	soon?: string;
}

export const presets: Preset[] = [
	{ id: 'admin', get label() { return t('accounts.preset.admin'); }, perms: ['ask', 'auto_approve', 'approve', 'manage_library', 'settings'] },
	{ id: 'manager', get label() { return t('accounts.preset.manager'); }, perms: ['ask', 'auto_approve', 'approve', 'manage_library'] },
	{ id: 'member', get label() { return t('accounts.preset.member'); }, perms: ['ask'] },
	{ id: 'kid', get label() { return t('accounts.preset.kid'); }, perms: ['ask'], maxRating: 'PG' }
];

/** The presets this person may hand out: every permission must be grantable. */
export function presetsFor(grantable: readonly Perm[]): Preset[] {
	return presets.filter((p) => p.soon || p.perms.every((x) => grantable.includes(x)));
}

/** The checkbox list a preset sets, or null when it cannot be chosen. */
export function presetPerms(id: PresetId, grantable: readonly Perm[]): Perm[] | null {
	const p = presetsFor(grantable).find((x) => x.id === id);
	if (!p || p.soon) return null;
	return [...p.perms];
}

/** The preset a permission list and age-rating limit match, or 'custom'. */
export function presetOf(perms: readonly Perm[], grantable: readonly Perm[], maxRating = ''): PresetId | 'custom' {
	const have = new Set(perms);
	for (const p of presetsFor(grantable)) {
		if (p.soon) continue;
		if (!!p.maxRating !== !!maxRating) continue;
		if (p.perms.length === have.size && p.perms.every((x) => have.has(x))) return p.id;
	}
	return 'custom';
}

/** The segments of the preset bar. Custom is always there so the bar keeps its shape; it can be picked only while there is a mix to show or bring back. */
export function presetOptions(grantable: readonly Perm[], customLabel: string, customUsable: boolean): { value: PresetId | 'custom'; label: string; disabled?: boolean }[] {
	const list: { value: PresetId | 'custom'; label: string; disabled?: boolean }[] = presetsFor(grantable).filter((x) => !x.soon).map((pr) => ({ value: pr.id, label: pr.label }));
	list.push({ value: 'custom', label: customLabel, disabled: !customUsable });
	return list;
}

export interface Mix {
	perms: Perm[];
	max: string;
}

/**
 * What picking a segment does. A preset replaces the switches, but the person's own mix is kept so
 * picking Custom again brings it back; Custom with nothing kept changes nothing (mix null).
 */
export function choosePreset(id: PresetId | 'custom', grantable: readonly Perm[], now: Mix, saved: Mix | null): { mix: Mix | null; saved: Mix | null } {
	if (id === 'custom') return { mix: saved, saved };
	const next = presetPerms(id, grantable);
	if (!next) return { mix: null, saved };
	const keep = presetOf(now.perms, grantable, now.max) === 'custom' ? { perms: [...now.perms], max: now.max } : saved;
	return { mix: { perms: next, max: presets.find((p) => p.id === id)?.maxRating ?? '' }, saved: keep };
}
