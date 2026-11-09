// Timing for the sign-in corner portals (SigninPortals.svelte): a hole opens in one corner, a short conga line of media symbols runs along the screen edge to a hole in the next corner, falls in, and both holes close. Pure functions so the timeline is unit tested.

/** Corners clockwise from top right: TR, TL, BL, BR (the run goes TR, TL, BL, BR, TR). */
export type Corner = 0 | 1 | 2 | 3;

export const START_S = 9; // First leg, once the page has settled.
export const LEG_GAP_S = 16; // One leg starts every 16 s: about 11 s of quiet between legs.
export const COUNT = 7; // Symbols per leg.
export const SPACING_S = 0.24; // Between symbols leaving the hole.
export const TRAVEL_S = 2.2; // One symbol, hole to hole.
export const OPEN_S = 0.6;
export const CLOSE_S = 0.6;

export const clamp01 = (v: number) => (v < 0 ? 0 : v > 1 ? 1 : v);
export const easeInOutSine = (p: number) => -(Math.cos(Math.PI * p) - 1) / 2;
export const easeOutCubic = (p: number) => 1 - (1 - p) ** 3;

/** Which leg runs at clock t (seconds since mount), and how far into it. Null before the first. */
export function legAt(t: number, legs: number): { index: number; local: number } | null {
	if (t < START_S || legs < 1) return null;
	const n = Math.floor((t - START_S) / LEG_GAP_S);
	return { index: n % legs, local: t - START_S - n * LEG_GAP_S };
}

/** The whole leg: the last symbol lands, then the far hole closes. */
export const LEAD_S = 0.36; // The first symbol leaves once the hole is half open.

/** A hole's size (0 is shut) and squash at leg time `local`. `from` holes open at 0 and close once the last symbol has left; `to` holes open just before the first symbol arrives and close after the last one falls in. */
export function holeAt(local: number, role: 'from' | 'to'): { s: number; squash: number } {
	const openAt = role === 'from' ? 0 : LEAD_S + 0.6 * TRAVEL_S - OPEN_S; // Fully open before the first symbol arrives (0.82 of its run).
	const closeAt = role === 'from' ? (COUNT - 1) * SPACING_S + LEAD_S + 0.5 : LEAD_S + (COUNT - 1) * SPACING_S + TRAVEL_S + 0.1;
	if (local < openAt || local >= closeAt + CLOSE_S) return { s: 0, squash: 0 };
	if (local < openAt + OPEN_S) {
		const p = (local - openAt) / OPEN_S;
		// Opens softly, with a faint breath of squash on the way.
		return { s: easeOutCubic(p), squash: Math.sin(p * Math.PI) * 0.06 };
	}
	if (local < closeAt) return { s: 1, squash: 0 };
	const p = (local - closeAt) / CLOSE_S;
	// Eases shut: zero speed at both ends, so nothing jumps.
	return { s: 1 - easeInOutSine(p), squash: -Math.sin(p * Math.PI) * 0.08 };
}

/** Symbol i's progress along the edge (0 to 1), or null when not out. */
export function symbolAt(local: number, i: number): number | null {
	const p = (local - LEAD_S - i * SPACING_S) / TRAVEL_S;
	return p < 0 || p > 1 ? null : p;
}

/** Whether a keydown should pause the portals: only a real (trusted) printable or editing key typed into a text field. Synthetic events from password managers and autofill, and focus changes, never pause it. */
export function pausesPortals(e: { isTrusted: boolean; key?: string; ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean }, inField: boolean): boolean {
	if (!e.isTrusted || !inField || !e.key) return false;
	if (e.ctrlKey || e.metaKey || e.altKey) return false;
	return e.key.length === 1 || e.key === 'Backspace' || e.key === 'Delete';
}
