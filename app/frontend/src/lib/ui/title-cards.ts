// Card and row shapes for explore grids, plus helpers that match cards to owned titles.
import { pickFrom, type CardArtPick } from './card-art.ts';
import type { Fresh } from './fresh.ts';
import type { StatusKind } from './StatusPill.svelte';
import { factsOf, titleStatus, type Audience } from './title-status.ts';
import type { Component } from 'svelte';
import type { Ask, AskBrief, AskState, TitleView } from '../core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type CardKind = 'movie' | 'show' | 'artist';

export type CardStatus = StatusKind;

export interface ExploreCard {
	key: string;
	kind: CardKind;
	name: string;
	year?: number;
	image?: string | null;
	subtitle?: string;
	titleId?: number;
	ref?: string;
	status?: CardStatus;
	statusLabel?: string;
	badges: string[];
	mine?: boolean;
	vtName?: string;
	ask?: AskBrief;
	wide?: string | null;
	declined?: boolean;
	fresh?: Fresh | null;
	art?: CardArtPick;
	streaming?: CardStreaming;
}

export interface ExploreRow {
	id: string;
	title: string;
	subtitle?: string;
	href?: string;
	cards: ExploreCard[];
	keep?: boolean;
	type?: string;
	wide?: boolean;
	titleLayer?: string;
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
	trailer_key?: string;
	card_art?: string;
	card_logo?: string;
	art_has_title?: boolean;
	streaming?: CardStreaming;
}

export interface CardStreaming {
	service: number;
	name: string;
	rank?: number;
}

export type OwnedState = 'available' | 'partial' | 'downloading' | 'requested' | 'declined';

export interface OwnedMark {
	state: OwnedState;
	label: string;
}

export interface OwnedIndex {
	byId: Map<number, TitleView>;
	byRef: Map<string, TitleView>;
}

export type QuickAskSheet = Component<
	{ onadded?: (t: TitleView) => void },
	{
		live: (c: ExploreCard) => ExploreCard;
		action: (c: ExploreCard) => 'add' | 'ask' | null;
		busy: (c: ExploreCard) => boolean;
		open: (c: ExploreCard, act: 'add' | 'ask') => Promise<void>;
	}
>;

// Most cards one row shows.
export const ROW_MAX = 20;

const kindWord = (k: CardKind): string => (k === 'movie' ? t('ui.titleCards.movie') : k === 'show' ? t('ui.titleCards.show') : t('ui.titleCards.artist'));

// Slot states that count as having the file.
const fullStates = new Set(['in_library', 'better_wanted', 'no_hardlink']);

const ownedWords = (): Record<OwnedState, string> => ({
	available: t('ui.titleCards.available'),
	partial: t('ui.titleCards.partial'),
	downloading: t('ui.titleCards.downloading'),
	requested: t('ui.titleCards.requested'),
	declined: t('ui.titleCards.declined')
});

export function kindOf(k: string): CardKind {
	return k === 'show' || k === 'artist' ? k : 'movie';
}

// Buckets are loose so odd crops like 1920x800 still read as 1080p.
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

export function titleCard(tv: TitleView, subtitle?: string, key = `t${tv.id}`): ExploreCard {
	const s = titleStatus(factsOf(tv), 'staff');
	const q = quality(tv);
	return {
		key,
		kind: kindOf(tv.kind),
		name: tv.name,
		year: tv.year,
		image: tv.artwork?.[0] ?? null,
		subtitle: subtitle ?? (tv.year ? String(tv.year) : kindWord(kindOf(tv.kind))),
		titleId: tv.id,
		status: s.kind,
		badges: q ? [q] : []
	};
}

export const tmdbArtName = (kind: 'movie' | 'show', tmdb: string) => `tmdb-art-${kind}-${tmdb}`;

export function cardAction(c: ExploreCard, can: { manage: boolean; ask: boolean }): 'add' | 'ask' | null {
	if (c.titleId !== undefined || c.status === 'in_library') return null;
	if (can.manage) return 'add';
	if (can.ask && !c.mine && c.ref) return 'ask';
	return null;
}

// Unreleased or episodic cards have no file yet, so they follow by TMDB id.
export function followsByTmdb(c: Pick<ExploreCard, 'kind' | 'titleId' | 'fresh' | 'year'>): boolean {
	return !c.titleId && (c.kind === 'show' || c.fresh?.kind === 'coming' || (!!c.year && c.year > new Date().getFullYear()));
}

