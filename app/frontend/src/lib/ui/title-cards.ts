// The card a row shows, however it was made, and the marks that say what the Library
// already holds. Pure functions so they can be tested.

import { pickFrom, type CardArtPick } from './card-art.ts';
import type { Fresh } from './fresh.ts';
import type { StatusKind } from './StatusPill.svelte';
import { audienceStatus, factsOf, titleStatus, type Audience, type TitleStatus } from './title-status.ts';
import type { Ask, AskBrief, AskState, TitleView } from '../core/types.ts';

export type CardKind = 'movie' | 'show' | 'artist';

export type CardStatus = StatusKind;

/** ExploreCard is one poster in a row, whatever it came from. */
export interface ExploreCard {
	/** Unique within its row. */
	key: string;
	kind: CardKind;
	name: string;
	year?: number;
	image?: string | null;
	subtitle?: string;
	/** Set when the title is in the Library. */
	titleId?: number;
	/** How it is added or asked for, such as tmdb:693134 (not in the Library). */
	ref?: string;
	status?: CardStatus;
	statusLabel?: string;
	badges: string[];
	/** Whether the signed-in person already asked for it. */
	mine?: boolean;
	/** view-transition-name; only the first card of a title on a page gets one. */
	vtName?: string;
	/** The open ask for it, so asking says it joins that ask. */
	ask?: AskBrief;
	/** 16:9 picture for wide cards: backdrop, else episode still. */
	wide?: string | null;
	/** An ask for it was turned down (shown to staff only). */
	declined?: boolean;
	/** The server's one freshness strip. */
	fresh?: Fresh | null;
	/** The wide card's picture, picked on the server (card system). */
	art?: CardArtPick;
	/** On one of the member's streaming services. */
	streaming?: CardStreaming;
}

export interface ExploreRow {
	id: string;
	title: string;
	subtitle?: string;
	href?: string;
	cards: ExploreCard[];
	/** Shown even with no cards (a row with its own picker). */
	keep?: boolean;
	/** Server rows: the row type, and wide cards (Coming soon). */
	type?: string;
	wide?: boolean;
	/** Server rows: where the title came from (fixed, plain or a template layer), for measuring. */
	titleLayer?: string;
}

export const ROW_MAX = 20;

const kindWord: Record<CardKind, string> = { movie: 'Movie', show: 'Show', artist: 'Artist' };

export function kindOf(k: string): CardKind {
	return k === 'show' || k === 'artist' ? k : 'movie';
}

/** quality is the best file's resolution in plain marks: 4K, 1080p, 720p. */
export function quality(t: Pick<TitleView, 'slots'>): string | undefined {
	let best = 0;
	for (const s of t.slots ?? []) {
		const r = Number((s.file?.Quality as { resolution?: unknown } | null | undefined)?.resolution ?? 0);
		if (r > best) best = r;
	}
	if (best >= 2000) return '4K';
	if (best >= 1000) return '1080p';
	if (best >= 700) return '720p';
	return best > 0 ? `${best}p` : undefined;
}

/** titleCard turns a Library title into a card. */
export function titleCard(t: TitleView, subtitle?: string, key = `t${t.id}`): ExploreCard {
	const s = titleStatus(factsOf(t), 'staff');
	const q = quality(t);
	return {
		key,
		kind: kindOf(t.kind),
		name: t.name,
		year: t.year,
		image: t.artwork?.[0] ?? null,
		subtitle: subtitle ?? (t.year ? String(t.year) : kindWord[kindOf(t.kind)]),
		titleId: t.id,
		status: s.kind,
		badges: q ? [q] : []
	};
}

/** tmdbArtName is the view-transition name of a not-added movie or show's art (card and hero). */
export const tmdbArtName = (kind: 'movie' | 'show', tmdb: string) => `tmdb-art-${kind}-${tmdb}`;

/**
 * cardPill is the StatusPill a card shows, in the shared status words
 * (title-status.ts) for this audience; a label set on the card wins.
 */
export function cardPill(c: ExploreCard, who: Audience): TitleStatus | null {
	if (!c.status) return null;
	if (c.statusLabel) return { kind: c.status, label: c.statusLabel };
	return audienceStatus(c.status, who, { tracked: c.titleId !== undefined, titleKind: c.kind });
}

