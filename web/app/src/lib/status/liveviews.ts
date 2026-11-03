// The ten shapes the Jobs tab's right half can take while Luis picks one.
// Preview only: the list and the ?live= switch go once a shape is chosen.
export const liveViews = [
	{ id: 'board', label: 'Board' },
	{ id: 'dial', label: 'Dial' },
	{ id: 'countdown', label: 'Countdown' },
	{ id: 'log', label: 'Log' },
	{ id: 'clock', label: 'Clock' },
	{ id: 'rings', label: 'Rings' },
	{ id: 'pipeline', label: 'Pipeline' },
	{ id: 'hours', label: 'Hours' },
	{ id: 'counters', label: 'Counters' },
	{ id: 'streaks', label: 'Streaks' }
] as const;

export type LiveViewId = (typeof liveViews)[number]['id'];

/** liveViewOf reads ?live= (a number 1–10 or an id) and falls back to the board. */
export function liveViewOf(s: string | null): LiveViewId {
	if (!s) return 'board';
	const n = Number(s);
	if (Number.isInteger(n) && n >= 1 && n <= liveViews.length) return liveViews[n - 1].id;
	return liveViews.find((v) => v.id === s)?.id ?? 'board';
}
