// What needs attention in Settings. The server works it out
// (GET /settings/status); this file turns its answer into page and group
// states for the Settings home and sidebar.
export type Severity = 'info' | 'warning' | 'error';

/** ServerStatus is the answer of GET /api/v1/settings/status. */
export interface ServerStatus {
	// group is the server's own grouping; the app sums pages by their href,
	// so the menu can be regrouped without the server knowing.
	items: { group: string; page: string; text: string; severity: Severity }[];
	unset: Partial<Record<string, string[]>>;
	known: string[];
	metadata_up?: boolean;
}

export interface Attention {
	text: string;
	href: string;
	group: string;
	severity: Severity;
}

export type GroupState = 'ok' | 'attention' | 'unset' | 'unknown';

export interface StatusAnswer {
	items: Attention[];
	unset: Partial<Record<string, string[]>>;
	/** Pages whose status is known: true when they are fine. */
	known: Set<string>;
	metadataUp: boolean;
}

/** fromServer turns the status route's answer into a StatusAnswer; null is "nothing known". */
export function fromServer(a: ServerStatus | null): StatusAnswer {
	return {
		items: (a?.items ?? []).map((i) => ({ group: i.group, href: i.page, text: i.text, severity: i.severity })),
		unset: { ...(a?.unset ?? {}) },
		known: new Set(a?.known ?? []),
		metadataUp: a?.metadata_up !== false
	};
}

/** groupState sums up the pages of one group: something to fix, not set up, all set, or not known. */
export function groupState(a: StatusAnswer, hrefs: string[]): { state: GroupState; count: number } {
	const mine = new Set(hrefs);
	const count = a.items.filter((i) => mine.has(i.href)).length;
	if (count) return { state: 'attention', count };
	if (Object.values(a.unset).some((hs) => hs?.some((h) => mine.has(h)))) return { state: 'unset', count: 0 };
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
