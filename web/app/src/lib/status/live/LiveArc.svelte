<script lang="ts">
	// View "Arc": the day as a sun path. Now sits at the top of a wide arc;
	// runs that already happened come down the left side and runs still to come
	// go down the right, each at a height set by how far it is from now on a
	// log scale (the last and next few minutes near the top, a day away near
	// the horizon). The left half is solid because it happened, the right half
	// dashed because it is planned. While a job runs, an accent stretch grows
	// across the top with its progress and the percent sits above it.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const CX = 180;
	const CY = 150;
	const RX = 152;
	const RY = 108;
	const TMAX = 84;
	const MIN = 60_000;
	const DAY = 24 * 3600_000;
	const LN = Math.log(1 + 1440 / 4);
	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };

	/** th is how far down the arc a span of time sits, in degrees from the top. */
	const th = (ms: number) => Math.max(2.5, (TMAX * Math.log(1 + Math.max(0, ms) / MIN / 4)) / LN);
	/** pos is a point on the arc; side -1 is the past (left), 1 the future (right). */
	const pos = (side: number, deg: number) => {
		const k = (deg * Math.PI) / 180;
		return { x: CX + side * RX * Math.sin(k), y: CY - RY * Math.cos(k) };
	};
	const f = (n: number) => n.toFixed(2);
	const L = pos(-1, 90), TOP = pos(1, 0), R = pos(1, 90);
	const pastPath = `M ${f(L.x)} ${f(L.y)} A ${RX} ${RY} 0 0 1 ${f(TOP.x)} ${f(TOP.y)}`;
	const nextPath = `M ${f(TOP.x)} ${f(TOP.y)} A ${RX} ${RY} 0 0 1 ${f(R.x)} ${f(R.y)}`;
	// Faint marks across the arc: 5 min, 30 min, 2 h, 12 h on both sides.
	const inner = (side: number, deg: number) => {
		const k = (deg * Math.PI) / 180;
		return { x: CX + side * (RX - 5) * Math.sin(k), y: CY - (RY - 5) * Math.cos(k) };
	};
	const marks = [5, 30, 120, 720].flatMap((m) =>
		[-1, 1].map((s) => {
			const d = th(m * MIN);
			return { key: `${s}${m}`, o: pos(s, d), i: inner(s, d) };
		})
	);

	const short = (s: string) => (s.length > 14 ? `${s.slice(0, 13).trimEnd()}…` : s);
	const pad = (n: number) => String(n).padStart(2, '0');
	const hm = (ms: number) => {
		const d = new Date(ms);
		return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
	};
	/** ago says how long back a run was, short: "just now", "12 min ago", "4 h ago". */
	const ago = (ms: number) => {
		if (ms < MIN) return 'just now';
		if (ms < 3600_000) return `${Math.floor(ms / MIN)} min ago`;
		return `${Math.floor(ms / 3600_000)} h ago`;
	};

	interface Dot {
		key: string;
		side: number;
		deg: number;
		x: number;
		y: number;
		c: string;
		name: string;
		when: string;
		tip: string;
		big: boolean;
		label?: { x: number; y: number; rot: number; anchor: 'start' | 'end' };
	}

	/** labels gives a tilted name to dots with room, at most four a side, nearest first. */
	function labels(dots: Dot[]): Dot[] {
		let last = -99;
		let n = 0;
		for (const d of dots) {
			if (d.deg - last < 11 || n >= 4) continue;
			last = d.deg;
			n++;
			// Just inside the arc, tilted with it (but never steeper than 30°).
			const vx = CX - d.x, vy = CY - d.y, len = Math.hypot(vx, vy) || 1;
			const k = (d.deg * Math.PI) / 180;
			const tilt = Math.min(30, (Math.atan2(RY * Math.sin(k), RX * Math.cos(k)) * 180) / Math.PI) * d.side;
			// Near the top a label runs away from the middle and down; lower down it runs inward and up.
			const out = d.deg < 45;
			const anchor = (d.side > 0) === out ? 'start' : 'end';
			d.label = { x: d.x + (vx / len) * 11, y: d.y + (vy / len) * 11, rot: tilt, anchor };
		}
		return dots;
	}

	const now = $derived(lj.tick);

	const past = $derived.by(() => {
		const list = recent
			.map((r) => ({ r, ms: t(r.lastRun) }))
			.filter((x) => Number.isFinite(x.ms) && x.ms <= now && now - x.ms <= DAY)
			.sort((a, b) => b.ms - a.ms)
			.slice(0, 6)
			.map((x): Dot => {
				const deg = th(now - x.ms);
				const p = pos(-1, deg);
				return { key: `p-${x.r.key}`, side: -1, deg, ...p, c: tone[x.r.tone] ?? 'var(--info)', name: short(x.r.name), when: ago(now - x.ms), tip: `${x.r.name}, ran at ${hm(x.ms)}`, big: false };
			});
		return labels(list);
	});

	const next = $derived.by(() => {
		const list = lj
			.coming(upcoming, 6)
			.map((r) => ({ r, ms: t(r.nextRun) }))
			.filter((x) => Number.isFinite(x.ms) && x.ms - now <= DAY)
			.map((x, i): Dot => {
				const due = x.ms - now;
				const deg = th(due);
				const p = pos(1, deg);
				return {
					key: `n-${x.r.key}`,
					side: 1,
					deg,
					...p,
					c: tone[x.r.tone] ?? 'var(--info)',
					name: short(x.r.name),
					when: due <= 0 ? 'due' : `in ${clock(due)}`,
					tip: due <= 0 ? `${x.r.name}, due now` : `${x.r.name} at ${hm(x.ms)}, in ${clock(due)}`,
					big: i === 0
				};
			});
		return labels(list);
	});

	const run = $derived(lj.running[0]);
	const pct = $derived(run ? Math.round(run.progress * 100) : 0);
	// The accent stretch over the top: wider as the running job gets further.
	const stretch = $derived.by(() => {
		if (!run) return '';
		const half = Math.max(3, 16 * Math.min(1, Math.max(0, run.progress)));
		const a = pos(-1, half), b = pos(1, half);
		return `M ${f(a.x)} ${f(a.y)} A ${RX} ${RY} 0 0 1 ${f(b.x)} ${f(b.y)}`;
	});
	const aria = $derived(
		[run ? `${run.label} running${pct > 0 ? `, ${pct}%` : ''}` : 'Nothing running', next[0] ? `next ${next[0].tip}` : 'nothing planned', past[0] ? `last ${past[0].tip}` : ''].filter(Boolean).join('; ')
	);
