// The server-built Explore page: the head in one request, the tail by pages.

import { discoverCard, type ExploreRow, type ProviderItem } from '#lib/ui/title-cards.ts';
import { type ExploreType } from './explore-type.ts';

export interface ServerRow {
	id: string;
	type: string;
	title: string;
	/** A small line under the title, chosen by the server. */
	subtitle?: string;
	title_layer?: string;
	wide?: boolean;
	items: ProviderItem[];
}

export interface HeadAnswer {
	type: ExploreType;
	building: boolean;
	seeds: number;
	tail_cap: number;
	tail_rows: number;
	/** Up to 6 titles not in the Library (from Trending now and Trending this week everywhere) for the hero. */
	hero?: ProviderItem[];
	/** The household's taste genres, strongest first (hero badges). */
	taste?: string[];
	head: ServerRow[];
}

export interface TailAnswer {
	page: number;
	more: boolean;
	rows: ServerRow[];
}

export interface HeadQuery {
	type: ExploreType;
	services?: number[];
	region?: string;
	/** The viewer's time zone (IANA), so row titles fit their time of day. */
	tz?: string;
}

/** Services are sent so the server builds On your services for them; empty means none picked. */
export function headPath(q: HeadQuery): string {
	const p = headParams(q);
	if (q.services && q.region) {
		p.set('services', q.services.join('|'));
		p.set('region', q.region);
	}
	return `/explore/head?${p.toString()}`;
}

export function tailPath(q: HeadQuery, page: number): string {
	const p = headParams(q);
	p.set('page', String(page));
	return `/explore/tail?${p.toString()}`;
}

/** Badges state facts only: no countdowns, no "leaving soon". */
export const FACTUAL_BADGES = /^(New season|New episodes|Coming .+|In library|Asked|Today|Tomorrow|Premiere|New|4K|1080p|720p|\d+p)$/;

export function factualBadges(badges: string[]): string[] {
	return badges.filter((b) => FACTUAL_BADGES.test(b));
}

/** Turns a server row into an Explore row; Coming soon keeps its wide pictures. */
export function serverRow(r: ServerRow): ExploreRow & { wide: boolean; type: string } {
	return {
		id: r.id,
		type: r.type,
		title: r.title,
		...(r.subtitle ? { subtitle: r.subtitle } : {}),
		...(r.title_layer ? { titleLayer: r.title_layer } : {}),
		wide: !!r.wide,
		cards: r.items.map((d) => {
			// Every movie and show row draws wide title cards, so keep the backdrop on each card.
			const c = { ...discoverCard(d, r.id), wide: d.backdrop ?? null };
			return { ...c, badges: factualBadges(c.badges) };
		}),
		keep: r.type === 'top_services'
	};
}

function headParams(q: HeadQuery): URLSearchParams {
	const p = new URLSearchParams({ type: q.type === 'music' ? 'all' : q.type });
	if (q.tz) p.set('tz', q.tz);
	return p;
}
