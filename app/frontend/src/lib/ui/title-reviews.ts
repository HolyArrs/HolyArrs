// Ratings & reviews on a title page: which sources the pill bar shows,
// which one is picked, and the words for scores and counts. The server
// lists only the sources that are set up (TMDB always; Trakt once the
// owner saved its key and VIP token), so a pill never leads nowhere.
// A new source (TVDB and so on) needs only a server entry.

export type ReviewSource = { id: string; name: string; orders: string[] };
export type Review = { author: string; date?: string; rating?: number; text: string; likes?: number; spoiler?: boolean };
export type ReviewsAnswer = {
	source: string;
	sources: ReviewSource[];
	order: string;
	score: number;
	votes: number;
	distribution?: number[];
	watching?: number;
	reviews: Review[];
	page: number;
	pages: number;
	reviews_hidden?: boolean;
};

export const DEFAULT_SOURCE = 'tmdb';

/** pills are the sources to show; one pill alone is still shown as a label. */
export function pillState(sources: ReviewSource[] | undefined, picked: string): { pills: ReviewSource[]; active: string } {
	const pills = (sources ?? []).filter((s) => s && typeof s.id === 'string' && s.id);
	if (!pills.length) return { pills: [], active: DEFAULT_SOURCE };
	const active = pills.some((s) => s.id === picked) ? picked : pills.some((s) => s.id === DEFAULT_SOURCE) ? DEFAULT_SOURCE : pills[0].id;
	return { pills, active };
}

/** The orders a source can sort by; the toggle shows only with two or more. */
export function ordersOf(sources: ReviewSource[] | undefined, id: string): string[] {
	return sources?.find((s) => s.id === id)?.orders ?? ['newest'];
}

export const orderWord = (o: string) => (o === 'helpful' ? 'Most liked' : 'Newest');

export function reviewsPath(kind: string, tmdb: string, source: string, page = 1, order = 'newest'): string {
	const q = new URLSearchParams({ source });
	if (page > 1) q.set('page', String(page));
	if (order !== 'newest') q.set('order', order);
	return `/metadata/${kind}/${encodeURIComponent(tmdb)}/reviews?${q}`;
}

/** 7.84 → "7.8"; empty for no score. */
export function scoreText(score: number | null | undefined): string {
	return typeof score === 'number' && score > 0 ? (Math.round(score * 10) / 10).toFixed(1) : '';
}

/** 12400 → "12.4K"; 1500000 → "1.5M". */
export function compactCount(n: number): string {
	if (!(n > 0)) return '0';
	const f = (v: number) => (v >= 100 ? String(Math.round(v)) : String(Math.round(v * 10) / 10));
	if (n >= 1e6) return f(n / 1e6) + 'M';
	if (n >= 1e3) return f(n / 1e3) + 'K';
	return String(n);
}

export const votesText = (n: number) => `${compactCount(n)} ${n === 1 ? 'rating' : 'ratings'}`;

/** Each point's share of the largest bar (0 to 1), points 10 down to 1. */
export function spread(distribution: number[] | undefined): { point: number; count: number; share: number }[] {
	if (!distribution || distribution.length !== 10) return [];
	const max = Math.max(...distribution);
	if (!(max > 0)) return [];
	return distribution.map((count, i) => ({ point: i + 1, count, share: count / max })).reverse();
}