/** cardAction says what the round + does for this person on this card. */
export function cardAction(c: ExploreCard, can: { manage: boolean; ask: boolean }): 'add' | 'ask' | null {
	if (c.titleId !== undefined || c.status === 'in_library') return null;
	if (can.manage) return 'add';
	if (can.ask && !c.mine && c.ref) return 'ask';
	return null;
}

/** followsByTmdb: the hover card's bell follows a title not in the Library by TMDB id: a show, or a movie not out yet. */
export function followsByTmdb(c: Pick<ExploreCard, 'kind' | 'titleId' | 'fresh' | 'year'>): boolean {
	return !c.titleId && (c.kind === 'show' || c.fresh?.kind === 'coming' || (!!c.year && c.year > new Date().getFullYear()));
}

/** cardTmdb is the TMDB id of a movie or show card not in the Library, or ''. */
export function cardTmdb(c: Pick<ExploreCard, 'kind' | 'ref'>): string {
	if ((c.kind !== 'movie' && c.kind !== 'show') || !c.ref?.startsWith('tmdb:')) return '';
	return c.ref.slice(5);
}

export interface ProviderItem {
	tmdb_id: string;
	kind: 'movie' | 'show';
	name: string;
	year?: number;
	poster?: string | null;
	backdrop?: string | null;
	overview?: string;
	genres?: string[];
	in_library: boolean;
	title_id?: number;
	ask?: { id: number; state: Ask['state']; state_label: string; mine: boolean; people: number } | null;
	fresh?: Fresh | null;
	/** Explore: the YouTube trailer, when the server already knows it. */
	trailer_key?: string;
	/** The wide card's picture, picked on the server (card system). */
	card_art?: string;
	card_logo?: string;
	art_has_title?: boolean;
	/** Explore: the member's streaming service the title is on (kept lists only). */
	streaming?: CardStreaming;
}

/** CardStreaming: on one of the member's services; rank is that service's own top today. */
export interface CardStreaming {
	service: number;
	name: string;
	rank?: number;
}

/** discoverCard turns a discover item into a card. */
export function discoverCard(d: ProviderItem, row: string): ExploreCard {
	return {
		key: `${row}-${d.kind}-${d.tmdb_id}`,
		kind: d.kind,
		name: d.name,
		year: d.year,
		image: d.poster ?? null,
		subtitle: d.year ? String(d.year) : undefined,
		titleId: d.in_library ? d.title_id : undefined,
		ref: `tmdb:${d.tmdb_id}`,
		status: d.in_library ? 'in_library' : d.ask && d.ask.state !== 'turned_down' ? titleStatus({ ask: d.ask.state }, 'staff').kind : undefined,
		badges: [],
		fresh: d.fresh ?? null,
		art: pickFrom(d),
		mine: !!d.ask?.mine,
		ask: d.ask && d.ask.state !== 'turned_down' ? d.ask : undefined,
		...(d.streaming ? { streaming: d.streaming } : {}),
		...(d.ask?.state === 'turned_down' ? { declined: true } : {})
	};
}

/**
 * liveCard is a card after something happened to it on this page: asked
 * (its pill follows the ask state the server returned, so an auto-approved
 * ask reads Approved) or refused because it is in the Library already.
 */
export function liveCard(c: ExploreCard, here: { ask?: AskState; titleId?: number } | undefined): ExploreCard {
	if (here?.titleId !== undefined) return { ...c, titleId: here.titleId, status: 'in_library', statusLabel: undefined };
	if (here?.ask) return { ...c, mine: true, status: titleStatus({ ask: here.ask }, 'staff').kind, statusLabel: undefined };
	return c;
}

/** OwnedState is the round status icon on a poster; none means not in the Library and not asked. */
export type OwnedState = 'available' | 'partial' | 'downloading' | 'requested' | 'declined';

export interface OwnedMark {
	state: OwnedState;
	/** aria-label and tooltip. */
	label: string;
}

/** ownedKey is how a title is found again: kind plus its TMDB id (MusicBrainz id for artists). */
function ownedKeys(kind: CardKind, ids: Record<string, string> | null | undefined): string[] {
	const out: string[] = [];
	if (ids?.tmdb) out.push(`${kind}:tmdb:${ids.tmdb}`);
	if (ids?.musicbrainz) out.push(`${kind}:mbid:${ids.musicbrainz}`);
	return out;
}

