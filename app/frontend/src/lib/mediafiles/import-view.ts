import { canAdopt, matchKey, score, type AdoptCandidate, type AdoptStatus } from './organise.ts';
import type { TitleView } from '../core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

// The scanned folders on the Import a library page: their order, the search and filter, the picks and the views.

export type ImportView = 'rows' | 'banners' | 'posters' | 'overview';
export type ImportFilter = 'all' | AdoptStatus;
/** A title picked by a lookup, kept by its id so the folder shows what was chosen. */
export type Named = { name: string; year?: number; poster?: string };
/** What a folder shows for its pick: the scan's match, one of its others, or a looked-up title. */
export type Picked = { name: string; year?: number; score?: number; poster: string };
type BannerTitle = Pick<TitleView, 'kind' | 'name' | 'ids' | 'artist' | 'artwork' | 'backdrop' | 'logo'>;

// Folders that need a look come first, then the sure ones, then the ones left alone.
const listOrder: AdoptStatus[] = ['unsure', 'unmatched', 'matched', 'in_library', 'never_add', 'no_video'];
// The filter keeps the scan's own order of the groups.
const filterOrder: AdoptStatus[] = ['matched', 'unsure', 'unmatched', 'in_library', 'never_add', 'no_video'];

export function readImportView(v: unknown): ImportView {
	return v === 'banners' || v === 'posters' || v === 'overview' ? v : 'rows';
}

export function needsLook(f: Pick<AdoptCandidate, 'status'>): boolean {
	return f.status === 'unsure' || f.status === 'unmatched';
}

// The pick shown for a folder; with nothing picked, the name read from the folder.
export function pickedOf(f: AdoptCandidate, id: string | undefined, named: Record<string, Named> = {}): Picked {
	const m = id ? [f.match, ...f.others].find((x) => x && matchKey(x) === id) : undefined;
	if (m) return { name: m.name, year: m.year || undefined, score: m.score, poster: m.poster ?? '' };
	const n = id ? named[id] : undefined;
	if (n) return { name: n.name, year: n.year, poster: n.poster ?? '' };
	return { name: f.parsed_name || f.folder, year: f.parsed_year, poster: '' };
}

// Unsure folders first, then by title; the folder name settles ties.
export function sortFolders(folders: AdoptCandidate[], locale?: string): AdoptCandidate[] {
	const rank = (f: AdoptCandidate) => {
		const i = listOrder.indexOf(f.status);
		return i < 0 ? listOrder.length : i;
	};
	const title = (f: AdoptCandidate) => f.match?.name || f.parsed_name || f.folder;
	return [...folders].sort((a, b) => rank(a) - rank(b) || title(a).localeCompare(title(b), locale) || a.folder.localeCompare(b.folder, locale));
}

// Matches the words against the title and the folder name, ignoring case and accents.
export function findFolders(folders: AdoptCandidate[], words: string, filter: ImportFilter = 'all'): AdoptCandidate[] {
	const want = fold(words).split(/\s+/).filter(Boolean);
	return folders.filter((f) => {
		if (filter !== 'all' && f.status !== filter) return false;
		if (!want.length) return true;
		const m = f.match;
		const hay = fold([f.folder, f.parsed_name, f.parsed_year ?? '', m?.name ?? '', m?.year ?? ''].join(' '));
		return want.every((w) => hay.includes(w));
	});
}

// One count per status found, in the filter's order.
export function statusCounts(folders: AdoptCandidate[]): { status: AdoptStatus; n: number }[] {
	return filterOrder.map((status) => ({ status, n: folders.filter((f) => f.status === status).length })).filter((c) => c.n > 0);
}

// The choices of the match picker: the scan's match and its others once each, plus a looked-up pick.
export function matchOptions(f: AdoptCandidate, id: string | undefined, named: Record<string, Named> = {}): { value: string; label: string }[] {
	const out: { value: string; label: string }[] = [];
	for (const m of [f.match, ...f.others]) {
		if (!m) continue;
		const value = matchKey(m);
		if (!value || out.some((o) => o.value === value)) continue;
		out.push({ value, label: `${m.name}${m.year ? ` (${m.year})` : ''} · ${score(m.score)}` });
	}
	if (id && !out.some((o) => o.value === id)) {
		const n = named[id];
		// A bare number is a movie's TMDB id, typed in the lookup.
		out.push({ value: id, label: n ? `${n.name}${n.year ? ` (${n.year})` : ''}` : /^\d+$/.test(id) ? t('library.import.movieId', { id }) : id });
	}
	return out;
}

// The status in words, as the filter and each folder's pill say it.
export function statusLabel(s: AdoptStatus, kind: 'movie' | 'show' | 'music' = 'movie'): string {
	if (s === 'in_library') return t('mediafiles.adopt.inLibrary');
	if (s === 'never_add') return t('mediafiles.adopt.neverAdd');
	if (s === 'no_video') return t(kind === 'music' ? 'mediafiles.adopt.noAudio' : 'mediafiles.adopt.noVideo');
	return t(`mediafiles.adopt.${s}`);
}

/** A folder can be ticked once it can be adopted and has a pick. */
export function canTick(f: AdoptCandidate, sel: Record<string, string>): boolean {
	return canAdopt(f) && !!sel[f.folder];
}

// The state of a tick-all box over the folders shown.
export function tickState(folders: AdoptCandidate[], sel: Record<string, string>, ticked: Record<string, boolean>): 'all' | 'some' | 'none' {
	const can = folders.filter((f) => canTick(f, sel));
	const on = can.filter((f) => ticked[f.folder]).length;
	return on === 0 ? 'none' : on === can.length ? 'all' : 'some';
}

// How many of the shown folders can be ticked, and how many of those are.
export function tickCounts(folders: AdoptCandidate[], sel: Record<string, string>, ticked: Record<string, boolean>): { can: number; on: number } {
	const can = folders.filter((f) => canTick(f, sel));
	return { can: can.length, on: can.filter((f) => ticked[f.folder]).length };
}

// Enough of a title for the Library's banner art: the id it asks the extras for, and the poster as the fallback.
export function bannerTitle(kind: 'movie' | 'show' | 'music', id: string | undefined, p: Picked): BannerTitle {
	const [src, raw] = id?.includes(':') ? [id.slice(0, id.indexOf(':')), id.slice(id.indexOf(':') + 1)] : ['tmdb', id ?? ''];
	const base = { name: p.name, artwork: p.poster ? [p.poster] : [], backdrop: '', logo: '' };
	if (kind === 'music') return { ...base, kind: 'artist', ids: null, artist: src === 'mbid' && raw ? ({ mbid: raw } as TitleView['artist']) : undefined };
	return { ...base, kind, ids: src === 'tmdb' && /^\d+$/.test(raw) ? { tmdb: raw } : {}, artist: undefined };
}

function fold(s: string | number): string {
	return String(s).normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
}
