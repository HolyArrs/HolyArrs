// An outage needs `fails` failures in a row or failures lasting `ms`, so one slow check never raises it.
export interface OutageRules {
	fails: number;
	ms: number;
}

export interface OutageState {
	fails: number;
	since: number;
	down: boolean;
	outage: number;
	dismissed: number;
}

export type OutageEvent = 'down' | 'up' | null;

export const OUTAGE_RULES: OutageRules = { fails: 2, ms: 20_000 };

export function outageStart(): OutageState {
	return { fails: 0, since: 0, down: false, outage: 0, dismissed: 0 };
}

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

// A dismissal is tied to the outage number, so the next outage shows again.
export function outageDismiss(s: OutageState): OutageState {
	return s.down ? { ...s, dismissed: s.outage } : s;
}

export function outageShown(s: OutageState): boolean {
	return s.down && s.dismissed !== s.outage;
}

export function nextCheckIn(s: OutageState, calm = 60_000, doubt = 10_000): number {
	return s.fails > 0 ? doubt : calm;
}
