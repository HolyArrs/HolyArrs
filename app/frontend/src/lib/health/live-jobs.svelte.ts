// The live job feed behind the Jobs tab's right half: jobs running or waiting
// now, the last finished runs, and a clock that ticks once a second. One
// feed is made per plate and handed to whichever view is showing. Reads
// again on every live job push, one read per 400 ms at most, so a steady
// stream of progress pushes cannot starve it.
import { api } from '../core/api.ts';
import { live } from '../core/live.svelte.ts';
import type { ActiveJob } from './index.ts';
import type { StatusRow } from './rows.ts';

/** A finished (or running) job as the history route gives it. */
export interface HistJob {
	ID: number;
	Kind: string;
	State: string;
	StartedAt?: string | null;
	EndedAt?: string | null;
	Progress: number;
	ProgressNote?: string | null;
}

/** One line of the live log: a note from a running job or a finished run. */
export interface LogEntry {
	key: string;
	at: number;
	name: string;
	text: string;
	tone: 'ok' | 'fail' | 'info' | 'idle';
}

export const t = (s?: string | null) => (s ? Date.parse(s) : NaN);

/** clock says a time so far in minutes and seconds: "0:42", "12:05", or "3 h 10 min" past an hour, the same words as wait(). */
export function clock(ms: number): string {
	const s = Math.max(0, Math.round(ms / 1000));
	if (s >= 3600) return wait(s * 1000);
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** wait says a time still to go the way the rest of the page does: "12 min", "3 h 10 min", "under 1 min". */
export function wait(ms: number): string {
	const m = Math.round(Math.max(0, ms) / 60000);
	if (m < 1) return 'under 1 min';
	if (m >= 60) return `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ''}`;
	return `${m} min`;
}

/** kindOf is the job kind a schedule row runs, or "" for rows that are not jobs. */
export const kindOf = (r: StatusRow) => (r.check?.kind === 'job' ? r.check.job : '');

export class LiveJobs {
	active = $state<ActiveJob[]>([]);
	history = $state<HistJob[]>([]);
	tick = $state(Date.now());
	running = $derived(this.active.filter((j) => j.state === 'running'));
	queued = $derived(this.active.filter((j) => j.state === 'queued'));
	runningKinds = $derived(new Set(this.running.map((j) => j.kind)));
	names: Record<string, string> = {};

	/** nameOf is a job kind's plain name, from the schedule rows or the kind with spaces. */
	nameOf(kind: string): string {
		return this.names[kind] ?? kind.replace(/_/g, ' ');
	}
	learn(rows: StatusRow[]) {
		for (const r of rows) {
			const k = kindOf(r);
			if (k) this.names[k] = r.name;
		}
		for (const j of this.active) this.names[j.kind] = j.label;
	}
	/** fill is how far a job is through its wait: 0 right after a run, 1 when the next is due. */
	fill(r: StatusRow): number {
		const a = t(r.lastRun), b = t(r.nextRun);
		if (!Number.isFinite(b)) return 0;
		if (!Number.isFinite(a) || b <= a) return this.tick >= b ? 1 : 0;
		return Math.min(1, Math.max(0, (this.tick - a) / (b - a)));
	}
	/** due is the ms until a row's next run; negative once it is overdue. */
	due(r: StatusRow): number {
		return t(r.nextRun) - this.tick;
	}
	/** coming is the next runs, leaving out jobs running at this moment. */
	coming(upcoming: StatusRow[], n: number): StatusRow[] {
		return upcoming.filter((r) => !this.runningKinds.has(kindOf(r))).slice(0, n);
	}
	/** finished is the last runs that ended, newest first. */
	finished = $derived(this.history.filter((j) => j.EndedAt && j.State !== 'running' && j.State !== 'queued').sort((a, b) => t(b.EndedAt) - t(a.EndedAt)));
	/** log is the live lines: running notes first, then the last finished runs. */
	log = $derived.by((): LogEntry[] => {
		const now: LogEntry[] = this.running.map((j) => ({ key: `r${j.id}`, at: t(j.started_at ?? j.queued_at), name: j.label, text: j.note || 'Working', tone: 'info' }));
		const past: LogEntry[] = this.finished.slice(0, 12).map((j) => ({
			key: `h${j.ID}`,
			at: t(j.EndedAt),
			name: this.nameOf(j.Kind),
			text: j.State === 'done' ? 'OK' : j.State === 'failed' ? 'Failed' : j.State === 'cancelled' ? 'Cancelled' : j.State,
			tone: j.State === 'done' ? 'ok' : j.State === 'failed' ? 'fail' : 'idle'
		}));
		return [...now, ...past];
	});

	#timer: ReturnType<typeof setTimeout> | undefined;
	#offs: (() => void)[] = [];
	#interval: ReturnType<typeof setInterval> | undefined;
	#seq = 0;

	async load() {
		// Reads can overlap; only the newest one's answer is kept, so a slow older
		// reply never brings back a job that has since finished.
		const n = ++this.#seq;
		try {
			const [a, h] = await Promise.all([api.get<{ jobs: ActiveJob[] }>('/jobs/active'), api.get<{ jobs: HistJob[] }>('/jobs?limit=80')]);
			if (n !== this.#seq) return;
			this.active = a.jobs;
			this.history = h.jobs;
			for (const j of a.jobs) this.names[j.kind] = j.label;
		} catch {
			/* the feed keeps what it had */
		}
	}
	start() {
		void this.load();
		// One read is kept on its way at a time, 400 ms after the first push; the
		// next push after it starts another. Never a reset timer that a steady
		// stream of pushes could keep from ever firing.
		const poke = () => {
			if (this.#timer) return;
			this.#timer = setTimeout(() => {
				this.#timer = undefined;
				void this.load();
			}, 400);
		};
		this.#offs = [live.on('job', poke), live.on('resync', poke)];
		this.#interval = setInterval(() => (this.tick = Date.now()), 1000);
	}
	stop() {
		this.#offs.forEach((off) => off());
		this.#offs = [];
		clearTimeout(this.#timer);
		clearInterval(this.#interval);
	}
}
