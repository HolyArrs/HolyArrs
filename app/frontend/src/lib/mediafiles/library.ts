// Library filters and the remembered add choices, kept apart from the pages
// so they can be tested.
import type { TitleView } from '../core/types.ts';
import type { StatusKind } from '../ui/StatusPill.svelte';
import { audienceStatus, factsOf, titleStatus, type Audience, type TitleStatus } from '../ui/title-status.ts';
import { libraryState } from './library-state.ts';

export type Filter = 'all' | 'missing' | 'not_out_yet' | 'better' | 'have';

export const filters: [Filter, string][] = [
	['all', 'All'],
	['missing', 'Missing'],
	['not_out_yet', 'Upcoming'],
	['better', 'Better copy'],
	['have', 'In library']
];

const haveStates = ['in_library', 'better_wanted'];
const busyStates = ['searching', 'held', 'chosen', 'downloading', 'stalled', 'failed', 'importing', 'needs_a_decision', 'rejected'];

export type Kind = 'all' | 'movie' | 'show' | 'artist';

export const kinds: [Kind, string][] = [
	['all', 'Everything'],
	['movie', 'Movies'],
	['show', 'TV shows'],
	['artist', 'Music']
];

/** ofKind says whether a title is of the kind picked. */
export function ofKind(t: Pick<TitleView, 'kind'>, k: Kind): boolean {
	return k === 'all' || t.kind === k;
}

/**
 * matches says whether a title shows under a filter. A movie counts when
 * any of its Slots does; a show when any of its kept episodes does, an
 * artist when any of its tracks does.
 */
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
			// Out (or gate open) but no copy yet: waiting to be found or on its way.
			return states.some((s) => s === 'wanted' || busyStates.includes(s));
	}
}

/** counts how many titles each filter shows. */
export function counts(ts: TitleView[]): Record<Filter, number> {
	const out = { all: 0, missing: 0, not_out_yet: 0, better: 0, have: 0 } as Record<Filter, number>;
	for (const t of ts) for (const [f] of filters) if (matches(t, f)) out[f]++;
	return out;
}

/** byName keeps titles whose name holds every word of q. */
export function byName(t: TitleView, q: string): boolean {
	const words = q.toLowerCase().split(/\s+/).filter(Boolean);
	const name = t.name.toLowerCase();
	return words.every((w) => name.includes(w) || String(t.year ?? '') === w);
}

/** titleQuery applies the title-list endpoint's character and word bounds. */
export function titleQuery(q: string): string {
	return q.slice(0, 256).trim().split(/\s+/).filter(Boolean).slice(0, 16).join(' ');
}

/** matchesTitleQuery keeps the existing local matches beside indexed aliases. */
export function matchesTitleQuery(t: TitleView, q: string, indexed: ReadonlySet<number>): boolean {
	return !q || byName(t, q) || indexed.has(t.id);
}

/** AddChoices is what this browser remembers from the last add. Folder, Pick
 * rules, Keep up to date and Search when added come from Movies and TV
 * defaults instead (title-defaults.ts). */
export interface AddChoices {
	release_gate: string;
}

const key = 'holyarrs.add-choices';

/** loadChoices reads the last add choices this browser used, if any. */
export function loadChoices(store: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): Partial<AddChoices> {
	try {
		const v = JSON.parse(store?.getItem(key) ?? '{}');
		return v && typeof v === 'object' && typeof v.release_gate === 'string' ? { release_gate: v.release_gate } : {};
	} catch {
		return {};
	}
}

/** saveChoices remembers add choices for next time. */
export function saveChoices(c: AddChoices, store: Pick<Storage, 'setItem'> | undefined = globalThis.localStorage) {
	try {
		store?.setItem(key, JSON.stringify(c));
	} catch {
		// Private windows may refuse; the defaults still work.
	}
}

/** pickId keeps a remembered id only while it is still offered. */
export function pickId(remembered: number | undefined, offered: { id: number }[], fallback: number): number {
	return remembered && offered.some((o) => o.id === remembered) ? remembered : fallback;
}

export interface SecondVersion {
	label: string;
	pick_rules_id: number;
	library_folder_id: number;
}

/**
 * secondVersion suggests a 4K copy next to the main one: the 4K Pick rules
 * when there are some, and a Library folder whose name or path says 4K or
 * UHD, else the main folder (the copy then gets its own folder inside it).
 */
