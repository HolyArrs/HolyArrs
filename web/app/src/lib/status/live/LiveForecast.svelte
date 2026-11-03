<script lang="ts">
	// View "Forecast": the next six hours as an hourly strip, like a weather
	// forecast. Each hour shows a small glyph for how many runs fall in it
	// (an empty ring for none, a core with one to three dots, a full burst
	// for four or more), coloured by the worst state among them, with a load
	// line across the strip. A job's later runs are projected from the gap
	// between its last and next run. While a job runs, the current hour shows
	// an accent disc with a ring that fills by its progress.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	type Hour = { i: number; from: number; to: number; n: number; worst: string; names: string[] };

	const HOUR = 3600_000;
	const MIN_GAP = 10 * 60_000; // shorter gaps are not projected
	const rank: Record<string, number> = { fail: 4, wait: 3, ok: 2, info: 1, idle: 0 };
	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };
	const hm = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
	const hh = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', hour12: false });

	const now = $derived(lj.tick);
	const h0 = $derived.by(() => {
		const d = new Date(now);
		d.setMinutes(0, 0, 0);
		return d.getTime();
	});

	const hoursList = $derived.by((): Hour[] => {
		const out: Hour[] = Array.from({ length: 6 }, (_, i) => ({ i, from: h0 + i * HOUR, to: h0 + (i + 1) * HOUR, n: 0, worst: '', names: [] }));
		const stop = h0 + 6 * HOUR;
		const runs: { at: number; r: StatusRow }[] = [];
		for (const r of lj.coming(upcoming, 99)) {
			const ms = t(r.nextRun);
			if (!Number.isFinite(ms) || ms >= stop) continue;
			runs.push({ at: Math.max(ms, now), r });
			const gap = ms - t(r.lastRun);
			if (Number.isFinite(gap) && gap >= MIN_GAP) for (let k = 1, at = ms + gap; at < stop && k <= 36; k++, at += gap) runs.push({ at, r });
		}
		runs.sort((a, b) => a.at - b.at);
		for (const { at, r } of runs) {
			const h = out[Math.floor((at - h0) / HOUR)];
			if (!h) continue;
			h.n++;
			if (!h.worst || (rank[r.tone] ?? 1) > (rank[h.worst] ?? 1)) h.worst = r.tone;
			if (h.names.length < 2 && !h.names.includes(r.name)) h.names.push(r.name);
		}
		return out;
	});

	const job = $derived(lj.running[0]);
	const pct = $derived(job ? Math.round(Math.min(1, Math.max(0, job.progress)) * 100) : 0);
	const busy = $derived(lj.running.length > 0);
	const total = $derived(hoursList.reduce((s, h) => s + h.n, 0));

	// The load line: one point per hour, scaled to the busiest hour.
	const peak = $derived(Math.max(1, ...hoursList.map((h) => h.n)));
	const ly = (n: number) => 30 - (n / peak) * 24;
	const pts = $derived(hoursList.map((h) => ({ x: (h.i + 0.5) * 100, y: ly(h.n) })));
	const curve = $derived.by(() => {
		const p = pts;
		let d = `M 0 ${p[0].y.toFixed(1)} L ${p[0].x} ${p[0].y.toFixed(1)}`;
		for (let i = 0; i < p.length - 1; i++) {
			const a = p[Math.max(0, i - 1)], b = p[i], c = p[i + 1], e = p[Math.min(p.length - 1, i + 2)];
			const c1x = b.x + (c.x - a.x) / 6, c1y = b.y + (c.y - a.y) / 6;
			const c2x = c.x - (e.x - b.x) / 6, c2y = c.y - (e.y - b.y) / 6;
			d += ` C ${c1x.toFixed(1)} ${c1y.toFixed(1)} ${c2x.toFixed(1)} ${c2y.toFixed(1)} ${c.x} ${c.y.toFixed(1)}`;
		}
		return `${d} L 600 ${p[p.length - 1].y.toFixed(1)}`;
	});

	const summary = $derived.by(() => {
		const parts: string[] = [];
		if (job) parts.push(`${job.label} running now${pct > 0 ? ` at ${pct}%` : ''}`);
		if (!total) {
			parts.push(busy ? 'nothing else planned for six hours' : 'nothing planned for the next six hours');
		} else {
			const top = hoursList.reduce((b, h) => (h.n > b.n ? h : b), hoursList[0]);
			parts.push(top.n >= 2 ? `busy at ${hm(top.from)} (${top.n} runs)` : `${total === 1 ? 'one run' : `${total} runs`}, never more than one an hour`);
			let last = -1;
			hoursList.forEach((h, i) => h.n && (last = i));
			if (last >= 0 && last < 5) parts.push(`quiet after ${hm(hoursList[last].to)}`);
		}
		const s = parts.join(', ');
		return s.charAt(0).toUpperCase() + s.slice(1);
	});

	// Satellite dots for one to three runs sit on the upper part of the orbit.
	const sats: Record<number, number[]> = { 1: [-90], 2: [-130, -50], 3: [-150, -90, -30] };
	const rays = Array.from({ length: 8 }, (_, k) => (k * 45 * Math.PI) / 180);
	const at = (deg: number, r: number) => ({ x: 24 + r * Math.cos((deg * Math.PI) / 180), y: 24 + r * Math.sin((deg * Math.PI) / 180) });
	const label = (h: Hour) => `${h.i === 0 ? 'This hour' : `From ${hm(h.from)}`}: ${h.n === 0 ? 'no runs' : `${h.n} run${h.n === 1 ? '' : 's'}${h.names.length ? `, ${h.names.join(', ')}` : ''}`}${h.i === 0 && job ? `, ${job.label} running` : ''}`;
