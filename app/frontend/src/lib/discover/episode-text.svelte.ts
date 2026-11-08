import { api } from '#lib/core/api.ts';

// Episode descriptions for calendar cards, read from the metadata server one season at a time (the same answer the title page's season list uses) and kept for the visit, so each show's season is asked once.
type Ep = { number: number; name?: string; overview?: string; still?: string };

const seasons = $state<Record<string, Record<number, Ep>>>({});
const asked = new Set<string>();

/** An episode's description once its season has been read; '' until then or when there is none. */
export function epText(tmdb: string | undefined, season: number | undefined, episode: number | undefined): string {
	if (!tmdb || season == null || episode == null) return '';
	const key = `${tmdb}:${season}`;
	if (!asked.has(key)) {
		asked.add(key);
		api.get<{ episodes?: Ep[] }>(`/metadata/show/${encodeURIComponent(tmdb)}/seasons/${season}`)
			.then((r) => (seasons[key] = Object.fromEntries((r.episodes ?? []).map((e) => [e.number, e]))))
			.catch(() => {});
	}
	return seasons[key]?.[episode]?.overview?.trim() ?? '';
}

/** "S2 E5", or a span joined by an en dash for several of one season. */
export function epCode(eps: { season?: number; episode?: number }[]): string {
	const e = eps.filter((x) => x.season != null && x.episode != null);
	if (!e.length) return '';
	const first = e[0], last = e[e.length - 1];
	return first.season === last.season && e.length > 1 ? `S${first.season} E${first.episode}–${last.episode}` : `S${first.season} E${first.episode}`;
}

const movies = $state<Record<string, string>>({});
const askedMovies = new Set<string>();

/** A movie's description, read once from the metadata server; '' until then or when there is none. */
export function movieText(tmdb: string | undefined): string {
	if (!tmdb) return '';
	if (!askedMovies.has(tmdb)) {
		askedMovies.add(tmdb);
		api.get<{ overview?: string }>(`/metadata/movie/${encodeURIComponent(tmdb)}`)
			.then((r) => (movies[tmdb] = r.overview?.trim() ?? ''))
			.catch(() => {});
	}
	return movies[tmdb] ?? '';
}
