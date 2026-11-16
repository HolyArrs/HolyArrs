// The signed-in person's Watched marks: one answer for the whole Library, read by every poster, row and season.

// GET /my/watched: the person's watched movie ids and, per show id and season number, how many episodes they watched.
export type MyWatched = { enabled: boolean; linked: boolean; movies: number[]; shows: Record<string, Record<string, number>> };

// seen of the of episodes in the Library (aired ones when that is not known) (1 of 1 for a movie); of is 0 when nothing is known to have aired.
export type WatchedMark = { state: 'watched' | 'partly'; seen: number; of: number };

export type WatchedIndex = { movies: Set<number>; shows: MyWatched['shows'] };

type SeasonLike = { number: number; aired?: number; in_library?: number };

// What a season counts against: the episodes in the Library, as Plex and Jellyfin count them.
const countOf = (s: SeasonLike) => s.in_library ?? s.aired ?? 0;
type TitleLike = { id: number; kind: string; show?: { seasons?: SeasonLike[] } | null };

const none: WatchedIndex = { movies: new Set(), shows: {} };

// Nothing is marked while watch history is off or the person has no media server account here.
export function watchedIndex(w: MyWatched | null | undefined): WatchedIndex {
	if (!w?.enabled || !w.linked) return none;
	return { movies: new Set(w.movies ?? []), shows: w.shows ?? {} };
}

const mark = (seen: number, of: number): WatchedMark => ({ state: of > 0 && seen >= of ? 'watched' : 'partly', seen, of });

// A movie is watched or not. A show counts its episodes in the Library outside the specials; anything seen short of all of them is partly watched.
export function titleWatched(t: TitleLike, ix: WatchedIndex): WatchedMark | null {
	if (t.kind === 'movie') return ix.movies.has(t.id) ? mark(1, 1) : null;
	if (t.kind !== 'show') return null;
	const mine = ix.shows[t.id];
	if (!mine) return null;
	const seasons = (t.show?.seasons ?? []).filter((s) => s.number > 0 && countOf(s) > 0);
	if (!seasons.length) {
		const seen = Object.entries(mine).reduce((n, [s, c]) => n + (s === '0' ? 0 : c), 0);
		return mark(seen, 0);
	}
	let seen = 0;
	let of = 0;
	for (const s of seasons) {
		of += countOf(s);
		seen += Math.min(mine[s.number] ?? 0, countOf(s));
	}
	return mark(seen, of);
}

export function seasonWatched(titleId: number, s: SeasonLike, ix: WatchedIndex): WatchedMark | null {
	const seen = ix.shows[titleId]?.[s.number] ?? 0;
	if (!seen) return null;
	const of = countOf(s);
	return mark(of > 0 ? Math.min(seen, of) : seen, of);
}

// "Watched" for a movie or a finished show or season, "Watched 3 of 8" for a part of one.
export function watchedWords(m: WatchedMark, t: (key: string, vars?: Record<string, string | number>) => string): string {
	if (m.state === 'watched') return t('mediafiles.watched.all');
	return m.of > 0 ? t('mediafiles.watched.part', { seen: m.seen, of: m.of }) : t('mediafiles.watched.partly');
}
