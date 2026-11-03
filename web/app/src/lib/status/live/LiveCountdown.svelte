<script lang="ts">
	// View 3, Countdown: big digits. Idle, the time until the next run with
	// its name over the digits; running, the elapsed time with the job's
	// progress as a hairline under the digits. Three "then" lines below.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const job = $derived(lj.running[0]);
	const coming = $derived(lj.coming(upcoming, 4));
	const first = $derived(coming[0]);
	const then = $derived(job ? coming.slice(0, 3) : coming.slice(1, 4));
	const digits = $derived(job ? clock(lj.tick - t(job.started_at ?? job.queued_at)) : first ? (lj.due(first) <= 0 ? 'due' : clock(lj.due(first))) : '—:——');
	const over = $derived(job ? 'Running' : first ? 'Next run in' : 'Nothing planned');
	const name = $derived(job ? job.label : first ? first.name : 'The queue is empty');
	const under = $derived(job ? [job.progress > 0 ? `${Math.round(job.progress * 100)}%` : '', job.note || 'Working', job.by_person ? 'started by hand' : ''].filter(Boolean).join(' · ') : first?.lastRun ? `Last ran ${clock(lj.tick - t(first.lastRun))} ago` : first ? 'Never ran yet' : '');
	const fill = $derived(job ? job.progress : first ? lj.fill(first) : 0);
	const hms = $derived(digits.includes('h'));
</script>

<div class="cd" class:on={!!job}>
	<p class="over">{over}</p>
	<p class="name">{name}</p>
	<p class="digits" class:hms aria-live="off">{digits}</p>
	<div class="hair" class:indet={!!job && job.progress <= 0}><i style:width="{fill * 100}%"></i></div>
	<p class="under">{under}</p>
	{#if then.length || lj.queued.length}
		<ol class="then">
			{#if lj.queued.length}<li><i class="d live" aria-hidden="true"></i><span class="nm">Waiting · {lj.queued.map((j) => j.label).join(', ')}</span></li>{/if}
			{#each then as r (r.key)}
				{@const due = lj.due(r)}
				<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span><span class="el" class:due={due <= 0}>{due <= 0 ? 'due' : clock(due)}</span></li>
			{/each}
		</ol>
	{/if}
</div>

<style>
	.cd { display: flex; flex-direction: column; min-width: 0; min-height: 100%; }
	.over { margin: 0; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.name { margin: 2px 0 0; font: 600 16px/1.25 var(--font-ui); color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.digits { margin: 6px 0 0; font: 600 56px/1 var(--font-ui); letter-spacing: -0.03em; color: var(--text); font-variant-numeric: tabular-nums; }
	.digits.hms { font-size: 40px; }
	.on .digits { color: var(--accent, var(--text)); }
	.hair { height: 3px; margin: 12px 0 0; border-radius: 2px; overflow: hidden; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.hair i { display: block; height: 100%; border-radius: inherit; background: color-mix(in srgb, var(--text) 40%, transparent); transition: width 0.8s linear; }
	.on .hair i { background: var(--accent, var(--text)); }
	.under { margin: 8px 0 0; min-height: 1.2em; font-size: 12.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.then { list-style: none; margin: 12px 0 0; padding: 10px 0 0; border-top: 1px solid var(--line); }
	.then li { display: flex; align-items: center; gap: 9px; min-height: 26px; }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 13px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.el { font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.el.due { color: var(--accent, var(--text)); font-weight: 600; }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); }
	.d.idle { background: var(--text-faint); }
	.d.live { background: var(--accent, var(--text)); }
	@media (prefers-reduced-motion: no-preference) {
		.hair.indet i { width: 35% !important; animation: slide 1.6s ease-in-out infinite; }
		@keyframes slide { 0% { transform: translateX(-100%); } 100% { transform: translateX(290%); } }
	}
	@media (prefers-reduced-motion: reduce) { .hair.indet i { width: 100% !important; opacity: 0.45; } }
	@media (max-width: 480px) { .digits { font-size: 48px; } .digits.hms { font-size: 34px; } }
</style>
