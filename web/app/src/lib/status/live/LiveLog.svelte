<script lang="ts">
	// View "Log": a live ticker. Running jobs sit on top with how long they
	// have run, their latest note and a thin progress line; under them the
	// last finished runs with the time they ended, growing quieter with age.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const lines = $derived(lj.log.slice(0, 9));
	// Running lines carry the key "r<id>", so their progress is found by key.
	const prog = $derived(new Map(lj.running.map((j) => [`r${j.id}`, j.progress])));
	const live = $derived(lines.filter((e) => prog.has(e.key)).length);

	const pad = (n: number) => String(n).padStart(2, '0');
	const hm = (ms: number) => {
		if (!Number.isFinite(ms)) return '';
		const d = new Date(ms);
		return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
	};
	const since = (ms: number) => (Number.isFinite(ms) ? clock(lj.tick - ms) : '');
</script>

<div class="log">
	<h3>
		Live log
		<span class="st" class:on={lj.running.length > 0}>{lj.running.length ? `${lj.running.length} running` : lines.length ? `${lines.length} recent` : 'Idle'}</span>
	</h3>

	{#if lines.length}
		<ol aria-live="polite">
			{#each lines as e, i (e.key)}
				{@const p = prog.get(e.key)}
				{@const run = p !== undefined}
				{@const pct = run ? Math.round((p ?? 0) * 100) : 0}
				<li class="ln {e.tone}" class:run style:opacity={run ? 1 : Math.max(0.62, 1 - (i - live) * 0.045)}>
					<i class="d" aria-hidden="true"></i>
					<span class="tm">{run ? since(e.at) : hm(e.at)}</span>
					<span class="nm">{e.name}</span>
					<span class="tx">{e.text}{run && pct > 0 ? ` · ${pct}%` : ''}</span>
					{#if run}
						<span class="pg" class:indet={pct <= 0} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct > 0 ? pct : undefined} aria-label={e.name}><i style:width="{pct > 0 ? pct : 100}%"></i></span>
					{/if}
				</li>
			{/each}
		</ol>
	{:else}
		<p class="none">Nothing has run yet.</p>
	{/if}
</div>

<style>
	.log { display: flex; flex-direction: column; min-width: 0; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	ol { position: relative; display: flex; flex-direction: column; gap: 1px; list-style: none; margin: 0; padding: 0; min-width: 0; }
	/* A faint thread through the dots, so the lines read as one stream. */
	ol::before { content: ''; position: absolute; left: 11px; top: 14px; bottom: 14px; width: 1px; background: var(--line); }

	.ln { position: relative; display: flex; align-items: center; gap: 9px; height: 28px; padding: 0 8px; border-radius: 8px; min-width: 0; font-size: 13px; }
	.ln.run { background: color-mix(in srgb, var(--accent, var(--text)) 8%, transparent); }

	.d { position: relative; flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
	.ok .d { background: var(--ok); }
	.fail .d { background: var(--fail); }
	.info .d { background: var(--info); }
	.run .d { background: var(--accent, var(--text)); }

	.tm { flex: none; min-width: 38px; font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.run .tm { color: var(--text); font-weight: 600; }
	.nm { flex: 0 1 auto; min-width: 0; max-width: 52%; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tx { flex: 1 1 0; min-width: 0; font-size: 12.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.fail .tx { color: var(--fail); font-weight: 600; }

	.pg { position: absolute; left: 24px; right: 8px; bottom: 3px; height: 2px; border-radius: 1px; overflow: hidden; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.pg i { display: block; height: 100%; border-radius: inherit; background: var(--accent, var(--text)); transition: width 0.8s linear; }

	.none { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); }

	@media (prefers-reduced-motion: no-preference) {
		.ln { animation: enter 0.35s ease-out; }
		.run .d { animation: pulse 2.4s ease-out infinite; }
		.pg.indet i { width: 35% !important; animation: slide 1.6s ease-in-out infinite; }
		@keyframes enter { from { opacity: 0; transform: translateY(-4px); } }
		@keyframes pulse { 0% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70%, 100% { box-shadow: 0 0 0 8px transparent; } }
		@keyframes slide { 0% { transform: translateX(-100%); } 100% { transform: translateX(300%); } }
	}
	@media (prefers-reduced-motion: reduce) {
		.pg i { transition: none; }
		.pg.indet i { width: 100% !important; opacity: 0.45; }
	}
</style>
