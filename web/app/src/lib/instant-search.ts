// The instant results in the ⌘K palette: Library titles first, then movies,
// TV shows and music from the online lookups, five each, with a More link
// per section and a See all row.
import { detailPath } from './title-details.ts';
import type { LookupArtist, LookupShow, Movie, TitleView } from './types.ts';

export type SectionId = 'library' | 'movies' | 'shows' | 'music';

export interface Hit {
	key: string;
	name: string;
	year?: number;
	kindWord: string;
	poster?: string | null;
	/** App path to open (detail page, Library page or a search). */
	href: string;
	inLibrary: boolean;
	/** For a first paint of the detail page. */
	detail?: { kind: 'movie' | 'show'; tmdb: string };
}

export interface Section {
	id: SectionId;
	label: string;
	hits: Hit[];
	state: 'idle' | 'loading' | 'done' | 'error';
	more?: string;
}

export const sectionLabels: Record<SectionId, string> = { library: 'Library', movies: 'Movies', shows: 'TV shows', music: 'Music' };
export const PER_SECTION = 5;

const kindWords: Record<string, string> = { movie: 'Movie', show: 'TV show', artist: 'Artist' };

export function searchPath(q: string, kind?: 'show' | 'music'): string {
	return `/search?q=${encodeURIComponent(q.trim())}${kind ? `&kind=${kind}` : ''}`;
}

export function libraryHits(titles: TitleView[]): Hit[] {
	return titles.slice(0, PER_SECTION).map((t) => ({ key: `l${t.id}`, name: t.name, year: t.year, kindWord: kindWords[t.kind] ?? 'Title', poster: t.artwork?.[0], href: `/library/${t.id}`, inLibrary: true }));
}

/** libraryByTmdb maps "kind:tmdb" to a Library title id, to mark online hits. */
export function libraryByTmdb(titles: TitleView[]): Map<string, number> {
	const m = new Map<string, number>();
	for (const t of titles) {
		if (t.ids?.tmdb) m.set(`${t.kind}:${t.ids.tmdb}`, t.id);
		m.set(nameKey(t.kind, t.name, t.year), t.id);
	}
	return m;
}

// Library titles can carry another source's ids, so the same name and year also match.
const nameKey = (kind: string, name: string, year?: number) => `${kind}:n:${name.trim().toLowerCase()}:${year ?? ''}`;
const inLib = (lib: Map<string, number>, kind: string, name: string, year: number | undefined, tmdb?: string) => (tmdb ? lib.get(`${kind}:${tmdb}`) : undefined) ?? lib.get(nameKey(kind, name, year));

export function movieHits(movies: Movie[], lib: Map<string, number>): Hit[] {
	return movies.slice(0, PER_SECTION).flatMap((m) => {
		const tmdb = m.ids.tmdb;
		if (!tmdb) return [];
		const id = inLib(lib, 'movie', m.name, m.year || undefined, tmdb);
		return [{ key: `m${tmdb}`, name: m.name, year: m.year || undefined, kindWord: 'Movie', poster: m.poster, href: id ? `/library/${id}` : detailPath('movie', tmdb), inLibrary: !!id, detail: id ? undefined : { kind: 'movie' as const, tmdb } }];
	});
}

export function showHits(shows: LookupShow[], lib: Map<string, number>): Hit[] {
	return shows.slice(0, PER_SECTION).map((s) => {
		const tmdb = s.ids.tmdb;
		const id = s.title_id ?? inLib(lib, 'show', s.name, s.year, tmdb);
		const href = id ? `/library/${id}` : tmdb ? detailPath('show', tmdb) : searchPath(s.name, 'show');
		return { key: `s${s.source}${s.ids[s.source] ?? s.name}`, name: s.name, year: s.year, kindWord: 'TV show', poster: s.poster, href, inLibrary: !!id, detail: !id && tmdb ? { kind: 'show' as const, tmdb } : undefined };
	});
}

export function artistHits(artists: LookupArtist[]): Hit[] {
	return artists.slice(0, PER_SECTION).map((a) => ({
		key: `a${a.mbid}`,
		name: a.disambiguation ? `${a.name} (${a.disambiguation})` : a.name,
		kindWord: 'Artist',
		href: a.title_id ? `/library/${a.title_id}` : searchPath(a.name, 'music'),
		inLibrary: !!a.title_id
	}));
}

/** Option is one keyboard stop in the palette list. */
export type Option = { kind: 'place'; index: number } | { kind: 'hit'; section: SectionId; hit: Hit } | { kind: 'more'; section: SectionId; href: string } | { kind: 'all'; href: string };

/** options flattens places, each section's hits and More link, then See all. */
export function options(places: number, sections: Section[], q: string): Option[] {
	const out: Option[] = [];
	for (let i = 0; i < places; i++) out.push({ kind: 'place', index: i });
	for (const s of sections) {
		for (const hit of s.hits) out.push({ kind: 'hit', section: s.id, hit });
		if (s.more && s.hits.length >= PER_SECTION) out.push({ kind: 'more', section: s.id, href: s.more });
	}
	if (q.trim()) out.push({ kind: 'all', href: searchPath(q) });
	return out;
}
