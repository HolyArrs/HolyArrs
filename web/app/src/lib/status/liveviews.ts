// The ten shapes the Jobs tab's right half can take while Luis picks one.
// Preview only: the list and the ?live= switch go once a shape is chosen.
export const liveViews = [
	{ id: 'radar', label: 'Radar' },
	{ id: 'arc', label: 'Arc' },
	{ id: 'tape', label: 'Tape' },
	{ id: 'pulse', label: 'Pulse' },
	{ id: 'deck', label: 'Deck' },
	{ id: 'gauges', label: 'Gauges' },
	{ id: 'day', label: 'Day' },
	{ id: 'forecast', label: 'Forecast' },
	{ id: 'narrator', label: 'Narrator' },
	{ id: 'matrix', label: 'Matrix' }
] as const;

export type LiveViewId = (typeof liveViews)[number]['id'];

/** liveViewOf reads ?live= (a number 1–10 or an id) and falls back to the first. */
export function liveViewOf(s: string | null): LiveViewId {
	if (!s) return liveViews[0].id;
	const n = Number(s);
	if (Number.isInteger(n) && n >= 1 && n <= liveViews.length) return liveViews[n - 1].id;
	return liveViews.find((v) => v.id === s)?.id ?? liveViews[0].id;
}
