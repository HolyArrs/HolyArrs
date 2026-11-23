// Poster card helpers: attention state, progress bar, tilt, detail level and move animation.
import type { LibState, LibStateKey } from './library-state.ts';
import { t } from '../core/i18n/i18n.svelte.ts';
import type { TitleView } from '../core/types.ts';
import { date } from '../core/format.ts';

export type BarTone = 'ok' | 'busy' | 'gap' | 'fail' | 'idle';
export type PosterDetail = 'title' | 'status' | 'full';
type Store = Pick<Storage, 'getItem' | 'setItem'> | undefined;

// States a person may need to act on or watch.
export const attentionKeys: readonly LibStateKey[] = [
	'failed', 'needs_decision', 'stalled', 'missing_file', 'missing', 'better_wanted',
	'paused', 'downloading', 'queued', 'waiting_approval', 'not_out_yet'
];

// The labels read their words when used, so they follow the language.
export const detailLabels: Record<PosterDetail, string> = {
	get title() {
		return t('mediafiles.poster.titleOnly');
	},
	get status() {
		return t('mediafiles.poster.titleProblems');
	},
	get full() {
		return t('mediafiles.poster.everything');
	}
};

const clamp01 = (n: number) => (Number.isFinite(n) ? Math.min(Math.max(n, 0), 1) : 0);
const detailKey = (member: number | undefined) => `holyarrs.poster-detail.${member ?? 'guest'}`;
const datesKey = (member: number | undefined) => `holyarrs.poster-dates.${member ?? 'guest'}`;

export function needsAttention(s: LibState | null | undefined): boolean {
	return !!s && attentionKeys.includes(s.key);
}

// Live percent wins while a download runs; otherwise the bar shows the share owned.
export function barOf(s: LibState | null | undefined, have: number, total: number): { tone: BarTone; fill: number } {
	const share = total > 0 ? clamp01(have / total) : 0;
	if (!s) return { tone: 'ok', fill: total > 0 ? share : 1 };
	switch (s.key) {
		case 'failed': case 'needs_decision': case 'stalled': case 'missing_file':
			return { tone: 'fail', fill: s.percent !== undefined ? clamp01(s.percent / 100) : 1 };
		case 'downloading': case 'queued': case 'paused':
			return { tone: 'busy', fill: s.percent !== undefined ? clamp01(s.percent / 100) : share };
		case 'missing': case 'better_wanted':
			return { tone: 'gap', fill: total > 0 ? share : 1 };
		case 'not_out_yet': case 'waiting_approval': case 'approved': case 'declined': case 'not_kept':
			return { tone: 'idle', fill: total > 0 ? share : 1 };
		default:
			return { tone: 'ok', fill: 1 };
	}
}

// Hover tilt in degrees from the pointer spot; a max of 4 keeps it subtle.
export function tiltFor(x: number, y: number, w: number, h: number, max = 4): { rx: number; ry: number } {
	if (!(w > 0) || !(h > 0)) return { rx: 0, ry: 0 };
	const nx = Math.min(Math.max(x / w, 0), 1) * 2 - 1;
	const ny = Math.min(Math.max(y / h, 0), 1) * 2 - 1;
	// The "|| 0" turns -0 into 0.
	const r = (n: number) => Math.round(n * max * 100) / 100 || 0;
	return { rx: r(-ny), ry: r(nx) };
}

export function loadPosterDetail(member: number | undefined, store: Store = globalThis.localStorage): PosterDetail {
	try {
		const v = store?.getItem(detailKey(member));
		return v === 'title' || v === 'full' ? v : 'status';
	} catch {
		return 'status';
	}
}

export function savePosterDetail(member: number | undefined, d: PosterDetail, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(detailKey(member), d);
	} catch {
		// Storage can be blocked (private window); nothing to do.
	}
}

// The poster caption's date: the year, or with full dates on, a movie's first release day, so it is the same day the year counts from
// (a later home release, such as a 4K re-release, would read as a different year). Shows and artists keep the year here; their day
// goes on its own line (posterNote).
type DatedTitle = Pick<TitleView, 'kind' | 'year' | 'digital_date' | 'physical_date' | 'cinema_date'> & {
	show?: Pick<NonNullable<TitleView['show']>, 'next_air_date' | 'last_air_date'>;
	artist?: Pick<NonNullable<TitleView['artist']>, 'last_release'>;
};
export function posterYear(x: DatedTitle, full: boolean): string {
	if (full && x.kind === 'movie') {
		const first = [x.cinema_date, x.digital_date, x.physical_date].filter((s): s is string => !!s && !Number.isNaN(Date.parse(s))).sort((a, b) => Date.parse(a) - Date.parse(b))[0];
		const d = date(first);
		if (d) return d;
	}
	return x.year ? String(x.year) : '';
}

// With full dates on, the labelled day under a show's or artist's caption: the next episode's air day (or, with none coming,
// the last one that aired) and an artist's newest album out. '' when off or unknown.
export function posterNote(x: DatedTitle, full: boolean): string {
	if (!full) return '';
	if (x.kind === 'show') {
		const next = date(x.show?.next_air_date);
		if (next) return t('library.page.dateNextEpisode', { date: next });
		const last = date(x.show?.last_air_date);
		return last ? t('library.page.dateLastAired', { date: last }) : '';
	}
	if (x.kind === 'artist') {
		const last = date(x.artist?.last_release);
		return last ? t('library.page.dateNewestRelease', { date: last }) : '';
	}
	return '';
}

export function loadPosterDates(member: number | undefined, store: Store = globalThis.localStorage): boolean {
	try {
		return store?.getItem(datesKey(member)) === '1';
	} catch {
		return false;
	}
}

export function savePosterDates(member: number | undefined, on: boolean, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(datesKey(member), on ? '1' : '0');
	} catch {
		// Storage can be blocked (private window); nothing to do.
	}
}

// Transform that slides a card from its new spot back to its old one; null when it barely moved.
export function flipFrom(a: { left: number; top: number; width: number }, b: { left: number; top: number; width: number }): string | null {
	const dx = a.left - b.left;
	const dy = a.top - b.top;
	const s = b.width > 0 ? a.width / b.width : 1;
	if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5 && Math.abs(s - 1) < 0.005) return null;
	return `translate(${Math.round(dx)}px, ${Math.round(dy)}px) scale(${Math.round(s * 1000) / 1000})`;
}
