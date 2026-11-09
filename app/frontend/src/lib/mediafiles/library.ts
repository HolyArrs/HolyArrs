// Library page filters, kinds, title search and poster status.

import type { TitleView } from '../core/types.ts';
import type { StatusKind } from '#lib/ui/StatusPill.svelte';
import { audienceStatus, factsOf, titleStatus, type Audience, type TitleStatus } from '#lib/ui/title-status.ts';
import { libraryState } from './library-state.ts';

export type Filter = 'all' | 'missing' | 'not_out_yet' | 'better' | 'have';

export type Kind = 'all' | 'movie' | 'show' | 'artist';

export interface AddChoices {
	release_gate: string;
}

export interface SecondVersion {
	label: string;
	pick_rules_id: number;
	library_folder_id: number;
}

// Filter chips and their labels.
export const filters: [Filter, string][] = [
	['all', 'All'],
	['missing', 'Missing'],
	['not_out_yet', 'Upcoming'],
	['better', 'Better copy'],
	['have', 'In library']
];

// Slot states that count as owned.
const haveStates = ['in_library', 'better_wanted'];

// Slot states that count as still missing, because the file is not there yet.
const busyStates = ['searching', 'held', 'chosen', 'downloading', 'stalled', 'failed', 'importing', 'needs_a_decision', 'rejected'];

export const kinds: [Kind, string][] = [
	['all', 'Everything'],
	['movie', 'Movies'],
	['show', 'TV shows'],
	['artist', 'Music']
];

// Browser storage key for the Add form choices.
const key = 'holyarrs.add-choices';

export const letters = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ', '#'];

// The status each filter already implies, so the poster only shows what is different.
const viewDefault: Record<Filter, StatusKind[]> = {
	all: ['in_library'],
	have: ['in_library'],
	missing: ['missing'],
	not_out_yet: ['coming_soon'],
	better: ['in_library']
};

export function ofKind(t: Pick<TitleView, 'kind'>, k: Kind): boolean {
	return k === 'all' || t.kind === k;
}

// Shows and artists use their totals; movies read their slots.
export function matches(t: TitleView, f: Filter): boolean {
	if (t.kind === 'show' || t.kind === 'artist') {
		const s = t.kind === 'show' ? t.show : t.artist;
		if (!s) return f === 'all';
		switch (f) {
			case 'all':
				return true;
			case 'have':
				return s.in_library > 0;
			case 'better':
				return s.better_wanted > 0;
			case 'not_out_yet':
				return s.not_out_yet > 0;
			case 'missing':
				return s.missing > 0;
		}
	}
	const states = t.slots.map((s) => s.state);
	switch (f) {
		case 'all':
			return true;
		case 'have':
			return states.some((s) => haveStates.includes(s));
		case 'better':
			return states.includes('better_wanted');
		case 'not_out_yet':
			return states.includes('not_out_yet');
		case 'missing':
			return states.some((s) => s === 'wanted' || busyStates.includes(s));
	}
}

export function counts(ts: TitleView[]): Record<Filter, number> {
	const out = { all: 0, missing: 0, not_out_yet: 0, better: 0, have: 0 } as Record<Filter, number>;
	for (const t of ts) for (const [f] of filters) if (matches(t, f)) out[f]++;
	return out;
}

export function byName(t: TitleView, q: string): boolean {
	const words = q.toLowerCase().split(/\s+/).filter(Boolean);
	const name = t.name.toLowerCase();
	return words.every((w) => name.includes(w) || String(t.year ?? '') === w);
}

// Caps the search text so a huge paste cannot reach the server.
export function titleQuery(q: string): string {
	return q.slice(0, 256).trim().split(/\s+/).filter(Boolean).slice(0, 16).join(' ');
}

// A title also matches when the server search found it by something other than its name.
export function matchesTitleQuery(t: TitleView, q: string, indexed: ReadonlySet<number>): boolean {
	return !q || byName(t, q) || indexed.has(t.id);
}

