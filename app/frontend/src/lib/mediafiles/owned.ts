// Loads the Library titles, with a light list for big libraries.

import { api, ApiError, type Client } from '../core/api.ts';
import type { AskState, SlotView, TitleView } from '../core/types.ts';

// One row of the light owned list.
export interface OwnedItem {
	kind: 'movie' | 'show' | 'artist' | 'album' | string;
	id: number;
	tmdb?: string | number;
	mbid?: string;
	state?: string;
	percent?: number;
	genres?: string[];
}

// How long, in ms, a loaded Library list is reused.
export const LIBRARY_KEEP = 60_000;

export async function libraryTitles(client: Client = api): Promise<TitleView[]> {
	return (await client.warm<{ titles?: TitleView[] }>('/titles', LIBRARY_KEEP)).titles ?? [];
}

export async function libraryBrief(client: Client = api): Promise<TitleView[]> {
	return (await client.warm<{ titles?: TitleView[] }>('/titles?brief=1', LIBRARY_KEEP)).titles ?? [];
}

// Builds a bare title from a light row so shelves can draw status without the full title.
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

// Falls back to the full list on servers without the light route.
export async function ownedLibrary(client: Client = api): Promise<{ titles: TitleView[]; light: boolean }> {
	try {
		const r = await client.warm<{ items?: OwnedItem[] }>('/titles/owned', LIBRARY_KEEP);
		return { titles: (r.items ?? []).map(stubTitle), light: true };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) return { titles: await libraryTitles(client), light: false };
		throw e;
	}
}
