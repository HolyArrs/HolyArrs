<script lang="ts">
	// View "Rings": up to four rings, one per job. While a job runs, the outer
	// ring is its progress in accent; the other rings are the next runs, each
	// filling as its start nears. The legend names each ring with its percent
	// or the time left.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const C = 95;
	const OUT = 87;
	const STEP = 18;
	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };
	const accent = 'var(--accent, var(--text))';

	interface Ring { key: string; name: string; frac: number; col: string; run: boolean; pct: number; due: number }

	const run = $derived(lj.running[0]);
	const next = $derived(lj.coming(upcoming, run ? 3 : 4));
	const rings: Ring[] = $derived([
		...(run ? [{ key: `r${run.id}`, name: run.label, frac: run.progress, col: accent, run: true, pct: Math.round(run.progress * 100), due: 0 }] : []),
		...next.map((r) => ({ key: r.key, name: r.name, frac: lj.fill(r), col: tone[r.tone] ?? 'var(--info)', run: false, pct: 0, due: lj.due(r) }))
	]);

	// The middle of the rings: the running percent, or how long until the next run.
	const mid = $derived.by(() => {
		const f = rings[0];
		if (!f) return null;
		if (f.run) return f.pct > 0 ? { v: `${f.pct}%`, c: 'done' } : null;
		if (f.due <= 0) return { v: 'due', c: 'next' };
		const s = Math.round(f.due / 1000);
		return { v: s >= 3600 ? `${Math.floor(s / 3600)} h` : clock(f.due), c: 'next' };
	});
</script>

<div class="rings">
	<h3>
		Coming up
		<span class="st" class:on={lj.running.length > 0}>{lj.running.length ? `${lj.running.length} running` : 'Idle'}</span>
	</h3>

	<div class="body">
		<svg class="dial" viewBox="0 0 190 190" role="img" aria-label="Rings for the running job and the next runs">
			{#if rings.length}
				{#each rings as g, i (g.key)}
					{@const r = OUT - i * STEP}
					{@const len = 2 * Math.PI * r}
					{@const indet = g.run && g.pct <= 0}
					<g style:--c={g.col}>
						<circle class="trk" cx={C} cy={C} {r} />
						<circle
							class="arc"
							class:indet
							cx={C}
							cy={C}
							{r}
							stroke-dasharray={indet ? `${len * 0.22} ${len}` : `${len} ${len}`}
							stroke-dashoffset={indet ? 0 : len * (1 - Math.min(1, Math.max(0, g.frac)))}
							transform="rotate(-90 {C} {C})"
						/>
					</g>
				{/each}
				{#if mid}
					<text class="mv" x={C} y={C + (rings.length > 3 ? 1 : 2)} text-anchor="middle">{mid.v}</text>
					{#if rings.length < 4}<text class="mc" x={C} y={C + 16} text-anchor="middle">{mid.c}</text>{/if}
				{/if}
			{:else}
				<circle class="empty" cx={C} cy={C} r={OUT} />
			{/if}
		</svg>

		<div class="lg">
			{#each rings as g (g.key)}
				{@const late = !g.run && g.due <= 0}
				<div class="it" style:--c={g.col}>
					<i class="sw" class:live={g.run} aria-hidden="true"></i>
					<span class="nm">{g.name}</span>
					<span class="el" class:on={g.run} class:due={late}>{g.run ? (g.pct > 0 ? `${g.pct}%` : 'working') : late ? 'due' : clock(g.due)}</span>
				</div>
			{/each}
			{#if lj.running.length > 1}<p class="more">and {lj.running.length - 1} more running</p>{/if}
			{#if !rings.length}<p class="none">Nothing planned.</p>{/if}
		</div>
	</div>
</div>

<style>
	.rings { display: flex; flex-direction: column; min-width: 0; container-type: inline-size; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	.body { display: flex; align-items: center; gap: 18px; min-width: 0; }
	.dial { flex: none; display: block; width: 190px; max-width: 100%; height: auto; }

	.trk { fill: none; stroke: color-mix(in srgb, var(--c) 18%, transparent); stroke-width: 13; }
	.arc { fill: none; stroke: var(--c); stroke-width: 13; stroke-linecap: round; transition: stroke-dashoffset 0.8s linear; }
	.empty { fill: none; stroke: var(--line); stroke-width: 13; }
	.mv { fill: var(--text); font: 600 14px/1 var(--font-ui); font-variant-numeric: tabular-nums; dominant-baseline: middle; }
	.mc { fill: var(--text-muted); font: 500 10.5px/1 var(--font-ui); dominant-baseline: middle; }

	.lg { flex: 1 1 0; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.it { display: flex; align-items: center; gap: 9px; min-height: 28px; min-width: 0; }
	.sw { flex: none; width: 10px; height: 10px; border-radius: 3px; background: var(--c); box-shadow: 0 0 0 3px color-mix(in srgb, var(--c) 18%, transparent); }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 13px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.el { flex: none; font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.el.on { color: var(--text); font-weight: 600; }
	.el.due { color: var(--accent, var(--text)); font-weight: 600; }
	.more, .none { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); }

	@container (max-width: 349px) {
		.body { flex-direction: column; align-items: stretch; gap: 12px; }
		.dial { align-self: center; }
	}

	@media (prefers-reduced-motion: no-preference) {
		.arc.indet { transform-box: view-box; transform-origin: 95px 95px; animation: spin 1.8s linear infinite; }
		.sw.live { animation: pulse 2.4s ease-out infinite; }
		@keyframes spin { from { transform: rotate(-90deg); } to { transform: rotate(270deg); } }
		@keyframes pulse { 0% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--c) 30%, transparent); } 70%, 100% { box-shadow: 0 0 0 8px transparent; } }
	}
	@media (prefers-reduced-motion: reduce) {
		.arc { transition: none; }
		.arc.indet { stroke-dasharray: none; opacity: 0.5; }
	}
</style>
