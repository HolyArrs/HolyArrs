<script lang="ts">
	// View "Tape": time as a strip of film sliding past a fixed needle. Runs
	// that finished sit to the left of the needle as small frames, runs still
	// to come to the right, placed on a log scale so the next minutes get room
	// and the next hours still fit. Marks on the strip are real clock times and
	// glide left a little every second. A running job is a frame pinned on the
	// needle that fills up with its progress. The ruler under the strip reads
	// time from now; the line under it says what runs now or what comes next.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { wait, t } from '../livejobs.svelte.ts';
	import { pastResult, type StatusRow } from '../status.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const MIN = 60_000;
	const NEEDLE = 40;
	const K = 2;
	// One log step in percent of the width, so that one hour back lands at 7%.
	const U = (NEEDLE - 7) / Math.log(1 + 60 / K);
	/** x is where a moment sits on the strip, in percent from the left; minutes from now in, signed. */
	const x = (m: number) => NEEDLE + Math.sign(m) * U * Math.log(1 + Math.abs(m) / K);
	/** span is the minutes from now that the edge at a given percent stands for. */
	const span = (p: number) => K * (Math.exp(Math.abs(p - NEEDLE) / U) - 1);
	const BACK = span(0);
	const AHEAD = span(100);
	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)', done: 'var(--ok)', failed: 'var(--fail)', cancelled: 'var(--text-faint)' };

	const ruler = [
		{ m: -60, label: '−1 h' },
		{ m: -10, label: '−10 min' },
		{ m: 0, label: 'Now' },
		{ m: 10, label: '+10 min' },
		{ m: 60, label: '+1 h' },
		{ m: 360, label: '+6 h' }
	].map((r) => ({ ...r, x: x(r.m) }));
	const minor = [-30, -5, -1, 1, 5, 30, 180, 720].map((m) => x(m));

	const pad = (n: number) => String(n).padStart(2, '0');
	const hm = (ms: number) => {
		const d = new Date(ms);
		return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
	};
	const abbr = (s: string) => {
		const w = s.split(/[\s_-]+/).filter(Boolean);
		return (w.length > 1 ? w[0][0] + w[1][0] : (w[0] ?? '?').slice(0, 2)).toUpperCase();
	};
	const word = (state: string) => (state === 'done' ? 'OK' : state === 'failed' ? 'failed' : state === 'cancelled' ? 'cancelled' : state);

	let w = $state(400);
	// Under reduced motion the strip moves once a minute instead of every second.
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

	// A tapped frame puts its name and time in the line under the strip for a while (touch screens have no hover).
	let pick = $state<{ key: string; tip: string } | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;
	function show(f: { key: string; tip: string }) {
		pick = pick?.key === f.key ? null : { key: f.key, tip: f.tip };
		clearTimeout(timer);
		timer = setTimeout(() => (pick = null), 6000);
	}
	$effect(() => () => clearTimeout(timer));

	// Marks at real clock times: every minute near now, then every 5 and 15
	// minutes, then every hour. Each is keyed by its time so it glides.
	const sprockets = $derived.by(() => {
		const out: { ms: number; x: number; hr: boolean; label: string }[] = [];
		const zones: [number, number, number][] = [
			[-BACK, -60, 15],
			[-60, -10, 5],
			[-10, 10, 1],
			[10, 60, 5],
			[60, 360, 15],
			[360, AHEAD, 60]
		];
		let lastLabel = -1e9;
		for (const [a, b, step] of zones) {
			const s = step * MIN;
			const from = Math.ceil((now + a * MIN) / s) * s;
			for (let ms = from; ms < now + b * MIN; ms += s) {
				const d = new Date(ms);
				const hr = d.getMinutes() === 0;
				const p = x((ms - now) / MIN);
				let label = '';
				if (hr && (p - lastLabel) * w >= 4400 && Math.abs(p - NEEDLE) * w >= 2600 && p > 4 && p < 96) {
					label = `${pad(d.getHours())}:00`;
					lastLabel = p;
				}
				out.push({ ms, x: p, hr, label });
			}
		}
		return out;
	});

	interface Frame {
		key: string;
		x: number;
		c: string;
		ab: string;
		tip: string;
		next: boolean;
		lane: number;
	}

	const frames = $derived.by(() => {
		const list: Omit<Frame, 'lane'>[] = [];
		const from = now - BACK * MIN;
		if (lj.history.length) {
			for (const j of lj.finished) {
				const ms = t(j.EndedAt);
				if (!Number.isFinite(ms) || ms < from) continue;
				const name = lj.nameOf(j.Kind);
				list.push({ key: `h${j.ID}`, x: x((ms - now) / MIN), c: tone[j.State] ?? 'var(--info)', ab: abbr(name), tip: `${name}, ended ${hm(ms)} (${word(j.State)})`, next: false });
			}
		} else {
			for (const r of recent) {
				const ms = t(r.lastRun);
				if (!Number.isFinite(ms) || ms < from || ms > now) continue;
				list.push({ key: `p${r.key}`, x: x((ms - now) / MIN), c: tone[pastResult(r).tone] ?? 'var(--info)', ab: abbr(r.name), tip: `${r.name}, ran at ${hm(ms)}`, next: false });
			}
		}
		for (const r of lj.coming(upcoming, 24)) {
			const ms = t(r.nextRun);
			if (!Number.isFinite(ms) || ms - now > AHEAD * MIN) continue;
			const due = ms - now;
			list.push({ key: `n${r.key}`, x: x(Math.max(0.5, due / MIN)), c: tone[r.tone] ?? 'var(--info)', ab: abbr(r.name), tip: due <= 0 ? `${r.name}, due now` : `${r.name} at ${hm(ms)}, in ${wait(due)}`, next: true });
		}
		// Two lanes: a frame drops to the lower lane when the upper one has no room.
		list.sort((a, b) => a.x - b.x);
		const last = [-1e9, -1e9];
		return list.map((f): Frame => {
			const px = (f.x / 100) * w;
			const lane = px - last[0] >= 48 ? 0 : px - last[1] >= 48 ? 1 : last[0] <= last[1] ? 0 : 1;
			last[lane] = px;
			return { ...f, lane };
		});
	});

	const run = $derived(lj.running[0]);
	const pct = $derived(run ? Math.round(run.progress * 100) : 0);
	const first = $derived(lj.coming(upcoming, 1)[0]);
	const caption = $derived.by(() => {
		const nextBit = first ? (lj.due(first) <= 0 ? `${first.name} is due` : `${first.name} in ${wait(lj.due(first))}`) : '';
		if (run) {
			const bits = [`${run.label} running`, pct > 0 ? `${pct}%` : '', lj.running.length > 1 ? `${lj.running.length - 1} more` : '', nextBit ? `then ${nextBit}` : ''];
			return bits.filter(Boolean).join(' · ');
		}
		return nextBit ? `Next up: ${nextBit}` : 'Nothing planned';
	});