</script>

<div class="arc">
	<svg class="sky" viewBox="0 0 360 172" role="img" aria-label="Sun path of job runs: past on the left, coming on the right, now at the top. {aria}.">
		<line class="hz" x1="8" y1={CY + 0.5} x2="352" y2={CY + 0.5} />
		<text class="hzl" x="8" y="166">earlier</text>
		<text class="hzl" x="352" y="166" text-anchor="end">later</text>

		<path class="path past" d={pastPath} />
		<path class="path next" d={nextPath} />
		{#each marks as m (m.key)}
			<line class="mk" x1={m.o.x} y1={m.o.y} x2={m.i.x} y2={m.i.y} />
		{/each}

		{#if stretch}<path class="run" d={stretch} />{/if}

		{#each [...past, ...next] as d (d.key)}
			<g style:--c={d.c}>
				{#if d.big}<circle class="halo" cx={d.x} cy={d.y} r="8.5" />{/if}
				<circle class="dot" class:old={d.side < 0} cx={d.x} cy={d.y} r={d.big ? 4.8 : 3.6}><title>{d.tip}</title></circle>
				{#if d.label}
					<g transform="translate({f(d.label.x)} {f(d.label.y)}) rotate({f(d.label.rot)})" text-anchor={d.label.anchor}>
						<text class="nm" x="0" y="0">{d.name}</text>
						<text class="when" x="0" y="9">{d.when}</text>
					</g>
				{/if}
			</g>
		{/each}

		<circle class="ping" cx={TOP.x} cy={TOP.y} r="4.5" />
		<circle class="now" cx={TOP.x} cy={TOP.y} r="4.5"><title>Now, {hm(now)}</title></circle>
		{#if run}
			<text class="who" x={TOP.x} y="13">{short(run.label)}{lj.running.length > 1 ? ` +${lj.running.length - 1}` : ''}</text>
			<text class="pct" x={TOP.x} y="29">{pct > 0 ? `${pct}%` : 'working'}</text>
		{:else}
			<text class="clk" x={TOP.x} y="29">now · {hm(now)}</text>
		{/if}
	</svg>
</div>

<style>
	.arc { display: flex; align-items: center; justify-content: center; min-width: 0; height: 100%; }
	.sky { display: block; width: 100%; max-width: 520px; height: auto; overflow: visible; }

	.hz { stroke: var(--line-strong); stroke-width: 0.8; }
	.hzl { font: 500 8.5px var(--font-ui); fill: var(--text-faint); }
	.path { fill: none; stroke-width: 1.4; stroke-linecap: round; }
	.path.past { stroke: var(--line-strong); }
	.path.next { stroke: var(--text-faint); stroke-dasharray: 2.5 3.5; opacity: 0.8; }
	.mk { stroke: var(--text-faint); stroke-width: 0.8; opacity: 0.6; }
	.run { fill: none; stroke: var(--accent, var(--text)); stroke-width: 4; stroke-linecap: round; transition: d 0.8s linear; }

	.dot { fill: var(--c); stroke: var(--surface-1); stroke-width: 1; }
	.dot.old { opacity: 0.6; }
	.halo { fill: color-mix(in srgb, var(--c) 20%, transparent); }
	.nm { font: 500 8.5px var(--font-ui); fill: var(--text); }
	.when { font: 500 7.5px var(--font-mono); fill: var(--text-muted); font-variant-numeric: tabular-nums; }

	.now { fill: var(--accent, var(--text)); stroke: var(--surface-1); stroke-width: 1.2; }
	.ping { fill: color-mix(in srgb, var(--accent, var(--text)) 45%, transparent); opacity: 0; transform-box: fill-box; transform-origin: center; }
	.who { font: 500 8.5px var(--font-ui); fill: var(--text-muted); text-anchor: middle; }
	.pct { font: 700 13px var(--font-ui); fill: var(--text); text-anchor: middle; font-variant-numeric: tabular-nums; }
	.clk { font: 500 9px var(--font-mono); fill: var(--text-muted); text-anchor: middle; font-variant-numeric: tabular-nums; }

	@media (prefers-reduced-motion: reduce) {
		.run { transition: none; }
	}
	@media (prefers-reduced-motion: no-preference) {
		.ping { animation: ping 2.6s ease-out infinite; }
		@keyframes ping { 0% { opacity: 0.9; transform: scale(1); } 70%, 100% { opacity: 0; transform: scale(3.4); } }
	}
</style>