export function secondVersion(
	rules: { id: number; name: string }[],
	folders: { id: number; label: string; path: string }[],
	mainFolder: number
): SecondVersion {
	const fourK = rules.find((r) => r.name.toLowerCase() === '4k') ?? rules.find((r) => /4k|2160|uhd/i.test(r.name));
	const folder = folders.find((f) => f.id !== mainFolder && /4k|2160|uhd/i.test(`${f.label} ${f.path}`));
	return { label: '4K', pick_rules_id: fourK?.id ?? rules[0]?.id ?? 0, library_folder_id: folder?.id ?? mainFolder };
}

/** slotNotes are the Versions' notes worth showing under the Why line: once each, without the gate notes the Why line already gives. */
export function slotNotes(slots: { reason?: string | null }[]): string[] {
	return [...new Set(slots.map((s) => s.reason ?? '').filter((r) => r && !r.startsWith('release gate:')))];
}

/** inLibraryWord says which copies of an episode are in the Library: "" when none; " · in the Library" with one Version; the Version names when it has several. */
export function inLibraryWord(state: string | undefined, versions?: { label: string; state: string }[]): string {
	if (versions && versions.length > 1) {
		const have = versions.filter((v) => v.state === 'in_library').map((v) => v.label);
		return have.length ? ` · in the Library: ${have.join(', ')}` : '';
	}
	return state === 'in_library' ? ' · in the Library' : '';
}

/** The letters of the jump rail, A to Z then # for names that start with a number or sign. */
export const letters = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ', '#'];

/** letterOf is the rail letter a name files under ("Élan" under E, "2 Guns" under #). */
export function letterOf(name: string): string {
	const c = name.trim().normalize('NFD').replace(/[̀-ͯ]/g, '').charAt(0).toUpperCase();
	return c >= 'A' && c <= 'Z' ? c : '#';
}

/** firstOfLetter maps each letter to the id of the first title under it, in the order given. */
export function firstOfLetter(ts: Pick<TitleView, 'id' | 'name'>[]): Map<string, number> {
	const out = new Map<string, number>();
	for (const t of ts) {
		const l = letterOf(t.name);
		if (!out.has(l)) out.set(l, t.id);
	}
	return out;
}

const viewDefault: Record<Filter, StatusKind[]> = {
	all: ['in_library'],
	have: ['in_library'],
	missing: ['missing'],
	not_out_yet: ['coming_soon'],
	better: ['in_library']
};

/**
 * exceptionChip keeps a poster's status only when it says something the view
 * does not already say: "In your Library" is dropped in the All and In library
 * views, but "Missing · 3" (a count) and any other status stay.
 */
export function exceptionChip<T extends { kind: StatusKind; label: string }>(f: Filter, chip: T | null): T | null {
	if (!chip) return null;
	return viewDefault[f].includes(chip.kind) && !/\d/.test(chip.label) ? null : chip;
}

/**
 * posterStatus is the one status a Library poster or row shows: for staff
 * the Library state, for members the shared words of title-status.ts. A show or an artist with missing
 * episodes or tracks puts the count on the shared word ("Missing · 3"); null when nothing is kept yet.
 */
export function posterStatus(t: TitleView, who: Audience): TitleStatus | null {
	// Staff see the Library state (library-state.ts), the same words as the Status column.
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
		// Members keep the unit ("Requested · 3 episodes"): their word does not say what is counted.
		const unit = t.kind === 'show' ? 'episode' : 'track';
		return { kind: st.kind, label: `${st.label} · ${s.missing} ${unit}${s.missing === 1 ? '' : 's'}` };
	}
	if (s.in_library > 0 || s.not_out_yet > 0 || facts.ask) return titleStatus({ titleKind, tracked: true, ask: facts.ask, inLibrary: s.in_library > 0, comingSoon: s.not_out_yet > 0 }, who);
	return null;
}

/** missingEpisodes keeps the episodes that are kept, have aired and have no copy yet (wanted or on the way). */
export function missingEpisodes<E extends { keep_up_to_date: boolean; slots: { state: string }[] }>(eps: E[]): E[] {
	return eps.filter((e) => e.keep_up_to_date && e.slots.some((s) => s.state === 'wanted' || busyStates.includes(s.state)));
}
