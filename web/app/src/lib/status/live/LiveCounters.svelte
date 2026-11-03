<script lang="ts">
	// View "Counters": four quiet numbers in one block split by hairlines.
	// How many jobs run now, how many ran today, how many failed lately, and
	// the countdown to the next run. Only the running bar and the next
	// countdown carry the accent; failures turn the fail colour when there are any.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const hm = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });

	const dayStart = $derived.by(() => {
		const d = new Date(lj.tick);
		d.setHours(0, 0, 0, 0);
		return d.getTime();
	});

	const run = $derived(lj.running[0]);
	const pct = $derived(run ? Math.round(run.progress * 100) : 0);

	const today = $derived(lj.finished.filter((j) => t(j.EndedAt) >= dayStart));
	const okToday = $derived(today.filter((j) => j.State === 'done').length);
	const failedToday = $derived(today.filter((j) => j.State === 'failed').length);
	const ranLine = $derived(
		!today.length
			? 'nothing yet'
			: failedToday
				? `${okToday} OK · ${failedToday} failed`
				: okToday === today.length
					? 'all OK'
					: `${okToday} OK · ${today.length - okToday} cancelled`
	);

	const failed = $derived(lj.finished.filter((j) => j.State === 'failed'));
	const lastFail = $derived(failed[0]);
	function when(ms: number): string {
		if (ms >= dayStart) return hm(ms);
		return `${new Date(ms).toLocaleDateString([], { weekday: 'short' })} ${hm(ms)}`;
	}

	const next = $derived(lj.coming(upcoming, 1)[0]);
	const due = $derived(next ? lj.due(next) : NaN);
	// The countdown split into digits and words, so "3 h 10 min" keeps big digits with small units.
	const parts = $derived(Number.isFinite(due) && due > 0 ? clock(due).split(/(\d+)/).filter(Boolean) : []);
</script>

<div class="ctr">
	<h3>
		At a glance
		{#if lj.queued.length}<span class="st">{lj.queued.length} waiting</span>{/if}
	</h3>

	<div class="tiles">
		<section class="tile">
			<h4>Running</h4>
			<p class="num" class:zero={!lj.running.length}>{lj.running.length}</p>
			<p class="sub">{run ? run.label : 'Idle'}{#if run && pct > 0}<span class="pc"> · {pct}%</span>{/if}</p>
			{#if run}
				<div class="prog" class:indet={pct <= 0} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct > 0 ? pct : undefined} aria-label={run.label}>
					<i style:width="{pct > 0 ? pct : 100}%"></i>
				</div>
			{/if}
		</section>

		<section class="tile">
			<h4>Ran today</h4>
			<p class="num" class:zero={!today.length}>{today.length}</p>
			<p class="sub">{ranLine}</p>
		</section>

		<section class="tile">
			<h4>Failed</h4>
			<p class="num" class:zero={!failed.length} class:bad={failed.length > 0}>{failed.length}</p>
			<p class="sub">
				{#if lastFail}{lj.nameOf(lastFail.Kind)}<span class="pc"> · {when(t(lastFail.EndedAt))}</span>{:else}none lately{/if}
			</p>
		</section>

		<section class="tile">
			<h4>Next</h4>
			{#if !next}
				<p class="num zero">–</p>
				<p class="sub">nothing planned</p>
			{:else}
				<p class="num acc" aria-label={due <= 0 ? 'due now' : `in ${clock(due)}`}>
					{#if due <= 0}due{:else}{#each parts as p, i (i)}{#if /^\d+$/.test(p)}{p}{:else}<small>{p}</small>{/if}{/each}{/if}
				</p>
				<p class="sub">{next.name}</p>
			{/if}
		</section>
	</div>
</div>

<style>
	.ctr { display: flex; flex-direction: column; min-width: 0; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; min-height: 20px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }

	.tiles { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.tile { min-width: 0; padding: 14px 16px 16px; }
	.tile:nth-child(odd) { padding-left: 0; border-right: 1px solid var(--line); }
	.tile:nth-child(even) { padding-right: 0; }
	.tile:nth-child(-n + 2) { border-bottom: 1px solid var(--line); }
	.tile:nth-child(-n + 2) { padding-top: 4px; }

	h4 { margin: 0; font: 500 12px/1.3 var(--font-ui); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.num {
		margin: 6px 0 0; font: 600 32px/1 var(--font-ui); letter-spacing: -0.01em; color: var(--text);
		font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
	}
	.num small { font-size: 14px; font-weight: 500; letter-spacing: 0; color: inherit; opacity: 0.8; }
	.num.zero { color: var(--text-faint); }
	.num.bad { color: var(--fail); }
	.num.acc { color: var(--accent, var(--text)); }
	.sub { margin: 6px 0 0; font-size: 12px; line-height: 1.35; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.pc { font-variant-numeric: tabular-nums; }

	.prog { height: 3px; margin-top: 7px; border-radius: 2px; overflow: hidden; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.prog i { display: block; height: 100%; border-radius: inherit; background: var(--accent, var(--text)); transition: width 0.8s linear; }

	@media (max-width: 420px) {
		.tile { padding: 12px 12px 14px; }
		.num { font-size: 30px; }
		.num small { font-size: 13px; }
	}
	@media (prefers-reduced-motion: no-preference) {
		.prog.indet i { width: 40% !important; animation: slide 1.6s ease-in-out infinite; }
		@keyframes slide { 0% { transform: translateX(-100%); } 100% { transform: translateX(250%); } }
	}
	@media (prefers-reduced-motion: reduce) {
		.prog i { transition: none; }
		.prog.indet i { width: 100% !important; opacity: 0.45; }
	}
</style>
