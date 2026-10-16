// The Schedules page: intervals in words, states and the unit an interval
// is edited in.
import type { StageState } from './types.ts';

export interface ScheduleRow {
	kind: string;
	label: string;
	about: string;
	every_s: number;
	min_s: number;
	max_s: number;
	step_s: number;
	enabled: boolean;
	last_run?: string;
	next_run?: string;
	state: 'waiting' | 'queued' | 'running' | 'off' | 'part_off' | 'safe_mode';
	last_job?: string;
	last_job_ended?: string;
	last_job_error?: string;
	part: string;
	settings?: string;
}

export interface LoopRow extends StageState {
	every_s?: number;
	last_pass?: string;
	next_pass?: string;
	part: string;
}

/** every says an interval in words: "every 15 minutes", "every day". */
export function every(s: number): string {
	if (s <= 0) return 'all the time';
	const units: [number, string][] = [
		[86400, 'day'],
		[3600, 'hour'],
		[60, 'minute']
	];
	for (const [n, w] of units) {
		if (s % n === 0) {
			const k = s / n;
			return k === 1 ? `every ${w}` : `every ${k} ${w}s`;
		}
	}
	return `every ${s} seconds`;
}

/** unit is how an interval is typed: minutes, or hours for whole-hour steps. */
export function unit(step: number): { size: number; word: string } {
	return step % 3600 === 0 ? { size: 3600, word: 'hours' } : { size: 60, word: 'minutes' };
}

/** until says when something happens next: "in 5 min", "in 3 h", "now". */
export function until(s: string | null | undefined, now = Date.now()): string {
	if (!s) return 'not planned';
	const t = new Date(s).getTime();
	if (Number.isNaN(t)) return 'not planned';
	const sec = Math.round((t - now) / 1000);
	if (sec <= 30) return 'now';
	if (sec < 3600) return `in ${Math.round(sec / 60)} min`;
	if (sec < 86400) return `in ${Math.round(sec / 3600)} h`;
	return `in ${Math.round(sec / 86400)} d`;
}

export const stateWords: Record<ScheduleRow['state'], string> = {
	waiting: 'Waiting',
	queued: 'Queued',
	running: 'Running',
	off: 'Turned off',
	part_off: 'Part is off',
	safe_mode: 'Safe mode'
};

/** stateTone colours a schedule's state chip. */
export function stateTone(s: ScheduleRow['state']): string {
	if (s === 'running' || s === 'queued') return 'wait';
	if (s === 'waiting') return 'ok';
	return '';
}

/** mayRun says whether Run now makes sense for a schedule. */
export function mayRun(s: Pick<ScheduleRow, 'state'>): boolean {
	return s.state !== 'part_off' && s.state !== 'safe_mode';
}
