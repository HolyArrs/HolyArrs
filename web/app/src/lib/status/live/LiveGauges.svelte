<script lang="ts">
	// View "Gauges": a row of slim upright gauges, one per coming run. Each
	// fills from the bottom as its wait goes by and reaches the dashed top
	// line when the run is due. A job running now takes the first gauge in
	// the accent colour, filled by its progress, with a soft glow.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, wait, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	type Gauge = { key: string; name: string; fill: number; c: string; sub: string; mark: string; live: boolean; indet: boolean; label: string };

	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };
	const short = (s: string) => (s.length > 12 ? `${s.slice(0, 11).trimEnd()}…` : s);
	const hm = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });

	const gauges = $derived.by((): Gauge[] => {
		const out: Gauge[] = [];
		const j = lj.running[0];
		if (j) {
			const p = Math.min(1, Math.max(0, j.progress));
			const pct = Math.round(p * 100);
			out.push({
				key: `r${j.id}`,
				name: j.label,
				fill: pct > 0 ? p : 1,
				c: 'var(--accent)',
				sub: pct > 0 ? `${pct}%` : clock(lj.tick - t(j.started_at ?? j.queued_at)),
				mark: 'now',
				live: true,
				indet: pct <= 0,
				label: `${j.label}, running${pct > 0 ? `, ${pct}% done` : ''}`
			});
		}
		for (const r of lj.coming(upcoming, 6 - out.length)) {
			const d = lj.due(r);
			const f = lj.fill(r);
			out.push({
				key: r.key,
				name: r.name,
				fill: f,
				c: tone[r.tone] ?? 'var(--info)',
				sub: d <= 0 ? 'due' : wait(d),
				mark: d <= 0 || f >= 0.9 ? 'due' : '',
				live: false,
				indet: false,
				label: `${r.name}, ${d <= 0 ? 'due now' : `next run in ${wait(d)}, at ${hm(t(r.nextRun))}`}, ${Math.round(f * 100)}% of the wait gone`
			});
		}
		return out;
	});
	const busy = $derived(lj.running.length > 0);
	const waiting = $derived(gauges.filter((g) => !g.live).length);

	// A tapped gauge says its full name and time in the line under the row for a while (touch screens have no hover).
	let pick = $state<{ key: string; label: string } | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;
	function show(g: Gauge) {
		pick = pick?.key === g.key ? null : { key: g.key, label: g.label };
		clearTimeout(timer);
		timer = setTimeout(() => (pick = null), 6000);
	}
	$effect(() => () => clearTimeout(timer));
</script>

