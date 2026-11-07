// Words and small rules for the show screens, kept apart from the pages so
// they can be tested.
import type { EpisodeView, SeasonSummary, ShowInfo } from './core/types.ts';
import { date } from './core/format.ts';

/** seasonChoices are what Keep up to date covers when a show is added. */
export const seasonChoices: [string, string, string][] = [
	['all', 'All seasons', 'Every episode of every season (specials stay off).'],
	['future', 'Future seasons only', 'Nothing that is out already: only episodes airing from today on, and every season to come.'],
	['latest', 'The latest season', 'Only the highest-numbered season.'],
	['none', 'None for now', 'Add the show without keeping any season up to date; turn seasons on later.']
];

/** episodeOrders are how episodes are numbered and matched. */
export const episodeOrders: [string, string, string][] = [
	['aired', 'Season and episode', 'S01E02, as the show aired. Right for most shows.'],
	['daily', 'By air day', 'For talk shows and news: the date names the episode.'],
	['absolute', 'One running number', 'For anime: episode 27, counted from the first.']
];

/** detailsSources are where a show's names and air dates come from. */
export const detailsSources: [string, string][] = [
	['tmdb', 'TMDB'],
	['tvmaze', 'TVMaze']
];

/** label finds the words for a stored value in one of the lists above. */
export function label(list: [string, string, ...string[]][], v: string | undefined): string {
	return list.find(([k]) => k === v)?.[1] ?? v ?? '';
}

/** seasonWords says which seasons an ask is for, like the server does. */
export function seasonWords(seasons: number[] | undefined): string {
	const s = seasons ?? [];
	if (s.length === 0) return 'the whole show';
	if (s.length === 1) return `season ${s[0]}`;
	return `seasons ${s.slice(0, -1).join(', ')} and ${s[s.length - 1]}`;
}

/** seasonName names a season: "Specials" for 0, else "Season 2". */
export function seasonName(n: number): string {
	return n === 0 ? 'Specials' : `Season ${n}`;
}

/** episodeCode is "S01E02". */
export function episodeCode(season: number, episode: number): string {
	const two = (n: number) => String(n).padStart(2, '0');
	return `S${two(season)}E${two(episode)}`;
}

/** episodeLabel numbers an episode in the show's own order. */
export function episodeLabel(e: Pick<EpisodeView, 'season' | 'episode' | 'absolute' | 'air_day'>, order: string): string {
	if (order === 'daily' && e.air_day) return `${e.air_day} (${episodeCode(e.season, e.episode)})`;
	if (order === 'absolute' && e.absolute) return `${e.absolute} (${episodeCode(e.season, e.episode)})`;
	return episodeCode(e.season, e.episode);
}

/** seasonLine sums up a season: "2 of 3 aired in the Library · 1 missing · next 5 Jan 2099". */
export function seasonLine(s: SeasonSummary): string {
	const parts = [`${s.in_library} of ${s.aired} aired in the Library`];
	if (s.missing) parts.push(`${s.missing} missing`);
	if (s.not_out_yet) parts.push(`${s.not_out_yet} not out yet`);
	if (s.next_air_date) parts.push(`next ${date(s.next_air_date)}`);
	return parts.join(' · ');
}

/** showState is one chip for a show card: the most pressing state first. */
export function showState(s: ShowInfo | undefined): { state: string; label: string } {
	if (!s) return { state: '', label: '' };
	if (s.missing) return { state: 'wanted', label: `${s.missing} missing` };
	if (s.better_wanted) return { state: 'better_wanted', label: 'Better copy sought' };
	if (s.in_library && s.in_library >= airedOf(s)) return { state: 'in_library', label: 'In library' };
	if (s.not_out_yet) return { state: 'not_out_yet', label: 'Not out yet' };
	if (s.in_library) return { state: 'in_library', label: `${s.in_library} in library` };
	return { state: '', label: 'Nothing kept yet' };
}

function airedOf(s: ShowInfo): number {
	return s.seasons.reduce((n, x) => n + x.aired, 0);
}

