// The Status page: every linked part and background job turned into one kind
// of row, grouped in a fixed order. Pure, so grouping and wording are testable.
import type { AlertView, HealthCheck, OutsideList, StageState } from '../types.ts';
import type { ScheduleRow } from '../schedules.ts';

export type Tone = 'ok' | 'wait' | 'fail' | 'idle';
export type GroupId = 'sources' | 'downloaders' | 'library' | 'media' | 'subtitles' | 'lists' | 'alerts' | 'metadata' | 'jobs';

/** What a "Check now" button calls; left out when no route exists. */
export type CheckAction = { kind: 'source' | 'downloader'; id: number } | { kind: 'job'; job: string } | { kind: 'checks' };

export interface StatusRow {
	group: GroupId;
	key: string;
	name: string;
	tone: Tone;
	word: string;
	reason: string;
	/** When the state began or was last checked. */
	at?: string;
	fix?: string;
	check?: CheckAction;
	/** Jobs only. */
	lastRun?: string;
	nextRun?: string;
}

export interface StatusGroup {
	id: GroupId;
	label: string;
	rows: StatusRow[];
}

export const groupOrder: { id: GroupId; label: string }[] = [
	{ id: 'sources', label: 'Sources' },
	{ id: 'downloaders', label: 'Downloaders' },
	{ id: 'library', label: 'Library' },
	{ id: 'media', label: 'Media server' },
	{ id: 'subtitles', label: 'Subtitles' },
	{ id: 'lists', label: 'Lists' },
	{ id: 'alerts', label: 'Alerts' },
	{ id: 'metadata', label: 'Metadata server' },
	{ id: 'jobs', label: 'Jobs' }
];

export interface StatusInput {
	states?: StageState[] | null;
	checks?: HealthCheck[] | null;
	schedules?: ScheduleRow[] | null;
	lists?: OutsideList[] | null;
	alerts?: AlertView[] | null;
	/** null or undefined while the answer is not in. */
	metadataUp?: boolean | null;
	metadataAt?: string;
	now?: number;
}

const linkGroup: Partial<Record<StageState['kind'], { g: GroupId; fix: string }>> = {
	source: { g: 'sources', fix: '/settings/sources' },
	downloader: { g: 'downloaders', fix: '/settings/downloaders' },
	media_server: { g: 'media', fix: '/settings/media-servers' },
	subtitle_site: { g: 'subtitles', fix: '/settings/subtitles' }
};

const checkFix: Record<string, string> = {
	library_folder: '/settings/library-folders',
	disk_space: '/settings/library-folders',
	backups: '/settings/backups',
	update: '/settings/this-app'
};
const checkName: Record<string, string> = { library_folder: 'Folder', disk_space: 'Free space', backups: 'Backups', update: 'Update' };

/** linkTone turns a linked part's state into a tone and a word. */
export function linkTone(s: Pick<StageState, 'state' | 'stuck'>): { tone: Tone; word: string } {
	if (s.stuck) return { tone: 'fail', word: 'Stuck' };
	switch (s.state) {
		case 'running':
			return { tone: 'ok', word: 'Working' };
		case 'starting':
			return { tone: 'wait', word: 'Starting' };
		case 'retrying':
			return { tone: 'wait', word: 'Retrying' };
		case 'failed':
			return { tone: 'fail', word: 'Broken' };
		default:
			return { tone: 'idle', word: 'Off' };
	}
}

/** linkId reads the number out of a "link:source:3" key. */
export function linkId(key: string): number | null {
	const n = Number(key.split(':').pop());
	return Number.isInteger(n) && n > 0 ? n : null;
}

const LATE_MS = 10 * 60_000;

/** realTime drops the server's empty time (year 1), so rows never say "739921 d ago". */
export function realTime(s: string | null | undefined): string | undefined {
	return s && !s.startsWith('0001-') ? s : undefined;
}

/** jobTone says how a background job stands: failed, late, not run yet, off or OK. */
export function jobTone(s: ScheduleRow, now = Date.now()): { tone: Tone; word: string; reason: string } {
	if (!s.enabled) return { tone: 'idle', word: 'Off', reason: 'Automatic runs are turned off.' };
	if (s.state === 'part_off') return { tone: 'idle', word: 'Off', reason: 'The part it belongs to is turned off.' };
	if (s.state === 'safe_mode') return { tone: 'idle', word: 'Paused', reason: 'HolyArrs is in safe mode.' };
	if (s.state === 'running') return { tone: 'ok', word: 'Running', reason: '' };
	if (s.last_job === 'failed') return { tone: 'fail', word: 'Failed', reason: s.last_job_error || 'The last run failed.' };
	const next = s.next_run ? new Date(s.next_run).getTime() : NaN;
	if (s.state === 'waiting' && !Number.isNaN(next) && now - next > LATE_MS) return { tone: 'wait', word: 'Late', reason: 'It should have run by now.' };
	if (!s.last_run) return { tone: 'idle', word: 'Not run yet', reason: '' };
	if (s.last_job === 'cancelled') return { tone: 'idle', word: 'Cancelled', reason: 'The last run was stopped.' };
	return { tone: 'ok', word: 'OK', reason: '' };
}

