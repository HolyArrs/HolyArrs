<script lang="ts">
	// View "Pipeline": jobs moving through three stages, left to right. Running
	// holds the jobs at work with their progress and note, Waiting the jobs in
	// line, and Coming the next planned runs counting down. A thin line joins
	// the stages so the flow reads at a glance.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };
	const next = $derived(lj.coming(upcoming, 4));
	const since = (iso?: string | null) => {
		const ms = t(iso);
		return Number.isFinite(ms) ? clock(lj.tick - ms) : '';
	};
	const busy = $derived(lj.running.length > 0);
</script>

<div class="pipe">
	<h3>
		Job flow
		<span class="st" class:on={busy}>{busy ? `${lj.running.length} running` : 'Idle'}</span>
	</h3>

	<div class="stages" class:busy>
		<section class="stage" aria-label="Running">
			<i class="node live" class:on={busy} aria-hidden="true"></i>
			<h4 class="sh">Running <span class="cnt">{lj.running.length}</span></h4>
			{#each lj.running.slice(0, 2) as j (j.id)}
				{@const pct = Math.round(j.progress * 100)}
				<div class="pl run">
					<div class="hd"><span class="nm">{j.label}</span><span class="el on">{pct > 0 ? `${pct}%` : since(j.started_at ?? j.queued_at)}</span></div>
					<div class="mb" class:indet={pct <= 0} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct > 0 ? pct : undefined} aria-label={j.label}><i style:width="{pct > 0 ? pct : 100}%"></i></div>
					<div class="nt">{j.note || 'Working'}</div>
				</div>
			{:else}
				<div class="slot">none</div>
			{/each}
			{#if lj.running.length > 2}<p class="more">and {lj.running.length - 2} more</p>{/if}
		</section>

		<section class="stage" aria-label="Waiting">
			<i class="node" class:on={lj.queued.length > 0} style:--c="var(--wait)" aria-hidden="true"></i>
			<h4 class="sh">Waiting <span class="cnt">{lj.queued.length}</span></h4>
			{#each lj.queued.slice(0, 3) as j (j.id)}
				<div class="pl" style:--c="var(--wait)">
					<div class="hd"><i class="d" aria-hidden="true"></i><span class="nm">{j.label}</span><span class="el">{since(j.queued_at)}</span></div>
				</div>
			{:else}
				<div class="slot">none</div>
			{/each}
			{#if lj.queued.length > 3}<p class="more">and {lj.queued.length - 3} more</p>{/if}
		</section>

		<section class="stage" aria-label="Coming">
			<i class="node" class:on={next.length > 0} style:--c="var(--info)" aria-hidden="true"></i>
			<h4 class="sh">Coming <span class="cnt">{next.length}</span></h4>
			{#each next as r (r.key)}
				{@const due = lj.due(r)}
				<div class="pl" style:--c={tone[r.tone] ?? 'var(--info)'}>
					<div class="hd"><i class="d" aria-hidden="true"></i><span class="nm">{r.name}</span><span class="el" class:due={due <= 0}>{due <= 0 ? 'due' : clock(due)}</span></div>
					<div class="fl"><i style:width="{lj.fill(r) * 100}%"></i></div>
				</div>
			{:else}
				<div class="slot">none</div>
			{/each}
		</section>
	</div>
</div>

<style>
	.pipe { display: flex; flex-direction: column; min-width: 0; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	.stages { position: relative; display: grid; grid-template-columns: minmax(0, 1.35fr) minmax(0, 0.75fr) minmax(0, 1.2fr); gap: 10px; min-width: 0; }
	/* The connector: one hairline through the stage nodes. */
	.stages::before { content: ''; position: absolute; left: 4px; right: 4px; top: 4px; height: 1px; background: var(--line-strong); }
	.stages::after { content: ''; position: absolute; left: 4px; top: 3px; width: 60px; height: 3px; border-radius: 2px; opacity: 0; background: linear-gradient(90deg, transparent, var(--accent, var(--text)), transparent); }

	.stage { position: relative; display: flex; flex-direction: column; gap: 5px; min-width: 0; }
	.node { position: relative; z-index: 1; display: block; width: 9px; height: 9px; margin-bottom: 4px; border-radius: 50%; box-sizing: border-box; border: 1.5px solid var(--line-strong); background: var(--surface-2, var(--surface-1)); }
	.node.on { border-color: transparent; background: var(--c, var(--info)); }
	.node.live.on { background: var(--accent, var(--text)); }

	.sh { display: flex; align-items: center; gap: 6px; margin: 0 0 1px; font: 600 12.5px/1.2 var(--font-ui); color: var(--text-muted); }
	.cnt { padding: 1px 7px; border-radius: var(--r-pill); background: color-mix(in srgb, var(--text) 8%, transparent); font-size: 11px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; }

	.pl { position: relative; min-width: 0; padding: 6px 9px; border-radius: 10px; border: 1px solid var(--line); background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 60%, transparent); overflow: hidden; }
	.pl.run { border-color: color-mix(in srgb, var(--accent, var(--text)) 35%, transparent); background: color-mix(in srgb, var(--accent, var(--text)) 8%, transparent); }
	.hd { display: flex; align-items: center; gap: 7px; min-width: 0; min-height: 18px; }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 12.5px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.el { flex: none; font-size: 11.5px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.el.on { color: var(--text); font-weight: 600; }
	.el.due { color: var(--accent, var(--text)); font-weight: 600; }
	.d { flex: none; width: 6px; height: 6px; border-radius: 50%; background: var(--c, var(--info)); }

	.mb { height: 3px; margin: 5px 0 0; border-radius: 2px; overflow: hidden; background: color-mix(in srgb, var(--text) 10%, transparent); }
	.mb i { display: block; height: 100%; border-radius: inherit; background: var(--accent, var(--text)); transition: width 0.8s linear; }
	.nt { margin: 4px 0 0; font-size: 11.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fl { position: absolute; left: 0; right: 0; bottom: 0; height: 2px; }
	.fl i { display: block; height: 100%; background: color-mix(in srgb, var(--c, var(--text)) 55%, transparent); transition: width 0.8s linear; }

	.slot { display: flex; align-items: center; justify-content: center; height: 30px; border-radius: 10px; border: 1px dashed var(--line-strong); font-size: 12px; color: var(--text-faint); }
	.more { margin: 0; font-size: 11.5px; color: var(--text-muted); }

	@media (max-width: 600px) {
		.stages { grid-template-columns: minmax(0, 1fr); gap: 12px; }
		.stages::before { left: 4px; right: auto; top: 6px; bottom: 6px; width: 1px; height: auto; }
		.stages::after { display: none; }
		.stage { padding-left: 20px; }
		.node { position: absolute; left: 0; top: 2px; margin: 0; }
	}

	@media (prefers-reduced-motion: no-preference) {
		.node.live.on { animation: pulse 2.4s ease-out infinite; }
		.stages.busy::after { animation: flow 2.6s linear infinite; }
		.mb.indet i { width: 40% !important; animation: slide 1.6s ease-in-out infinite; }
		@keyframes pulse { 0% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70%, 100% { box-shadow: 0 0 0 8px transparent; } }
		@keyframes flow { 0% { left: 0; opacity: 0; } 15% { opacity: 0.9; } 85% { opacity: 0.9; } 100% { left: calc(100% - 60px); opacity: 0; } }
		@keyframes slide { 0% { transform: translateX(-100%); } 100% { transform: translateX(250%); } }
	}
	@media (prefers-reduced-motion: reduce) {
		.mb i, .fl i { transition: none; }
		.mb.indet i { width: 100% !important; opacity: 0.45; }
	}
</style>
