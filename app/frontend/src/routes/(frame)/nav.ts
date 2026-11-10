// The app's places, grouped the way the sidebar shows them. Pure, so the permission rules are testable without a browser.

import type { IconName } from '#lib/ui/Icon.svelte';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type Need = 'manage_library' | 'settings' | 'wishlist' | 'approve' | 'staff' | 'manage' | null;

export interface NavItem {
	href: string;
	label: string;
	icon: IconName;
	need: Need;
	/** Other paths that count as this place (for the selected row). */
	also?: string[];
	badge?: number;
	badgeLabel?: string;
	/** Overall progress, 0 to 100, drawn as a ring around the icon. */
	ring?: number;
	/** A small "needs attention" flag (Settings, for people who change settings). */
	dot?: boolean;
}

export interface NavSection {
	id: 'top' | 'library' | 'yours' | 'manage';
	label: string | null;
	items: NavItem[];
}

export interface NavInput {
	allowed: (need: Exclude<Need, null>) => boolean;
	/** True when the part that owns this path is turned off. */
	off: (href: string) => boolean;
	/** Library kinds whose part is on: movie, show, artist. */
	kinds: ('movie' | 'show' | 'artist')[];
	waitingAsks: number;
	decisions: number;
	/** Downloads moving now and their average percent (staff only). */
	active?: { count: number; percent: number };
	unread: number;
	/** A Kid (age-rating limit) never sees music: it carries no rating. */
	kid?: boolean;
	/** Something in Settings needs a fix (only known to people who change settings). */
	settingsAttention?: boolean;
}

export interface Destination {
	href: string;
	label: string;
	icon: IconName;
	section: string;
}

export type SidebarPref = 'expanded' | 'collapsed';

export type ShellMode = 'phone' | 'tablet' | 'desktop';

const kindItem = (k: 'movie' | 'show' | 'artist'): NavItem => {
	if (k === 'movie') return { href: '/library?kind=movie', label: t('frame.nav.movies'), icon: 'film', need: null };
	if (k === 'show') return { href: '/library?kind=show', label: t('frame.nav.shows'), icon: 'tv', need: null };
	return { href: '/library?kind=artist', label: t('frame.nav.music'), icon: 'music', need: null };
};

/** The sections this person may see, empty ones left out. */
export function buildNav(input: NavInput): NavSection[] {
	const ok = (i: NavItem) => (i.need === null || input.allowed(i.need)) && !input.off(i.href.split('?')[0]);
	const approver = input.allowed('approve');
	const sections: NavSection[] = [
		{
			id: 'top',
			label: null,
			items: [
				{ href: '/', label: t('frame.nav.home'), icon: 'home', need: null },
				{ href: '/explore', label: t('frame.nav.explore'), icon: 'compass', need: null },
				{ href: '/popular', label: t('frame.nav.newPopular'), icon: 'sparkle', need: null }
			]
		},
		{
			id: 'library',
			label: t('frame.nav.library'),
			items: [
				// Movie series live in the Library's Filter menu and its Group by series choice.
				...input.kinds.filter((k) => !(input.kid && k === 'artist')).map((k) => kindItem(k))
			]
		},
		{
			id: 'yours',
			label: t('frame.nav.yours'),
			items: [
				// Everyone else watches the downloads for what they asked for.
				...(!input.allowed('manage_library') ? [{ href: '/downloads', label: t('frame.nav.downloads'), icon: 'download', need: null } as NavItem] : []),
				{ href: '/calendar', label: t('frame.nav.calendar'), icon: 'calendar', need: null },
				// Everyone finds Requests here; people who approve see how many are waiting.
				{ href: '/requests', label: t('frame.nav.requests'), icon: 'requests', need: 'wishlist', ...(approver ? { badge: input.waitingAsks, badgeLabel: t('frame.nav.waiting', { n: input.waitingAsks }) } : { badge: input.unread, badgeLabel: t('frame.nav.new', { n: input.unread }) }) }
			]
		},
		{
			id: 'manage',
			label: t('frame.nav.manage'),
			items: input.allowed('manage') || input.allowed('staff')
				? [
						{ href: '/downloads', label: t('frame.nav.downloads'), icon: 'download', need: 'manage_library', also: ['/downloads/decide'], ...downloadsBadge(input) },
						{ href: '/downloads/history', label: t('frame.nav.history'), icon: 'clock', need: 'manage_library' },
						{ href: '/health', label: t('frame.nav.status'), icon: 'shield', need: 'manage_library' },
						{ href: '/settings', label: t('frame.nav.settings'), icon: 'settings', need: 'staff', ...(input.settingsAttention && input.allowed('settings') ? { dot: true } : {}) }
					]
				: []
		}
	];
	return sections.map((s) => ({ ...s, items: s.items.filter(ok) })).filter((s) => s.items.length > 0);
}

