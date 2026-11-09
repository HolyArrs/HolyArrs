// Status page rows: linked apps, checks and jobs as one kind of row, grouped in a fixed order.
import type { AlertView, HealthCheck } from '../core/types.ts';
import type { OutsideList } from '../requests/index.ts';
import type { StageState } from './types.ts';
import type { ScheduleRow } from './schedules.ts';

export type Tone = 'ok' | 'wait' | 'fail' | 'idle';
export type GroupId = 'sources' | 'downloaders' | 'library' | 'media' | 'subtitles' | 'lists' | 'alerts' | 'metadata' | 'app' | 'jobs';

// What the row's Check or Run now button calls.
export type CheckAction = { kind: 'source' | 'downloader'; id: number } | { kind: 'job'; job: string } | { kind: 'checks' };

export interface StatusRow {
	group: GroupId;
	key: string;
	name: string;
	tone: Tone;
	word: string;
	reason: string;
	at?: string;
	// Settings page that fixes the problem.
	fix?: string;
	check?: CheckAction;
	// Job rows only: when it last ran, when it runs next and how the last run ended.
	lastRun?: string;
	nextRun?: string;
	lastResult?: string;
}

export interface StatusGroup {
	id: GroupId;
	label: string;
	rows: StatusRow[];
}

export interface StatusInput {
	states?: StageState[] | null;
	checks?: HealthCheck[] | null;
	schedules?: ScheduleRow[] | null;
	lists?: OutsideList[] | null;
	alerts?: AlertView[] | null;
	// Null or missing while the answer is not in.
	metadataUp?: boolean | null;
	metadataAt?: string;
	now?: number;
}

export const groupOrder: { id: GroupId; label: string }[] = [
	{ id: 'sources', label: 'Sources' },
	{ id: 'downloaders', label: 'Downloaders' },
	// Folder, free space and backup checks all sit under Storage.
	{ id: 'library', label: 'Storage' },
	{ id: 'media', label: 'Media servers' },
	{ id: 'subtitles', label: 'Subtitle sites' },
	{ id: 'lists', label: 'Outside lists' },
	{ id: 'alerts', label: 'Alerts' },
	{ id: 'metadata', label: 'Metadata server' },
	{ id: 'app', label: 'Updates' },
	{ id: 'jobs', label: 'Jobs' }
];

// Where each kind of linked app lands and where its settings live.
const linkGroup: Partial<Record<StageState['kind'], { g: GroupId; fix: string }>> = {
	source: { g: 'sources', fix: '/settings/sources' },
	downloader: { g: 'downloaders', fix: '/settings/downloaders' },
	media_server: { g: 'media', fix: '/settings/media-servers' },
	subtitle_site: { g: 'subtitles', fix: '/settings/subtitles' }
};

// Which checks the page shows and the settings page that fixes each.
const checkFix: Record<string, string> = {
	library_folder: '/settings/library-folders',
	disk_space: '/settings/library-folders',
	backups: '/settings/backups',
	update: '/settings/this-app',
	config_disk: '/settings/this-app',
	// An unmapped /config cannot be fixed in the app, so send people to make a backup.
	config_folder: '/settings/backups',
	waiting_downloads: '/downloads?show=waiting.paused'
};
const checkName: Record<string, string> = { library_folder: 'Folder', disk_space: 'Free space', backups: 'Backups', update: 'Update', config_disk: 'Config folder space', config_folder: 'Config folder mapping', waiting_downloads: 'Downloads waiting for files' };

// The server's own steps need a shell, so say what a container screen can do.
const configSteps = 'Make a backup first and download it. Then map /config to a folder in your container settings, start HolyArrs and restore the backup.';

// A waiting job counts as late after ten minutes past its time.
const LATE_MS = 10 * 60_000;

// Library lives in Storage and Jobs has its own tab, so neither is listed.
export const partKinds: { id: GroupId; label: string; add: string; one: string }[] = [
	{ id: 'sources', label: 'Sources', add: '/settings/sources', one: 'source' },
	{ id: 'downloaders', label: 'Downloaders', add: '/settings/downloaders', one: 'downloader' },
	{ id: 'media', label: 'Media servers', add: '/settings/media-servers', one: 'media server' },
	{ id: 'subtitles', label: 'Subtitle sites', add: '/settings/subtitles', one: 'subtitle site' },
	{ id: 'lists', label: 'Outside lists', add: '/settings/outside-lists', one: 'outside list' },
	{ id: 'alerts', label: 'Alerts', add: '/settings/alerts', one: 'alert' },
	{ id: 'metadata', label: 'Metadata server', add: '/settings/this-app', one: 'metadata server' }
];

