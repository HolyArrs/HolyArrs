// Search in plain words ("that space movie with the robot from 2014"). The server reads the words; this file decides when to ask it and says back what it understood.
import type { ProviderItem } from '#lib/ui/title-cards.ts';
import type { TitleView } from '../core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** What the server understood the words as. */
export interface PlainQuery {
	text: string;
	plain: boolean;
	name: string;
	kind?: 'movie' | 'show' | 'music';
	year_from?: number;
	year_to?: number;
	genres: string[];
	people: string[];
	words: string[];
}

export interface PlainAnswer {
	query: PlainQuery;
	titles: TitleView[];
	more: ProviderItem[];
	note?: string;
}

const KIND = /\b(movies?|films?|shows?|series|tv|sitcom)\b/i;
const YEARS = /\b(from|in|after|before|since|around)\s+(18[7-9]\d|19\d\d|20\d\d)\b|\b'?\d0'?s\b|\b(18|19|20)\d0s\b|\b(twenties|thirties|forties|fifties|sixties|seventies|eighties|nineties)\b/i;
const ABOUT = /\b(starring|featuring|about|with)\b/i;

/** Whether the text is worth a plain-words search: at least two words and a kind, a year phrase, a decade, or "with" or "about". A plain name ("2012", "Se7en", "Dune 2021") is left to the normal search. */
export function describes(text: string): boolean {
	const t = text.trim();
	if (t.length > 300 || t.split(/\s+/).length < 2 || /^[a-z]+:\S+$/i.test(t)) return false;
	return KIND.test(t.replace(/\bshow me\b/i, '')) || YEARS.test(t) || (ABOUT.test(t) && t.split(/\s+/).length >= 3);
}

/** The server address for a plain-words search. */
export function plainPath(text: string): string {
	return `/search?q=${encodeURIComponent(text.trim())}`;
}

/** The short "Understood as" line, such as "Science Fiction movies · 2014 · about robot"; "" when nothing was read. */
export function understood(q: PlainQuery | null | undefined): string {
	if (!q || q.plain) return '';
	const kind = q.kind === 'movie' ? t('discover.searchPlain.movies') : q.kind === 'show' ? t('discover.searchPlain.shows') : q.kind === 'music' ? t('discover.searchPlain.music') : t('discover.searchPlain.titles');
	const head = [q.genres.join(t('discover.searchPlain.or')), kind].filter(Boolean).join(' ');
	const parts = [q.name ? `“${q.name}”` : '', head, years(q), q.people.length ? t('discover.searchPlain.with', { names: q.people.join(', ') }) : '', q.words.length ? t('discover.searchPlain.about', { words: q.words.join(', ') }) : ''];
	return parts.filter(Boolean).join(' · ');
}

/** Whether the answer has something to show. */
export function found(a: PlainAnswer | null | undefined): boolean {
	return !!a && !a.query.plain && (a.titles.length > 0 || a.more.length > 0);
}

function years(q: PlainQuery): string {
	const from = q.year_from ?? 0;
	const to = q.year_to ?? 0;
	if (from && to && from === to) return String(from);
	if (from && to && to - from === 9 && from % 10 === 0) return t('discover.searchPlain.decade', { from });
	if (from && to) return t('discover.searchPlain.fromTo', { from, to });
	if (from) return t('discover.searchPlain.orLater', { from });
	if (to) return t('discover.searchPlain.orEarlier', { to });
	return '';
}