/** Whether a nav item is the page at path (with its query). */
export function isCurrent(item: Pick<NavItem, 'href' | 'also'>, path: string, search: Pick<URLSearchParams, 'get'>): boolean {
	const [base, query] = item.href.split('?');
	if (query) {
		const want = new URLSearchParams(query);
		if (path !== base) return false;
		for (const [k, v] of want.entries()) if (search.get(k) !== v) return false;
		return true;
	}
	if (item.also?.some((a) => path === a || path.startsWith(a + '/'))) return true;
	if (base === '/') return path === '/';
	return path === base || path.startsWith(base + '/');
}

/** Picks the single most specific item that matches, so only one row is selected. */
export function currentHref(items: Pick<NavItem, 'href' | 'also'>[], path: string, search: Pick<URLSearchParams, 'get'>): string | null {
	const hits = items.filter((i) => isCurrent(i, path, search));
	// Exact query matches beat plain prefixes; longer paths beat shorter ones.
	hits.sort((a, b) => Number(b.href.includes('?')) - Number(a.href.includes('?')) || b.href.length - a.href.length);
	return hits[0]?.href ?? null;
}

/** Flattens the sections for the command palette, one per href and label. */
export function destinations(sections: NavSection[], extra: Destination[] = []): Destination[] {
	const out: Destination[] = [];
	const seen = new Set<string>();
	for (const s of sections)
		for (const i of s.items) {
			const key = i.href + i.label;
			if (seen.has(key)) continue;
			seen.add(key);
			out.push({ href: i.href, label: i.label, icon: i.icon, section: s.label ?? t('frame.nav.goTo') });
		}
	for (const d of extra) if (!seen.has(d.href + d.label)) out.push(d);
	return out;
}

/** Ranks places for a typed query: starts-with first, then contains. */
export function filterDestinations(list: Destination[], query: string): Destination[] {
	const q = query.trim().toLowerCase();
	if (!q) return list;
	const scored = list
		.map((d) => {
			const l = d.label.toLowerCase();
			const words = l.split(/\s+/);
			const score = l.startsWith(q) ? 0 : words.some((w) => w.startsWith(q)) ? 1 : l.includes(q) || d.section.toLowerCase().includes(q) ? 2 : -1;
			return { d, score };
		})
		.filter((x) => x.score >= 0);
	scored.sort((a, b) => a.score - b.score);
	return scored.map((x) => x.d);
}

export const SIDEBAR_KEY = 'holyarrs.sidebar';

/** Reads the saved choice; storage may be missing or refuse. */
export function readSidebarPref(storage: Pick<Storage, 'getItem'> | undefined): SidebarPref {
	try {
		return storage?.getItem(SIDEBAR_KEY) === 'collapsed' ? 'collapsed' : 'expanded';
	} catch {
		return 'expanded';
	}
}

/** Saves the choice; a refusal only means it lasts this visit. */
export function writeSidebarPref(storage: Pick<Storage, 'setItem'> | undefined, pref: SidebarPref): void {
	try {
		storage?.setItem(SIDEBAR_KEY, pref);
	} catch {
		// Private windows may refuse storage.
	}
}

/** Maps a viewport width onto the three shell layouts. */
export function shellMode(width: number): ShellMode {
	return width < 768 ? 'phone' : width < 1024 ? 'tablet' : 'desktop';
}

/** Where the logo goes: Home, which every signed-in person may use (members see their own shelves). */
export function homeHref(): string {
	return '/';
}

function downloadsBadge(input: NavInput): Pick<NavItem, 'badge' | 'badgeLabel' | 'ring'> {
	const a = input.active;
	if (a && a.count > 0) return { badge: a.count, badgeLabel: t('frame.nav.active', { n: a.count, percent: a.percent }), ring: a.percent };
	return { badge: input.decisions, badgeLabel: t('frame.nav.decide', { n: input.decisions }) };
}
