import { to } from './paths.ts';

// TMDB pictures in the size a row or hero shows: w342 posters and w780
// backdrops, loaded through HolyArrs' artwork cache
// (/api/v1/artwork/tmdb/<size>/<file>): the server fetches each picture
// once at TMDB's own size, keeps it on disk and serves it with a year-long
// cache header. Full-size pictures in long rows are what made Explore heavy.
// Other pictures (HolyArrs' own artwork cache, other sites) are left as
// they are.
//
// No srcset on purpose: Chrome keeps every picture with a srcset in a
// viewport listener after its page is gone, and with it that whole page
// (measured: 20 Explore visits kept 120,000 DOM nodes and 1,700 listeners).
const tmdb = /^https:\/\/image\.tmdb\.org\/t\/p\/(?:w\d+|original)\/([A-Za-z0-9_-]+\.(?:jpg|png|webp))$/;

const width = { poster: 342, backdrop: 780, large: 1280 } as const;

export function sized(url: string | null | undefined, as: 'poster' | 'backdrop' | 'large'): string {
	const m = url ? tmdb.exec(url) : null;
	return m ? to(`/api/v1/artwork/tmdb/w${width[as]}/${m[1]}`) : (url ?? '');
}