// Gives the row time a verb so "3 d ago" is not read as "not checked for 3 days".
export const atVerb: Partial<Record<GroupId, string>> = { sources: 'Since', downloaders: 'Since', media: 'Since', subtitles: 'Since', library: 'Checked', app: 'Checked', lists: 'Synced', alerts: 'Sent', metadata: 'Checked', jobs: 'Ran' };

// Wording for how a job's last run ended.
const ended: Record<string, { tone: Tone; word: string }> = {
	done: { tone: 'ok', word: 'OK' },
	failed: { tone: 'fail', word: 'Failed' },
	cancelled: { tone: 'idle', word: 'Cancelled' }
};

// Starting is normal after every restart, so it is idle, not a problem.
export function linkTone(s: Pick<StageState, 'state' | 'stuck'>): { tone: Tone; word: string } {
	if (s.stuck) return { tone: 'fail', word: 'Stuck' };
	switch (s.state) {
		case 'running':
			return { tone: 'ok', word: 'Working' };
		case 'starting':
			return { tone: 'idle', word: 'Starting' };
		case 'retrying':
			return { tone: 'wait', word: 'Retrying' };
		case 'failed':
			return { tone: 'fail', word: 'Broken' };
		default:
			return { tone: 'idle', word: 'Off' };
	}
}

// Keys end in the row id, like link:source:3.

export function linkId(key: string): number | null {
	const n = Number(key.split(':').pop());
	return Number.isInteger(n) && n > 0 ? n : null;
}

// The server sends year 1 for no time; hide it so rows never say "739921 d ago".

export function realTime(s: string | null | undefined): string | undefined {
	return s && !s.startsWith('0001-') ? s : undefined;
}

// Same words as the Schedules settings page.

export function jobTone(s: ScheduleRow, now = Date.now()): { tone: Tone; word: string; reason: string } {
	if (!s.enabled) return { tone: 'idle', word: 'Turned off', reason: 'Automatic runs are turned off.' };
	if (s.state === 'part_off') return { tone: 'idle', word: 'Part is off', reason: 'The part it belongs to is turned off.' };
	if (s.state === 'safe_mode') return { tone: 'idle', word: 'Safe mode', reason: 'HolyArrs is in safe mode.' };
	if (s.state === 'running') return { tone: 'ok', word: 'Running', reason: '' };
	if (s.last_job === 'failed') return { tone: 'fail', word: 'Failed', reason: s.last_job_error || 'The last run failed.' };
	const next = s.next_run ? new Date(s.next_run).getTime() : NaN;
	if (s.state === 'waiting' && !Number.isNaN(next) && now - next > LATE_MS) return { tone: 'wait', word: 'Late', reason: 'It should have run by now.' };
	if (!s.last_run) return { tone: 'idle', word: 'Not run yet', reason: '' };
	if (s.last_job === 'cancelled') return { tone: 'idle', word: 'Cancelled', reason: 'The last run was stopped.' };
	return { tone: 'ok', word: 'OK', reason: '' };
}

// One pass per kind of input, so group order is set later by groupRows.