// Only TMDB refs of movies and shows give an id.
export function cardTmdb(c: Pick<ExploreCard, 'kind' | 'ref'>): string {
	if ((c.kind !== 'movie' && c.kind !== 'show') || !c.ref?.startsWith('tmdb:')) return '';
	return c.ref.slice(5);
}

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

// Overlays fresh ask or library state on a cached card.
export function liveCard(c: ExploreCard, here: { ask?: AskState; titleId?: number } | undefined): ExploreCard {
	if (here?.titleId !== undefined) return { ...c, titleId: here.titleId, status: 'in_library', statusLabel: undefined };
	if (here?.ask) return { ...c, mine: true, status: titleStatus({ ask: here.ask }, 'staff').kind, statusLabel: undefined };
	return c;
}

// Indexes titles by id and by external ref so cards from providers can find them.
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

export function ownedTitle(c: Pick<ExploreCard, 'kind' | 'ref' | 'titleId'>, idx: OwnedIndex): TitleView | undefined {
	if (c.titleId !== undefined) return idx.byId.get(c.titleId);
	if (!c.ref) return undefined;
	const [scheme, id] = [c.ref.slice(0, c.ref.indexOf(':')), c.ref.slice(c.ref.indexOf(':') + 1)];
	if (!id || (scheme !== 'tmdb' && scheme !== 'mbid')) return undefined;
	return idx.byRef.get(`${c.kind}:${scheme}:${id}`);
}

// Picks the badge for a card; declined only shows to staff.
export function ownedMark(c: ExploreCard, tv: TitleView | undefined, who: Audience): OwnedMark | null {
	const mark = (state: OwnedState, label = ownedWords()[state]): OwnedMark => ({ state, label });
	if (tv) {
		const f = factsOf(tv);
		if (f.downloading) return mark('downloading', f.percent !== undefined ? t('ui.titleCards.downloadingPercent', { percent: f.percent }) : undefined);
		if (f.inLibrary) {
			const slots = tv.slots ?? [];
			const part = (tv.show?.missing ?? 0) > 0 || (tv.artist?.missing ?? 0) > 0 || slots.some((s) => !fullStates.has(s.state) && !s.file && s.state !== 'not_out_yet');
			return mark(part ? 'partial' : 'available');
		}
		if (f.ask === 'turned_down') return who === 'staff' ? mark('declined') : null;
		return mark('requested', f.ask === 'approved' ? t('ui.titleCards.approvedWaiting') : f.ask ? t('ui.titleCards.requested') : t('ui.titleCards.waitingForCopy'));
	}
	switch (c.status) {
		case 'in_library':
			return mark('available');
		case 'downloading':
			return mark('downloading');
		case 'missing':
			return c.titleId !== undefined ? mark('requested', t('ui.titleCards.waitingForCopy')) : null;
		case 'requested':
			return mark('requested');
		case 'approved':
			return mark('requested', t('ui.titleCards.approvedWaiting'));
		case 'turned_down':
			return who === 'staff' && c.ask?.state === 'turned_down' ? mark('declined') : null;
	}
	return c.declined && who === 'staff' ? mark('declined') : null;
}

export function matchOwned(c: ExploreCard, idx: OwnedIndex): ExploreCard {
	const t = ownedTitle(c, idx);
	if (!t || c.titleId === t.id) return c;
	return { ...c, titleId: t.id, status: titleStatus(factsOf(t), 'staff').kind, statusLabel: undefined };
}

// Stable id across rows: provider ref first, then library id, then row key.
export function cardIdentity(c: Pick<ExploreCard, 'key' | 'kind' | 'ref' | 'titleId'>): string {
	if (c.ref) return `${c.kind}:${c.ref}`;
	if (c.titleId !== undefined) return `t${c.titleId}`;
	return c.key;
}

// Artists match by MusicBrainz id, the rest by TMDB id.
function ownedKeys(kind: CardKind, ids: Record<string, string> | null | undefined): string[] {
	const out: string[] = [];
	if (ids?.tmdb) out.push(`${kind}:tmdb:${ids.tmdb}`);
	if (ids?.musicbrainz) out.push(`${kind}:mbid:${ids.musicbrainz}`);
	return out;
}
