<script lang="ts">
	// View "Pulse": the last hour drawn like a heart monitor. A flat line runs
	// left to right with one beat for every run that finished, placed at the
	// minute it ended: a clean spike when it went fine, a taller one in the
	// fail colour when it failed, a small bump when it was cancelled. The pen
	// sits at the right edge, which is now, and the line slides left as the
	// minutes pass. While a job runs, the line lifts into a plateau that grows
	// with its progress. One line underneath puts the rate into words.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { wait, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();
	const uid = $props.id();

	const X0 = 8;
	const X1 = 384;
	const B = 80;
	const MIN = 60_000;
	const HOUR = 60 * MIN;
	const PX = (X1 - X0) / HOUR;
	const grid = [20, 40, 60, 80, 100];
	const ticks = Array.from({ length: 13 }, (_, i) => ({ i, x: X0 + (i * (X1 - X0)) / 12, label: i === 0 ? '−60 min' : i === 6 ? '−30 min' : i === 12 ? 'now' : '' }));

	type Kind = 'ok' | 'fail' | 'cut';
	// Each beat's shape, as offsets from where it sits on the line.
	const shape: Record<Kind, [number, number][]> = {
		ok: [[-7, 0], [-4, 4], [-1.5, -36], [1.5, 8], [4.5, 0]],
		fail: [[-7, 0], [-4, 5], [-1.5, -56], [1.5, 12], [5, 0]],
		cut: [[-6, 0], [-3, -6], [0, -9], [3, -6], [6, 0]]
	};
	const kindOf = (state: string): Kind => (state === 'failed' ? 'fail' : state === 'done' ? 'ok' : 'cut');
	const word: Record<Kind, string> = { ok: 'OK', fail: 'failed', cut: 'cancelled' };

	const pad = (n: number) => String(n).padStart(2, '0');
	const hm = (ms: number) => {
		const d = new Date(ms);
		return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
	};
	const f = (n: number) => n.toFixed(2);
	const pts = (p: [number, number][]) => p.map(([x, y]) => `${f(x)},${f(y)}`).join(' ');

	// Under reduced motion the line moves once a minute instead of every second.
	let still = $state(false);
	$effect(() => {
		if (typeof matchMedia !== 'function') return;
		const q = matchMedia('(prefers-reduced-motion: reduce)');
		still = q.matches;
		const on = () => (still = q.matches);
		q.addEventListener('change', on);
		return () => q.removeEventListener('change', on);
	});
	const now = $derived(still ? Math.floor(lj.tick / MIN) * MIN : lj.tick);

	const run = $derived(lj.running[0]);
	const pct = $derived(run ? Math.round(run.progress * 100) : 0);
	const P = B - 22;
	// Where the plateau starts: its length grows with progress, or with time run so far when the job gives no progress.
	const lift = $derived.by(() => {
		if (!run) return X1;
		const len = run.progress > 0 ? 8 + Math.min(1, run.progress) * 72 : Math.min(80, Math.max(6, (lj.tick - t(run.started_at ?? run.queued_at)) * PX));
		return X1 - len;
	});

	const beats = $derived.by(() => {
		const out: { key: string; x: number; kind: Kind; tip: string }[] = [];
		const done = lj.finished
			.map((j) => ({ j, ms: t(j.EndedAt) }))
			.filter((b) => Number.isFinite(b.ms) && b.ms <= lj.tick && lj.tick - b.ms <= HOUR)
			.sort((a, b) => a.ms - b.ms);
		let last = -1e9;
		const edge = run ? lift - 14 : X1 - 7;
		for (const { j, ms } of done) {
			const kind = kindOf(j.State);
			// Beats ending in the same minute stand side by side instead of on top of each other.
			const x = Math.min(edge, Math.max(X1 - (now - ms) * PX, last + 10));
			last = x;
			out.push({ key: `b${j.ID}`, x, kind, tip: `${lj.nameOf(j.Kind)}, ${word[kind]} at ${hm(ms)}` });
		}
		return out;
	});

	const trace = $derived.by(() => {
		const p: [number, number][] = [[X0 - 12, B]];
		for (const b of beats) for (const [dx, dy] of shape[b.kind]) p.push([b.x + dx, B + dy]);
		if (run) {
			p.push([lift, B], [lift + 6, P], [X1, P]);
		} else {
			p.push([X1, B]);
		}
		return pts(p);
	});
	const penY = $derived(run ? P : B);

	const hourCount = $derived(lj.finished.filter((j) => lj.tick - t(j.EndedAt) <= HOUR).length);
	const failCount = $derived(lj.finished.filter((j) => j.State === 'failed' && lj.tick - t(j.EndedAt) <= HOUR).length);
	const first = $derived(lj.coming(upcoming, 1)[0]);
	const rate = $derived.by(() => {
		const runs = `${hourCount} ${hourCount === 1 ? 'run' : 'runs'} in the last hour`;
		const failed = failCount ? ` · ${failCount} failed` : '';
		const due = first ? lj.due(first) : NaN;
		const next = first ? (due <= 0 ? `${first.name} is due` : `next in ${wait(due)}`) : 'nothing planned';
		if (run) return `${run.label} running · ${pct > 0 ? `${pct}%` : 'working'} · ${runs}${failed}`;
		return `${hourCount ? runs : 'Quiet'}${failed} · ${next}`;
	});
