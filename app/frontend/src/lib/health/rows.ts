// Status page rows: linked apps, checks and jobs as one kind of row, grouped in a fixed order.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
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

// Labels are getters, so they follow the language picked when they are read.
export const groupOrder: { id: GroupId; label: string }[] = [
	{ id: 'sources', get label() { return t('health.groups.sources'); } },
	{ id: 'downloaders', get label() { return t('health.groups.downloaders'); } },
	// Folder, free space and backup checks all sit under Storage.
	{ id: 'library', get label() { return t('health.groups.library'); } },
	{ id: 'media', get label() { return t('health.groups.media'); } },
	{ id: 'subtitles', get label() { return t('health.groups.subtitles'); } },
	{ id: 'lists', get label() { return t('health.groups.lists'); } },
	{ id: 'alerts', get label() { return t('health.groups.alerts'); } },
	{ id: 'metadata', get label() { return t('health.groups.metadata'); } },
	{ id: 'app', get label() { return t('health.groups.app'); } },
	{ id: 'jobs', get label() { return t('health.groups.jobs'); } }
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
const checkName: Record<string, string> = {
	get library_folder() { return t('health.check.folder'); },
	get disk_space() { return t('health.check.freeSpace'); },
	get backups() { return t('health.check.backups'); },
	get update() { return t('health.check.update'); },
	get config_disk() { return t('health.check.configDisk'); },
	get config_folder() { return t('health.check.configFolder'); },
	get waiting_downloads() { return t('health.check.waiting'); }
};

// The server's own steps need a shell, so say what a container screen can do.
// A waiting job counts as late after ten minutes past its time.
const LATE_MS = 10 * 60_000;

// Library lives in Storage and Jobs has its own tab, so neither is listed.
export const partKinds: { id: GroupId; label: string; add: string; one: string }[] = [
	{ id: 'sources', get label() { return t('health.groups.sources'); }, add: '/settings/sources', get one() { return t('health.one.source'); } },
	{ id: 'downloaders', get label() { return t('health.groups.downloaders'); }, add: '/settings/downloaders', get one() { return t('health.one.downloader'); } },
	{ id: 'media', get label() { return t('health.groups.media'); }, add: '/settings/media-servers', get one() { return t('health.one.media'); } },
	{ id: 'subtitles', get label() { return t('health.groups.subtitles'); }, add: '/settings/subtitles', get one() { return t('health.one.subtitles'); } },
	{ id: 'lists', get label() { return t('health.groups.lists'); }, add: '/settings/outside-lists', get one() { return t('health.one.lists'); } },
	{ id: 'alerts', get label() { return t('health.groups.alerts'); }, add: '/settings/alerts', get one() { return t('health.one.alert'); } },
	{ id: 'metadata', get label() { return t('health.groups.metadata'); }, add: '/settings/this-app', get one() { return t('health.one.metadata'); } }
];

// Gives the row time a verb so "3 d ago" is not read as "not checked for 3 days".
export const atVerb: Partial<Record<GroupId, string>> = {
	get sources() { return t('health.at.since'); },
	get downloaders() { return t('health.at.since'); },
	get media() { return t('health.at.since'); },
	get subtitles() { return t('health.at.since'); },
	get library() { return t('health.at.checked'); },
	get app() { return t('health.at.checked'); },
	get lists() { return t('health.at.synced'); },
	get alerts() { return t('health.at.sent'); },
	get metadata() { return t('health.at.checked'); },
	get jobs() { return t('health.at.ran'); }
};

// Wording for how a job's last run ended.
const ended: Record<string, { tone: Tone; word: string }> = {
	done: { tone: 'ok', get word() { return t('health.result.ok'); } },
	failed: { tone: 'fail', get word() { return t('health.result.failed'); } },
	cancelled: { tone: 'idle', get word() { return t('health.result.cancelled'); } }
};

// Starting is normal after every restart, so it is idle, not a problem.
export function linkTone(s: Pick<StageState, 'state' | 'stuck'>): { tone: Tone; word: string } {
	if (s.stuck) return { tone: 'fail', word: t('health.run.stuck') };
	switch (s.state) {
		case 'running':
			return { tone: 'ok', word: t('health.word.working') };
		case 'starting':
			return { tone: 'idle', word: t('health.run.starting') };
		case 'retrying':
			return { tone: 'wait', word: t('health.run.retrying') };
		case 'failed':
			return { tone: 'fail', word: t('health.word.broken') };
		default:
			return { tone: 'idle', word: t('health.run.off') };
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
	if (!s.enabled) return { tone: 'idle', word: t('health.job.turnedOff'), reason: t('health.job.turnedOffWhy') };
	if (s.state === 'part_off') return { tone: 'idle', word: t('health.job.partOff'), reason: t('health.job.partOffWhy') };
	if (s.state === 'safe_mode') return { tone: 'idle', word: t('health.job.safeMode'), reason: t('health.job.safeModeWhy') };
	if (s.state === 'running') return { tone: 'ok', word: t('health.job.running'), reason: '' };
	if (s.last_job === 'failed') return { tone: 'fail', word: t('health.result.failed'), reason: s.last_job_error || t('health.job.failedWhy') };
	const next = s.next_run ? new Date(s.next_run).getTime() : NaN;
	if (s.state === 'waiting' && !Number.isNaN(next) && now - next > LATE_MS) return { tone: 'wait', word: t('health.job.late'), reason: t('health.job.lateWhy') };
	if (!s.last_run) return { tone: 'idle', word: t('health.job.notRun'), reason: '' };
	if (s.last_job === 'cancelled') return { tone: 'idle', word: t('health.result.cancelled'), reason: t('health.job.cancelledWhy') };
	return { tone: 'ok', word: t('health.result.ok'), reason: '' };
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
			word: tone === 'ok' ? t('health.word.working') : tone === 'wait' ? t('health.state.warning') : t('health.word.broken'),
			reason: c.kind === 'config_folder' && c.state !== 'ok' ? t('health.check.configSteps') : c.message,
			at: c.checked_at || c.since,
			fix: checkFix[c.kind] || undefined,
			check: { kind: 'checks' }
		});
	}
	for (const l of i.lists ?? []) {
		const base = { group: 'lists' as const, key: `list:${l.id}`, name: l.name, at: l.last_synced_at, fix: '/settings/outside-lists' };
		if (!l.enabled) rows.push({ ...base, tone: 'idle', word: t('health.run.off'), reason: '' });
		else if (l.last_error) rows.push({ ...base, tone: 'fail', word: t('health.word.broken'), reason: l.last_error });
		else if (!l.last_synced_at) rows.push({ ...base, tone: 'idle', word: t('health.word.notSynced'), reason: '' });
		else rows.push({ ...base, tone: 'ok', word: t('health.word.working'), reason: l.last_result });
	}
	for (const a of i.alerts ?? []) {
		const base = { group: 'alerts' as const, key: `alert:${a.id}`, name: a.name, at: a.last_sent_at, fix: '/settings/alerts' };
		if (!a.enabled) rows.push({ ...base, tone: 'idle', word: t('health.run.off'), reason: '' });
		else if (a.last_error) rows.push({ ...base, tone: 'fail', word: t('health.word.broken'), reason: a.last_error });
		else if (!a.last_sent_at) rows.push({ ...base, tone: 'idle', word: t('health.word.notSent'), reason: '' });
		else rows.push({ ...base, tone: 'ok', word: t('health.word.working'), reason: '' });
	}
	if (i.metadataUp != null) {
		const m = i.metadataUp
			? { tone: 'ok' as const, word: t('health.word.working'), reason: '' }
			: { tone: 'fail' as const, word: t('health.word.broken'), reason: t('health.metadata.down') };
		rows.push({ group: 'metadata', key: 'metadata', name: t('health.metadata.name'), at: i.metadataAt, ...m });
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
	if (!bad.length) return !linkedKnown ? t('health.headline.none') : nothingLinked(rows) ? t('health.headline.nothingLinked') : t('health.headline.fine');
	const checks = bad.filter((r) => r.group !== 'jobs').length;
	const jobs = bad.length - checks;
	const out: string[] = [];
	if (checks) out.push(t(checks === 1 ? 'health.headline.checks.one' : 'health.headline.checks.other', { count: checks }));
	if (jobs) out.push(t(jobs === 1 ? 'health.headline.jobs.one' : 'health.headline.jobs.other', { count: jobs }));
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
	return r.word === t('health.job.turnedOff') || r.word === t('health.job.partOff') || r.word === t('health.job.safeMode');
}

// Avoids saying the same word twice, like "Backups Backups".
function checkTitle(c: Pick<HealthCheck, 'kind' | 'subject'>): string {
	const n = checkName[c.kind];
	if (!c.subject || c.subject === n || c.kind === 'waiting_downloads' || c.kind.startsWith('config_')) return n;
	return c.kind === 'update' ? c.subject : `${n} ${c.subject}`;
}
