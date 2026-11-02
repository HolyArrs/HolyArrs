<script lang="ts">
	// Coming up and Recent runs on the Jobs tab, in five preview shapes (?runs=):
	//   1 Ledger     ruled rows, name left, time right, the result as a tinted pill
	//   2 Rail       a vertical rail with a dot per run, time over the name, the next run marked
	//   3 Countdown  each coming run carries a track that fills as the run nears
	//   4 Buckets    runs under small time headings: Now, Within the hour, Later today…
	//   5 Timeline   one plate, past runs above a Now line, coming runs below it
	import type { Snippet } from 'svelte';
	import { ago, dateTime } from '../format.ts';
	import { until } from '../schedules.ts';
	import type { StatusRow } from './status.ts';

	let { upcoming, recent, view, now }: { upcoming: StatusRow[]; recent: StatusRow[]; view: 1 | 2 | 3 | 4 | 5; now: number } = $props();

	const MAX = 5;
	const up = $derived(upcoming.slice(0, MAX));
	const past = $derived(recent.slice(0, MAX));
	const t = (s?: string) => (s ? Date.parse(s) : NaN);
	/** progress is how far a job is through its wait: 0 right after a run, 1 when the next is due. */
	function progress(r: StatusRow): number {
		const a = t(r.lastRun), b = t(r.nextRun);
		if (!Number.isFinite(a) || !Number.isFinite(b) || b <= a) return 1;
		return Math.min(1, Math.max(0, (now - a) / (b - a)));
	}
	const H = 3600_000, D = 86400_000;
	function bucketUp(r: StatusRow): string {
		const d = t(r.nextRun) - now;
		return d <= 30_000 ? 'Now' : d < H ? 'Within the hour' : d < D ? 'Later today' : 'Later';
	}
	function bucketPast(r: StatusRow): string {
		const d = now - t(r.lastRun);
		return d < H ? 'Past hour' : d < D ? 'Past day' : 'Earlier';
	}
	function buckets(rows: StatusRow[], by: (r: StatusRow) => string): { label: string; rows: StatusRow[] }[] {
		const out: { label: string; rows: StatusRow[] }[] = [];
		for (const r of rows) {
			const label = by(r);
			const b = out.find((x) => x.label === label);
			if (b) b.rows.push(r);
			else out.push({ label, rows: [r] });
		}
		return out;
	}
	const upB = $derived(buckets(up, bucketUp));
	const pastB = $derived(buckets(past, bucketPast));
	// Timeline: oldest run at the top, the newest right above Now.
	const pastUp = $derived([...past].reverse());
</script>

