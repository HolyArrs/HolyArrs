
// Below this many votes a score is too noisy to show.
export const MIN_VOTES = 25;

const pos = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined);

export function shownScore(score: unknown, votes: unknown): number | undefined {
	const s = pos(score);
	const n = pos(votes);
	return s !== undefined && n !== undefined && n >= MIN_VOTES ? s : undefined;
}