/** OwnedIndex finds Library titles by id or by their TMDB / MusicBrainz id. */
export interface OwnedIndex {
	byId: Map<number, TitleView>;
	byRef: Map<string, TitleView>;
}

export function ownedIndex(titles: TitleView[]): OwnedIndex {
	const byId = new Map<number, TitleView>();
	const byRef = new Map<string, TitleView>();
	for (const t of titles) {
		byId.set(t.id, t);
		const mbid = t.artist?.mbid;
		for (const k of ownedKeys(kindOf(t.kind), { ...(t.ids ?? {}), ...(mbid ? { musicbrainz: mbid } : {}) })) byRef.set(k, t);
	}
	return { byId, byRef };
}

/** ownedTitle is the Library title behind a card, if any. */
export function ownedTitle(c: Pick<ExploreCard, 'kind' | 'ref' | 'titleId'>, idx: OwnedIndex): TitleView | undefined {
	if (c.titleId !== undefined) return idx.byId.get(c.titleId);
	if (!c.ref) return undefined;
	const [scheme, id] = [c.ref.slice(0, c.ref.indexOf(':')), c.ref.slice(c.ref.indexOf(':') + 1)];
	if (!id || (scheme !== 'tmdb' && scheme !== 'mbid')) return undefined;
	return idx.byRef.get(`${c.kind}:${scheme}:${id}`);
}

const fullStates = new Set(['in_library', 'better_wanted', 'no_hardlink']);

const ownedWords: Record<OwnedState, string> = {
	available: 'In your library',
	partial: 'Partly available',
	downloading: 'Downloading',
	requested: 'Requested',
	declined: 'Declined'
};

/**
 * ownedMark says which round icon a card shows. A Library title is checked
 * first (downloading, then all or part of its files); otherwise the card's
 * own ask. Declined shows only to people who manage or approve (staff).
 */
export function ownedMark(c: ExploreCard, t: TitleView | undefined, who: Audience): OwnedMark | null {
	const mark = (state: OwnedState, label = ownedWords[state]): OwnedMark => ({ state, label });
	if (t) {
		const f = factsOf(t);
		if (f.downloading) return mark('downloading', f.percent !== undefined ? `Downloading ${f.percent}%` : undefined);
		if (f.inLibrary) {
			const slots = t.slots ?? [];
			const part = (t.show?.missing ?? 0) > 0 || (t.artist?.missing ?? 0) > 0 || slots.some((s) => !fullStates.has(s.state) && !s.file && s.state !== 'not_out_yet');
			return mark(part ? 'partial' : 'available');
		}
		if (f.ask === 'turned_down') return who === 'staff' ? mark('declined') : null;
		return mark('requested', f.ask === 'approved' ? 'Approved, waiting for a copy' : f.ask ? 'Requested' : 'Waiting for a copy');
	}
	switch (c.status) {
		case 'in_library':
			return mark('available');
		case 'downloading':
			return mark('downloading');
		case 'missing':
			return c.titleId !== undefined ? mark('requested', 'Waiting for a copy') : null;
		case 'requested':
			return mark('requested');
		case 'approved':
			return mark('requested', 'Approved, waiting for a copy');
		case 'turned_down':
			return who === 'staff' && c.ask?.state === 'turned_down' ? mark('declined') : null;
	}
	return c.declined && who === 'staff' ? mark('declined') : null;
}

/** matchOwned ties a card to its Library title (found by TMDB or MusicBrainz id), so owned cards open the title page and lose the +. */
export function matchOwned(c: ExploreCard, idx: OwnedIndex): ExploreCard {
	const t = ownedTitle(c, idx);
	if (!t || c.titleId === t.id) return c;
	return { ...c, titleId: t.id, status: titleStatus(factsOf(t), 'staff').kind, statusLabel: undefined };
}

/** cardIdentity names a title across rows (row keys differ per row). */
export function cardIdentity(c: Pick<ExploreCard, 'key' | 'kind' | 'ref' | 'titleId'>): string {
	if (c.ref) return `${c.kind}:${c.ref}`;
	if (c.titleId !== undefined) return `t${c.titleId}`;
	return c.key;
}
