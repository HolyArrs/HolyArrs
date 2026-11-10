<script lang="ts">
	// Day column: a 12 hour ruler with finished, running and planned runs as chips around a Now line.
	import type { LiveJobs } from './live-jobs.svelte.ts';
	import { t } from './live-jobs.svelte.ts';
	import { t as say, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import type { StatusRow } from './rows.ts';

	type Ev = { key: string; at: number; top: number; name: string; time: string; kind: 'past' | 'next' | 'run'; c: string; fill: number; label: string; lane: number; lanes: number };

	const HOUR = 3600_000;
	const PX = 24; // pixels per hour
	const CH = 20; // chip height in pixels
	const PAD = 7; // room above the first and below the last hour line
	// More than three chips side by side would be too narrow to read.
	const LANES = 3;
	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	let pick = $state<{ key: string; label: string } | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;

	const now = $derived(lj.tick);
	// The window starts on a whole hour, three to four hours back, so the ruler only shifts once an hour.
	const start = $derived.by(() => {
		const d = new Date(now - 3 * HOUR);
		d.setMinutes(0, 0, 0);
		return d.getTime();
	});
	const end = $derived(start + 12 * HOUR);
	const y = (ms: number) => ((ms - start) / HOUR) * PX;
	const yNow = $derived(y(now));

	const hours = $derived(Array.from({ length: 13 }, (_, i) => ({ i, ms: start + i * HOUR, y: i * PX })));

	// Past, running and planned chips with their lane, or hidden when all lanes are taken.
	const placed = $derived.by(() => {
		const evs: Ev[] = [];
		for (const j of lj.running) {
			const s = t(j.started_at ?? j.queued_at);
			const p = Math.min(1, Math.max(0, j.progress));
			const top = Math.min(yNow - 4, Math.max(yNow - CH + 4, Number.isFinite(s) ? y(s) : yNow));
			evs.push({ key: `r${j.id}`, at: s, top, name: j.label, time: Number.isFinite(s) ? hm(s) : say('health.until.now'), kind: 'run', c: 'var(--accent)', fill: p, label: p > 0 ? say('health.day.runningPct', { name: j.label, pct: Math.round(p * 100) }) : say('health.day.running', { name: j.label }), lane: 0, lanes: 1 });
		}
		for (const j of lj.finished) {
			const s = Number.isFinite(t(j.StartedAt)) ? t(j.StartedAt) : t(j.EndedAt);
			if (!Number.isFinite(s) || s < start || s > now) continue;
			const word = j.State === 'done' ? say('health.day.finished') : j.State === 'failed' ? say('health.day.failed') : j.State === 'cancelled' ? say('health.day.cancelled') : j.State;
			const c = j.State === 'done' ? tone.ok : j.State === 'failed' ? tone.fail : tone.idle;
			evs.push({ key: `h${j.ID}`, at: s, top: y(s), name: lj.nameOf(j.Kind), time: hm(s), kind: 'past', c, fill: 0, label: say('health.day.ran', { name: lj.nameOf(j.Kind), time: hm(s), word }), lane: 0, lanes: 1 });
		}
		for (const r of lj.coming(upcoming, 40)) {
			const ms = t(r.nextRun);
			if (!Number.isFinite(ms) || ms > end) continue;
			const late = ms <= now;
			evs.push({ key: `n${r.key}`, at: ms, top: y(Math.max(ms, now)), name: r.name, time: late ? say('health.day.due') : hm(ms), kind: 'next', c: tone[r.tone] ?? tone.info, fill: 0, label: late ? say('health.day.dueNow', { name: r.name }) : say('health.day.runsAt', { name: r.name, time: hm(ms) }), lane: 0, lanes: 1 });
		}

		// Chips that touch form a group; each takes the first free lane and the group shares the width.
		evs.sort((a, b) => a.top - b.top || (a.kind === 'run' ? -1 : b.kind === 'run' ? 1 : 0));
		const shown: Ev[] = [];
		let hidden = 0;
		let group: Ev[] = [];
		let ends: number[] = [];
		let groupEnd = -Infinity;
		const close = () => {
			for (const e of group) e.lanes = ends.length;
			group = [];
			ends = [];
		};
		for (const e of evs) {
			if (e.top >= groupEnd) close();
			let lane = ends.findIndex((b) => b <= e.top);
			if (lane < 0 && ends.length < LANES) lane = ends.push(0) - 1;
			if (lane < 0) {
				hidden++;
				continue;
			}
			ends[lane] = e.top + CH + 1;
			groupEnd = Math.max(groupEnd, e.top + CH + 1);
			e.lane = lane;
			group.push(e);
			shown.push(e);
		}
		close();
		return { shown, hidden };
	});

	const dayWord = $derived.by(() => {
		const today = new Date(now).toDateString();
		if (new Date(start).toDateString() !== today) return say('health.day.sinceYesterday');
		if (new Date(end - 1).toDateString() !== today) return say('health.day.intoTomorrow');
		return say('health.day.today');
	});
	const busy = $derived(lj.running.length > 0);
	const ahead = $derived(placed.shown.filter((e) => e.kind === 'next').length);

	$effect(() => () => clearTimeout(timer));

	// A tapped chip shows its full name and time below the column, since touch screens have no hover.
	function show(e: Ev) {
		pick = pick?.key === e.key ? null : { key: e.key, label: e.label };
		clearTimeout(timer);
		timer = setTimeout(() => (pick = null), 6000);
	}

	// Local clock time helpers.
	const hm = (ms: number) => new Date(ms).toLocaleTimeString(appLocale(), { hour: '2-digit', minute: '2-digit', hour12: false });
	const hh = (ms: number) => new Date(ms).toLocaleTimeString(appLocale(), { hour: '2-digit', hour12: false });
</script>

<div class="day">
	<h3>
		{dayWord}
		<span class="rg">{hm(start)}–{hm(end)}</span>
		{#if placed.hidden}<span class="rg">{say('health.day.hidden', { count: placed.hidden })}</span>{/if}
		<span class="st" class:on={busy}>{busy ? say('health.day.nRunning', { count: lj.running.length }) : ahead ? say('health.day.ahead', { count: ahead }) : say('health.day.idle')}</span>
	</h3>

	<div class="sc">
		<div class="col" style:height="{12 * PX + PAD * 2}px" style:--pad="{PAD}px" style:--ch="{CH}px">
			<div class="gone" style:height="{Math.max(0, yNow)}px" aria-hidden="true"></div>

			{#each hours as h (h.i)}
				<i class="hr" style:top="{h.y}px" aria-hidden="true"></i>
				{#if h.i < 12}<i class="hf" style:top="{h.y + PX / 2}px" aria-hidden="true"></i>{/if}
				{#if Math.abs(h.y - yNow) > 9}<span class="hl" style:top="{h.y}px" aria-hidden="true">{hh(h.ms)}</span>{/if}
			{/each}

			<div class="nw" style:top="{yNow}px" aria-hidden="true"><span class="nt">{hm(now)}</span><i class="nd"></i></div>

			<div class="evs" role="list" aria-label={say('health.day.runsFromTo', { from: hm(start), to: hm(end) })}>
				{#each placed.shown as e (e.key)}
					<div
						class="ev {e.kind}"
						class:shared={e.lanes > 1}
						role="listitem"
						title={e.label}
						style:--c={e.c}
						style:top="{e.top}px"
						style:left="calc({e.lane} * 100% / {e.lanes})"
						style:width="calc(100% / {e.lanes} - 3px)"
					>
						{#if e.kind === 'run'}<i class="fi" style:transform="scaleX({e.fill > 0 ? e.fill : 1})" class:indet={e.fill <= 0}></i>{/if}
						<span class="en" aria-hidden="true">{e.name}</span><span class="et" aria-hidden="true">{e.time}</span>
						<button type="button" class="hit" aria-label={e.label} onclick={() => show(e)}></button>
					</div>
				{/each}
			</div>
		</div>
	</div>
	<p class="pick" aria-live="polite">{pick?.label ?? ''}</p>
</div>

<style>
	.day { display: flex; flex-direction: column; min-width: 0; height: 100%; container-type: inline-size; }
	h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 6px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); white-space: nowrap; overflow: hidden; min-width: 0; }
	.rg { min-width: 0; overflow: hidden; text-overflow: ellipsis; font-size: 11.5px; font-weight: 500; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.st { flex: none; }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	.sc { flex: 1 1 auto; min-height: 0; overflow-y: auto; overflow-x: hidden; scrollbar-width: thin; }
	.col { --rule: 40px; position: relative; box-sizing: border-box; padding: var(--pad) 0; }

	/* Faint wash over hours already gone. */
	.gone { position: absolute; top: var(--pad); left: var(--rule); right: 0; border-radius: 6px 6px 0 0; background: color-mix(in srgb, var(--text) 3.5%, transparent); }
	.hr, .hf { position: absolute; left: var(--rule); right: 0; height: 0; margin-top: var(--pad); border-top: 1px solid var(--line); }
	.hf { border-top: 1px dashed color-mix(in srgb, var(--text) 5%, transparent); }
	.hl { position: absolute; left: 0; width: calc(var(--rule) - 10px); margin-top: var(--pad); transform: translateY(-50%); text-align: right; font: 500 10.5px/1 var(--font-ui); color: var(--text-muted); font-variant-numeric: tabular-nums; }

	.nw { position: absolute; left: 0; right: 0; height: 0; margin-top: var(--pad); z-index: 3; pointer-events: none; }
	.nw::after { content: ''; position: absolute; left: calc(var(--rule) - 3px); right: 0; top: -1px; height: 2px; border-radius: 1px; background: var(--accent); }
	.nt { position: absolute; left: 0; width: calc(var(--rule) - 10px); transform: translateY(-50%); text-align: right; font: 700 10.5px/1 var(--font-ui); color: var(--accent-text); font-variant-numeric: tabular-nums; }
	.nd { position: absolute; left: calc(var(--rule) - 6px); top: -4px; width: 8px; height: 8px; border-radius: 50%; background: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 20%, transparent); }

	.evs { position: absolute; top: var(--pad); bottom: var(--pad); left: calc(var(--rule) + 6px); right: 2px; z-index: 2; }
	.ev {
		position: absolute;
		height: var(--ch);
		display: flex;
		align-items: center;
		gap: 6px;
		box-sizing: border-box;
		padding: 0 7px 0 9px;
		border-radius: 6px;
		overflow: hidden;
		font: 500 11.5px/1 var(--font-ui);
		min-width: 0;
	}
	.en { position: relative; flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.et { position: relative; flex: none; font-size: 10.5px; font-variant-numeric: tabular-nums; }

	/* Finished: grey with a tone stripe. */
	.ev.past { background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text-muted); }
	.ev.past::before { content: ''; position: absolute; left: 0; top: 0; bottom: 0; width: 3px; background: var(--c); }
	.ev.past .et { color: var(--text-muted); }
	.hit { position: absolute; inset: 0; z-index: 2; margin: 0; padding: 0; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Touch: a 44px tap area around each 20px chip. */
	@media (max-width: 767px) and (pointer: coarse) {
		.ev { overflow: visible; }
		.fi { border-radius: inherit; }
		.hit::after { content: ''; position: absolute; inset-inline: 0; top: 50%; height: 44px; transform: translateY(-50%); }
	}
	.pick { flex: none; min-height: 1.3em; margin: 6px 0 0; font: 500 12px/1.3 var(--font-ui); color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pick:empty { display: none; }
	/* Planned: outlined in the row's tone. */
	.ev.next { background: color-mix(in srgb, var(--c) 9%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--c) 60%, transparent); color: var(--text); }
	.ev.next .et { color: var(--text-muted); }
	/* Running: accent, filled by progress. */
	.ev.run { background: color-mix(in srgb, var(--accent) 12%, transparent); box-shadow: inset 0 0 0 1px var(--accent); color: var(--text); font-weight: 600; z-index: 1; }
	.ev.run .et { color: var(--accent-text); }
	.fi { position: absolute; inset: 0; background: color-mix(in srgb, var(--accent) 26%, transparent); transform-origin: left center; transition: transform 0.8s linear; }
	.fi.indet { opacity: 0.5; }

	@container (max-width: 340px) {
		.col { --rule: 34px; }
		.nt { font-size: 9.5px; }
		/* Side by side, a chip keeps the name; the ruler and a tap give the time. */
		.ev.shared .et { display: none; }
		.ev.shared { padding: 0 5px 0 7px; }
	}

	@media (prefers-reduced-motion: no-preference) {
		.nd { animation: beat 2.4s ease-in-out infinite; }
		.fi.indet { animation: breathe 2s ease-in-out infinite; }
		@keyframes beat { 0%, 100% { transform: scale(1); } 50% { transform: scale(0.75); } }
		@keyframes breathe { 0%, 100% { opacity: 0.25; } 50% { opacity: 0.7; } }
	}
	@media (prefers-reduced-motion: reduce) {
		.fi { transition: none; }
	}
</style>