/** showProgress says "4 of 6 aired episodes in the Library". */
export function showProgress(s: ShowInfo | undefined): string {
	if (!s) return '';
	return `${s.in_library} of ${airedOf(s)} aired episodes in the Library`;
}

/** mainSlot is an episode's Slot in the main Version (the first one). */
export function mainSlot(e: EpisodeView) {
	return e.slots[0];
}

/** episodeWhy says in plain words why an episode is or is not looked for. */
export function episodeWhy(e: EpisodeView, seasonKept: boolean, showKept: boolean): string {
	const s = mainSlot(e);
	if (s && (s.state === 'in_library' || s.state === 'better_wanted')) return s.reason ?? '';
	if (!showKept) return 'Keep up to date is off for the show.';
	if (!seasonKept) return 'Keep up to date is off for this season.';
	if (!e.keep_up_to_date) return 'Keep up to date is off for this episode.';
	if (s?.state === 'not_out_yet') return e.air_date ? `Counts as missing once it airs (${date(e.air_date)}).` : 'No air date yet.';
	// The air-time notes ("aired …", "airs …") repeat the date shown beside the episode.
	if (s?.reason && !/^airs? |^aired /.test(s.reason)) return s.reason;
	return '';
}

/** ReleaseScope is what a Choose a release list on the show page is for. */
export interface ReleaseScope {
	/** "S01E02" or "Season 1". */
	label: string;
	/** Query for GET /titles/{id}/releases: "item_id=5" or "season=1". */
	query: string;
	/** The episode's Slot (main Version) for an episode; none for a season. */
	slotId?: number;
	slotState?: string;
}

const searchStates = ['wanted', 'better_wanted', 'failed', 'rejected', 'stalled', 'not_out_yet', 'held', 'needs_a_decision'];

/** searchWord is the button for searching one episode Slot by hand, or ''
 * when it cannot be searched now (busy, or good enough already). */
export function searchWord(state: string | undefined): string {
	if (!state || !searchStates.includes(state)) return '';
	return state === 'better_wanted' ? 'Find a better copy' : 'Search now';
}

/** seasonLooking counts the episodes of a season that a search could fill. */
export function seasonLooking(eps: EpisodeView[]): number {
	return eps.filter((e) => ['wanted', 'better_wanted', 'failed', 'rejected', 'stalled'].includes(mainSlot(e)?.state ?? '')).length;
}

type SeasonLike = { number: number; aired?: number; episodes?: number; not_out_yet?: number; next_air_date?: string; year?: number };

/**
 * newestSeason is the season a show page opens on: the highest regular
 * season that has aired or upcoming episodes, else the highest regular
 * season, else the first one listed (specials only).
 */
export function newestSeason(seasons: SeasonLike[]): number | undefined {
	const regular = seasons.filter((s) => s.number > 0).sort((a, b) => b.number - a.number);
	const live = regular.find((s) => (s.aired ?? s.episodes ?? 0) > 0 || (s.not_out_yet ?? 0) > 0 || !!s.next_air_date);
	return (live ?? regular[0] ?? seasons[0])?.number;
}

/**
 * newSeason is the season that gets the "New" mark in the season picker:
 * the newest season when it is still airing or about to (an upcoming
 * episode), or, without episode counts, when it came out this year or
 * last. Undefined when no season is new.
 */
export function newSeason(seasons: SeasonLike[], now = new Date()): number | undefined {
	const n = newestSeason(seasons);
	const s = seasons.find((x) => x.number === n);
	if (!s || s.number <= 0) return undefined;
	if ((s.not_out_yet ?? 0) > 0 || s.next_air_date) return s.number;
	if (s.aired === undefined && s.year && s.year >= now.getFullYear() - 1) return s.number;
	return undefined;
}

/** keepOffNote is the one Keep up to date note for a whole season, said once on the season line. */
export function keepOffNote(seasonKept: boolean, showKept: boolean): string {
	if (!showKept) return 'Keep up to date is off for the show.';
	if (!seasonKept) return 'Keep up to date is off for this season.';
	return '';
}
