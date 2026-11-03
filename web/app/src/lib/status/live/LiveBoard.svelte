<script lang="ts">
	// View 1, Board: running jobs as cards with a progress bar and note, who
	// is waiting in line, and the next runs counting down with a track that
	// fills as each run nears.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();
	// As many next runs as fit beside the rail once the running cards have taken their room.
	const next = $derived(lj.coming(upcoming, [6, 4, 3, 2][Math.min(lj.running.length, 3)]));
</script>

<div class="board">
	<h3>
		Right now
		<span class="st" class:on={lj.running.length > 0}>{lj.running.length ? `${lj.running.length} running` : 'Idle'}</span>
	</h3>

	{#each lj.running.slice(0, 3) as j (j.id)}
		{@const pct = Math.round(j.progress * 100)}
		<div class="run">
			<div class="ln"><i class="d live" aria-hidden="true"></i><span class="nm">{j.label}</span><span class="el">{clock(lj.tick - t(j.started_at ?? j.queued_at))}</span></div>
			<div class="bar" class:indet={pct <= 0} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct > 0 ? pct : undefined} aria-label={j.label}><i style:width="{pct > 0 ? pct : 100}%"></i></div>
			<div class="note">{[j.note || 'Working', pct > 0 ? `${pct}%` : '', j.by_person ? 'started by hand' : ''].filter(Boolean).join(' · ')}</div>
		</div>
	{/each}
	{#if lj.running.length > 3}<p class="more">and {lj.running.length - 3} more running</p>{/if}

	{#if lj.queued.length}
		<p class="queue"><span class="ql">Waiting · {lj.queued.length}</span>{lj.queued.slice(0, 3).map((j) => j.label).join(', ')}{#if lj.queued.length > 3} and {lj.queued.length - 3} more{/if}</p>
	{/if}

	{#if next.length}
		<h4>Next up</h4>
		<ol class="next">
			{#each next as r (r.key)}
				{@const due = lj.due(r)}
				<li>
					<div class="ln"><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span><span class="el" class:due={due <= 0}>{due <= 0 ? 'due' : clock(due)}</span></div>
					<div class="bar thin"><i style:width="{lj.fill(r) * 100}%"></i></div>
				</li>
			{/each}
		</ol>
	{:else if !lj.running.length}
		<p class="none">Nothing running and nothing planned.</p>
	{/if}
</div>

<style>
	.board { display: flex; flex-direction: column; min-width: 0; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	h4 { margin: 12px 0 6px; font: 600 12px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }
	ol { list-style: none; margin: 0; padding: 0; }
	.ln { display: flex; align-items: center; gap: 9px; min-height: 24px; }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 13.5px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.el { font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.el.due { color: var(--accent, var(--text)); font-weight: 600; }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); }
	.d.idle { background: var(--text-faint); }
	.d.live { background: var(--accent, var(--text)); }
	.run { padding: 4px 0 8px; }
	.run + .run { border-top: 1px solid var(--line); }
	.bar { height: 6px; margin: 4px 0 0 16px; border-radius: 3px; overflow: hidden; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.bar i { display: block; height: 100%; border-radius: inherit; background: var(--accent, var(--text)); transition: width 0.8s linear; }
	.bar.thin { height: 3px; margin-top: 3px; }
	.bar.thin i { background: color-mix(in srgb, var(--text) 38%, transparent); }
	.note { margin: 5px 0 0 16px; font-size: 12px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.queue { display: flex; align-items: baseline; gap: 8px; margin: 6px 0 0; padding: 8px 0 0; border-top: 1px solid var(--line); font-size: 12.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ql { flex: none; font-weight: 600; color: var(--text); }
	.next li { padding: 3px 0 5px; }
	.more, .none { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); }
	@media (prefers-reduced-motion: no-preference) {
		.d.live { animation: pulse 2.4s ease-out infinite; }
		.bar.indet i { width: 40% !important; animation: slide 1.6s ease-in-out infinite; }
		@keyframes pulse { 0% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70%, 100% { box-shadow: 0 0 0 9px transparent; } }
		@keyframes slide { 0% { transform: translateX(-100%); } 100% { transform: translateX(250%); } }
	}
	@media (prefers-reduced-motion: reduce) { .bar.indet i { width: 100% !important; opacity: 0.45; } }
</style>
