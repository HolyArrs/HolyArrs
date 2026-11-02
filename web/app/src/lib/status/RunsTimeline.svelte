<script lang="ts">
	// The Jobs tab's first plate. Left: the last runs above a Now line and the
	// coming runs below it, on one rail, five each way. Right: the next 24
	// hours as one lane per job, a dot for every run it will make, so a job's
	// cadence shows at a glance; jobs that run more often than every 30
	// minutes draw a band instead of hundreds of dots.
	import { ago, dateTime } from '../format.ts';
	import { until, type ScheduleRow } from '../schedules.ts';
	import type { StatusRow } from './status.ts';

	let { upcoming, recent, schedules, now }: { upcoming: StatusRow[]; recent: StatusRow[]; schedules: ScheduleRow[]; now: number } = $props();

	const MAX = 5;
	const up = $derived(upcoming.slice(0, MAX));
	// Oldest run at the top, the newest right above Now.
	const past = $derived(recent.slice(0, MAX).reverse());

	const DAY = 86_400_000;
	type Lane = { kind: string; label: string; running: boolean; band: boolean; every: string; dots: number[] };
	/** everyWord says a cadence in two words: "every 5 min", "every 6 h", "daily". */
	function everyWord(s: number): string {
		if (s >= DAY / 1000 - 60 && s <= DAY / 1000 + 60) return 'daily';
		if (s < 3600) return `every ${Math.round(s / 60)} min`;
		if (s < 86400) return `every ${Math.round(s / 3600)} h`;
		return `every ${Math.round(s / 86400)} d`;
	}
	const lanes = $derived.by((): Lane[] => {
		const out: Lane[] = [];
		for (const s of schedules) {
			if (!s.enabled || !s.next_run) continue;
			let t = Date.parse(s.next_run);
			if (!Number.isFinite(t)) continue;
			const step = s.every_s > 0 ? s.every_s * 1000 : 0;
			const band = step > 0 && step < 1_800_000;
			const dots: number[] = [];
			for (let i = 0; t < now + DAY && i < 60; i++) {
				dots.push(Math.max(0, (t - now) / DAY));
				if (!step) break;
				t += step;
			}
			out.push({ kind: s.kind, label: s.label, running: s.state === 'running' || s.state === 'queued', band, every: everyWord(s.every_s), dots });
		}
		// Soonest first.
		return out.sort((a, b) => (a.dots[0] ?? 1) - (b.dots[0] ?? 1));
	});
	const hour = (k: number) => new Date(now + k * (DAY / 4)).toLocaleTimeString([], { hour: 'numeric' });
</script>

