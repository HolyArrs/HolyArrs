<script lang="ts">
	// View "Radar": a round scope where distance from the centre is time. Rings
	// mark 5 minutes, 30 minutes, 2 hours and 12 hours ahead, spaced on a log
	// scale so near runs do not pile up in the middle. Each coming run is a
	// blip at its time until, turned to the hour it starts on a 12-hour face,
	// so the next runs sit clockwise of the dashed "now" bearing and runs from
	// the last 12 hours sit faintly behind it. A slow sweep lights each blip as
	// it passes. A running job fills the centre, its ring growing with progress.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { wait, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const C = 120;
	const R0 = 24;
	const R1 = 106;
	const MIN = 60_000;
	const H12 = 12 * 3600_000;
	const SWEEP = 6000;
	const LN = Math.log(1 + 720);
	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };

	/** rad is how far from the centre a span of time sits: log of minutes, 12 hours at the edge. */
	const rad = (ms: number) => R0 + (R1 - R0) * Math.min(1, Math.log(1 + Math.max(0, ms) / MIN) / LN);
	const rings = [
		{ m: 5, label: '5 min' },
		{ m: 30, label: '30 min' },
		{ m: 120, label: '2 h' },
		{ m: 720, label: '12 h' }
	].map((x) => ({ ...x, r: rad(x.m * MIN) }));

	/** deg is a moment's place on a 12-hour face, in degrees clockwise from the top. */
	const deg = (ms: number) => {
		const d = new Date(ms);
		return ((d.getHours() % 12) + d.getMinutes() / 60 + d.getSeconds() / 3600) * 30;
	};
	const pt = (a: number, r: number) => {
		const k = (a * Math.PI) / 180;
		return { x: C + r * Math.sin(k), y: C - r * Math.cos(k) };
	};
	const sector = (a0: number, a1: number, r: number) => {
		const p0 = pt(a0, r), p1 = pt(a1, r);
		return `M ${C} ${C} L ${p0.x.toFixed(2)} ${p0.y.toFixed(2)} A ${r} ${r} 0 0 1 ${p1.x.toFixed(2)} ${p1.y.toFixed(2)} Z`;
	};
	// The sweep: a bright leading edge with a fading tail behind it.
	const tail = [
		{ d: sector(-44, -30, R1), o: 0.03 },
		{ d: sector(-30, -18, R1), o: 0.06 },
		{ d: sector(-18, -8, R1), o: 0.1 },
		{ d: sector(-8, 0, R1), o: 0.16 }
	];
	const lead = pt(0, R1);

	const pad = (n: number) => String(n).padStart(2, '0');
	const hm = (ms: number) => {
		const d = new Date(ms);
		return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
	};

	interface Blip {
		key: string;
		name: string;
		ms: number;
		a: number;
		x: number;
		y: number;
		c: string;
		near: boolean;
		tip: string;
	}

	/** place turns each blip a little away from blips already set, never far from its real time. */
	function place(list: { key: string; name: string; ms: number; a: number; r: number; c: string; tip: string }[], taken: { x: number; y: number }[], gap: number): Blip[] {
		const out: Blip[] = [];
		list.forEach((b, i) => {
			let a = b.a;
			let p = pt(a, b.r);
			for (let n = 0; n < 3 && taken.some((q) => Math.hypot(q.x - p.x, q.y - p.y) < gap); n++) {
				a += 4;
				p = pt(a, b.r);
			}
			taken.push(p);
			out.push({ key: b.key, name: b.name, ms: b.ms, a: ((a % 360) + 360) % 360, x: p.x, y: p.y, c: b.c, near: i === 0, tip: b.tip });
		});
		return out;
	}

	const now = $derived(lj.tick);
	const bp = $derived(pt(deg(now), R1 + 4));
	const next = $derived(lj.coming(upcoming, 40).filter((r) => Number.isFinite(t(r.nextRun)) && t(r.nextRun) - now <= H12));

	const blips = $derived.by(() => {
		const taken: { x: number; y: number }[] = [];
		const ahead = place(
			next.map((r) => {
				const ms = t(r.nextRun);
				const due = ms - now;
				return {
					key: r.key,
					name: r.name,
					ms,
					a: deg(Math.max(ms, now)),
					r: due <= 0 ? R0 + 4 : rad(due),
					c: tone[r.tone] ?? 'var(--info)',
					tip: due <= 0 ? `${r.name}, due now` : `${r.name} at ${hm(ms)}, in ${wait(due)}`
				};
			}),
			taken,
			10
		);
		const behind = place(
			recent
				.map((r) => ({ r, ms: t(r.lastRun) }))
				.filter((x) => Number.isFinite(x.ms) && x.ms <= now && now - x.ms <= H12)
				.map((x) => ({ key: `p-${x.r.key}`, name: x.r.name, ms: x.ms, a: deg(x.ms), r: rad(now - x.ms), c: 'var(--text-faint)', tip: `${x.r.name}, ran at ${hm(x.ms)}` })),
			taken,
			7
		);
		return { ahead, behind };
	});

	const run = $derived(lj.running[0]);
	const pct = $derived(run ? Math.round(run.progress * 100) : 0);
	const RING = 2 * Math.PI * (R0 - 4);

	const caption = $derived.by(() => {
		if (run) {
			const more = lj.running.length > 1 ? ` · ${lj.running.length - 1} more running` : '';
			return `${run.label} · ${pct > 0 ? `${pct}%` : 'working'}${more}`;
		}
		const first = next[0];
		if (!first) return 'Nothing planned in the next 12 hours';
		const due = lj.due(first);
		return due <= 0 ? `${first.name} is due now` : `${first.name} in ${wait(due)}`;
	});

	// Reduced motion turns the sweep and the flashes off, also when the setting changes while the page is open.
	let still = $state(false);
	$effect(() => {
		if (typeof matchMedia !== 'function') return;
		const q = matchMedia('(prefers-reduced-motion: reduce)');
		still = q.matches;
		const on = () => (still = q.matches);
		q.addEventListener('change', on);
		return () => q.removeEventListener('change', on);
	});
	// Both the sweep and each blip's flash run on the page clock from its start,
	// so a blip lights exactly when the sweep's edge reaches its angle, even for
	// blips that appear later.
	function spin(el: SVGGElement) {
		if (typeof el.animate !== 'function') return;
		const anim = el.animate([{ transform: 'rotate(0deg)' }, { transform: 'rotate(360deg)' }], { duration: SWEEP, iterations: Infinity });
		anim.startTime = 0;
		return { destroy: () => anim.cancel() };
	}
	function flash(el: SVGCircleElement, a: number) {
		if (typeof el.animate !== 'function') return;
		let anim: Animation | undefined;
		const go = (deg: number) => {
			anim?.cancel();
			anim = el.animate(
				[
					{ opacity: 0.85, transform: 'scale(1)' },
					{ opacity: 0, transform: 'scale(2.4)', offset: 0.22 },
					{ opacity: 0, transform: 'scale(2.4)' }
				],
				{ duration: SWEEP, iterations: Infinity, delay: (deg / 360) * SWEEP, easing: 'ease-out' }
			);
			anim.startTime = 0;
		};
		go(a);
		return { update: (deg: number) => go(deg), destroy: () => anim?.cancel() };
	}
