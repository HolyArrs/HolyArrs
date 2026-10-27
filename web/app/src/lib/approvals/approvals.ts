// Approvals page looks: the five design variants Luis compares on live data
// (?view=1…5; no view is the page as it is today) and the small rules they share.

import type { Ask } from '../types.ts';
import type { StatusKind } from '../ui/StatusPill.svelte';
import type { AskTab } from '../wishlist.ts';
import type { TitleDetails } from '../title-details.ts';
import type { LibraryFolder, PickRulesView } from '../types.ts';
import { detailPath } from '../title-details.ts';

/** The variants on the bottom bar; 0 is the page as it is today. */
export const approvalViews: { value: number; label: string }[] = [
	{ value: 0, label: 'Now' },
	{ value: 1, label: '1 Cards' },
	{ value: 2, label: '2 By person' },
	{ value: 3, label: '3 Focus' },
	{ value: 4, label: '4 Inbox' },
	{ value: 5, label: '5 Board' }
];

export const viewKey = 'holyarrs.view.approvals';

/** askHref is where a title's name links: the Library page once added, else its title page or artist page. */
export function askHref(a: Pick<Ask, 'kind' | 'tmdb_id' | 'ref' | 'title_id'>): string {
	if (a.title_id && a.title_id > 0) return `/library/${a.title_id}`;
	if (a.kind === 'artist') {
		const mbid = a.ref?.startsWith('mbid:') ? a.ref.slice(5) : '';
		return mbid ? `/music/artist/${encodeURIComponent(mbid)}` : '';
	}
	if (!a.tmdb_id) return '';
	return detailPath(a.kind === 'show' ? 'show' : 'movie', a.tmdb_id);
}

/** kindWord names an ask's kind for a small tag. */
export function kindWord(kind: string): string {
	return kind === 'show' ? 'Show' : kind === 'artist' ? 'Music' : 'Movie';
}

/** A person who asked, with what the page knows about them. */
export interface Asker {
	id: number;
	name: string;
	avatar: string | null;
	/** "Kid · up to PG", or '' for no age limit. */
	kid: string;
}

/** A household member as GET /users answers it (only the parts this page reads). */
export interface Member {
	id: number;
	name: string;
	avatar?: string | null;
	max_rating?: string;
}

/** personGroups puts asks under the first person who asked, biggest group first, then by name. */
export function personGroups<T extends Pick<Ask, 'asked_by' | 'people'>>(asks: T[]): { id: number; name: string; asks: T[] }[] {
	const by = new Map<number, { id: number; name: string; asks: T[] }>();
	for (const a of asks) {
		const who = a.people[0] ?? a.asked_by;
		const g = by.get(who.id) ?? { id: who.id, name: who.name, asks: [] };
		g.asks.push(a);
		by.set(who.id, g);
	}
	return [...by.values()].sort((x, y) => y.asks.length - x.asks.length || x.name.localeCompare(y.name));
}

/** The board's columns, left to right. */
export type BoardColumn = 'waiting' | 'looking' | 'downloading' | 'done' | 'turned_down';
export const boardColumns: { col: BoardColumn; label: string; short: string }[] = [
	{ col: 'waiting', label: 'Waiting', short: 'Waiting' },
	{ col: 'looking', label: 'Approved, looking', short: 'Looking' },
	{ col: 'downloading', label: 'Downloading', short: 'Getting' },
	{ col: 'done', label: 'Done', short: 'Done' },
	{ col: 'turned_down', label: 'Turned down', short: 'Turned down' }
];

/** boardColumn says where an ask sits on the board; downloading holds the Library ids with a download running. */
export function boardColumn(a: Pick<Ask, 'state' | 'title_id'>, downloading: ReadonlySet<number>): BoardColumn {
	if (a.state === 'asked') return 'waiting';
	if (a.state === 'turned_down') return 'turned_down';
	if (a.state === 'available') return 'done';
	return a.title_id && downloading.has(a.title_id) ? 'downloading' : 'looking';
}

/** stepIndex keeps a focus position inside a list of n after a move (wraps). */
export function stepIndex(i: number, step: number, n: number): number {
	if (n <= 0) return 0;
	return (((i + step) % n) + n) % n;
}

/** detailKey is the cache key for a title's details; '' when the ask has none to fetch (music, or no TMDB id). */
export function detailKey(a: Pick<Ask, 'kind' | 'tmdb_id'>): string {
	if (a.kind === 'artist' || !a.tmdb_id) return '';
	return `${a.kind === 'show' ? 'show' : 'movie'}:${a.tmdb_id}`;
}

/** runtimeLine says a runtime in words: "1 h 52 min", "48 min". */
export function runtimeLine(min: number | undefined): string {
	if (!min) return '';
	const h = Math.floor(min / 60);
	const m = min % 60;
	return h ? (m ? `${h} h ${m} min` : `${h} h`) : `${m} min`;
}

/** What every variant gets from the page: the asks, the real handlers and the shared words. */
export interface ApprovalsCtx {
	/** The asks in the chosen tab, kind and Mine filter. */
	readonly shown: Ask[];
	/** Every ask in the kind and Mine filter, all tabs (the board). */
	readonly viewed: Ask[];
	readonly tab: AskTab;
	readonly counts: Record<AskTab, number>;
	setTab(t: AskTab): void;
	readonly approver: boolean;
	readonly choosable: boolean;
	isChosen(id: number): boolean;
	toggleChosen(id: number): void;
	/** Choose all of these, or clear the choice when all are chosen already. */
	chooseAll(list: Ask[]): void;
	isPending(id: number): boolean;
	/** Approve with the Undo toast (the quick ✓). */
	approve(list: Ask[]): void;
	/** Turn down with an optional note (the sheet with Undo). */
	turnDown(list: Ask[]): void;
	/** The Approve with options dialog. */
	withOptions(a: Ask): void;
	/** The review sheet for one ask. */
	review(a: Ask, trigger?: HTMLElement): void;
	/** Take back one of my own waiting asks (confirm sheet). */
	takeBack(a: Ask): void;
	/** Decide straight away with options and a note; resolves to an error message, or '' when done. */
	decideNow(a: Ask, mode: 'approve' | 'turn_down', note: string, folderId: number, rulesId: number): Promise<string>;
	foldersFor(a: Ask): LibraryFolder[];
	rulesFor(a: Ask): PickRulesView[];
	who(a: Ask): Asker;
	what(a: Ask): string;
	year(a: Ask): string;
	askedBy(a: Ask): string;
	decided(a: Ask): string;
	pill(a: Ask): { kind: StatusKind; label: string };
	approvedLine(a: Ask): string;
	href(a: Ask): string;
	details(a: Ask): TitleDetails | null | undefined;
	want(a: Ask): void;
	readonly downloading: ReadonlySet<number>;
}
