import type { MenuItem } from '#lib/ui/Menu.svelte';

// One of the extra episode orders TMDB lists for a show.
export interface EpisodeGroup {
	id: string;
	name: string;
	type: number;
	seasons: number;
	episodes: number;
}

export interface EpisodeGroupList {
	groups: EpisodeGroup[];
	chosen: string | null;
}

// The save body: a group sets its id; a numbering clears any group first.
export function orderBody(pick: { group: string } | { order: string }, chosen: string | null): Record<string, string> {
	if ('group' in pick) return { episode_group_id: pick.group };
	return chosen ? { episode_group_id: '', episode_order: pick.order } : { episode_order: pick.order };
}

// The Episode order menu: the three numberings, then the show's TMDB orders when it has any.
export function orderMenuItems(a: {
	orders: [string, string][];
	order: string;
	chosen: string | null;
	groups: EpisodeGroup[];
	words: { numbering: string; fromTmdb: string; unavailable: string; counts: (g: EpisodeGroup) => string };
	pick: (body: Record<string, string>) => void;
}): MenuItem[] {
	const plain: MenuItem[] = a.orders.map(([v, l]) => ({ label: l, checked: !a.chosen && a.order === v, onselect: () => a.pick(orderBody({ order: v }, a.chosen)) }));
	// A chosen order TMDB did not list stays checked under a fallback label.
	const lost: MenuItem[] = a.chosen && !a.groups.some((g) => g.id === a.chosen) ? [{ label: a.words.unavailable, checked: true }] : [];
	if (!a.groups.length) return lost.length ? [...plain, { kind: 'separator' }, ...lost] : plain;
	return [
		{ kind: 'heading', label: a.words.numbering },
		...plain,
		{ kind: 'separator' },
		{ kind: 'heading', label: a.words.fromTmdb },
		...a.groups.map((g) => ({ label: g.name, sub: a.words.counts(g), checked: a.chosen === g.id, onselect: () => a.pick(orderBody({ group: g.id }, a.chosen)) })),
		...lost
	];
}

// The order picker's label: the chosen order's name, a fallback when TMDB gave none, or the numbering.
export function orderLabel(chosen: { id: string; name: string } | null, plain: string, unavailable: string): string {
	if (!chosen) return plain;
	return chosen.name || unavailable;
}

// Episodes the chosen order leaves out go in their own list.
export function splitOutside<T extends { outside_order?: boolean }>(eps: T[]): { inside: T[]; outside: T[] } {
	return { inside: eps.filter((e) => !e.outside_order), outside: eps.filter((e) => e.outside_order) };
}
