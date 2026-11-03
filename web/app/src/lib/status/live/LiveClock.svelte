<script lang="ts">
	// View "Clock": a 12-hour face showing the time now. Runs planned in the
	// next 12 hours sit as dots on the rim at the hour they start, runs from
	// the last 12 hours as fainter dots just inside it, and an accent arc
	// spans from now to the next run. The legend names the next runs.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const C = 100;
	const RIM = 88;
	const H12 = 12 * 3600_000;
	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };

	const pad = (n: number) => String(n).padStart(2, '0');
	const hm = (ms: number) => {
		const d = new Date(ms);
		return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
	};
	// at says when a run starts: the time of day, with the weekday once it is not today.
	const at = (ms: number, now: number) => {
		if (!Number.isFinite(ms)) return '';
		const d = new Date(ms);
		return d.toDateString() === new Date(now).toDateString() ? hm(ms) : `${d.toLocaleDateString(undefined, { weekday: 'short' })} ${hm(ms)}`;
	};
	/** deg is a moment's place on a 12-hour face, in degrees clockwise from 12. */
	const deg = (ms: number) => {
		const d = new Date(ms);
		return ((d.getHours() % 12) + d.getMinutes() / 60 + d.getSeconds() / 3600) * 30;
	};
	const pt = (a: number, r: number) => {
		const rad = (a * Math.PI) / 180;
		return { x: C + r * Math.sin(rad), y: C - r * Math.cos(rad) };
	};

	const now = $derived(lj.tick);
	const d = $derived(new Date(now));
	const hourDeg = $derived(((d.getHours() % 12) + d.getMinutes() / 60) * 30);
	const minDeg = $derived((d.getMinutes() + d.getSeconds() / 60) * 6);
	const ticks = Array.from({ length: 12 }, (_, i) => ({ a: i * 30, strong: i % 3 === 0 }));

	// Planned runs in the next 12 hours; late ones wait at "now" on the rim.
	const ahead = $derived(
		upcoming
			.map((r) => ({ r, ms: t(r.nextRun) }))
			.filter((x) => Number.isFinite(x.ms) && x.ms - now <= H12 && now - x.ms <= 10 * 60_000)
			.map((x) => ({ ...x, p: pt(deg(Math.max(x.ms, now)), RIM) }))
	);
	const past = $derived(
		recent
			.map((r) => ({ r, ms: t(r.lastRun) }))
			.filter((x) => Number.isFinite(x.ms) && x.ms <= now && now - x.ms <= H12)
			.map((x) => ({ ...x, p: pt(deg(x.ms), RIM - 19) }))
	);

	const next = $derived(lj.coming(upcoming, 3));
	// The accent arc from now to the first coming run, when that run is within 12 hours.
	const arc = $derived.by(() => {
		const first = next[0];
		if (!first) return '';
		const ms = t(first.nextRun);
		if (!Number.isFinite(ms) || ms <= now || ms - now > H12) return '';
		const a0 = deg(now);
		const sweep = (((deg(ms) - a0) % 360) + 360) % 360;
		if (sweep < 1.5) return '';
		const p0 = pt(a0, RIM), p1 = pt(a0 + sweep, RIM);
		return `M ${p0.x.toFixed(2)} ${p0.y.toFixed(2)} A ${RIM} ${RIM} 0 ${sweep > 180 ? 1 : 0} 1 ${p1.x.toFixed(2)} ${p1.y.toFixed(2)}`;
	});
	const busy = $derived(lj.running.length > 0);
</script>

