// The star rating (TMDB's average vote) is shown only when enough people
// voted for it to mean something.

/** MIN_VOTES is the fewest votes a star rating needs to be shown. */
export const MIN_VOTES = 25;

const pos = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined);

/** shownScore is the score to show, or undefined when it is missing or has fewer than MIN_VOTES votes. */
export function shownScore(score: unknown, votes: unknown): number | undefined {
	const s = pos(score);
	const n = pos(votes);
	return s !== undefined && n !== undefined && n >= MIN_VOTES ? s : undefined;
}
