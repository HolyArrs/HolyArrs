// Live job feed for the Jobs tab: active jobs, recent runs and a one second clock, shared by the views.
import { t as say } from '#lib/core/i18n/i18n.svelte.ts';
import { api } from '../core/api.ts';
import { live } from '../core/live.svelte.ts';
import type { ActiveJob } from './types.ts';
import type { StatusRow } from './rows.ts';

// A job as the history route gives it, with its own capitalised field names.
export interface HistJob {
	ID: number;
	Kind: string;
	State: string;
	StartedAt?: string | null;
	EndedAt?: string | null;
	Progress: number;
	ProgressNote?: string | null;
}

// One line of the live log, from a running job or a finished run.
export interface LogEntry {
	key: string;
	at: number;
	name: string;
	text: string;
	tone: 'ok' | 'fail' | 'info' | 'idle';
}

// NaN for a missing time so callers can test it with isFinite.
export const t = (s?: string | null) => (s ? Date.parse(s) : NaN);

// Past an hour it uses the same words as wait().
export function clock(ms: number): string {
	const s = Math.max(0, Math.round(ms / 1000));
	if (s >= 3600) return wait(s * 1000);
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

// Matches the wording used elsewhere on the page.
export function wait(ms: number): string {
	const m = Math.round(Math.max(0, ms) / 60000);
	if (m < 1) return say('health.wait.under');
	if (m >= 60) return m % 60 ? say('health.wait.hourMin', { h: Math.floor(m / 60), m: m % 60 }) : say('health.wait.hour', { h: Math.floor(m / 60) });
	return say('health.wait.min', { m });
}

// Empty for rows that are not jobs.
export const kindOf = (r: StatusRow) => (r.check?.kind === 'job' ? r.check.job : '');

// One feed per plate; reads again on each job push.
export class LiveJobs {
	active = $state<ActiveJob[]>([]);
	history = $state<HistJob[]>([]);
	tick = $state(Date.now());
	running = $derived(this.active.filter((j) => j.state === 'running'));
	queued = $derived(this.active.filter((j) => j.state === 'queued'));
	runningKinds = $derived(new Set(this.running.map((j) => j.kind)));
	names: Record<string, string> = {};

	// Falls back to the kind with spaces when no row names it.
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
	// 0 right after a run, 1 when the next one is due.
	fill(r: StatusRow): number {
		const a = t(r.lastRun), b = t(r.nextRun);
		if (!Number.isFinite(b)) return 0;
		if (!Number.isFinite(a) || b <= a) return this.tick >= b ? 1 : 0;
		return Math.min(1, Math.max(0, (this.tick - a) / (b - a)));
	}
	// Negative once overdue.
	due(r: StatusRow): number {
		return t(r.nextRun) - this.tick;
	}
	// Skips jobs already running so they do not show twice.
	coming(upcoming: StatusRow[], n: number): StatusRow[] {
		return upcoming.filter((r) => !this.runningKinds.has(kindOf(r))).slice(0, n);
	}
	// Newest first.
	finished = $derived(this.history.filter((j) => j.EndedAt && j.State !== 'running' && j.State !== 'queued').sort((a, b) => t(b.EndedAt) - t(a.EndedAt)));
	log = $derived.by((): LogEntry[] => {
		const now: LogEntry[] = this.running.map((j) => ({ key: `r${j.id}`, at: t(j.started_at ?? j.queued_at), name: j.label, text: j.note || say('health.live.working'), tone: 'info' }));
		const past: LogEntry[] = this.finished.slice(0, 12).map((j) => ({
			key: `h${j.ID}`,
			at: t(j.EndedAt),
			name: this.nameOf(j.Kind),
			text: j.State === 'done' ? say('health.result.ok') : j.State === 'failed' ? say('health.result.failed') : j.State === 'cancelled' ? say('health.result.cancelled') : j.State,
			tone: j.State === 'done' ? 'ok' : j.State === 'failed' ? 'fail' : 'idle'
		}));
		return [...now, ...past];
	});

	// The 400 ms wait groups bursts of pushes into one read.
	#timer: ReturnType<typeof setTimeout> | undefined;
	#offs: (() => void)[] = [];
	#interval: ReturnType<typeof setInterval> | undefined;
	#seq = 0;

	async load() {
		// Only the newest read is kept, so a slow older reply cannot bring back a finished job.
		const n = ++this.#seq;
		try {
			const [a, h] = await Promise.all([api.get<{ jobs: ActiveJob[] }>('/jobs/active'), api.get<{ jobs: HistJob[] }>('/jobs?limit=80')]);
			if (n !== this.#seq) return;
			this.active = a.jobs;
			this.history = h.jobs;
			for (const j of a.jobs) this.names[j.kind] = j.label;
		} catch {
			// Keep the last good data.
		}
	}
	start() {
		void this.load();
		// At most one read waits at a time; a reset timer could be starved by steady pushes.
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