<div class="clock">
	<h3>
		Next 12 hours
		<span class="st" class:on={busy}>{busy ? `${lj.running.length} running` : 'Idle'}</span>
	</h3>

	<div class="body">
		<svg class="face" viewBox="0 0 200 200" role="img" aria-label="Clock face with the runs of the last and next 12 hours">
			<circle class="plate" cx={C} cy={C} r="96" />
			<circle class="rim" cx={C} cy={C} r={RIM} />
			{#each ticks as k (k.a)}
				{@const o = pt(k.a, RIM - 3)}
				{@const i = pt(k.a, k.strong ? RIM - 13 : RIM - 9)}
				<line class="tk" class:strong={k.strong} x1={o.x} y1={o.y} x2={i.x} y2={i.y} />
			{/each}

			{#each past as x (x.r.key)}
				<circle class="past" cx={x.p.x} cy={x.p.y} r="2.6" style:--c={tone[x.r.tone] ?? 'var(--info)'}><title>{x.r.name}, ran at {hm(x.ms)}</title></circle>
			{/each}

			{#if arc}<path class="arc" d={arc} />{/if}

			{#each ahead as x (x.r.key)}
				<g style:--c={tone[x.r.tone] ?? 'var(--info)'}>
					<circle class="halo" cx={x.p.x} cy={x.p.y} r="7" />
					<circle class="up" cx={x.p.x} cy={x.p.y} r="4"><title>{x.r.name} at {hm(x.ms)}</title></circle>
				</g>
			{/each}

			<line class="hh" x1={C} y1={C + 9} x2={C} y2={C - 44} transform="rotate({hourDeg} {C} {C})" />
			<line class="mh" x1={C} y1={C + 12} x2={C} y2={C - 66} transform="rotate({minDeg} {C} {C})" />
			{#if busy}<circle class="ping" cx={C} cy={C} r="4.5" />{/if}
			<circle class="hub" cx={C} cy={C} r="4.5" />
		</svg>

		<div class="lg">
			<p class="now"><span>Now</span><b>{hm(now)}</b></p>
			{#each lj.running.slice(0, 2) as j (j.id)}
				{@const pct = Math.round(j.progress * 100)}
				<div class="it run"><i class="d live" aria-hidden="true"></i><span class="nm">{j.label}</span><span class="el on">{pct > 0 ? `${pct}%` : 'working'}</span></div>
			{/each}
			{#if lj.running.length > 2}<p class="more">and {lj.running.length - 2} more running</p>{/if}
			{#if next.length}
				<h4>Next up</h4>
				{#each next as r (r.key)}
					{@const ms = t(r.nextRun)}
					<div class="it"><i class="d" style:--c={tone[r.tone] ?? 'var(--info)'} aria-hidden="true"></i><span class="nm">{r.name}</span><span class="el" class:due={ms <= now}>{ms <= now ? 'due' : at(ms, now)}</span></div>
				{/each}
			{:else if !busy}
				<p class="none">Nothing planned.</p>
			{/if}
		</div>
	</div>
</div>

<style>
	.clock { display: flex; flex-direction: column; min-width: 0; container-type: inline-size; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	h4 { margin: 8px 0 4px; font: 600 12px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	.body { display: flex; align-items: center; gap: 18px; min-width: 0; }
	.face { flex: none; display: block; width: 200px; max-width: 100%; height: auto; overflow: visible; }

	.plate { fill: color-mix(in srgb, var(--surface-2, var(--surface-1)) 70%, transparent); stroke: var(--line); stroke-width: 1; }
	.rim { fill: none; stroke: var(--line-strong); stroke-width: 1; }
	.tk { stroke: var(--text-faint); stroke-width: 1.2; stroke-linecap: round; }
	.tk.strong { stroke: var(--text-muted); stroke-width: 2.2; }
	.past { fill: var(--c); opacity: 0.45; }
	.arc { fill: none; stroke: var(--accent, var(--text)); stroke-width: 3.5; stroke-linecap: round; opacity: 0.9; }
	.halo { fill: color-mix(in srgb, var(--c) 22%, transparent); }
	.up { fill: var(--c); }
	.hh, .mh { stroke: var(--text); stroke-linecap: round; }
	.hh { stroke-width: 3.6; }
	.mh { stroke-width: 2; opacity: 0.85; }
	.hub { fill: var(--accent, var(--text)); }
	.ping { fill: color-mix(in srgb, var(--accent, var(--text)) 40%, transparent); opacity: 0; transform-box: fill-box; transform-origin: center; }

	.lg { flex: 1 1 0; min-width: 0; display: flex; flex-direction: column; }
	.now { display: flex; align-items: baseline; gap: 8px; margin: 0 0 4px; font-size: 12px; color: var(--text-muted); }
	.now b { font-size: 20px; font-weight: 600; color: var(--text); font-variant-numeric: tabular-nums; letter-spacing: 0.01em; }
	.it { display: flex; align-items: center; gap: 8px; min-height: 24px; min-width: 0; }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 13px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.el { flex: none; font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.el.on { color: var(--text); font-weight: 600; }
	.el.due { color: var(--accent, var(--text)); font-weight: 600; }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--c, var(--info)); }
	.d.live { background: var(--accent, var(--text)); }
	.more, .none { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); }

	@container (max-width: 359px) {
		.body { flex-direction: column; align-items: stretch; gap: 12px; }
		.face { align-self: center; }
	}

	@media (prefers-reduced-motion: no-preference) {
		.ping { animation: ping 2.4s ease-out infinite; }
		.d.live { animation: pulse 2.4s ease-out infinite; }
		@keyframes ping { 0% { opacity: 0.9; transform: scale(1); } 70%, 100% { opacity: 0; transform: scale(3.2); } }
		@keyframes pulse { 0% { box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70%, 100% { box-shadow: 0 0 0 8px transparent; } }
	}
</style>
