// Labels, choices and status text for shows, seasons and episodes.
import type { EpisodeView, SeasonSummary, ShowInfo } from '../core/types.ts';
import { date } from '../core/format.ts';
import { t } from '../core/i18n/i18n.svelte.ts';
import { episodeCode } from '../core/episodes.ts';

export { episodeCode };

type SeasonLike = { number: number; aired?: number; episodes?: number; not_out_yet?: number; next_air_date?: string; year?: number };

// Slot states where a manual search makes sense.
const searchStates = ['wanted', 'better_wanted', 'failed', 'rejected', 'stalled', 'not_out_yet', 'held', 'needs_a_decision'];

// Choices are [value, name, help text]. The name and help read their words when used, so they follow the language.
function choice(value: string, name: string, help: string): [string, string, string] {
	const row = [value, '', ''] as [string, string, string];
	Object.defineProperty(row, 1, { get: () => t(name), enumerable: true });
	Object.defineProperty(row, 2, { get: () => t(help), enumerable: true });
	return row;
}

export const seasonChoices: [string, string, string][] = [
	choice('all', 'shows.seasonChoice.all', 'shows.seasonChoice.allHelp'),
	choice('future', 'shows.seasonChoice.future', 'shows.seasonChoice.futureHelp'),
	choice('latest', 'shows.seasonChoice.latest', 'shows.seasonChoice.latestHelp'),
	choice('none', 'shows.seasonChoice.none', 'shows.seasonChoice.noneHelp')
];

export const episodeOrders: [string, string, string][] = [
	choice('aired', 'shows.episodeOrder.aired', 'shows.episodeOrder.airedHelp'),
	choice('daily', 'shows.episodeOrder.daily', 'shows.episodeOrder.dailyHelp'),
	choice('absolute', 'shows.episodeOrder.absolute', 'shows.episodeOrder.absoluteHelp')
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
	if (s.length === 0) return t('shows.seasonWords.whole');
	if (s.length === 1) return t('shows.seasonWords.one', { n: s[0] });
	return t('shows.seasonWords.many', { list: s.slice(0, -1).join(', '), last: s[s.length - 1] });
}

export function seasonName(n: number): string {
	return n === 0 ? t('shows.season.specials') : t('shows.season.named', { n });
}

export function episodeLabel(e: Pick<EpisodeView, 'season' | 'episode' | 'absolute' | 'air_day'>, order: string): string {
	if (order === 'daily' && e.air_day) return `${e.air_day} (${episodeCode(e.season, e.episode)})`;
	if (order === 'absolute' && e.absolute) return `${e.absolute} (${episodeCode(e.season, e.episode)})`;
	return episodeCode(e.season, e.episode);
}

export function seasonLine(s: SeasonSummary): string {
	const parts = [t('shows.season.inLibrary', { in: s.in_library, aired: s.aired })];
	if (s.missing) parts.push(t('shows.season.missing', { n: s.missing }));
	if (s.not_out_yet) parts.push(t('shows.season.notOutYet', { n: s.not_out_yet }));
	if (s.next_air_date) parts.push(t('shows.season.next', { date: date(s.next_air_date) }));
	return parts.join(' · ');
}

export function showProgress(s: ShowInfo | undefined): string {
	if (!s) return '';
	return t('shows.progress', { in: s.in_library, aired: airedOf(s) });
}

export function mainSlot(e: EpisodeView) {
	return e.slots[0];
}

// Explains why an episode is or is not kept, from the broadest switch to the narrowest.
export function episodeWhy(e: EpisodeView, seasonKept: boolean, showKept: boolean): string {
	const s = mainSlot(e);
	if (s && (s.state === 'in_library' || s.state === 'better_wanted')) return s.reason ?? '';
	if (!showKept) return t('shows.keepOff.show');
	if (!seasonKept) return t('shows.keepOff.season');
	if (!e.keep_up_to_date) return t('shows.keepOff.episode');
	if (s?.state === 'not_out_yet') return e.air_date ? t('shows.why.missingOnAir', { date: date(e.air_date) }) : t('shows.why.noAirDate');
	// Notes like "aired ..." repeat the date already shown beside the episode.
	if (s?.reason && !/^airs? |^aired /.test(s.reason)) return s.reason;
	return '';
}

export function searchWord(state: string | undefined): string {
	if (!state || !searchStates.includes(state)) return '';
	return state === 'better_wanted' ? t('shows.search.better') : t('shows.search.now');
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
	if (!showKept) return t('shows.keepOff.show');
	if (!seasonKept) return t('shows.keepOff.season');
	return '';
}

// Total aired episodes across all seasons.
function airedOf(s: ShowInfo): number {
	return s.seasons.reduce((n, x) => n + x.aired, 0);
}
