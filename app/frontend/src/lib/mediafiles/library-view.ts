// What the Library prints for a title, and how its lists move.
import { fade, slide } from 'svelte/transition';
import { ago, bytes, date } from '../core/format.ts';
import { reducedMotion } from '../core/motion.ts';
import { peopleNames } from '../core/plain.ts';
import { DUR, easeOut } from '../core/transitions.ts';
import type { TitleView } from '../core/types.ts';
import { quality } from '#lib/ui/title-cards.ts';
import { t as tr } from '../core/i18n/i18n.svelte.ts';
import { codecLabel, languageName } from './copies.ts';
import { libraryState } from './library-state.ts';
import { shownScore } from '../core/rating.ts';
import { progressOf, releaseDate, type Column } from './table.ts';

export type View = 'rows' | 'table' | 'posters' | 'overview';

export type Entry = { type: 'head'; key: string; id: number; name: string; count: number } | { type: 'row'; key: string; t: TitleView };

// Rows of chips and folds grow in; a fade under reduced motion.
export const grow = (n: Element) => (reducedMotion() ? fade(n, { duration: DUR.fast }) : slide(n, { duration: DUR.base, easing: easeOut }));
export const shrink = (n: Element) => (reducedMotion() ? fade(n, { duration: DUR.fast }) : slide(n, { duration: DUR.fast, easing: easeOut }));

export function linkedPeople(t: TitleView) {
	const people = new Map<number, { id: number; name: string }>();
	for (const ask of t.asks ?? []) {
		if (ask.title_id !== t.id) continue;
		for (const person of [ask.asked_by, ...ask.people]) if (person.name.trim()) people.set(person.id, person);
	}
	return [...people.values()];
}

export function requestedBy(t: TitleView): string {
	return peopleNames(linkedPeople(t).map((p) => p.name));
}

export const isProgressCol = (c: Column) => c.id === 'episodes' || c.id === 'tracks';

