import { t } from '../core/i18n/i18n.svelte.ts';

// Anime mapping types and helpers that turn them into labels.
export type AnimeTitle = { anilist: number; romaji?: string; english?: string; native?: string };
export type AnimeSeasonPart = { anilist?: number; offset: number; title?: AnimeTitle };
export type AnimeSeason = {
	number: number;
	anilist?: number;
	mal?: number;
	title?: AnimeTitle;
	offset?: number;
	absolute_from?: number;
	parts?: AnimeSeasonPart[];
};
export type AnimeView = { anime: boolean; mapped: boolean; alt_names: string[]; titles: AnimeTitle[]; seasons: AnimeSeason[] };

// Path of the anime metadata for one show.
export const animePath = (kind: string, tmdb: string) => `/metadata/${kind}/${encodeURIComponent(tmdb)}/anime`;

// Joins the distinct names of a title, skipping repeats that differ only by case.
export function titleLine(title: AnimeTitle | undefined): string {
	if (!title) return '';
	const seen = new Set<string>();
	const out: string[] = [];
	for (const v of [title.romaji, title.english, title.native]) {
		const k = (v ?? '').trim();
		if (k && !seen.has(k.toLowerCase())) {
			seen.add(k.toLowerCase());
			out.push(k);
		}
	}
	return out.join(' · ');
}

// Builds the small info line for a season: names, running numbers and later parts.
export function seasonAnimeLine(view: AnimeView | null | undefined, season: number, episodes: number): string {
	const s = view?.seasons.find((x) => x.number === season);
	if (!s) return '';
	const bits: string[] = [];
	const name = titleLine(s.title);
	if (name) bits.push(name);
	if (s.absolute_from && episodes > 0) {
		const end = s.absolute_from + episodes - 1;
		bits.push(episodes === 1 ? t('shows.anime.absoluteOne', { n: s.absolute_from }) : t('shows.anime.absoluteMany', { from: s.absolute_from, to: end }));
	}
	const later = (s.parts ?? []).filter((p) => p.offset > 0);
	for (const [i, p] of later.entries()) {
		bits.push(t('shows.anime.part', { part: i + 2, episode: p.offset + 1 }) + (p.title ? ` (${titleLine(p.title).split(' · ')[0]})` : ''));
	}
	return bits.join(' · ');
}

// A season can be split into cours; the episode is numbered from the last cour that started before it.
export function animeEpisode(view: AnimeView | null | undefined, season: number, episode: number): { anilist?: number; episode: number; absolute?: number } {
	const s = view?.seasons.find((x) => x.number === season);
	if (!s) return { episode };
	const parts = s.parts?.length ? [...s.parts].sort((a, b) => a.offset - b.offset) : [{ anilist: s.anilist, offset: s.offset ?? 0 }];
	let part = parts[0];
	for (const p of parts) if (p.offset < episode) part = p;
	return { anilist: part.anilist, episode: episode - part.offset, absolute: s.absolute_from ? s.absolute_from + episode - 1 : undefined };
}

