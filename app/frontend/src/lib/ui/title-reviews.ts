// Types and formatting helpers for review sources, scores and rating spreads.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
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

// Falls back to the default source, then the first, if the pick is gone.
export function pillState(sources: ReviewSource[] | undefined, picked: string): { pills: ReviewSource[]; active: string } {
	const pills = (sources ?? []).filter((s) => s && typeof s.id === 'string' && s.id);
	if (!pills.length) return { pills: [], active: DEFAULT_SOURCE };
	const active = pills.some((s) => s.id === picked) ? picked : pills.some((s) => s.id === DEFAULT_SOURCE) ? DEFAULT_SOURCE : pills[0].id;
	return { pills, active };
}

export function ordersOf(sources: ReviewSource[] | undefined, id: string): string[] {
	return sources?.find((s) => s.id === id)?.orders ?? ['newest'];
}

export const orderWord = (o: string) => (o === 'helpful' ? t('ui.titleReviews.mostLiked') : t('ui.titleReviews.newest'));

export function reviewsPath(kind: string, tmdb: string, source: string, page = 1, order = 'newest'): string {
	const q = new URLSearchParams({ source });
	if (page > 1) q.set('page', String(page));
	if (order !== 'newest') q.set('order', order);
	return `/metadata/${kind}/${encodeURIComponent(tmdb)}/reviews?${q}`;
}

export function scoreText(score: number | null | undefined): string {
	return typeof score === 'number' && score > 0 ? (Math.round(score * 10) / 10).toFixed(1) : '';
}

// 1200 -> 1.2K; values of 100 and up drop the decimal.
export function compactCount(n: number): string {
	if (!(n > 0)) return '0';
	const f = (v: number) => (v >= 100 ? String(Math.round(v)) : String(Math.round(v * 10) / 10));
	if (n >= 1e6) return f(n / 1e6) + 'M';
	if (n >= 1e3) return f(n / 1e3) + 'K';
	return String(n);
}

export const votesText = (n: number) => `${compactCount(n)} ${n === 1 ? 'rating' : 'ratings'}`;

// Needs exactly ten buckets (1 to 10 stars); highest score first.
export function spread(distribution: number[] | undefined): { point: number; count: number; share: number }[] {
	if (!distribution || distribution.length !== 10) return [];
	const max = Math.max(...distribution);
	if (!(max > 0)) return [];
	return distribution.map((count, i) => ({ point: i + 1, count, share: count / max })).reverse();
}