</script>

<div class="pulse">
	<svg class="mon" viewBox="0 0 400 124" role="img" aria-label="Heart monitor of the last hour: one beat per finished run. {rate}.">
		<defs>
			<linearGradient id="{uid}-fade" x1="0" x2="1" y1="0" y2="0">
				<stop offset="0" stop-color="currentColor" stop-opacity="0" />
				<stop offset="0.12" stop-color="currentColor" stop-opacity="1" />
			</linearGradient>
			<mask id="{uid}-m" maskUnits="userSpaceOnUse" x="0" y="0" width="400" height="124" style="mask-type: alpha">
				<rect x="0" y="0" width="400" height="124" fill="url(#{uid}-fade)" />
			</mask>
		</defs>

		{#each grid as y (y)}<line class="gr" class:base={y === B} x1={X0} y1={y} x2={X1} y2={y} />{/each}

		<g mask="url(#{uid}-m)">
			<polyline class="trace" points={trace} />
			{#each beats as b (b.key)}
				<polyline class="beat {b.kind}" points={pts(shape[b.kind].map(([dx, dy]) => [b.x + dx, B + dy]))}><title>{b.tip}</title></polyline>
			{/each}
			{#if run}
				<polyline class="lift" points={pts([[lift, B], [lift + 6, P], [X1, P]])}><title>{run.label}, {pct > 0 ? `${pct}% done` : 'working'}</title></polyline>
			{/if}
		</g>

		{#if run}<circle class="glow" cx={X1} cy={penY} r="11" />{/if}
		<circle class="pen" class:on={!!run} cx={X1} cy={penY} r="3.4"><title>Now, {hm(lj.tick)}</title></circle>
		{#if run && pct > 0}<text class="pct" x={X1 - 4} y={P - 8}>{pct}%</text>{/if}

		{#each ticks as k (k.i)}
			<line class="tk" class:big={!!k.label} x1={k.x} y1="107" x2={k.x} y2={k.label ? 112 : 110} />
			{#if k.label}<text class="tl" x={k.x} y="122" text-anchor={k.i === 0 ? 'start' : k.i === 12 ? 'end' : 'middle'}>{k.label}</text>{/if}
		{/each}
	</svg>
	<p class="rate" class:on={!!run}>{rate}</p>
</div>

<style>
	.pulse { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; min-width: 0; height: 100%; color: var(--text); container-type: inline-size; }
	.mon { display: block; width: 100%; max-width: 640px; height: auto; overflow: visible; }

	.gr { stroke: color-mix(in srgb, var(--text-faint) 25%, transparent); stroke-width: 0.7; }
	.gr.base { stroke: color-mix(in srgb, var(--text-faint) 40%, transparent); }
	.trace { fill: none; stroke: var(--text-muted); stroke-width: 1.4; stroke-linejoin: round; stroke-linecap: round; opacity: 0.7; }
	.beat { fill: none; stroke-width: 1.9; stroke-linejoin: round; stroke-linecap: round; }
	.beat.ok { stroke: var(--ok); }
	.beat.fail { stroke: var(--fail); stroke-width: 2.1; }
	.beat.cut { stroke: var(--text-faint); }
	.lift { fill: none; stroke: var(--accent, var(--text)); stroke-width: 2.2; stroke-linejoin: round; stroke-linecap: round; }

	.pen { fill: var(--text-muted); stroke: var(--surface-1); stroke-width: 1; }
	.pen.on { fill: var(--accent, var(--text)); }
	.glow { fill: color-mix(in srgb, var(--accent, var(--text)) 22%, transparent); transform-box: fill-box; transform-origin: center; }
	.pct { font: 700 10px var(--font-ui); fill: var(--text); text-anchor: end; font-variant-numeric: tabular-nums; }

	.tk { stroke: var(--text-faint); stroke-width: 0.8; opacity: 0.6; }
	.tk.big { opacity: 1; }
	.tl { font: 500 9.5px var(--font-mono); fill: var(--text-muted); font-variant-numeric: tabular-nums; }
	/* On a narrow plate the drawing shrinks, so its words grow to stay readable. */
	@container (max-width: 480px) {
		.tl { font-size: 14px; }
		.pct { font-size: 13px; }
	}

	.rate { margin: 0; max-width: 100%; font: 500 12.5px/1.3 var(--font-ui); color: var(--text-muted); font-variant-numeric: tabular-nums; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.rate.on { color: var(--text); }

	@media (prefers-reduced-motion: no-preference) {
		.glow { animation: glow 2.2s ease-in-out infinite; }
		@keyframes glow { 0%, 100% { opacity: 0.5; transform: scale(0.8); } 50% { opacity: 1; transform: scale(1.15); } }
	}
</style>
