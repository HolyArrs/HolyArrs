// Live search: the search field's dropdown. It opens at 2 characters, waits 150 ms after the last key, aborts the request a newer key replaced, and never lets an older answer overwrite a newer one.
import { detailPath } from '../metadata/index.ts';
import { albumHref, artistHref } from '../music/index.ts';
import { to } from '../core/paths.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type SuggestKind = 'movie' | 'show' | 'person' | 'artist' | 'album' | 'song';

export type SuggestRow = {
	kind: SuggestKind;
	name: string;
	sub?: string;
	art?: string;
	state?: 'have' | 'missing' | 'asked' | 'following';
	title_id?: number;
	tmdb?: string;
	person_id?: string;
	mbid?: string;
	artist_mbid?: string;
};

export type SuggestAnswer = { q: string; completions: string[]; items: SuggestRow[] };

type Fetcher = (q: string, signal: AbortSignal) => Promise<SuggestAnswer>;

export const SUGGEST_MIN = 2;
export const SUGGEST_WAIT = 150;

/** null until the first answer; false when the server has live search off (404). */
export const live: { on: boolean | null } = { on: null };

/** A row's chip text. */
export const stateWord: Record<NonNullable<SuggestRow['state']>, string> = {
	get have() { return t('discover.liveSearch.have'); },
	get missing() { return t('discover.liveSearch.missing'); },
	get asked() { return t('discover.liveSearch.asked'); },
	get following() { return t('discover.liveSearch.following'); }
};

/** People and artists get round art; the rest square. */
export const roundArt = (k: SuggestKind) => k === 'artist' || k === 'person';

const musicSearch = (words: string) => to(`/search?q=${encodeURIComponent(words)}&kind=music`);

/** Where a row opens. */
export function suggestHref(r: SuggestRow): string {
	switch (r.kind) {
		case 'movie':
		case 'show':
			if (r.title_id) return to(`/library/${r.title_id}`);
			return r.tmdb ? to(detailPath(r.kind, r.tmdb)) : to(`/search?q=${encodeURIComponent(r.name)}&kind=${r.kind}`);
		case 'person':
			if (r.person_id) return to(`/people/${encodeURIComponent(r.person_id)}`);
			return r.title_id ? to(`/library/${r.title_id}`) : to(`/search?q=${encodeURIComponent(r.name)}&kind=people`);
		case 'artist':
			if (r.mbid) return artistHref(r.mbid);
			return r.title_id ? to(`/library/${r.title_id}`) : musicSearch(r.name);
		case 'album':
			if (r.mbid && r.artist_mbid) return albumHref(r.mbid, r.artist_mbid);
			return r.title_id ? to(`/library/${r.title_id}`) : musicSearch(`${r.name} ${r.sub ?? ''}`.trim());
		default:
			return musicSearch(`${r.name} ${r.sub ?? ''}`.trim());
	}
}

/** The type label at the start of a row's subline. */
export const kindWord: Record<SuggestKind, string> = {
	get movie() { return t('discover.liveSearch.movie'); },
	get show() { return t('discover.liveSearch.show'); },
	get person() { return t('discover.liveSearch.person'); },
	get artist() { return t('discover.liveSearch.artist'); },
	get album() { return t('discover.liveSearch.album'); },
	get song() { return t('discover.liveSearch.song'); }
};
// The server writes the type in English at the start of its sublines.
const ENGLISH_KIND: Record<SuggestKind, string> = { movie: 'Movie', show: 'Show', person: 'Person', artist: 'Artist', album: 'Album', song: 'Song' };

/** "Album · Artist" or "Show · 2019"; the server's movie, show and person sublines already start with the type. */
export function subline(r: SuggestRow): string {
	const s = r.sub?.trim() ?? '';
	if (!s) return kindWord[r.kind];
	return s.startsWith(ENGLISH_KIND[r.kind]) ? kindWord[r.kind] + s.slice(ENGLISH_KIND[r.kind].length) : `${kindWord[r.kind]} · ${s}`;
}

/** Moves the highlight through n entries (-1 is none): down from none goes to the first, up from the first back to none. */
export function step(i: number, n: number, dir: 1 | -1): number {
	if (n <= 0) return -1;
	if (dir === 1) return i >= n - 1 ? 0 : i + 1;
	return i <= 0 ? -1 : i - 1;
}

/** Runs the dropdown's requests. type(q) waits SUGGEST_WAIT, then asks; a newer type() aborts the older request and only the newest answer reaches onAnswer. clear() drops everything. */
export function createSuggest(fetch: Fetcher, onAnswer: (a: SuggestAnswer | null) => void, wait = SUGGEST_WAIT) {
	let timer: ReturnType<typeof setTimeout> | undefined;
	let ctl: AbortController | undefined;
	let seq = 0;
	function clear() {
		clearTimeout(timer);
		ctl?.abort();
		ctl = undefined;
		seq++;
		onAnswer(null);
	}
	function type(raw: string) {
		const q = raw.trim();
		if ([...q].length < SUGGEST_MIN || live.on === false) return clear();
		clearTimeout(timer);
		const mine = ++seq;
		timer = setTimeout(() => {
			ctl?.abort();
			const c = (ctl = new AbortController());
			fetch(q, c.signal).then(
				(a) => {
					live.on = true;
					if (mine === seq) onAnswer(a);
				},
				(err: unknown) => {
					if ((err as { status?: number })?.status === 404) live.on = false;
					if (mine === seq && !c.signal.aborted) onAnswer(null);
				}
			);
		}, wait);
	}
	return { type, clear };
}
