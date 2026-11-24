// Schedules page helpers: intervals in words, state wording and grouping.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
	// Why the newest Job did nothing (setting_off, nobody, starting) and
	// how long it took, in seconds.
	last_job_note?: string;
	last_job_s?: number;
	// The newest scheduled check that found nothing to do, and why.
	last_checked?: string;
	last_skip?: string;
	part: string;
	settings?: string;
}

// When the schedule last ran: the later of its clock start (last_run) and
// its newest finished Job, so a run by hand counts too. last_run
// alone only moves on scheduled runs.
export function lastRan(s: Pick<ScheduleRow, 'last_run' | 'last_job_ended'>): string | undefined {
	const a = s.last_run, b = s.last_job_ended;
	if (!a || !b) return a || b;
	return Date.parse(b) > Date.parse(a) ? b : a;
}

// What the Last run column shows: the newest run, run by hand or check
// that found nothing to do. why is the reason code when that newest
// thing did nothing; took is how long a run took, in seconds.
export function lastSeen(s: Pick<ScheduleRow, 'last_run' | 'last_job_ended' | 'last_job_note' | 'last_job_s' | 'last_checked' | 'last_skip'>): { at?: string; checked: boolean; why?: string; took?: number } {
	const ran = lastRan(s);
	const chk = s.last_checked;
	if (chk && (!ran || Date.parse(chk) > Date.parse(ran))) return { at: chk, checked: true, why: s.last_skip || 'nothing' };
	if (!ran) return { checked: false };
	const byJob = ran === s.last_job_ended;
	return { at: ran, checked: false, why: byJob && s.last_job_note ? s.last_job_note : undefined, took: byJob ? s.last_job_s : undefined };
}

// A reason code in words. Plex watchlists name the switch and the page it
// is on; any other code reads as plain "nothing to do".
export function skipWords(kind: string, code: string | undefined): string {
	const plex = kind === 'sync_plex_watchlists';
	switch (code) {
		case 'setting_off':
			return plex ? t('settings.schedules.skip.plexOff', { setting: t('settings.requests.plexOn'), page: t('core.settings.requests.label') }) : t('settings.schedules.skip.settingOff');
		case 'nobody':
			return plex ? t('settings.schedules.skip.plexNobody') : t('settings.schedules.skip.nothing');
		case 'starting':
			return t('settings.schedules.skip.starting');
		default:
			return t('settings.schedules.skip.nothing');
	}
}

// A reason that points at a setting gets a link to it.
export function skipLinks(code: string | undefined): boolean {
	return code === 'setting_off' || code === 'nobody';
}

export interface LoopRow extends StageState {
	every_s?: number;
	last_pass?: string;
	next_pass?: string;
	part: string;
}

export type ScheduleGroupId = 'library' | 'copies' | 'metadata' | 'upkeep';

// Page order of the groups.
export const scheduleGroups: { id: ScheduleGroupId; label: string }[] = [
	{ id: 'library', get label() { return t('health.group.library'); } },
	{ id: 'copies', get label() { return t('health.group.copies'); } },
	{ id: 'metadata', get label() { return t('health.group.metadata'); } },
	{ id: 'upkeep', get label() { return t('health.group.upkeep'); } }
];

// Which group each known job kind sits in.
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
	clean_history: 'upkeep',
	clean_releases: 'upkeep'
};

// Words for each state chip.
export const stateWords: Record<ScheduleRow['state'], string> = {
	get waiting() { return t('health.job.waiting'); },
	get queued() { return t('health.job.queued'); },
	get running() { return t('health.job.running'); },
	get off() { return t('health.job.turnedOff'); },
	get part_off() { return t('health.job.partOff'); },
	get safe_mode() { return t('health.job.safeMode'); }
};

// Picks the largest unit that divides evenly so the words stay exact.
export function every(s: number): string {
	if (s <= 0) return t('health.every.always');
	const units: [number, string][] = [
		[86400, 'day'],
		[3600, 'hour'],
		[60, 'minute']
	];
	for (const [n, w] of units) {
		if (s % n === 0) {
			const k = s / n;
			return k === 1 ? t(`health.every.${w}.one`) : t(`health.every.${w}.other`, { n: k });
		}
	}
	return t('health.every.seconds', { n: s });
}

// Whole-hour steps are edited in hours, anything finer in minutes.
export function unit(step: number): { size: number; word: string } {
	return step % 3600 === 0 ? { size: 3600, word: t('health.unit.hours') } : { size: 60, word: t('health.unit.minutes') };
}

// A 30 second margin keeps imminent runs from reading "in 0 min".
export function until(s: string | null | undefined, now = Date.now()): string {
	if (!s) return t('health.until.none');
	const at = new Date(s).getTime();
	if (Number.isNaN(at)) return t('health.until.none');
	const sec = Math.round((at - now) / 1000);
	if (sec <= 30) return t('health.until.now');
	if (sec < 3600) return t('health.until.min', { n: Math.round(sec / 60) });
	if (sec < 86400) return t('health.until.hour', { n: Math.round(sec / 3600) });
	return t('health.until.day', { n: Math.round(sec / 86400) });
}

// Past the one minute margin a missed run reads as late, never as "now".
export function nextRun(s: string | null | undefined, now = Date.now()): string {
	const at = s ? new Date(s).getTime() : NaN;
	const late = Math.round((now - at) / 1000);
	if (Number.isNaN(at) || late <= 60) return until(s, now);
	if (late < 3600) return t('health.due.min', { n: Math.round(late / 60) });
	if (late < 86400) return t('health.due.hour', { n: Math.round(late / 3600) });
	return t('health.due.day', { n: Math.round(late / 86400) });
}

// Parts that are off or in safe mode would refuse the run.
export function mayRun(s: Pick<ScheduleRow, 'state'>): boolean {
	return s.state !== 'part_off' && s.state !== 'safe_mode';
}

// Unknown kinds fall back to Upkeep so a new job still shows up.
export function scheduleGroup(kind: string): ScheduleGroupId {
	return groupOfKind[kind] ?? 'upkeep';
}

// Server order is kept inside each group.
export function groupSchedules<T extends Pick<ScheduleRow, 'kind'>>(rows: T[]): { id: ScheduleGroupId; label: string; rows: T[] }[] {
	return scheduleGroups.map((g) => ({ ...g, rows: rows.filter((r) => scheduleGroup(r.kind) === g.id) })).filter((g) => g.rows.length > 0);
}

export function busyNow<T extends Pick<ScheduleRow, 'state'>>(rows: T[]): T[] {
	return rows.filter((r) => r.state === 'running' || r.state === 'queued');
}

// A failed last run beats the state so problems stay visible.
export function dot(s: Pick<ScheduleRow, 'state' | 'last_job'>): 'ok' | 'busy' | 'fail' | 'off' {
	if (s.state === 'running' || s.state === 'queued') return 'busy';
	if (s.last_job === 'failed') return 'fail';
	if (s.state === 'waiting') return 'ok';
	return 'off';
}
