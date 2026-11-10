import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type Severity = 'info' | 'warning' | 'error';

export interface ServerStatus {
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
	known: Set<string>;
	metadataUp: boolean;
}

// null means nothing is known.
export function fromServer(a: ServerStatus | null): StatusAnswer {
	return {
		items: (a?.items ?? []).map((i) => ({ group: i.group, href: i.page, text: i.text, severity: i.severity })),
		unset: { ...(a?.unset ?? {}) },
		known: new Set(a?.known ?? []),
		metadataUp: a?.metadata_up !== false
	};
}

// Pages are summed by href, so the menu can be regrouped without the server.
export function groupState(a: StatusAnswer, hrefs: string[]): { state: GroupState; count: number } {
	const mine = new Set(hrefs);
	const count = a.items.filter((i) => mine.has(i.href)).length;
	if (count) return { state: 'attention', count };
	if (Object.values(a.unset).some((hs) => hs?.some((h) => mine.has(h)))) return { state: 'unset', count: 0 };
	if (hrefs.some((h) => a.known.has(h))) return { state: 'ok', count: 0 };
	return { state: 'unknown', count: 0 };
}

export function pageState(a: StatusAnswer, href: string): { state: GroupState; count: number } {
	const count = a.items.filter((i) => i.href === href).length;
	if (count) return { state: 'attention', count };
	if (Object.values(a.unset).some((hs) => hs?.includes(href))) return { state: 'unset', count: 0 };
	if (a.known.has(href)) return { state: 'ok', count: 0 };
	return { state: 'unknown', count: 0 };
}

export function stateWords(s: { state: GroupState; count: number }): string {
	if (s.state === 'attention') return s.count === 1 ? t('alerts.state.oneNeeds') : t('alerts.state.manyNeed', { count: s.count });
	if (s.state === 'unset') return t('alerts.state.unset');
	if (s.state === 'ok') return t('alerts.state.ok');
	return '';
}

export function needsAttention(a: StatusAnswer | null): boolean {
	return !!a && a.items.length > 0;
}