</script>

<div class="tape">
	<div class="track" bind:clientWidth={w} role="group" aria-label="Timeline strip: finished runs left of now, coming runs right. {caption}.">
		<div class="belt">
			{#each sprockets as s (s.ms)}
				<div class="mv" style:transform="translateX({s.x.toFixed(3)}%)" aria-hidden="true">
					<i class="sp" class:hr={s.hr}></i>
					{#if s.label}<span class="hl">{s.label}</span>{/if}
				</div>
			{/each}
			{#each frames as f (f.key)}
				<div class="mv" style:transform="translateX({f.x.toFixed(3)}%)">
					<button type="button" class="fr" class:next={f.next} class:low={f.lane === 1} class:sel={pick?.key === f.key} style:--c={f.c} title={f.tip} aria-label={f.tip} onclick={() => show(f)}>
						<i class="tb"></i><span aria-hidden="true">{f.ab}</span>
					</button>
				</div>
			{/each}
		</div>

		<div class="needle" style:left="{NEEDLE}%"><i></i></div>
		{#if run}
			<div class="pin" style:left="{NEEDLE}%" title="{run.label}, {pct > 0 ? `${pct}% done` : 'working'}">
				<i class="fill" class:indet={pct <= 0} style:transform="scaleY({pct > 0 ? pct / 100 : 1})"></i>
				<span class="pab">{abbr(run.label)}</span>
				<span class="pp">{pct > 0 ? `${pct}%` : '…'}</span>
			</div>
		{/if}
	</div>

	<div class="ruler" aria-hidden="true">
		{#each minor as m (m)}<i class="mt" style:left="{m}%"></i>{/each}
		{#each ruler as r (r.m)}
			<i class="tk" class:zero={r.m === 0} style:left="{r.x}%"></i>
			<span class="rl" class:zero={r.m === 0} style:left="{r.x}%">{r.label}</span>
		{/each}
	</div>

	<p class="cap" class:on={!!run || !!pick} aria-live="polite">{pick ? pick.tip : caption}</p>
</div>

<style>
	.tape { display: flex; flex-direction: column; justify-content: center; gap: 6px; min-width: 0; height: 100%; }

	.track { position: relative; height: 104px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 65%, transparent); overflow: hidden; }
	.belt { position: absolute; inset: 0; -webkit-mask-image: linear-gradient(to right, transparent, var(--text) 9%, var(--text) 91%, transparent); mask-image: linear-gradient(to right, transparent, var(--text) 9%, var(--text) 91%, transparent); }
	.mv { position: absolute; inset: 0; pointer-events: none; will-change: transform; }

	.sp { position: absolute; left: -0.5px; bottom: 6px; width: 1px; height: 5px; background: var(--text-faint); opacity: 0.55; }
	.sp.hr { height: 9px; opacity: 0.9; background: var(--text-muted); }
	.hl { position: absolute; left: 4px; bottom: 4px; font: 500 9.5px/1 var(--font-mono); color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }

	.fr { position: absolute; top: 10px; left: -22px; width: 44px; height: 28px; box-sizing: border-box; display: grid; place-items: center; padding: 3px 0 0; margin: 0; border: 1px solid color-mix(in srgb, var(--c) 45%, var(--line)); border-radius: 7px; background: var(--surface-1); overflow: hidden; pointer-events: auto; cursor: pointer; opacity: 0.72; transition: transform 0.4s ease; font: inherit; color: inherit; }
	.fr.sel { opacity: 1; border-color: var(--c); }
	.fr:focus-visible { outline: none; box-shadow: var(--focus-ring); opacity: 1; }
	.fr.next { opacity: 1; }
	.fr.low { transform: translateY(40px); }
	.fr .tb { position: absolute; inset: 0 0 auto; height: 3px; background: var(--c); }
	.fr span { font: 600 11px/1 var(--font-ui); color: var(--text); letter-spacing: 0.02em; }

	.needle { position: absolute; top: 0; bottom: 0; width: 0; z-index: 1; pointer-events: none; }
	.needle::before { content: ''; position: absolute; top: 0; bottom: 0; left: -1px; width: 2px; background: var(--accent, var(--text)); opacity: 0.9; }
	.needle i { position: absolute; top: -4px; left: -5px; width: 10px; height: 10px; border-radius: 50%; background: var(--accent, var(--text)); }

	.pin { position: absolute; top: 26px; margin-left: -27px; width: 54px; height: 38px; box-sizing: border-box; z-index: 2; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 2px; border: 1.5px solid var(--accent, var(--text)); border-radius: 8px; background: var(--surface-1); overflow: hidden; box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 14%, transparent); }
	.pin .fill { position: absolute; inset: 0; background: color-mix(in srgb, var(--accent, var(--text)) 30%, transparent); transform-origin: bottom; transition: transform 0.8s linear; }
	.pab, .pp { position: relative; line-height: 1; font-variant-numeric: tabular-nums; }
	.pab { font: 600 11px/1 var(--font-ui); color: var(--text); }
	.pp { font: 600 10px/1 var(--font-mono); color: var(--text-muted); }

	.ruler { position: relative; height: 24px; margin: 0 1px; border-top: 1px solid var(--line-strong); }
	.mt, .tk { position: absolute; top: 0; width: 1px; margin-left: -0.5px; background: var(--text-faint); }
	.mt { height: 3px; opacity: 0.6; }
	.tk { height: 6px; }
	.tk.zero { height: 8px; width: 2px; margin-left: -1px; background: var(--accent, var(--text)); }
	.rl { position: absolute; top: 9px; transform: translateX(-50%); font: 500 10.5px/1 var(--font-mono); color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.rl.zero { font-family: var(--font-ui); font-weight: 600; color: var(--text); }

	.cap { margin: 2px 0 0; font: 500 12.5px/1.3 var(--font-ui); color: var(--text-muted); font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cap.on { color: var(--text); }

	@media (prefers-reduced-motion: no-preference) {
		.mv { transition: transform 1s linear; }
		.pin .fill.indet { animation: breathe 2.4s ease-in-out infinite; }
		@keyframes breathe { 0%, 100% { opacity: 0.35; } 50% { opacity: 0.9; } }
	}
	@media (prefers-reduced-motion: reduce) {
		.fr, .pin .fill { transition: none; }
	}
</style>
