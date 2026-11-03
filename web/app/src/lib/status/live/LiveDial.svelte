<script lang="ts">
	// View 2, Dial: one big ring. While a job runs it is that job's progress,
	// with the percent inside and the note under it. Idle, it is the wait for
	// the next run, filling as the run nears, with the countdown inside.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const job = $derived(lj.running[0]);
	const coming = $derived(lj.coming(upcoming, 4));
	const first = $derived(coming[0]);
	const then = $derived(job ? coming.slice(0, 3) : coming.slice(1, 4));
	// What the ring shows: a job's progress, or how far the first wait has come.
	const value = $derived(job ? job.progress : first ? lj.fill(first) : 0);
	const indet = $derived(!!job && job.progress <= 0);
	const big = $derived(job ? (job.progress > 0 ? `${Math.round(job.progress * 100)}%` : clock(lj.tick - t(job.started_at ?? job.queued_at))) : first ? (lj.due(first) <= 0 ? 'due' : clock(lj.due(first))) : '—');
	const small = $derived(job ? (job.progress > 0 ? clock(lj.tick - t(job.started_at ?? job.queued_at)) : 'running') : first ? 'until next' : 'nothing planned');
	const title = $derived(job ? job.label : first ? first.name : 'Idle');
	const sub = $derived(job ? [job.note || 'Working', job.by_person ? 'started by hand' : ''].filter(Boolean).join(' · ') : first ? `Last ran ${first.lastRun ? clock(lj.tick - t(first.lastRun)) + ' ago' : 'never'}` : 'No runs are planned.');

	const R = 64, C = 2 * Math.PI * R;
</script>

<div class="dial" class:on={!!job}>
	<svg viewBox="0 0 160 160" width="160" height="160" role="img" aria-label="{title}: {big}">
		<circle class="track" cx="80" cy="80" r={R} />
		<circle class="arc" class:indet cx="80" cy="80" r={R} stroke-dasharray={C} stroke-dashoffset={C * (1 - (indet ? 0.28 : value))} transform="rotate(-90 80 80)" />
		<text class="big" x="80" y="78">{big}</text>
		<text class="sm" x="80" y="100">{small}</text>
	</svg>
	<div class="side">
		<h3>{job ? 'Running' : 'Next run'}</h3>
		<p class="ttl">{title}</p>
		<p class="sub">{sub}</p>
		{#if then.length}
			<ol class="then">
				{#each then as r (r.key)}
					{@const due = lj.due(r)}
					<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span><span class="el" class:due={due <= 0}>{due <= 0 ? 'due' : clock(due)}</span></li>
				{/each}
			</ol>
		{/if}
		{#if lj.queued.length}<p class="q">Waiting · {lj.queued.map((j) => j.label).join(', ')}</p>{/if}
	</div>
</div>

<style>
	.dial { display: grid; grid-template-columns: 160px minmax(0, 1fr); gap: 22px; align-items: center; min-height: 100%; }
	svg { display: block; overflow: visible; }
	.track { fill: none; stroke: color-mix(in srgb, var(--text) 9%, transparent); stroke-width: 10; }
	.arc { fill: none; stroke: color-mix(in srgb, var(--text) 42%, transparent); stroke-width: 10; stroke-linecap: round; transition: stroke-dashoffset 0.8s linear; }
	.on .arc { stroke: var(--accent, var(--text)); }
	.big { fill: var(--text); font: 600 28px/1 var(--font-ui); font-variant-numeric: tabular-nums; text-anchor: middle; letter-spacing: -0.02em; }
	.sm { fill: var(--text-muted); font: 500 11.5px/1 var(--font-ui); text-anchor: middle; }
	.side { min-width: 0; }
	h3 { margin: 0 0 4px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.ttl { margin: 0; font: 600 17px/1.25 var(--font-ui); color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sub { margin: 3px 0 0; font-size: 12.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.then { list-style: none; margin: 14px 0 0; padding: 10px 0 0; border-top: 1px solid var(--line); }
	.then li { display: flex; align-items: center; gap: 9px; min-height: 26px; }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 13px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.el { font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.el.due { color: var(--accent, var(--text)); font-weight: 600; }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); }
	.d.idle { background: var(--text-faint); }
	.q { margin: 8px 0 0; font-size: 12px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (prefers-reduced-motion: no-preference) {
		.arc.indet { animation: spin 1.4s linear infinite; transform-origin: 80px 80px; }
		@keyframes spin { from { transform: rotate(-90deg); } to { transform: rotate(270deg); } }
	}
	@media (max-width: 480px) {
		.dial { grid-template-columns: 1fr; justify-items: center; gap: 12px; }
		.side { width: 100%; text-align: center; }
	}
</style>
