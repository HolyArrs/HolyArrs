// What needs attention in Settings. The server works it out
// (GET /settings/status); this file turns its answer into page and group
// states for the Settings home and sidebar.
import type { GroupId } from './settings.ts';

export type Severity = 'info' | 'warning' | 'error';

/** ServerStatus is the answer of GET /api/v1/settings/status. */
export interface ServerStatus {
	items: { group: GroupId; page: string; text: string; severity: Severity }[];
	unset: Partial<Record<GroupId, string[]>>;
	known: string[];
	tmdb_key_set: boolean;
}

export interface Attention {
	text: string;
	href: string;
	group: GroupId;
	severity: Severity;
}

export type GroupState = 'ok' | 'attention' | 'unset' | 'unknown';

export interface StatusAnswer {
	items: Attention[];
	unset: Partial<Record<GroupId, string[]>>;
	/** Pages whose status is known: true when they are fine. */
	known: Set<string>;
	tmdbKeySet: boolean;
}

/** fromServer turns the status route's answer into a StatusAnswer; null is "nothing known". */
export function fromServer(a: ServerStatus | null): StatusAnswer {
	return {
		items: (a?.items ?? []).map((i) => ({ group: i.group, href: i.page, text: i.text, severity: i.severity })),
		unset: { ...(a?.unset ?? {}) },
		known: new Set(a?.known ?? []),
		tmdbKeySet: !!a?.tmdb_key_set
	};
}

/** groupState sums up one group: something to fix, not set up, all set, or not known. */
export function groupState(a: StatusAnswer, group: GroupId, hrefs: string[]): { state: GroupState; count: number } {
	const count = a.items.filter((i) => i.group === group).length;
	if (count) return { state: 'attention', count };
	if (a.unset[group]?.length) return { state: 'unset', count: 0 };
	if (hrefs.some((h) => a.known.has(h))) return { state: 'ok', count: 0 };
	return { state: 'unknown', count: 0 };
}

/** pageState sums up one page the same way. */
export function pageState(a: StatusAnswer, href: string): { state: GroupState; count: number } {
	const count = a.items.filter((i) => i.href === href).length;
	if (count) return { state: 'attention', count };
	if (Object.values(a.unset).some((hs) => hs?.includes(href))) return { state: 'unset', count: 0 };
	if (a.known.has(href)) return { state: 'ok', count: 0 };
	return { state: 'unknown', count: 0 };
}

/** stateWords says a status in plain words. */
export function stateWords(s: { state: GroupState; count: number }): string {
	if (s.state === 'attention') return s.count === 1 ? '1 needs attention' : `${s.count} need attention`;
	if (s.state === 'unset') return 'Not set up';
	if (s.state === 'ok') return 'All set';
	return '';
}