<div class="gauges">
	<h3>
		Filling up to the next run
		<span class="st" class:on={busy}>{busy ? `${lj.running.length} running` : waiting ? `${waiting} coming` : 'Idle'}</span>
	</h3>

	{#if gauges.length}
		<div class="field" style:--count={gauges.length}>
			<div class="due-line" aria-hidden="true"></div>
			{#each gauges as g (g.key)}
				<button type="button" class="gg" class:live={g.live} class:sel={pick?.key === g.key} style:--c={g.c} aria-label={g.label} onclick={() => show(g)}>
					<span class="mk" class:on={g.mark !== ''}>{g.mark || '·'}</span>
					<div class="tr" class:indet={g.indet}>
						<i class="tk" style:bottom="25%"></i><i class="tk" style:bottom="50%"></i><i class="tk" style:bottom="75%"></i>
						<div class="fl" style:transform="translateY({(1 - g.fill) * 100}%)"><span class="cap"></span></div>
					</div>
					<span class="nm" title={g.name}>{short(g.name)}</span>
					<span class="cd" class:due={g.sub === 'due'} class:on={g.live}>{g.sub}</span>
				</button>
			{/each}
		</div>
	{:else}
		<p class="none">Nothing running and nothing planned.</p>
	{/if}
	<p class="pick" aria-live="polite">{pick?.label ?? ''}</p>
</div>

<style>
	.gauges { display: flex; flex-direction: column; min-width: 0; height: 100%; container-type: inline-size; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	.field {
		--w: 18px;
		--gh: 150px;
		position: relative;
		display: grid;
		grid-template-columns: repeat(var(--count), minmax(0, 1fr));
		align-items: start;
		margin: auto 0;
		padding: 4px 0 2px;
	}
	/* The dashed line every gauge fills up to, and the floor they stand on. */
	.due-line { position: absolute; left: 0; right: 0; top: 24px; border-top: 1px dashed color-mix(in srgb, var(--text) 18%, transparent); pointer-events: none; }
	.field::after { content: ''; position: absolute; left: 0; right: 0; top: calc(24px + var(--gh)); border-top: 1px solid var(--line); pointer-events: none; }

	.gg { display: flex; flex-direction: column; align-items: center; min-width: 0; margin: 0; padding: 0 2px; border: 0; border-radius: 10px; background: none; color: inherit; font: inherit; cursor: pointer; }
	.gg:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.gg.sel .nm { color: var(--text); }
	.pick { flex: none; margin: 8px 0 0; font-size: 12px; line-height: 1.3; color: var(--text); text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pick:empty { display: none; }
	.mk { height: 16px; margin-bottom: 4px; padding: 0 6px; border-radius: var(--r-pill); font: 600 10.5px/16px var(--font-ui); color: transparent; }
	.mk.on { color: var(--accent-text); background: color-mix(in srgb, var(--accent) 14%, transparent); }

	.tr {
		position: relative;
		width: var(--w);
		height: var(--gh);
		border-radius: var(--r-pill);
		overflow: hidden;
		background: color-mix(in srgb, var(--text) 7%, transparent);
		box-shadow: inset 0 0 0 1px var(--line);
	}
	.tk { position: absolute; left: 25%; right: 25%; height: 1px; background: color-mix(in srgb, var(--text) 14%, transparent); z-index: 1; }
	.fl {
		position: absolute;
		inset: 0;
		border-radius: inherit;
		background: linear-gradient(to top, color-mix(in srgb, var(--c) 45%, transparent), var(--c));
		transition: transform 0.9s linear;
	}
	.cap { position: absolute; left: 2px; right: 2px; top: 1px; height: 3px; border-radius: var(--r-pill); background: color-mix(in srgb, var(--c) 55%, var(--text)); box-shadow: 0 0 6px var(--c); }

	.gg.live .tr { box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 45%, transparent), 0 0 0 3px color-mix(in srgb, var(--accent) 12%, transparent); }
	.gg.live .tr::after { content: ''; position: absolute; inset: 0; border-radius: inherit; background: color-mix(in srgb, var(--accent) 30%, transparent); opacity: 0; pointer-events: none; }
	.tr.indet .fl { opacity: 0.4; }

	.nm { max-width: 100%; margin-top: 8px; font-size: 11px; line-height: 1.2; color: var(--text-muted); text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cd { margin-top: 2px; max-width: 100%; font-size: 11px; line-height: 1.2; color: var(--text); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.cd.due, .cd.on { color: var(--accent-text); font-weight: 600; }
	.none { margin: auto 0; font-size: 12.5px; color: var(--text-muted); text-align: center; }

	@container (max-width: 400px) {
		.field { --w: 14px; }
	}

	@media (prefers-reduced-motion: no-preference) {
		.gg.live .tr::after { animation: glow 2.4s ease-in-out infinite; }
		.tr.indet .fl { opacity: 1; animation: rise 2s ease-in-out infinite; }
		@keyframes glow { 0%, 100% { opacity: 0; } 50% { opacity: 1; } }
		@keyframes rise { 0% { transform: translateY(100%); } 100% { transform: translateY(-100%); } }
	}
	@media (prefers-reduced-motion: reduce) {
		.fl { transition: none; }
	}
</style>