/** A column's width for the Rows grid (a stretching column takes its floor). */
export const colPx = (c: Column) => Math.max(84, parseInt(c.width.replace(/^minmax\((\d+)px.*$/, '$1'), 10) || 150);

/** A fact for the Rows view: counts for progress columns, "14 d ago" for dates that are recent by nature. */
export function rowCell(t: TitleView, c: Column, p: { have: number; total: number }, series: string): string {
	if (isProgressCol(c)) return p.total ? `${p.have}/${p.total}` : '';
	if (c.id === 'release' && t.kind === 'movie' && t.digital_date && Date.parse(t.digital_date) <= Date.now()) return date(t.digital_date);
	if (c.id === 'added') return ago(t.added_at);
	if (c.id === 'activity') return ago(t.last_activity);
	return cell(t, c, series);
}

/** Bar colour: all in, some missing, nothing yet, or only future ones. */
export function tone(t: TitleView): 'full' | 'gap' | 'none' | 'later' {
	const p = progressOf(t);
	const s = t.kind === 'show' ? t.show : t.kind === 'artist' ? t.artist : undefined;
	if (p.total === 0) return 'later';
	if (p.have >= p.total) return 'full';
	if (!t.keep_up_to_date && p.have === 0) return 'none';
	return s && s.missing === 0 && p.have < p.total ? 'later' : 'gap';
}

function movieOut(t: TitleView): string {
	const now = Date.now();
	if (t.digital_date) return Date.parse(t.digital_date) <= now ? tr('mediafiles.view.out', { date: date(t.digital_date) }) : tr('mediafiles.view.homeRelease', { date: date(t.digital_date) });
	if (t.cinema_date) return Date.parse(t.cinema_date) <= now ? tr('mediafiles.view.inCinemas', { date: date(t.cinema_date) }) : tr('mediafiles.view.cinemas', { date: date(t.cinema_date) });
	return date(releaseDate(t)) || tr('mediafiles.view.notKnown');
}

/** A title's score to one decimal, or '' when it has none or too few votes. */
export function scoreText(t: TitleView): string {
	return shownScore(t.score, t.votes)?.toFixed(1) ?? '';
}

export function cell(t: TitleView, c: Column, series: string): string {
	switch (c.id) {
		case 'network': return t.show?.network ?? '';
		case 'rules': return t.pick_rules_name ?? '';
		case 'next': return t.show?.next_air_date ? date(t.show.next_air_date) : t.show?.status === 'ended' ? tr('mediafiles.view.ended') : '';
		case 'last': return date(t.show?.last_air_date);
		case 'seasons': return t.show?.season_count ? String(t.show.season_count) : '';
		case 'added': return date(t.added_at);
		case 'year': return t.year ? String(t.year) : '';
		case 'size': return t.size_bytes ? bytes(t.size_bytes) : '';
		case 'codec': return t.copy && t.copy.codec !== 'unknown' ? codecLabel(t.copy.codec) : '';
		case 'audio': return (t.copy?.audio_languages ?? []).map(languageName).join(', ');
		case 'genres': return (t.genres ?? []).slice(0, 3).join(', ');
		case 'rating': return t.rating ?? '';
		case 'score': return scoreText(t) ? `★ ${scoreText(t)}` : '';
		case 'path': return t.folder_path ?? '';
		case 'release': return t.kind === 'movie' ? movieOut(t) : '';
		case 'runtime': return t.runtime_minutes ? tr('mediafiles.view.runtime', { h: Math.floor(t.runtime_minutes / 60), m: t.runtime_minutes % 60 }) : '';
		case 'series': return series;
		case 'albums': return t.artist ? `${t.artist.included}/${t.artist.albums}` : '';
		case 'latest': return date(t.artist?.next_release);
		case 'status': return libraryState(t)?.label ?? '';
		case 'requested': return requestedBy(t);
		case 'quality': {
			const have = quality(t);
			const want = t.versions?.[0]?.label;
			return have || want ? `${have ?? '—'} → ${want ?? '—'}` : '';
		}
		case 'kept': return t.keep_up_to_date ? tr('mediafiles.view.yes') : tr('mediafiles.view.no');
		case 'showstate': return t.show?.status === 'ended' ? tr('mediafiles.view.ended') : t.show?.status ? tr('mediafiles.view.continuing') : '';
		case 'activity': return date(t.last_activity);
		default: return '';
	}
}

/** The "Everything" line under a poster: next airing for shows, else rules and quality. */
export function posterExtra(t: TitleView): string {
	if (t.kind === 'show' && t.show?.next_air_date) return tr('mediafiles.view.next', { date: date(t.show.next_air_date) });
	return [t.pick_rules_name, quality(t)].filter(Boolean).join(' · ');
}

/** The hover panel: quality and pick rules, next airing, added, who asked. */
export function posterFacts(t: TitleView): string[] {
	const asked = requestedBy(t);
	return [
		[quality(t), t.pick_rules_name].filter(Boolean).join(' · '),
		t.kind === 'show' && t.show?.next_air_date ? tr('mediafiles.view.next', { date: date(t.show.next_air_date) }) : '',
		t.added_at ? tr('mediafiles.view.added', { date: date(t.added_at) }) : '',
		asked ? tr('mediafiles.view.askedBy', { name: asked }) : ''
	].filter(Boolean);
}

/** The Library's rows: each movie, show or artist, with a heading before every series when grouped. */
export function buildEntries(shown: TitleView[], grouped: boolean, seriesOfTitle: Map<number, number>, seriesName: (id: number) => string): Entry[] {
	if (!grouped) return shown.map((t) => ({ type: 'row', key: `r${t.id}`, t }));
	const bySet = new Map<number, TitleView[]>();
	const loose: TitleView[] = [];
	for (const t of shown) {
		const s = seriesOfTitle.get(t.id);
		if (s) bySet.set(s, [...(bySet.get(s) ?? []), t]);
		else loose.push(t);
	}
	const out: Entry[] = [];
	for (const [id, ts] of [...bySet].sort((a, b) => seriesName(a[0]).localeCompare(seriesName(b[0])))) {
		out.push({ type: 'head', key: `h${id}`, id, name: seriesName(id), count: ts.length });
		for (const t of ts) out.push({ type: 'row', key: `r${t.id}`, t });
	}
	if (loose.length) {
		out.push({ type: 'head', key: 'h0', id: 0, name: tr('mediafiles.view.noCollection'), count: loose.length });
		for (const t of loose) out.push({ type: 'row', key: `r${t.id}`, t });
	}
	return out;
}