{#snippet when(r: StatusRow, kind: 'up' | 'past')}
	{#if kind === 'up'}<span class="tm">{until(r.nextRun, now)}</span>{:else}<time class="tm" datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun, now)}</time>{/if}
{/snippet}

{#snippet result(r: StatusRow)}
	<span class="res {r.tone}">{r.word}</span>
{/snippet}

{#snippet pair(left: Snippet, right: Snippet)}
	<div class="pair">
		<section class="st-plate" aria-labelledby="s-next"><h2 id="s-next">Coming up<span class="cnt">{upcoming.length}</span></h2>{@render left()}</section>
		<section class="st-plate" aria-labelledby="s-last"><h2 id="s-last">Recent runs<span class="cnt">{recent.length}</span></h2>{@render right()}</section>
	</div>
{/snippet}

{#if view === 1}
	{#snippet l1()}
		<ol class="led">
			{#each up as r (r.key)}<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>{@render when(r, 'up')}</li>{:else}<li class="none">No runs planned.</li>{/each}
		</ol>
	{/snippet}
	{#snippet r1()}
		<ol class="led">
			{#each past as r (r.key)}<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>{@render when(r, 'past')}{@render result(r)}</li>{:else}<li class="none">Nothing has run yet.</li>{/each}
		</ol>
	{/snippet}
	{@render pair(l1, r1)}
{:else if view === 2}
	{#snippet l2()}
		<ol class="rail">
			{#each up as r, i (r.key)}<li class:next={i === 0}><i class="d {r.tone}" aria-hidden="true"></i><span class="tm">{i === 0 ? 'Next · ' : ''}{until(r.nextRun, now)}</span><span class="nm">{r.name}</span></li>{:else}<li class="none">No runs planned.</li>{/each}
		</ol>
	{/snippet}
	{#snippet r2()}
		<ol class="rail">
			{#each past as r (r.key)}<li><i class="d {r.tone}" aria-hidden="true"></i>{@render when(r, 'past')}<span class="nm">{r.name}</span>{@render result(r)}</li>{:else}<li class="none">Nothing has run yet.</li>{/each}
		</ol>
	{/snippet}
	{@render pair(l2, r2)}
{:else if view === 3}
	{#snippet l3()}
		<ol class="cd">
			{#each up as r (r.key)}
				{@const p = progress(r)}
				<li>
					<div class="ln"><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>{@render when(r, 'up')}</div>
					<div class="trk" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(p * 100)} aria-label="{r.name}: time until the next run"><i style:width="{p * 100}%" class:due={p >= 0.98}></i></div>
				</li>
			{:else}<li class="none">No runs planned.</li>{/each}
		</ol>
	{/snippet}
	{#snippet r3()}
		<ol class="cd">
			{#each past as r (r.key)}
				<li><div class="ln"><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>{@render when(r, 'past')}{@render result(r)}</div></li>
			{:else}<li class="none">Nothing has run yet.</li>{/each}
		</ol>
	{/snippet}
	{@render pair(l3, r3)}
{:else if view === 4}
	{#snippet l4()}
		{#each upB as b (b.label)}
			<h3 class="bh">{b.label}<span>{b.rows.length}</span></h3>
			<ol class="bk">{#each b.rows as r (r.key)}<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>{@render when(r, 'up')}</li>{/each}</ol>
		{:else}<p class="none">No runs planned.</p>{/each}
	{/snippet}
	{#snippet r4()}
		{#each pastB as b (b.label)}
			<h3 class="bh">{b.label}<span>{b.rows.length}</span></h3>
			<ol class="bk">{#each b.rows as r (r.key)}<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>{@render when(r, 'past')}{@render result(r)}</li>{/each}</ol>
		{:else}<p class="none">Nothing has run yet.</p>{/each}
	{/snippet}
	{@render pair(l4, r4)}
{:else}
	<section class="st-plate" aria-labelledby="s-tl">
		<h2 id="s-tl">Timeline<span class="cnt">{recent.length} ran · {upcoming.length} coming</span></h2>
		<ol class="tl">
			{#each pastUp as r (r.key)}
				<li class="past">{@render when(r, 'past')}<i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>{@render result(r)}</li>
			{/each}
			<li class="nowline" aria-label="Now"><span class="tm">Now</span><i class="d pulse" aria-hidden="true"></i><span class="hr"></span></li>
			{#each up as r (r.key)}
				<li>{@render when(r, 'up')}<i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span></li>
			{:else}<li class="none">No runs planned.</li>{/each}
		</ol>
	</section>
{/if}

<style>
	ol { list-style: none; margin: 0; padding: 0; }
	.pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 22px; align-items: stretch; }
	.cnt { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 12px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.d { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.d.idle { background: var(--text-faint); }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 14px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tm { flex: none; font-size: 12.5px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.res { flex: none; min-width: 42px; padding: 2px 8px; border-radius: var(--r-pill); text-align: center; font-size: 12px; font-weight: 600; color: var(--text-muted); background: var(--surface-2, var(--surface-1)); }
	.res.ok { color: var(--ok-text); background: color-mix(in srgb, var(--ok) 14%, transparent); }
	.res.fail { color: var(--fail-text); background: color-mix(in srgb, var(--fail) 16%, transparent); }
	.res.wait { color: var(--text); background: color-mix(in srgb, var(--wait) 20%, transparent); }
	.none { display: block; margin: 0; min-height: 44px; line-height: 44px; font-size: 14px; color: var(--text-muted); }

	/* 1 ledger */
	.led { border: 1px solid var(--line); border-radius: 12px; overflow: hidden; background: var(--surface-1); }
	.led li { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 14px; border-top: 1px solid var(--line); }
	.led li:first-child { border-top: 0; }
	.led li:nth-child(even) { background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 45%, transparent); }
	.led .none { padding: 0 14px; }

	/* 2 rail */
	.rail { position: relative; padding-left: 4px; }
	.rail::before { content: ''; position: absolute; left: 7px; top: 14px; bottom: 14px; width: 2px; border-radius: 1px; background: var(--line); }
	.rail li { position: relative; display: grid; grid-template-columns: 8px minmax(0, 1fr) auto; grid-template-rows: auto auto; column-gap: 14px; align-items: center; padding: 7px 0; }
	.rail .d { grid-row: 1 / 3; align-self: center; box-shadow: 0 0 0 4px color-mix(in srgb, var(--surface-1) 90%, transparent); }
	.rail .tm { grid-column: 2; grid-row: 1; }
	.rail .nm { grid-column: 2; grid-row: 2; font-size: 14.5px; }
	.rail .res { grid-column: 3; grid-row: 1 / 3; }
	.rail .next .tm { color: var(--accent, var(--text)); font-weight: 600; }
	.rail .next .d { box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 22%, transparent); }
	.rail .none { padding-left: 22px; }

	/* 3 countdown */
	.cd { display: flex; flex-direction: column; }
	.cd li { padding: 8px 0 10px; border-top: 1px solid var(--line); }
	.cd li:first-child { border-top: 0; padding-top: 2px; }
	.ln { display: flex; align-items: center; gap: 10px; min-height: 30px; }
	.trk { height: 3px; margin: 6px 0 0 18px; border-radius: 2px; overflow: hidden; background: color-mix(in srgb, var(--text) 10%, transparent); }
	.trk i { display: block; height: 100%; border-radius: inherit; background: color-mix(in srgb, var(--text) 45%, transparent); transition: width 0.6s ease-out; }
	.trk i.due { background: var(--accent, var(--text)); }

	/* 4 buckets */
	.bh { display: flex; align-items: center; justify-content: space-between; margin: 14px 0 4px; padding-bottom: 6px; border-bottom: 1px solid var(--line); font: 600 12.5px/1 var(--font-ui); color: var(--text-muted); }
	.bh:first-child { margin-top: 0; }
	.bh span { font-weight: 500; font-variant-numeric: tabular-nums; }
	.bk li { display: flex; align-items: center; gap: 10px; min-height: 40px; }

	/* 5 timeline */
	.tl { position: relative; }
	.tl::before { content: ''; position: absolute; left: 96px; top: 10px; bottom: 10px; width: 2px; border-radius: 1px; background: var(--line); }
	.tl li { position: relative; display: grid; grid-template-columns: 84px 8px minmax(0, 1fr) auto; align-items: center; gap: 14px; min-height: 40px; }
	.tl .tm { text-align: right; }
	.tl .d { box-shadow: 0 0 0 4px color-mix(in srgb, var(--surface-1) 90%, transparent); }
	.tl .past .nm { color: var(--text-muted); }
	.tl .nowline { min-height: 32px; }
	.tl .nowline .tm { color: var(--accent, var(--text)); font-weight: 600; }
	.tl .nowline .d { width: 10px; height: 10px; margin-left: -1px; background: var(--accent, var(--text)); box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); }
	.tl .hr { grid-column: 3 / 5; height: 1px; background: linear-gradient(90deg, color-mix(in srgb, var(--accent, var(--text)) 50%, transparent), transparent); }
	.tl .none { grid-column: 3 / 5; }
	@media (prefers-reduced-motion: no-preference) {
		.pulse { animation: pulse 2.4s ease-out infinite; }
		@keyframes pulse { 0% { box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70% { box-shadow: 0 0 0 10px transparent; } 100% { box-shadow: 0 0 0 10px transparent; } }
	}

	@media (max-width: 900px) { .pair { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 767px) {
		.tl::before { left: 76px; }
		.tl li { grid-template-columns: 64px 8px minmax(0, 1fr) auto; gap: 12px; }
	}
</style>
