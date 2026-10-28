<script lang="ts">
	// The Downloads page body, laid out like a Downloader's own web page: a
	// header strip (speed with a small graph, time left), the Queue with
	// full-cell progress bars, and History with a ✓ or ! per row, a search box
	// and the totals at its foot. ▾ opens a row's details in place; the bin at
	// the far right removes it (through the page's Remove dialog).
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import DecisionPanel from './DecisionPanel.svelte';
	import { ago, bytes, duration, percent } from '../format.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { manyWords, rowName, type Decision, type DownloadRow } from '../downloaders.ts';
	import type { ImportRecord, TitleView } from '../types.ts';
	import { statusWord } from './status-tabs.ts';
	import { statusDetail } from './imports.ts';
	import { poster, who } from './shared.ts';
	import { barTone, facts, historyStatus, historyTotals, inHistory, matches, sizeLine, sparkPath, totals } from './sections.ts';

	let {
		rows,
		importOf,
		titles,
		busy,
		samples,
		decisionsOf,
		openId = $bindable(0),
		onfail,
		onremove,
		ondone
	}: {
		rows: DownloadRow[];
		importOf: Map<number, ImportRecord>;
		titles: Map<number, TitleView>;
		busy: number;
		/** Recent total-speed readings, oldest first. */
		samples: number[];
		decisionsOf: (r: DownloadRow) => Decision[];
		/** The row whose details are open (0: none). */
		openId?: number;
		onfail: (r: DownloadRow) => void;
		onremove: (r: DownloadRow) => void;
		ondone: () => void;
	} = $props();

	const queue = $derived(rows.filter((r) => !inHistory(r)));
	const history = $derived(rows.filter(inHistory).sort((a, b) => (b.finished_at || b.updated_at).localeCompare(a.finished_at || a.updated_at)));
	let q = $state('');
	const found = $derived(history.filter((r) => matches(r, q)));
	const sum = $derived(totals(queue));
	const done = $derived(historyTotals(history));
	const spark = $derived(sparkPath(samples, 64, 18));

	function toggle(id: number) {
		openId = openId === id ? 0 : id;
	}
	function xIn(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.base, easing: easeOut });
	}
	function xOut(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
	const sub = (r: DownloadRow) => (manyWords(r.kind) && r.covers ? r.covers : r.name);
</script>

{#snippet head(r: DownloadRow)}
	<button type="button" class="tg" class:on={openId === r.id} aria-expanded={openId === r.id} aria-controls="dlx-{r.id}" aria-label="{openId === r.id ? 'Hide' : 'Show'} details for {who(r)}" onclick={() => toggle(r.id)} data-testid="dl-toggle">
		<Icon name="chevronDown" size={14} stroke={2.2} />
	</button>
	<span class="post" class:square={r.kind === 'artist'}><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
	<span class="nm">
		<span class="tt" title={rowName(r)}>{rowName(r)}{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
		<span class="rel" title={r.name}>{sub(r)}</span>
	</span>
{/snippet}

{#snippet bin(r: DownloadRow)}
	<span class="del">
		{#if r.can_act}
			<button type="button" class="bin" aria-label="Remove {who(r)}" title="Remove" disabled={busy === r.id} onclick={() => onremove(r)} data-testid="dl-remove"><Icon name="trash" size={16} /></button>
		{/if}
	</span>
{/snippet}

{#snippet panel(r: DownloadRow)}
	{@const ds = decisionsOf(r)}
	<div class="x" id="dlx-{r.id}" data-testid="dl-panel" in:xIn out:xOut>
		<dl class="kv">
			{#each facts(r, importOf.get(r.id)) as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}
		</dl>
		{#if ds.length}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
		{#if r.can_act && !inHistory(r)}
			<p class="xa"><button type="button" class="lk" disabled={busy === r.id} onclick={() => onfail(r)}>Mark failed</button></p>
		{/if}
	</div>
{/snippet}

<div class="strip" data-testid="dl-strip">
	<span class="sp"><span class="k">Speed</span><b data-testid="dl-speed">{bytes(sum.speed)}/s</b>
		<svg class="spark" viewBox="0 0 64 18" width="64" height="18" aria-hidden="true">{#if spark}<path d={spark} />{/if}</svg>
	</span>
	<span><span class="k">Time left</span><b>{sum.eta ? duration(sum.eta) : '–'}</b></span>
	<span><span class="k">Left</span><b>{bytes(sum.left)}</b></span>
	<span><span class="k">In queue</span><b>{queue.length}</b></span>
</div>

<section class="sec" aria-labelledby="dl-queue">
	<h2 id="dl-queue" class="sh">Queue <span class="n">{queue.length}</span></h2>
	{#if queue.length === 0}
		<p class="none">Nothing in the queue.</p>
	{:else}
		<ul class="rows" data-testid="dl-queue">
			{#each queue as r (r.id)}
				{@const pct = percent(r.progress)}
				{@const t = barTone(r)}
				{@const more = statusDetail(r, importOf.get(r.id))}
				<li class="row" class:open={openId === r.id} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
					<div class="line q">
						{@render head(r)}
						<span class="bar {t}" role="progressbar" aria-label="{who(r)}: {statusWord(r)}{more ? `, ${more}` : ''}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.floor(pct)} title={more ? `${statusWord(r)} · ${more}` : statusWord(r)} data-testid="dl-bar">
							<i style="transform: translateX({pct - 100}%)"></i>
							<span class="bl">{Math.floor(pct)}%{#if t !== 'go' || pct === 0}<span class="bw"> · {statusWord(r)}</span>{/if}</span>
							<span class="br">{sizeLine(r)}</span>
						</span>
						<span class="eta num">{r.active && r.eta ? duration(r.eta) : '–'}</span>
						{@render bin(r)}
					</div>
					{#if openId === r.id}{@render panel(r)}{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<section class="sec" aria-labelledby="dl-history">
	<h2 id="dl-history" class="sh">History <span class="n">{history.length}</span></h2>
	{#if found.length === 0}
		<p class="none">{q.trim() ? 'Nothing matches.' : 'Nothing finished yet.'}</p>
	{:else}
		<ul class="rows" data-testid="dl-history">
			{#each found as r (r.id)}
				{@const s = historyStatus(r, importOf.get(r.id))}
				<li class="row" class:open={openId === r.id} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
					<div class="line h">
						{@render head(r)}
						<span class="hs" class:bad={s.bad} title={s.text}><span class="ic" aria-hidden="true">{s.bad ? '!' : '✓'}</span><span class="tx">{s.text}</span></span>
						<span class="when num">{r.size ? bytes(r.size) : ''}</span>
						<span class="eta num" title={r.finished_at || r.updated_at}>{ago(r.finished_at || r.updated_at)}</span>
						{@render bin(r)}
					</div>
					{#if openId === r.id}{@render panel(r)}{/if}
				</li>
			{/each}
		</ul>
	{/if}
	<div class="foot">
		<label class="find"><Icon name="search" size={14} /><input type="search" placeholder="Search History" aria-label="Search History" bind:value={q} /></label>
		<span class="tot" data-testid="dl-totals">{bytes(done.today)} today · {bytes(done.month)} this month</span>
	</div>
</section>

<style>
	.strip { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 22px; padding: 8px 12px; margin-bottom: 14px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.strip > span { display: inline-flex; align-items: center; gap: 6px; }
	.k { color: var(--text-muted); font: var(--t-footnote); }
	.strip b { font-weight: 600; }
	.spark { flex: none; }
	.spark path { fill: none; stroke: var(--ok); stroke-width: 1.5; stroke-linejoin: round; stroke-linecap: round; }

	.sec { margin-bottom: 22px; }
	.sh { display: flex; align-items: baseline; gap: 8px; margin: 0 0 6px; font: var(--t-headline); }
	.n { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.none { padding: 10px 12px; color: var(--text-muted); font: var(--t-callout); }

	.rows { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--line); }
	.row { border-bottom: 1px solid var(--line); }
	.row.open { background: var(--surface-2); }
	.line { display: grid; align-items: center; gap: 10px; min-height: 38px; padding: 1px 4px; }
	.line.q { grid-template-columns: 24px 24px minmax(0, 1fr) minmax(220px, 34%) 64px 32px; }
	.line.h { grid-template-columns: 24px 24px minmax(0, 1fr) minmax(220px, 34%) 64px 64px 32px; }
	.line:hover { background: var(--surface-2); }

	.tg { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.tg :global(svg) { transform: rotate(-90deg); transition: transform var(--dur-fast) var(--ease-out); }
	.tg.on :global(svg) { transform: none; }
	.tg:hover { color: var(--text); background: var(--surface-3); }
	.tg:focus-visible, .bin:focus-visible, .lk:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.post { width: 24px; --poster-ratio: 2 / 3; }
	.post.square { --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 3px; }

	.nm { display: flex; flex-direction: column; min-width: 0; line-height: 1.2; }
	.tt, .rel { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tt { font: var(--t-callout); font-weight: 500; }
	.rel { color: var(--text-muted); font: var(--t-caption); }
	.ver { margin-left: 6px; padding: 0 5px; border-radius: 4px; background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); }

	.bar { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 8px; height: 24px; padding: 0 8px; border-radius: 5px; overflow: hidden; background: var(--surface-3); color: var(--text); font: var(--t-caption); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.bar i { position: absolute; inset: 0; background: var(--ok); opacity: 0.85; transition: transform var(--dur-slow) var(--ease-out); }
	.bar.queued i { background: var(--info); }
	.bar.held i { background: var(--wait); }
	.bar.bad i { background: var(--fail); }
	.bar.queued, .bar.held, .bar.bad { box-shadow: inset 3px 0 0 var(--bar-edge, transparent); }
	.bar.queued { --bar-edge: var(--info); }
	.bar.held { --bar-edge: var(--wait); }
	.bar.bad { --bar-edge: var(--fail); }
	.bl, .br { position: relative; font-weight: 600; text-shadow: 0 0 3px var(--surface-1); }
	.bw { font-weight: 500; }
	.br { overflow: hidden; text-overflow: ellipsis; }

	.num { text-align: right; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.hs { display: flex; align-items: center; gap: 6px; min-width: 0; font: var(--t-footnote); }
	.hs .tx { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.hs.bad { color: var(--fail); }
	.ic { display: grid; place-items: center; flex: none; width: 16px; height: 16px; border-radius: 50%; background: var(--ok); color: var(--surface-1); font-size: 10px; font-weight: 700; }
	.hs.bad .ic { background: var(--fail); }

	.del { display: flex; justify-content: flex-end; }
	.bin { display: grid; place-items: center; width: 30px; height: 30px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; transition: color var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.bin:hover { color: var(--fail); background: var(--surface-3); }
	.bin:disabled { opacity: 0.4; cursor: default; }

	.x { padding: 6px 12px 12px 62px; }
	.kv { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 3px 16px; margin: 0; font: var(--t-footnote); }
	.kv dt { color: var(--text-muted); }
	.kv dd { margin: 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
	.dp { margin-top: 10px; }
	.xa { margin: 8px 0 0; }
	.lk { min-height: 28px; padding: 0 4px; border: 0; border-radius: 6px; background: none; color: var(--accent-text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }

	.foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; margin-top: 10px; }
	.find { display: inline-flex; align-items: center; gap: 6px; height: 32px; padding: 0 10px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text-muted); }
	.find:focus-within { box-shadow: var(--focus-ring); }
	.find input { width: 180px; border: 0; background: none; color: var(--text); font: var(--t-footnote); outline: none; }
	.tot { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }

	/* Phones: the bar goes under the name, the time stays on the bar's row. */
	@media (max-width: 767px) {
		.line.q, .line.h { grid-template-columns: 24px 24px minmax(0, 1fr) auto 32px; grid-template-areas: 'tg post nm nm del' '. post bar eta del'; row-gap: 3px; padding: 5px 2px; }
		.line.h { grid-template-areas: 'tg post nm nm del' '. post bar eta del'; }
		.tg { grid-area: tg; }
		.post { grid-area: post; align-self: start; }
		.nm { grid-area: nm; }
		.bar, .hs { grid-area: bar; min-width: 0; height: 20px; }
		.line .eta { grid-area: eta; }
		.when { display: none; }
		.del { grid-area: del; }
		.x { padding: 4px 8px 12px 30px; }
		.find { flex: 1; }
		.find input { width: 100%; }
		.strip { gap: 4px 14px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.bar i, .tg :global(svg), .bin { transition: none; }
	}
</style>
