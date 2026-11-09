// Labels, choices and status text for shows, seasons and episodes.
import type { EpisodeView, SeasonSummary, ShowInfo } from '../core/types.ts';
import { date } from '../core/format.ts';
import { episodeCode } from '../core/episodes.ts';

export { episodeCode };

type SeasonLike = { number: number; aired?: number; episodes?: number; not_out_yet?: number; next_air_date?: string; year?: number };

// Slot states where a manual search makes sense.
const searchStates = ['wanted', 'better_wanted', 'failed', 'rejected', 'stalled', 'not_out_yet', 'held', 'needs_a_decision'];

// Choices are [value, name, help text].
export const seasonChoices: [string, string, string][] = [
	['all', 'All seasons', 'Every episode of every season (specials stay off).'],
	['future', 'Future seasons only', 'Nothing that is out already: only episodes airing from today on, and every season to come.'],
	['latest', 'The latest season', 'Only the highest-numbered season.'],
	['none', 'None for now', 'Add the show without keeping any season up to date; turn seasons on later.']
];

export const episodeOrders: [string, string, string][] = [
	['aired', 'Season and episode', 'S01E02, as the show aired. Right for most shows.'],
	['daily', 'By air day', 'For talk shows and news: the date names the episode.'],
	['absolute', 'One running number', 'For anime: episode 27, counted from the first.']
];

export const detailsSources: [string, string][] = [
	['tmdb', 'TMDB'],
	['tvmaze', 'TVMaze']
];

export function label(list: [string, string, ...string[]][], v: string | undefined): string {
	return list.find(([k]) => k === v)?.[1] ?? v ?? '';
}

// Matches the wording the server uses for the same list.
export function seasonWords(seasons: number[] | undefined): string {
	const s = seasons ?? [];
	if (s.length === 0) return 'the whole show';
	if (s.length === 1) return `season ${s[0]}`;
	return `seasons ${s.slice(0, -1).join(', ')} and ${s[s.length - 1]}`;
}

export function seasonName(n: number): string {
	return n === 0 ? 'Specials' : `Season ${n}`;
}

export function episodeLabel(e: Pick<EpisodeView, 'season' | 'episode' | 'absolute' | 'air_day'>, order: string): string {
	if (order === 'daily' && e.air_day) return `${e.air_day} (${episodeCode(e.season, e.episode)})`;
	if (order === 'absolute' && e.absolute) return `${e.absolute} (${episodeCode(e.season, e.episode)})`;
	return episodeCode(e.season, e.episode);
}

export function seasonLine(s: SeasonSummary): string {
	const parts = [`${s.in_library} of ${s.aired} aired in the Library`];
	if (s.missing) parts.push(`${s.missing} missing`);
	if (s.not_out_yet) parts.push(`${s.not_out_yet} not out yet`);
	if (s.next_air_date) parts.push(`next ${date(s.next_air_date)}`);
	return parts.join(' · ');
}

export function showProgress(s: ShowInfo | undefined): string {
	if (!s) return '';
	return `${s.in_library} of ${airedOf(s)} aired episodes in the Library`;
}

export function mainSlot(e: EpisodeView) {
	return e.slots[0];
}

// Explains why an episode is or is not kept, from the broadest switch to the narrowest.
export function episodeWhy(e: EpisodeView, seasonKept: boolean, showKept: boolean): string {
	const s = mainSlot(e);
	if (s && (s.state === 'in_library' || s.state === 'better_wanted')) return s.reason ?? '';
	if (!showKept) return 'Keep up to date is off for the show.';
	if (!seasonKept) return 'Keep up to date is off for this season.';
	if (!e.keep_up_to_date) return 'Keep up to date is off for this episode.';
	if (s?.state === 'not_out_yet') return e.air_date ? `Counts as missing once it airs (${date(e.air_date)}).` : 'No air date yet.';
	// Notes like "aired ..." repeat the date already shown beside the episode.
	if (s?.reason && !/^airs? |^aired /.test(s.reason)) return s.reason;
	return '';
}

export function searchWord(state: string | undefined): string {
	if (!state || !searchStates.includes(state)) return '';
	return state === 'better_wanted' ? 'Find a better copy' : 'Search now';
}

export function seasonLooking(eps: EpisodeView[]): number {
	return eps.filter((e) => ['wanted', 'better_wanted', 'failed', 'rejected', 'stalled'].includes(mainSlot(e)?.state ?? '')).length;
}

// Skips specials and seasons with nothing aired or coming.
export function newestSeason(seasons: SeasonLike[]): number | undefined {
	const regular = seasons.filter((s) => s.number > 0).sort((a, b) => b.number - a.number);
	const live = regular.find((s) => (s.aired ?? s.episodes ?? 0) > 0 || (s.not_out_yet ?? 0) > 0 || !!s.next_air_date);
	return (live ?? regular[0] ?? seasons[0])?.number;
}

// Without episode counts, a season from this year or last counts as new.
export function newSeason(seasons: SeasonLike[], now = new Date()): number | undefined {
	const n = newestSeason(seasons);
	const s = seasons.find((x) => x.number === n);
	if (!s || s.number <= 0) return undefined;
	if ((s.not_out_yet ?? 0) > 0 || s.next_air_date) return s.number;
	if (s.aired === undefined && s.year && s.year >= now.getFullYear() - 1) return s.number;
	return undefined;
}

export function keepOffNote(seasonKept: boolean, showKept: boolean): string {
	if (!showKept) return 'Keep up to date is off for the show.';
	if (!seasonKept) return 'Keep up to date is off for this season.';
	return '';
}

// Total aired episodes across all seasons.
function airedOf(s: ShowInfo): number {
	return s.seasons.reduce((n, x) => n + x.aired, 0);
}
