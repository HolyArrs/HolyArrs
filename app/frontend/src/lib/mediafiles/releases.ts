// Plain-words labels, warnings and paging for the Releases list and search status lines.
import type { Reason, ReleasePage, ReleaseView } from '../core/types.ts';
import { t } from '../core/i18n/i18n.svelte.ts';

export interface Pager {
	from: number;
	to: number;
	prev: number | null;
	next: number | null;
}

export interface ReleaseScope {
	// Heading shown for what is being searched.
	label: string;
	// Search text sent to the sources.
	query: string;
	// Set when the search is for one slot only.
	slotId?: number;
	slotState?: string;
}

const outcomeKeys = ['chosen', 'held', 'passed_over', 'refused', 'turned_down'];
const outcomeWord = (k: string): string | undefined => (outcomeKeys.includes(k) ? t(`mediafiles.release.${k}`) : undefined);

// A real state beats the judged outcome; an outcome alone is only what "would" happen.
export function verdict(r: Pick<ReleaseView, 'state' | 'outcome'>): string {
	if (r.state === 'turned_down') return t('mediafiles.release.turned_down');
	if (r.state === 'chosen') return t('mediafiles.release.chosen');
	if (r.state === 'held') return t('mediafiles.release.held');
	if (r.outcome === 'chosen') return t('mediafiles.release.wouldChoose');
	if (r.outcome === 'held') return t('mediafiles.release.wouldHold');
	return outcomeWord(r.outcome) ?? outcomeWord(r.state) ?? t('mediafiles.release.judged');
}

export function verdictTone(r: Pick<ReleaseView, 'state' | 'outcome'>): 'red' | 'ok' | 'wait' | 'fail' | 'idle' {
	if (r.state === 'chosen') return 'red';
	if (r.state === 'turned_down' || r.outcome === 'refused') return 'fail';
	if (r.outcome === 'chosen') return 'ok';
	if (r.outcome === 'held' || r.state === 'held') return 'wait';
	return 'idle';
}

// Only reasons that changed the result are worth showing up front.
export function keyReasons(reasons: Reason[]): Reason[] {
	return reasons.filter((x) => x.effect === 'fail' || x.effect === 'rank' || x.effect === 'hold' || x.effect === 'warn');
}

export function facts(r: ReleaseView, size: (n: number) => string): string {
	return [
		r.source_name,
		size(r.size),
		r.seeders != null ? t('mediafiles.release.seeders', { n: r.seeders }) : '',
		r.protocol,
		r.passed ? t('mediafiles.release.points', { n: r.points }) : '',
		r.passed && r.rank ? t('mediafiles.release.ranked', { n: r.rank }) : ''
	]
		.filter(Boolean)
		.join(' · ');
}

// Tells the user what choosing this release overrides, worded for a movie, show or album.
export function chooseWarnings(r: ReleaseView, slotState?: string, show = false, music = false): string[] {
	const out: string[] = [];
	if (r.needs_force) {
		const fails = r.reasons.filter((x) => x.effect === 'fail').map((x) => x.text);
		out.push(t('mediafiles.release.didNotPass', { why: fails.length ? fails.join('; ') : t('mediafiles.release.wasRefused') }));
	}
	if (music) {
		if (r.gate_skipped || slotState === 'not_out_yet') out.push(t('mediafiles.release.albumNotOut'));
		const n = r.slot_ids?.length ?? 0;
		if (n > 1) out.push(r.album ? t('mediafiles.release.fillsTracksOf', { n, album: r.album }) : t('mediafiles.release.fillsTracks', { n }));
		return out;
	}
	if (show) {
		if (r.gate_skipped || slotState === 'not_out_yet') out.push(t('mediafiles.release.episodeNotAired'));
		if (slotState === 'in_library') out.push(t('mediafiles.release.episodeHasCopy'));
		if ((r.slot_ids?.length ?? 0) > 1 && r.episode) out.push(t('mediafiles.release.fillsEpisode', { episode: r.episode }));
		return out;
	}
	if (r.gate_skipped || slotState === 'not_out_yet') out.push(t('mediafiles.release.gateSkipped'));
	if (slotState === 'in_library') out.push(t('mediafiles.release.versionHasCopy'));
	return out;
}

export function listedName(r: Pick<ReleaseView, 'title_name' | 'episode' | 'turned_down_for' | 'state' | 'album'>): string {
	if (r.album) return [r.title_name, r.album].filter(Boolean).join(' – ');
	const ep = r.state === 'turned_down' && r.turned_down_for ? r.turned_down_for : r.episode;
	return [r.title_name, ep].filter(Boolean).join(' ');
}

// Shows "from to total" bounds, clamping an offset that ran past the end.
export function pager(total: number, limit: number, offset: number): Pager {
	if (total <= 0) return { from: 0, to: 0, prev: null, next: null };
	const from = Math.min(offset, total - 1) + 1;
	const to = Math.min(offset + limit, total);
	return { from, to, prev: offset > 0 ? Math.max(0, offset - limit) : null, next: to < total ? offset + limit : null };
}

// 0 turns feed checks off; otherwise 10 to 120 minutes.
export function feedMinutesOk(n: number): boolean {
	return Number.isInteger(n) && (n === 0 || (n >= 10 && n <= 120));
}