/** buildRows turns the answers of the status routes into rows. */
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
		if (!checkFix[c.kind]) continue; // sources and downloaders come from their linked state
		const tone: Tone = c.state === 'ok' ? 'ok' : c.state === 'warning' ? 'wait' : 'fail';
		rows.push({
			group: 'library',
			key: c.key,
			name: c.subject ? `${checkName[c.kind]} ${c.subject}` : checkName[c.kind],
			tone,
			word: tone === 'ok' ? 'Working' : tone === 'wait' ? 'Warning' : 'Broken',
			reason: c.message,
			at: c.checked_at || c.since,
			fix: checkFix[c.kind],
			check: { kind: 'checks' }
		});
	}
	for (const l of i.lists ?? []) {
		const base = { group: 'lists' as const, key: `list:${l.id}`, name: l.name, at: l.last_synced_at, fix: '/settings/outside-lists' };
		if (!l.enabled) rows.push({ ...base, tone: 'idle', word: 'Off', reason: '' });
		else if (l.last_error) rows.push({ ...base, tone: 'fail', word: 'Failed', reason: l.last_error });
		else if (!l.last_synced_at) rows.push({ ...base, tone: 'idle', word: 'Not synced yet', reason: '' });
		else rows.push({ ...base, tone: 'ok', word: 'Working', reason: l.last_result });
	}
	for (const a of i.alerts ?? []) {
		const base = { group: 'alerts' as const, key: `alert:${a.id}`, name: a.name, at: a.last_sent_at, fix: '/settings/alerts' };
		if (!a.enabled) rows.push({ ...base, tone: 'idle', word: 'Off', reason: '' });
		else if (a.last_error) rows.push({ ...base, tone: 'fail', word: 'Failed', reason: a.last_error });
		else rows.push({ ...base, tone: 'ok', word: 'Working', reason: a.last_sent_at ? '' : 'Nothing sent yet.' });
	}
	if (i.metadataUp != null) {
		const t = i.metadataUp
			? { tone: 'ok' as const, word: 'Working', reason: '' }
			: { tone: 'fail' as const, word: 'Down', reason: 'Search and new artwork wait until it answers again.' };
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
			fix: '/settings/schedules',
			check: { kind: 'job', job: s.kind }
		});
	}
	return rows;
}

/** groupRows puts rows into the fixed group order, empty groups left out. */
export function groupRows(rows: StatusRow[]): StatusGroup[] {
	return groupOrder.map((g) => ({ id: g.id, label: g.label, rows: rows.filter((r) => r.group === g.id) })).filter((g) => g.rows.length > 0);
}

/** needsAttention keeps broken rows first, then warnings. Off and paused are not problems. */
export function needsAttention(rows: StatusRow[]): StatusRow[] {
	return rows.filter((r) => r.tone === 'fail').concat(rows.filter((r) => r.tone === 'wait'));
}

/** groupLine says a group in one line: "3 of 3 working", "1 of 3 working · 1 needs attention · 1 off". */
export function groupLine(g: StatusGroup): string {
	if (g.rows.length === 1) return g.rows[0].word;
	const ok = g.rows.filter((r) => r.tone === 'ok').length;
	const off = g.rows.filter((r) => r.tone === 'idle').length;
	const bad = g.rows.length - ok - off;
	const parts = [`${ok} of ${g.rows.length} ${g.id === 'jobs' ? 'OK' : 'working'}`];
	if (bad) parts.push(`${bad} need${bad === 1 ? 's' : ''} attention`);
	if (off) parts.push(`${off} off`);
	return parts.join(' · ');
}

/** headline is the line at the top: "Everything is working" or what needs attention. */
export function headline(rows: StatusRow[]): string {
	const bad = needsAttention(rows);
	if (!bad.length) return 'Everything is working';
	const parts = bad.filter((r) => r.group !== 'jobs').length;
	const jobs = bad.length - parts;
	const out: string[] = [];
	if (parts) out.push(`${parts} part${parts === 1 ? ' needs' : 's need'} attention`);
	if (jobs) out.push(`${jobs} job${jobs === 1 ? ' needs' : 's need'} attention`);
	return out.join(', ');
}

export const looks = [
	{ key: 'e', label: 'Split' },
	{ key: 'd', label: 'Board' },
	{ key: 'a', label: 'One list' },
	{ key: 'b', label: 'Problems first' },
	{ key: 'c', label: 'Two tabs' }
] as const;
export type LookKey = (typeof looks)[number]['key'];

/** lookOf reads ?look=, defaulting to the split view. */
export function lookOf(v: string | null): LookKey {
	return v === 'a' || v === 'b' || v === 'c' || v === 'd' ? v : 'e';
}

/** groupCounts splits a group into working, needs attention and off. */
export function groupCounts(g: StatusGroup): { ok: number; bad: number; off: number } {
	const ok = g.rows.filter((r) => r.tone === 'ok').length;
	const off = g.rows.filter((r) => r.tone === 'idle').length;
	return { ok, bad: g.rows.length - ok - off, off };
}
