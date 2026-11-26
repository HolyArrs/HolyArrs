// Points column and "Below good enough" filter for a season's episode rows.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

type PointsSlot = { file?: unknown; file_id?: number; good_enough?: boolean; points?: number; good_enough_points?: number };

// The saved flag, not a fresh score, so the filter matches what keeps searching.
export const belowGoodEnough = (e: { slots: PointsSlot[] }) => e.slots.some((x) => (x.file || x.file_id) && x.good_enough === false);

// "42/60" for the first scored copy, with its spoken label; empty when none is scored.
export function pointsOf(e: { slots: PointsSlot[] }): { text: string; label: string } {
	const sl = e.slots.find((x) => x.points !== undefined);
	if (!sl || sl.points === undefined) return { text: '', label: '' };
	const need = sl.good_enough_points;
	if (need === undefined) return { text: String(sl.points), label: `${t('shows.list.points')}: ${sl.points}` };
	return { text: `${sl.points}/${need}`, label: t('shows.list.pointsOf', { n: sl.points, need }) };
}

// The episodes a season shows: only those below good enough while the filter is on.
export const shownEpisodes = <E extends { slots: PointsSlot[] }>(eps: E[], below: boolean): E[] => (below ? eps.filter(belowGoodEnough) : eps);
