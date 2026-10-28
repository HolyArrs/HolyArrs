// One shared read of what the Library holds, for owned marks.
// Explore and New use the light /titles/owned answer (one entry per title:
// kind, ids, Library id, state) when the server has it; an older server
// (404) falls back to the full /titles list. The full list is shared too:
// kept 60 s across pages, forgotten on any change (api.ts).

import { api, ApiError, type Client } from './api.ts';
import type { AskState, SlotView, TitleView } from './types.ts';

/** One Library title in the light answer. */
export interface OwnedItem {
	kind: 'movie' | 'show' | 'artist' | 'album' | string;
	id: number;
	tmdb?: string | number;
	mbid?: string;
	/** available | partial | downloading | requested | approved | declined | missing */
	state?: string;
	percent?: number;
	genres?: string[];
}

/** How long the shared Library reads are kept between pages (any change forgets them sooner). */
export const LIBRARY_KEEP = 60_000;

/** libraryTitles is the full Library list, one request shared by every page that needs it. */
export async function libraryTitles(client: Client = api): Promise<TitleView[]> {
	return (await client.warm<{ titles?: TitleView[] }>('/titles', LIBRARY_KEEP)).titles ?? [];
}

/**
 * stubTitle turns a light entry into the few TitleView fields the owned
 * marks read (ids, slots, asks), so ownedIndex / ownedMark / matchOwned
 * answer exactly as they would for the full title.
 */
export function stubTitle(it: OwnedItem): TitleView {
	const slot = (state: string, extra: Partial<SlotView> = {}) => ({ state, ...extra }) as unknown as SlotView;
	const ask = (state: AskState) => [{ state }] as unknown as TitleView['asks'];
	let slots: SlotView[] = [];
	let asks: TitleView['asks'] = [];
	switch (it.state) {
		case 'available':
		case 'in_library':
			slots = [slot('in_library')];
			break;
		case 'partial':
			slots = [slot('in_library'), slot('missing')];
			break;
		case 'downloading':
			slots = [slot('downloading', { download: { active: true, progress: (it.percent ?? 0) / 100 } } as Partial<SlotView>)];
			break;
		case 'requested':
		case 'asked':
			asks = ask('asked');
			break;
		case 'approved':
			asks = ask('approved');
			break;
		case 'declined':
		case 'turned_down':
			asks = ask('turned_down');
			break;
	}
	const ids: Record<string, string> = {};
	if (it.tmdb !== undefined && it.tmdb !== '') ids.tmdb = String(it.tmdb);
	if (it.mbid) ids.musicbrainz = it.mbid;
	return {
		id: it.id,
		kind: it.kind,
		name: '',
		ids,
		genres: it.genres,
		slots,
		asks,
		versions: [],
		artwork: [],
		...(it.kind === 'artist' && it.mbid ? { artist: { mbid: it.mbid } as TitleView['artist'] } : {})
	} as unknown as TitleView;
}

/** ownedLibrary is what Explore and New match cards against; light says the light answer was used. */
export async function ownedLibrary(client: Client = api): Promise<{ titles: TitleView[]; light: boolean }> {
	try {
		const r = await client.warm<{ items?: OwnedItem[] }>('/titles/owned', LIBRARY_KEEP);
		return { titles: (r.items ?? []).map(stubTitle), light: true };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) return { titles: await libraryTitles(client), light: false };
		throw e;
	}
}
