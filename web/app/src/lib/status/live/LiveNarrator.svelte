<script lang="ts">
	// View "Narrator": one sentence in display type that rewrites itself as
	// things happen. Names and times are set off in the sentence, the
	// countdown ticks in place, and a hairline under it fills while a job runs.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const job = $derived(lj.running[0]);
	const others = $derived(lj.running.length - 1);
	const coming = $derived(lj.coming(upcoming, 3));
	const last = $derived(lj.finished[0]);
	const fill = $derived(job ? job.progress : coming[0] ? lj.fill(coming[0]) : 0);
	const hm = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
	const inWords = (r: StatusRow) => (lj.due(r) <= 0 ? 'is due now' : `in ${clock(lj.due(r))}`);
</script>

<div class="nar" class:on={!!job}>
	<p class="say">
		{#if job}
			<b>{job.label}</b> is running{#if job.note}{', '}<span class="q">{job.note.toLowerCase()}</span>{/if}{#if job.progress > 0}{', '}<em>{Math.round(job.progress * 100)}%</em> done{/if}{#if others > 0}{`, with ${others} more job${others > 1 ? 's' : ''} alongside`}{/if}.
		{:else}
			Nothing is running.
		{/if}
		{#if lj.queued.length}
			<b>{lj.queued[0].label}</b>{#if lj.queued.length > 1}{` and ${lj.queued.length - 1} more`}{/if} {lj.queued.length > 1 ? 'are' : 'is'} waiting in line.
		{/if}
		{#if coming.length}
			{job ? 'Then' : 'Next'} <b>{coming[0].name}</b> <em>{inWords(coming[0])}</em>{#if coming[1]}{', then '}<b>{coming[1].name}</b> {inWords(coming[1])}{/if}{#if coming[2]}{' and '}<b>{coming[2].name}</b> {inWords(coming[2])}{/if}.
		{:else if !job}
			Nothing is planned.
		{/if}
	</p>
	<div class="hair"><i style:width="{fill * 100}%"></i></div>
	{#if last}
		<p class="was">Last finished: {lj.nameOf(last.Kind)}, {last.State === 'done' ? 'OK' : last.State === 'failed' ? 'failed' : last.State} at {hm(t(last.EndedAt))}.</p>
	{/if}
</div>

<style>
	.nar { display: flex; flex-direction: column; justify-content: center; min-height: 100%; min-width: 0; }
	.say { margin: 0; font: 500 21px/1.4 var(--font-display); letter-spacing: -0.012em; color: var(--text-muted); text-wrap: pretty; }
	.say b { font-weight: 600; color: var(--text); }
	.say em { font-style: normal; font-weight: 600; color: var(--text); font-variant-numeric: tabular-nums; }
	.on .say em { color: var(--accent, var(--text)); }
	.q { color: var(--text); }
	.hair { height: 2px; margin: 18px 0 0; border-radius: 1px; overflow: hidden; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.hair i { display: block; height: 100%; background: color-mix(in srgb, var(--text) 40%, transparent); transition: width 0.8s linear; }
	.on .hair i { background: var(--accent, var(--text)); }
	.was { margin: 10px 0 0; font-size: 12.5px; color: var(--text-faint); }
	@media (max-width: 480px) { .say { font-size: 18px; } }
</style>
