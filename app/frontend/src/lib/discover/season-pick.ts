export type SeasonPick = 'all' | 'latest' | 'none';

/**
 * Derives the Seasons dropdown value from a show's kept seasons and policies.
 * Uses season_choice as a tie-breaker on 1-season shows so "latest" does not collapse to "all".
 */
export function deriveSeasonPick(
	seasons: { number: number; keep_up_to_date: boolean }[],
	newSeasons: string | undefined,
	seasonChoice: string | undefined
): SeasonPick | 'custom' {
	const real = seasons.filter((x) => x.number > 0);
	const kept = real.filter((x) => x.keep_up_to_date);
	const newOn = newSeasons !== 'none';
	const latest = Math.max(0, ...real.map((x) => x.number));

	if (kept.length === 0 && !newOn) return 'none';
	if (real.length === 1 && kept.length === 1 && newOn) {
		return seasonChoice === 'latest' ? 'latest' : 'all';
	}
	if (kept.length === real.length && newOn) return 'all';
	if (kept.length === 1 && kept[0].number === latest && newOn) return 'latest';
	return 'custom';
}