export function buildRows(i: StatusInput): StatusRow[] {
	const rows: StatusRow[] = [];
	for (const s of i.states ?? []) {
		const m = linkGroup[s.kind];
		if (!m) continue;
		const id = linkId(s.key);
		const check: CheckAction | undefined = id && (s.kind === 'source' || s.kind === 'downloader') ? { kind: s.kind, id } : undefined;
		rows.push({ group: m.g, key: s.key, name: s.name, ...linkTone(s), reason: s.reason, at: realTime(s.since), fix: m.fix, check });
	}
	for (const c of i.checks ?? []) {
		if (typeof checkFix[c.kind] !== 'string') continue;
		const tone: Tone = c.state === 'ok' ? 'ok' : c.state === 'warning' ? 'wait' : 'fail';
		rows.push({
			group: c.kind === 'update' ? 'app' : 'library',
			key: c.key,
			name: checkTitle(c),
			tone,
			word: tone === 'ok' ? 'Working' : tone === 'wait' ? 'Warning' : 'Broken',
			reason: c.kind === 'config_folder' && c.state !== 'ok' ? configSteps : c.message,
			at: c.checked_at || c.since,
			fix: checkFix[c.kind] || undefined,
			check: { kind: 'checks' }
		});
	}
	for (const l of i.lists ?? []) {
		const base = { group: 'lists' as const, key: `list:${l.id}`, name: l.name, at: l.last_synced_at, fix: '/settings/outside-lists' };
		if (!l.enabled) rows.push({ ...base, tone: 'idle', word: 'Off', reason: '' });
		else if (l.last_error) rows.push({ ...base, tone: 'fail', word: 'Broken', reason: l.last_error });
		else if (!l.last_synced_at) rows.push({ ...base, tone: 'idle', word: 'Not synced yet', reason: '' });
		else rows.push({ ...base, tone: 'ok', word: 'Working', reason: l.last_result });
	}
	for (const a of i.alerts ?? []) {
		const base = { group: 'alerts' as const, key: `alert:${a.id}`, name: a.name, at: a.last_sent_at, fix: '/settings/alerts' };
		if (!a.enabled) rows.push({ ...base, tone: 'idle', word: 'Off', reason: '' });
		else if (a.last_error) rows.push({ ...base, tone: 'fail', word: 'Broken', reason: a.last_error });
		else if (!a.last_sent_at) rows.push({ ...base, tone: 'idle', word: 'Not sent yet', reason: '' });
		else rows.push({ ...base, tone: 'ok', word: 'Working', reason: '' });
	}
	if (i.metadataUp != null) {
		const t = i.metadataUp
			? { tone: 'ok' as const, word: 'Working', reason: '' }
			: { tone: 'fail' as const, word: 'Broken', reason: 'Search and new artwork wait until it answers again.' };
		rows.push({ group: 'metadata', key: 'metadata', name: 'HolyArrs metadata', at: i.metadataAt, ...t });
	}
	for (const s of i.schedules ?? []) {
		rows.push({
			group: 'jobs',
			key: `job:${s.kind}`,
			name: s.label,
			...jobTone(s, i.now),
			at: s.last_job_ended || s.last_run,
			lastRun: s.last_run,
			nextRun: s.next_run,
			lastResult: s.last_job,
			fix: '/settings/schedules',
			check: { kind: 'job', job: s.kind }
		});
	}
	return rows;
}

export function groupRows(rows: StatusRow[]): StatusGroup[] {
	return groupOrder.map((g) => ({ id: g.id, label: g.label, rows: rows.filter((r) => r.group === g.id) })).filter((g) => g.rows.length > 0);
}

// Broken first, then warnings. Off and paused are not problems.

export function needsAttention(rows: StatusRow[]): StatusRow[] {
	return rows.filter((r) => r.tone === 'fail').concat(rows.filter((r) => r.tone === 'wait'));
}

export function nothingLinked(rows: StatusRow[]): boolean {
	return !rows.some((r) => r.group === 'sources' || r.group === 'downloaders');
}

// linkedKnown is false when linked apps could not be read, so an empty list is not read as none added.

export function headline(rows: StatusRow[], linkedKnown = true): string {
	const bad = needsAttention(rows);
	if (!bad.length) return !linkedKnown ? 'No problems found' : nothingLinked(rows) ? 'No sources or downloaders added yet' : 'Everything is working';
	const checks = bad.filter((r) => r.group !== 'jobs').length;
	const jobs = bad.length - checks;
	const out: string[] = [];
	if (checks) out.push(`${checks} check${checks === 1 ? ' needs' : 's need'} attention`);
	if (jobs) out.push(`${jobs} job${jobs === 1 ? ' needs' : 's need'} attention`);
	return out.join(', ');
}

// Keeps empty kinds so the Status grid keeps its shape.

export function everyKind(groups: StatusGroup[]): StatusGroup[] {
	return partKinds.map((k) => ({ id: k.id, label: k.label, rows: groups.find((g) => g.id === k.id)?.rows ?? [] }));
}

// Shows how the last run ended, not what the job does now.

export function pastResult(r: Pick<StatusRow, 'tone' | 'word' | 'lastResult'>): { tone: Tone; word: string } {
	return ended[r.lastResult ?? ''] ?? { tone: r.tone, word: r.word };
}

// These jobs never run on their own, so they have no next run.

export function stopped(r: Pick<StatusRow, 'word'>): boolean {
	return r.word === 'Turned off' || r.word === 'Part is off' || r.word === 'Safe mode';
}

// Avoids saying the same word twice, like "Backups Backups".
function checkTitle(c: Pick<HealthCheck, 'kind' | 'subject'>): string {
	const n = checkName[c.kind];
	if (!c.subject || c.subject === n || c.kind === 'waiting_downloads' || c.kind.startsWith('config_')) return n;
	return c.kind === 'update' ? c.subject : `${n} ${c.subject}`;
}
