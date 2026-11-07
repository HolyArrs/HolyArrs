// Pure outage logic for the metadata notification. A single slow or failed
// check never raises it: it needs `fails` failures in a row, or failures
// lasting `ms`. Each outage gets a number so a dismissal only lasts for it.

export interface OutageRules {
	fails: number;
	ms: number;
}
export const OUTAGE_RULES: OutageRules = { fails: 2, ms: 20_000 };

export interface OutageState {
	/** Failed checks in a row. */
	fails: number;
	/** When the first failure of this run happened (0 = none). */
	since: number;
	/** The outage is confirmed and showing. */
	down: boolean;
	/** Number of the current or last outage. */
	outage: number;
	/** The outage the person dismissed (0 = none). */
	dismissed: number;
}

export type OutageEvent = 'down' | 'up' | null;

export function outageStart(): OutageState {
	return { fails: 0, since: 0, down: false, outage: 0, dismissed: 0 };
}

/** outageStep folds one check result into the state and says what changed. */
export function outageStep(s: OutageState, ok: boolean, now: number, rules: OutageRules = OUTAGE_RULES): { state: OutageState; event: OutageEvent } {
	if (ok) {
		const event: OutageEvent = s.down ? 'up' : null;
		return { state: { ...s, fails: 0, since: 0, down: false }, event };
	}
	const fails = s.fails + 1;
	const since = s.fails > 0 ? s.since : now;
	if (s.down) return { state: { ...s, fails, since }, event: null };
	if (fails >= rules.fails || now - since >= rules.ms) {
		return { state: { ...s, fails, since, down: true, outage: s.outage + 1 }, event: 'down' };
	}
	return { state: { ...s, fails, since }, event: null };
}

/** outageDismiss hides the current outage for good. */
export function outageDismiss(s: OutageState): OutageState {
	return s.down ? { ...s, dismissed: s.outage } : s;
}

/** outageShown says whether the down notification should be on screen. */
export function outageShown(s: OutageState): boolean {
	return s.down && s.dismissed !== s.outage;
}

/** nextCheckIn is how long to wait before asking again: quicker while in doubt. */
export function nextCheckIn(s: OutageState, calm = 60_000, doubt = 10_000): number {
	return s.fails > 0 ? doubt : calm;
}
