// Requests page: the context every Inbox arrangement and the one-by-one review get from the page, and the small rules they share.

import type { StatusKind } from '#lib/ui/StatusPill.svelte';
import type { ApproverTab, AskTab } from './ask-words.ts';
import type { AskInfo, EpisodeInfo } from './inbox.ts';
import type { LifeFacts } from './lifecycle.ts';
import type { TitleDetails } from '../metadata/index.ts';
import type { Ask } from '../core/types.ts';
import type { LibraryFolder, PickRulesView } from '../core/types.ts';
import { detailPath } from '../metadata/index.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** The approver's choices for one ask: Pick rules (0 is the default), audio languages (empty is any), a note, and the seasons or albums kept (absent means all of the ask). */
export interface DecideOptions {
	rulesId: number;
	langs: string[];
	note?: string;
	seasons?: number[];
	/** Single episodes kept in partly ticked seasons. */
	episodes?: { season: number; episodes: number[] }[];
	albums?: string[];
	/** Songs kept in partly ticked albums (recording ids). */
	songs?: { album: string; recordings: string[] }[];
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

/** What every variant gets from the page: the asks, the real handlers and the shared words. */
export interface BoardCtx {
	/** Unread news about this ask came in for this visit (the card shows New). */
	isNew(id: number): boolean;
	/** Open reported problems for the Reports button; -1 hides it (no Library rights). */
	readonly reports: number;
	/** The asks in the chosen tab, kind and Mine filter. */
	readonly shown: Ask[];
	/** What the pipeline is doing for an approved ask right now (searching, downloading, importing, failed); {} when nothing. */
	live(a: Ask): LifeFacts;
	/** Every ask in the kind, person and Mine filter, all tabs. */
	readonly viewed: Ask[];
	readonly tab: AskTab;
	readonly counts: Record<AskTab, number>;
	setTab(t: AskTab): void;
	/** The approvers' three tabs (Pending, Processing, Done); `shown` follows it for approvers. */
	readonly approverTab: ApproverTab;
	/** Asks per approver tab, and how many in Processing failed. */
	readonly approverCounts: Record<ApproverTab, number> & { failed: number };
	setApproverTab(t: ApproverTab): void;
	readonly approver: boolean;
	readonly choosable: boolean;
	isChosen(id: number): boolean;
	toggleChosen(id: number): void;
	/** Choose all of these, or clear the choice when all are chosen already. */
	chooseAll(list: Ask[]): void;
	isPending(id: number): boolean;
	/** Approve with the Undo toast (the quick ✓). */
	approve(list: Ask[]): void;
	/** Approve one ask with the approver's rule, language and note (with the Undo toast). */
	approveWith(a: Ask, opt: DecideOptions): void;
	/** Turn down with an optional note (the sheet with Undo). */
	turnDown(list: Ask[]): void;
	/** Turn down one ask with this note straight into the Undo toast. */
	turnDownWith(a: Ask, note: string): void;
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
	/** Just the name of who settled the ask; empty while open. */
	decidedBy(a: Ask): string;
	/** Others who declined the same title before, display names only: "bo declined it before." */
	alsoDeclined(a: Ask): string;
	pill(a: Ask): { kind: StatusKind; label: string };
	approvedLine(a: Ask): string;
	href(a: Ask): string;
	details(a: Ask): TitleDetails | null | undefined;
	want(a: Ask): void;
	/** Seasons and release dates for the summary (null until known, or music). */
	info(a: Ask): AskInfo | null;
	/** One season's episodes (null until known); asks once. */
	episodes(tmdb: string, season: number): EpisodeInfo[] | null;
	/** The Requests page's default Pick rules for the ask's kind, by name ('' when unknown). */
	defaultRule(a: Ask): string;
	readonly kind: 'all' | 'movie' | 'show' | 'artist';
	setKind(k: 'all' | 'movie' | 'show' | 'artist'): void;
	/** Asked by: a person's id, 0 for everyone. */
	readonly person: number;
	setPerson(id: number): void;
	readonly people: { id: number; name: string; n: number }[];
	/** The signed-in member (the poster size and panel are kept per member). */
	readonly me: number | undefined;
}

/** Where a title's name links: the Library page once added, else its title page or artist page. */
export function askHref(a: Pick<Ask, 'kind' | 'tmdb_id' | 'ref' | 'title_id'>): string {
	if (a.title_id && a.title_id > 0) return `/library/${a.title_id}`;
	if (a.kind === 'artist') {
		const mbid = a.ref?.startsWith('mbid:') ? a.ref.slice(5) : '';
		return mbid ? `/music/artist/${encodeURIComponent(mbid)}` : '';
	}
	if (!a.tmdb_id) return '';
	return detailPath(a.kind === 'show' ? 'show' : 'movie', a.tmdb_id);
}

/** An ask's kind for a small tag. */
export function kindWord(kind: string): string {
	return t(kind === 'show' ? 'requests.kind.show' : kind === 'artist' ? 'requests.kind.artist' : 'requests.kind.movie');
}

/** Keeps a focus position inside a list of n after a move (wraps). */
export function stepIndex(i: number, step: number, n: number): number {
	if (n <= 0) return 0;
	return (((i + step) % n) + n) % n;
}

/** The cache key for a title's details; '' when the ask has none to fetch (music, or no TMDB id). */
export function detailKey(a: Pick<Ask, 'kind' | 'tmdb_id'>): string {
	if (a.kind === 'artist' || !a.tmdb_id) return '';
	return `${a.kind === 'show' ? 'show' : 'movie'}:${a.tmdb_id}`;
}