// Bad or missing storage gives no remembered choices.
export function loadChoices(store: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): Partial<AddChoices> {
	try {
		const v = JSON.parse(store?.getItem(key) ?? '{}');
		return v && typeof v === 'object' && typeof v.release_gate === 'string' ? { release_gate: v.release_gate } : {};
	} catch {
		return {};
	}
}

export function saveChoices(c: AddChoices, store: Pick<Storage, 'setItem'> | undefined = globalThis.localStorage) {
	try {
		store?.setItem(key, JSON.stringify(c));
	} catch {
	}
}

// Keeps the remembered id only while it is still offered.
export function pickId(remembered: number | undefined, offered: { id: number }[], fallback: number): number {
	return remembered && offered.some((o) => o.id === remembered) ? remembered : fallback;
}

// Guesses a 4K rules set and folder for the second version.
export function secondVersion(
	rules: { id: number; name: string }[],
	folders: { id: number; label: string; path: string }[],
	mainFolder: number
): SecondVersion {
	const fourK = rules.find((r) => r.name.toLowerCase() === '4k') ?? rules.find((r) => /4k|2160|uhd/i.test(r.name));
	const folder = folders.find((f) => f.id !== mainFolder && /4k|2160|uhd/i.test(`${f.label} ${f.path}`));
	return { label: '4K', pick_rules_id: fourK?.id ?? rules[0]?.id ?? 0, library_folder_id: folder?.id ?? mainFolder };
}

// Release gate notes are left out; they are shown elsewhere.
export function slotNotes(slots: { reason?: string | null }[]): string[] {
	return [...new Set(slots.map((s) => s.reason ?? '').filter((r) => r && !r.startsWith('release gate:')))];
}

// Strips accents so a name starting with an accented letter files under its plain letter.
export function letterOf(name: string): string {
	const c = name.trim().normalize('NFD').replace(/[̀-ͯ]/g, '').charAt(0).toUpperCase();
	return c >= 'A' && c <= 'Z' ? c : '#';
}

export function firstOfLetter(ts: Pick<TitleView, 'id' | 'name'>[]): Map<string, number> {
	const out = new Map<string, number>();
	for (const t of ts) {
		const l = letterOf(t.name);
		if (!out.has(l)) out.set(l, t.id);
	}
	return out;
}

// Hides the chip when it just repeats the filter, unless it carries a number.
export function exceptionChip<T extends { kind: StatusKind; label: string }>(f: Filter, chip: T | null): T | null {
	if (!chip) return null;
	return viewDefault[f].includes(chip.kind) && !/\d/.test(chip.label) ? null : chip;
}

// Staff see the Library state; others see the audience status.
export function posterStatus(t: TitleView, who: Audience): TitleStatus | null {
	if (who === 'staff') {
		const ls = libraryState(t);
		return ls ? { kind: ls.pill, label: ls.label } : null;
	}
	const facts = factsOf(t);
	if (t.kind !== 'show' && t.kind !== 'artist') return titleStatus({ ...facts, titleKind: 'movie' }, who);
	const s = t.kind === 'show' ? t.show : t.artist;
	const titleKind = t.kind;
	if (!s) return null;
	if (s.missing > 0) {
		const st = audienceStatus('missing', who, { tracked: true, ask: facts.ask });
		const unit = t.kind === 'show' ? 'episode' : 'track';
		return { kind: st.kind, label: `${st.label} · ${s.missing} ${unit}${s.missing === 1 ? '' : 's'}` };
	}
	if (s.in_library > 0 || s.not_out_yet > 0 || facts.ask) return titleStatus({ titleKind, tracked: true, ask: facts.ask, inLibrary: s.in_library > 0, comingSoon: s.not_out_yet > 0 }, who);
	return null;
}

// Episodes kept up to date that are still wanted or busy.
export function missingEpisodes<E extends { keep_up_to_date: boolean; slots: { state: string }[] }>(eps: E[]): E[] {
	return eps.filter((e) => e.keep_up_to_date && e.slots.some((s) => s.state === 'wanted' || busyStates.includes(s.state)));
}
