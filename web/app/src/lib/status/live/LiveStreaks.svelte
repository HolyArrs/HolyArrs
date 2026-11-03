<script lang="ts">
	// View "Streaks": one strip per job of its last ten results, oldest at the
	// left and newest at the right, so a job that keeps failing shows as a run
	// of red. The jobs with the most recent activity come first, and each line
	// ends with how long its last run took.
	import type { HistJob, LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	type Strip = { kind: string; name: string; runs: HistJob[]; latest: number; ok: number; failed: number };

	const hm = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
	const word: Record<string, string> = { done: 'OK', failed: 'Failed', cancelled: 'Cancelled', running: 'Running', queued: 'Waiting' };
	// When a run happened: its start, or its end when the start is missing.
	const at = (j: HistJob) => {
		const s = t(j.StartedAt);
		return Number.isFinite(s) ? s : t(j.EndedAt);
	};

	const dayStart = $derived.by(() => {
		const d = new Date(lj.tick);
		d.setHours(0, 0, 0, 0);
		return d.getTime();
	});
	const when = (ms: number) => (!Number.isFinite(ms) ? '' : ms >= dayStart ? hm(ms) : `${new Date(ms).toLocaleDateString([], { weekday: 'short' })} ${hm(ms)}`);

	const strips = $derived.by((): Strip[] => {
		const by = new Map<string, HistJob[]>();
		for (const j of lj.history) {
			const list = by.get(j.Kind);
			if (list) list.push(j);
			else by.set(j.Kind, [j]);
		}
		const out: Strip[] = [];
		for (const [kind, all] of by) {
			const sorted = all.slice().sort((a, b) => (at(a) || 0) - (at(b) || 0));
			const runs = sorted.slice(-10);
			const latest = Math.max(...all.map((j) => (j.State === 'running' ? lj.tick : Math.max(t(j.EndedAt) || 0, at(j) || 0))));
			out.push({
				kind,
				name: lj.nameOf(kind),
				runs,
				latest,
				ok: runs.filter((j) => j.State === 'done').length,
				failed: runs.filter((j) => j.State === 'failed').length
			});
		}
		return out.sort((a, b) => b.latest - a.latest).slice(0, 7);
	});

	const okPct = $derived.by(() => {
		const done = lj.finished.filter((j) => j.State === 'done').length;
		return lj.finished.length ? Math.round((done / lj.finished.length) * 100) : null;
	});

	function took(j: HistJob | undefined): string {
		if (!j) return '—';
		if (j.State === 'running') return 'running';
		if (j.State === 'queued') return 'waiting';
		const a = t(j.StartedAt), b = t(j.EndedAt);
		return Number.isFinite(a) && Number.isFinite(b) && b >= a ? clock(b - a) : '—';
	}
	function tip(j: HistJob): string {
		if (j.State === 'running') return `Running since ${when(at(j))}`;
		const end = t(j.EndedAt);
		return `${word[j.State] ?? j.State} · ${when(Number.isFinite(end) ? end : at(j))}`;
	}
	const summary = (s: Strip) => `${s.name}: last ${s.runs.length} runs, ${s.ok} OK, ${s.failed} failed`;
</script>

<div class="stk">
	<h3>
		Reliability
		{#if okPct !== null}<span class="st" class:on={okPct === 100}>{okPct}% OK</span>{/if}
	</h3>

	{#if strips.length}
		<ol class="list">
			{#each strips as s (s.kind)}
				{@const last = s.runs[s.runs.length - 1]}
				<li class="kd">
					<span class="nm" title={s.name}>{s.name}</span>
					<span class="strip" role="img" aria-label={summary(s)}>
						{#each s.runs as j (j.ID)}
							<i class="sq {j.State}" title={tip(j)}></i>
						{/each}
					</span>
					<span class="el" class:live={last?.State === 'running'}>{took(last)}</span>
				</li>
			{/each}
		</ol>
		<div class="key" aria-hidden="true">
			<span class="hint">Last 10 runs, newest at right</span>
			<span><i class="sq done"></i>OK</span>
			<span><i class="sq failed"></i>Failed</span>
			<span><i class="sq cancelled"></i>Cancelled</span>
		</div>
	{:else}
		<p class="none">No runs yet.</p>
	{/if}
</div>

<style>
	.stk { display: flex; flex-direction: column; min-width: 0; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.st.on { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--text); }

	.list { list-style: none; margin: 0; padding: 0; }
	.kd { display: grid; grid-template-columns: minmax(0, 1fr) auto 62px; align-items: center; gap: 12px; min-height: 28px; }
	.kd + .kd { border-top: 1px solid var(--line); }
	.nm { min-width: 0; font-size: 13px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.strip { display: flex; justify-content: flex-end; gap: 3px; width: 127px; }
	.el { justify-self: end; font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.el.live { color: var(--accent, var(--text)); font-weight: 600; }

	.sq { flex: none; display: inline-block; width: 10px; height: 10px; border-radius: 2px; box-sizing: border-box; background: color-mix(in srgb, var(--text) 10%, transparent); }
	.sq.done { background: color-mix(in srgb, var(--ok) 72%, transparent); }
	.sq.failed { background: var(--fail); }
	.sq.cancelled { background: color-mix(in srgb, var(--text-faint) 45%, transparent); }
	.sq.queued { background: transparent; box-shadow: inset 0 0 0 1px var(--line-strong); }
	.sq.running { background: var(--accent, var(--text)); }

	.key { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; margin-top: 10px; padding-top: 9px; border-top: 1px solid var(--line); font-size: 11.5px; color: var(--text-muted); }
	.key span { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
	.key .hint { margin-right: auto; }
	.none { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); }

	@media (max-width: 420px) {
		.kd { grid-template-columns: minmax(0, 1fr) auto 52px; gap: 8px; }
		.key .hint { flex-basis: 100%; }
	}
	@media (prefers-reduced-motion: no-preference) {
		.sq.running { animation: beat 1.8s ease-in-out infinite; }
		@keyframes beat { 0%, 100% { opacity: 1; } 50% { opacity: 0.45; } }
	}
</style>