<section class="st-plate" aria-labelledby="s-tl" data-testid="status-timeline">
	<h2 id="s-tl">Timeline<span class="cnt">{recent.length} ran · {upcoming.length} coming</span></h2>
	<div class="halves">
		<ol class="tl">
			{#each past as r (r.key)}
				<li class="past">
					<time class="tm" datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun, now)}</time>
					<i class="d {r.tone}" aria-hidden="true"></i>
					<span class="nm">{r.name}</span>
					<span class="res {r.tone}">{r.word}</span>
				</li>
			{/each}
			<li class="nowline" aria-label="Now"><span class="tm">Now</span><i class="d pulse" aria-hidden="true"></i><span class="hr"></span></li>
			{#each up as r (r.key)}
				<li><span class="tm">{until(r.nextRun, now)}</span><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span></li>
			{:else}<li class="none">No runs planned.</li>{/each}
		</ol>

		<div class="day" aria-labelledby="s-day">
			<h3 id="s-day">Next 24 hours</h3>
			{#if lanes.length}
				<ol class="lanes">
					{#each lanes as l (l.kind)}
						<li class="lane" title="{l.label} · {l.every}">
							<span class="ln">{l.label}</span>
							<span class="track" aria-label="{l.label}: {l.band ? l.every : `${l.dots.length} run${l.dots.length === 1 ? '' : 's'}`}">
								{#if l.band}
									<i class="band" style:left="{l.dots[0] * 100}%"></i>
								{:else}
									{#each l.dots as x, i (i)}<i class="dot" class:live={l.running && i === 0} style:left="{x * 100}%"></i>{/each}
								{/if}
							</span>
							<span class="ev">{l.every}</span>
						</li>
					{/each}
				</ol>
				<div class="hours" aria-hidden="true"><span>Now</span><span>{hour(1)}</span><span>{hour(2)}</span><span>{hour(3)}</span><span>{hour(4)}</span></div>
			{:else}
				<p class="none">No runs planned.</p>
			{/if}
		</div>
	</div>
</section>

<style>
	ol { list-style: none; margin: 0; padding: 0; }
	.cnt { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 12px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.halves { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.15fr); gap: 0 28px; }
	.d { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--info); box-shadow: 0 0 0 4px color-mix(in srgb, var(--surface-1) 90%, transparent); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.d.idle { background: var(--text-faint); }
	.nm { min-width: 0; font-size: 14px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tm { font-size: 12.5px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; text-align: right; }
	.res { min-width: 42px; padding: 2px 8px; border-radius: var(--r-pill); text-align: center; font-size: 12px; font-weight: 600; color: var(--text-muted); background: var(--surface-2, var(--surface-1)); }
	.res.ok { color: var(--ok-text); background: color-mix(in srgb, var(--ok) 14%, transparent); }
	.res.fail { color: var(--fail-text); background: color-mix(in srgb, var(--fail) 16%, transparent); }
	.res.wait { color: var(--text); background: color-mix(in srgb, var(--wait) 20%, transparent); }
	.none { font-size: 14px; color: var(--text-muted); }

	/* the rail */
	.tl { position: relative; }
	.tl::before { content: ''; position: absolute; left: 96px; top: 10px; bottom: 10px; width: 2px; border-radius: 1px; background: var(--line); }
	.tl li { position: relative; display: grid; grid-template-columns: 84px 8px minmax(0, 1fr) auto; align-items: center; gap: 14px; min-height: 40px; }
	.tl .past .nm { color: var(--text-muted); }
	.nowline { min-height: 32px; }
	.nowline .tm { color: var(--accent, var(--text)); font-weight: 600; }
	.nowline .d { width: 10px; height: 10px; margin-left: -1px; background: var(--accent, var(--text)); box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); }
	.hr { grid-column: 3 / 5; height: 1px; background: linear-gradient(90deg, color-mix(in srgb, var(--accent, var(--text)) 50%, transparent), transparent); }
	.tl .none { grid-column: 3 / 5; }
	@media (prefers-reduced-motion: no-preference) {
		.pulse, .dot.live { animation: pulse 2.4s ease-out infinite; }
		@keyframes pulse { 0% { box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70%, 100% { box-shadow: 0 0 0 10px transparent; } }
	}

	/* the day lanes */
	.day { display: flex; flex-direction: column; padding-left: 28px; border-left: 1px solid var(--line); }
	.day h3 { margin: 0 0 6px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.lanes { display: flex; flex-direction: column; }
	.lane { display: grid; grid-template-columns: minmax(0, 118px) minmax(0, 1fr) 72px; align-items: center; gap: 10px; min-height: 22px; }
	.ln { font-size: 12.5px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ev { font-size: 11.5px; color: var(--text-muted); text-align: right; white-space: nowrap; }
	.track { position: relative; height: 14px; border-radius: 7px; background: color-mix(in srgb, var(--text) 5%, transparent); overflow: hidden; }
	.track::before { content: ''; position: absolute; left: 0; top: 0; bottom: 0; width: 2px; background: var(--accent, var(--text)); opacity: 0.8; }
	.dot { position: absolute; top: 4px; width: 6px; height: 6px; margin-left: -3px; border-radius: 50%; background: color-mix(in srgb, var(--text) 55%, transparent); }
	.dot.live { background: var(--accent, var(--text)); }
	.band { position: absolute; top: 4px; right: 0; height: 6px; border-radius: 3px; background: repeating-linear-gradient(90deg, color-mix(in srgb, var(--text) 50%, transparent) 0 4px, transparent 4px 8px); }
	.hours { display: flex; justify-content: space-between; margin: 4px 0 0 128px; padding-right: 82px; font-size: 11px; color: var(--text-muted); font-variant-numeric: tabular-nums; }

	@media (max-width: 1000px) {
		.halves { grid-template-columns: minmax(0, 1fr); gap: 18px; }
		.day { padding: 16px 0 0; border-left: 0; border-top: 1px solid var(--line); }
	}
	@media (max-width: 767px) {
		.tl::before { left: 76px; }
		.tl li { grid-template-columns: 64px 8px minmax(0, 1fr) auto; gap: 12px; }
		.lane { grid-template-columns: minmax(0, 96px) minmax(0, 1fr); }
		.ev { display: none; }
		.hours { margin-left: 106px; padding-right: 0; }
	}
</style>
