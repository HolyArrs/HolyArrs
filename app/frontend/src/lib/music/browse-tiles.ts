// Music Browse tiles as the pages get them. GET /music/browse-tiles answers Search > Music's sections and the mix that Explore and New & Popular offer. A tile always has real album covers to try, in order.
import { coverFor, appPicture } from './music-pages.ts';
import { api, type Client } from '../core/api.ts';
import { getBase, to } from '../core/paths.ts';

export interface BrowseTileData {
	key: string;
	name: string;
	href: string;
	/** The tile's colour index from the server; the pages colour tiles by key, so it is not used. */
	hue?: number;
	/** Album covers to try in order. */
	covers: string[];
}

export interface BrowseSectionData {
	id: string;
	title: string;
	tiles: BrowseTileData[];
}

export interface BrowseTilesAnswer {
	kid?: boolean;
	sections?: BrowseSectionData[];
	/** Explore's and New & Popular's mix: genres, moods, a season, decades and activities, interleaved. */
	explore?: BrowseTileData[];
}

export function browseTilesPath(region: string): string {
	return `/music/browse-tiles?${new URLSearchParams({ region })}`;
}

/** Reads the Browse sections and the Explore mix for a region. */
export function fetchBrowseTiles(region: string, client: Pick<Client, 'get'> = api, signal?: AbortSignal): Promise<BrowseTilesAnswer> {
	return client.get<BrowseTilesAnswer>(browseTilesPath(region), signal);
}

/** A tile cover's address at the tile's size (one size per slot, 250 for a cover this small). A path the page already put under the app's base (Explore's music image) is left as it is. */
export function coverSrc(src: string, css = 110): string {
	const base = getBase();
	const own = base && src.startsWith(`${base}/`) ? src : (appPicture(src) ?? to(src));
	return coverFor(own, css).src ?? own;
}