// Short badge words for what the server read from a Release name. Technical names stay as they are in every language.
const sourceWord: Record<string, string> = { remux: 'Remux', bluray: 'BluRay', web_dl: 'WEB-DL', web_rip: 'WEBRip', tv: 'HDTV', dvd: 'DVD', disc_image: 'Disc', cam: 'CAM', telesync: 'TS' };
const hdrWord: Record<string, string> = { dv: 'Dolby Vision', hdr10plus: 'HDR10+', hdr10: 'HDR10', hlg: 'HLG' };
const codecWord: Record<string, string> = { x265: 'x265', h265: 'x265', hevc: 'x265', x264: 'x264', h264: 'x264', av1: 'AV1' };
export const resolutionWord = (n: number): string => (n >= 2160 ? '4K' : `${n}p`);

export function readTags(r: Pick<ReleaseView, 'read'>): string[] {
	const x = r.read;
	if (!x) return [];
	const out: string[] = [];
	if (x.format) {
		out.push(x.format.toUpperCase());
		if (x.bit_depth) out.push(`${x.bit_depth}-bit`);
		else if (x.bitrate) out.push(`${x.bitrate} kbps`);
	}
	if (x.resolution) out.push(resolutionWord(x.resolution));
	if (x.source && sourceWord[x.source]) out.push(sourceWord[x.source]);
	for (const h of x.hdr ?? []) if (hdrWord[h]) out.push(hdrWord[h]);
	if (x.codec) out.push(codecWord[x.codec.toLowerCase()] ?? x.codec);
	if (x.atmos) out.push('Atmos');
	else if (x.channels) out.push(x.channels);
	for (const l of x.languages ?? []) out.push(l.toUpperCase());
	if (x.proper) out.push('PROPER');
	return out;
}

// Rank of the read quality, higher is better: resolution first, then source.
const sourceRank: Record<string, number> = { remux: 9, bluray: 8, disc_image: 8, web_dl: 7, web_rip: 6, tv: 5, dvd: 4, telesync: 1, cam: 0 };
export const qualityRank = (r: Pick<ReleaseView, 'read'>): number => (r.read?.resolution ?? 0) * 10 + (sourceRank[r.read?.source ?? ''] ?? 3) + (r.read?.bit_depth ?? 0) + (r.read?.lossless ? 5 : 0);

export type ReleaseSort = 'best' | 'points' | 'quality' | 'size' | 'seeders' | 'age';
export const releaseSorts: ReleaseSort[] = ['best', 'points', 'quality', 'size', 'seeders', 'age'];

// The judge's order: chosen, then passed, then best rank and points.
const byBest = (a: ReleaseView, b: ReleaseView): number => Number(b.outcome === 'chosen') - Number(a.outcome === 'chosen') || Number(b.passed) - Number(a.passed) || (a.rank || 999) - (b.rank || 999) || b.points - a.points;
const when = (r: ReleaseView): number => (r.published_at ? Date.parse(r.published_at) : 0);

export function sortReleases(list: ReleaseView[], by: ReleaseSort): ReleaseView[] {
	const cmp: Record<ReleaseSort, (a: ReleaseView, b: ReleaseView) => number> = {
		best: byBest,
		points: (a, b) => b.points - a.points || byBest(a, b),
		quality: (a, b) => qualityRank(b) - qualityRank(a) || byBest(a, b),
		size: (a, b) => b.size - a.size || byBest(a, b),
		seeders: (a, b) => (b.seeders ?? -1) - (a.seeders ?? -1) || byBest(a, b),
		age: (a, b) => when(b) - when(a) || byBest(a, b)
	};
	return [...list].sort(cmp[by]);
}

// Resolutions present in a list, best first, for the filter chips.
export const resolutionsOf = (list: ReleaseView[]): number[] => [...new Set(list.map((r) => r.read?.resolution ?? 0).filter(Boolean))].sort((a, b) => b - a);

// Name text filter: every word must appear, dots and spaces alike.
export function nameMatches(name: string, q: string): boolean {
	const n = name.toLowerCase().replace(/[._\-]+/g, ' ');
	return q.toLowerCase().replace(/[._\-]+/g, ' ').split(/\s+/).filter(Boolean).every((w) => n.includes(w));
}

// Source names present in a list, A to Z, for the Source filter.
export const sourcesOf = (list: ReleaseView[]): string[] => [...new Set(list.map((r) => r.source_name).filter(Boolean))].sort((a, b) => a.localeCompare(b));

// Audio languages read from the names, most common first, for the Language filter.
export function languagesOf(list: ReleaseView[]): string[] {
	const n = new Map<string, number>();
	for (const r of list) for (const l of new Set(r.read?.languages ?? [])) n.set(l, (n.get(l) ?? 0) + 1);
	return [...n].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([l]) => l);
}

/** A release list for a movie not in the Library yet: the sheet loads, searches and picks through it. */
export interface ReleaseCheck {
	load(): Promise<ReleasePage>;
	/** Starts a check and gives its Job ids. */
	search(): Promise<number[]>;
	/** Adds the movie and sends the release; rejects to keep the sheet open. */
	pick(r: ReleaseView): Promise<void>;
}
