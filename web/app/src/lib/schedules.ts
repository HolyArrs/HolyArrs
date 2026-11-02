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

export type ScheduleGroupId = 'library' | 'copies' | 'metadata' | 'upkeep';

/** The four groups the Schedules page sorts its rows into, in page order. */
export const scheduleGroups: { id: ScheduleGroupId; label: string }[] = [
	{ id: 'library', label: 'Library and files' },
	{ id: 'copies', label: 'Getting copies' },
	{ id: 'metadata', label: 'Metadata and lists' },
	{ id: 'upkeep', label: 'Upkeep' }
];

const groupOfKind: Record<string, ScheduleGroupId> = {
	check_library_folders: 'library',
	rescan_library: 'library',
	clean_holding: 'library',
	feed_check: 'copies',
	grab_held: 'copies',
	search_missing: 'copies',
	subtitle_missing: 'copies',
	subtitle_better: 'copies',
	refresh_titles: 'metadata',
	refresh_shows: 'metadata',
	refresh_artists: 'metadata',
	artist_pictures: 'metadata',
	sync_outside_lists: 'metadata',
	health_check: 'upkeep',
	clean_history: 'upkeep'
};

/** scheduleGroup says which group a schedule belongs in; anything new lands in Upkeep. */
export function scheduleGroup(kind: string): ScheduleGroupId {
	return groupOfKind[kind] ?? 'upkeep';
}

/** groupSchedules splits rows into the page's groups, keeping the server's order and leaving out empty groups. */
export function groupSchedules<T extends Pick<ScheduleRow, 'kind'>>(rows: T[]): { id: ScheduleGroupId; label: string; rows: T[] }[] {
	return scheduleGroups.map((g) => ({ ...g, rows: rows.filter((r) => scheduleGroup(r.kind) === g.id) })).filter((g) => g.rows.length > 0);
}

/** busyNow lists the schedules running or waiting their turn, for the Running now box. */
export function busyNow<T extends Pick<ScheduleRow, 'state'>>(rows: T[]): T[] {
	return rows.filter((r) => r.state === 'running' || r.state === 'queued');
}

/** dot is the status dot's colour: a failed last run wins over the schedule's state. */
export function dot(s: Pick<ScheduleRow, 'state' | 'last_job'>): 'ok' | 'busy' | 'fail' | 'off' {
	if (s.state === 'running' || s.state === 'queued') return 'busy';
	if (s.last_job === 'failed') return 'fail';
	if (s.state === 'waiting') return 'ok';
	return 'off';
}