</script>

<div class="fc">
	<h3>
		Next six hours
		<span class="st" class:on={busy}>{busy ? `${lj.running.length} running` : total ? `${total} planned` : 'Idle'}</span>
	</h3>

	<div class="strip">
		<svg class="load" viewBox="0 0 600 36" preserveAspectRatio="none" aria-hidden="true">
			<path class="area" d="{curve} L 600 36 L 0 36 Z" />
			<path class="ln" d={curve} />
		</svg>

		{#each hoursList as h (h.i)}
			{@const c = h.n ? (tone[h.worst] ?? 'var(--info)') : 'var(--text-faint)'}
			{@const live = h.i === 0 && !!job}
			<div class="col" class:cur={h.i === 0} style:--c={c} role="img" aria-label={label(h)}>
				<span class="hr">{h.i === 0 ? 'now' : hh(h.from)}</span>
				<svg class="gl" viewBox="0 0 48 48" aria-hidden="true">
					{#if live}
						<circle class="trk" cx="24" cy="24" r="17" />
						<g class="spin" class:indet={pct <= 0}>
							<circle class="prog" cx="24" cy="24" r="17" pathLength="100" stroke-dasharray={pct > 0 ? `${pct} 100` : '22 78'} transform="rotate(-90 24 24)" />
						</g>
						<circle class="core acc" cx="24" cy="24" r="8" />
					{:else if h.n === 0}
						<circle class="empty" cx="24" cy="24" r="8" />
					{:else if h.n <= 3}
						<circle class="orbit" cx="24" cy="24" r="15" />
						<circle class="core" cx="24" cy="24" r="7" />
						{#each sats[h.n] as a (a)}
							{@const p = at(a, 15)}
							<circle class="sat" cx={p.x} cy={p.y} r="3.2" />
						{/each}
					{:else}
						<circle class="halo" cx="24" cy="24" r="13" />
						{#each rays as a (a)}
							<line class="ray" x1={24 + 14.5 * Math.cos(a)} y1={24 + 14.5 * Math.sin(a)} x2={24 + 20 * Math.cos(a)} y2={24 + 20 * Math.sin(a)} />
						{/each}
						<circle class="core" cx="24" cy="24" r="9" />
					{/if}
				</svg>
				<span class="slot"><i class="pt" style:top="{(ly(h.n) / 36) * 100}%"></i></span>
				<span class="ct" class:zero={!h.n && !live}>
					{#if live}<b>{pct > 0 ? `${pct}%` : 'busy'}</b>{#if h.n}{` · ${h.n}`}{/if}{:else}{h.n || 'none'}{/if}
				</span>
				<span class="nm">
					{#each live && job ? [job.label, ...h.names.filter((x) => x !== job.label)].slice(0, 2) : h.names as n (n)}<span>{n}</span>{/each}
				</span>
			</div>
		{/each}
	</div>

	<p class="sum">{summary}</p>
</div>

<style>
	.fc { display: flex; flex-direction: column; min-width: 0; height: 100%; container-type: inline-size; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	.strip { --top: 86px; position: relative; display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); margin: auto 0 0; }
	/* The load line runs through the dot slot of every column (hour label 26px + glyph 52px + gap 8px). */
	.load { position: absolute; left: 0; right: 0; top: var(--top); width: 100%; height: 36px; z-index: 1; pointer-events: none; }
	.area { fill: color-mix(in srgb, var(--text) 5%, transparent); }
	.ln { fill: none; stroke: color-mix(in srgb, var(--text) 30%, transparent); stroke-width: 1.5; vector-effect: non-scaling-stroke; }

	.col { position: relative; display: flex; flex-direction: column; align-items: center; min-width: 0; padding: 8px 3px 10px; border-radius: 12px; }
	.col.cur { background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 88%, var(--text)); box-shadow: inset 0 1px 0 color-mix(in srgb, var(--text) 8%, transparent); }
	.hr { height: 18px; font: 600 11.5px/18px var(--font-ui); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.col.cur .hr { color: var(--text); }
	.gl { display: block; width: 100%; max-width: 48px; height: 52px; margin-top: 0; }
	.slot { position: relative; display: block; width: 100%; height: 36px; margin-top: 8px; z-index: 2; }
	.pt { position: absolute; left: 50%; width: 7px; height: 7px; margin: -3.5px 0 0 -3.5px; border-radius: 50%; background: var(--c); box-shadow: 0 0 0 2px var(--surface-1); transition: top 0.6s ease; }
	.ct { margin-top: 4px; font: 600 12.5px/1.2 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.ct.zero { font-weight: 500; color: var(--text-muted); }
	.ct b { color: var(--accent-text); }
	.nm { display: flex; flex-direction: column; align-items: center; gap: 1px; width: 100%; min-height: 26px; margin-top: 4px; }
	.nm span { max-width: 100%; font-size: 10.5px; line-height: 1.2; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.empty { fill: none; stroke: var(--text-faint); stroke-width: 1.5; }
	.orbit { fill: none; stroke: color-mix(in srgb, var(--c) 30%, transparent); stroke-width: 1; }
	.core { fill: var(--c); }
	.sat { fill: var(--c); }
	.halo { fill: color-mix(in srgb, var(--c) 18%, transparent); }
	.ray { stroke: var(--c); stroke-width: 2.6; stroke-linecap: round; }
	.trk { fill: none; stroke: color-mix(in srgb, var(--text) 10%, transparent); stroke-width: 3.5; }
	.prog { fill: none; stroke: var(--accent); stroke-width: 3.5; stroke-linecap: round; transition: stroke-dasharray 0.8s linear; }
	.core.acc { fill: var(--accent); }
	.spin { transform-box: view-box; transform-origin: 24px 24px; }

	.sum { margin: 10px 0 0; font-size: 12.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	@container (max-width: 400px) {
		.nm { display: none; }
		.ct { font-size: 11.5px; }
	}

	@media (prefers-reduced-motion: no-preference) {
		.spin.indet { animation: turn 1.6s linear infinite; }
		.core.acc { transform-box: fill-box; transform-origin: center; animation: beat 2.4s ease-in-out infinite; }
		@keyframes turn { to { transform: rotate(360deg); } }
		@keyframes beat { 0%, 100% { transform: scale(1); } 50% { transform: scale(0.85); } }
	}
	@media (prefers-reduced-motion: reduce) {
		.pt, .prog { transition: none; }
	}
</style>