</script>

<div class="radar">
	<svg class="scope" viewBox="0 0 240 240" role="img" aria-label="Radar of job runs: distance from the centre is time until the run, out to 12 hours. {caption}.">
		<circle class="plate" cx={C} cy={C} r={R1 + 8} />
		{#each rings as g (g.m)}
			<circle class="ring" cx={C} cy={C} r={g.r} />
			<text class="rl" x={C + 3} y={C - g.r + 1} dy="1em">{g.label}</text>
		{/each}
		<line class="cross" x1={C - R1} y1={C} x2={C + R1} y2={C} />
		<line class="cross" x1={C} y1={C - R1} x2={C} y2={C + R1} />

		<line class="bearing" x1={C} y1={C} x2={bp.x} y2={bp.y}><title>Now, {hm(now)}</title></line>

		{#if !still}
			<g class="sweep" use:spin>
				{#each tail as s (s.d)}<path class="wedge" d={s.d} style:fill-opacity={s.o} />{/each}
				<line class="edge" x1={C} y1={C} x2={lead.x} y2={lead.y} />
			</g>
		{/if}

		{#each blips.behind as p (p.key)}
			<circle class="past" cx={p.x} cy={p.y} r="2.4"><title>{p.tip}</title></circle>
		{/each}

		{#each blips.ahead as p (p.key)}
			<g style:--c={p.c}>
				{#if !still}<circle class="glow" cx={p.x} cy={p.y} r={p.near ? 5.2 : 3.8} use:flash={p.a} />{/if}
				{#if p.near}<circle class="halo" cx={p.x} cy={p.y} r="9" />{/if}
				<circle class="blip" cx={p.x} cy={p.y} r={p.near ? 5.2 : 3.8}><title>{p.tip}</title></circle>
			</g>
		{/each}

		{#if run}
			<circle class="core" class:indet={pct <= 0} cx={C} cy={C} r={R0 - 7}><title>{run.label}, {pct > 0 ? `${pct}% done` : 'working'}</title></circle>
			<circle class="track" cx={C} cy={C} r={R0 - 4} />
			{#if pct > 0}
				<circle class="prog" cx={C} cy={C} r={R0 - 4} stroke-dasharray="{(RING * pct) / 100} {RING}" transform="rotate(-90 {C} {C})" />
				<text class="pct" x={C} y={C + 3.2}>{pct}%</text>
			{/if}
		{:else}
			<circle class="hub" cx={C} cy={C} r="3" />
		{/if}
	</svg>
	<p class="cap" class:on={!!run}>{caption}</p>
</div>

<style>
	.radar { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; min-width: 0; height: 100%; }
	.scope { display: block; width: 100%; max-width: 236px; height: auto; }

	.plate { fill: color-mix(in srgb, var(--surface-2, var(--surface-1)) 70%, transparent); stroke: var(--line); stroke-width: 1; }
	.ring { fill: none; stroke: var(--line-strong); stroke-width: 0.8; }
	.rl { font: 500 10px var(--font-mono); fill: var(--text-muted); font-variant-numeric: tabular-nums; }
	.cross { stroke: var(--line); stroke-width: 0.7; }
	.bearing { stroke: var(--text-muted); stroke-width: 0.9; stroke-dasharray: 2 3; opacity: 0.7; }

	.sweep { transform-box: view-box; transform-origin: 120px 120px; }
	.wedge { fill: var(--accent, var(--text)); }
	.edge { stroke: color-mix(in srgb, var(--accent, var(--text)) 55%, transparent); stroke-width: 1; }

	.past { fill: var(--text-faint); opacity: 0.55; }
	.blip { fill: var(--c); stroke: var(--surface-1); stroke-width: 0.8; }
	.halo { fill: color-mix(in srgb, var(--c) 20%, transparent); }
	.glow { fill: var(--c); opacity: 0; transform-box: fill-box; transform-origin: center; pointer-events: none; }

	.core { fill: var(--accent, var(--text)); transform-box: fill-box; transform-origin: center; }
	.track { fill: none; stroke: color-mix(in srgb, var(--accent, var(--text)) 22%, transparent); stroke-width: 3; }
	.prog { fill: none; stroke: var(--accent, var(--text)); stroke-width: 3; stroke-linecap: round; transition: stroke-dasharray 0.8s linear; }
	.pct { font: 700 10px var(--font-ui); fill: var(--surface-1); text-anchor: middle; font-variant-numeric: tabular-nums; }
	.hub { fill: var(--text-muted); }

	.cap { margin: 0; max-width: 100%; font: 500 12.5px/1.3 var(--font-ui); color: var(--text-muted); font-variant-numeric: tabular-nums; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cap.on { color: var(--text); }

	@media (prefers-reduced-motion: reduce) {
		.sweep { display: none; }
		.prog { transition: none; }
	}
	@media (prefers-reduced-motion: no-preference) {
		.core.indet { animation: breathe 2.4s ease-in-out infinite; }
		@keyframes breathe { 0%, 100% { transform: scale(0.82); opacity: 0.75; } 50% { transform: scale(1); opacity: 1; } }
	}
</style>
