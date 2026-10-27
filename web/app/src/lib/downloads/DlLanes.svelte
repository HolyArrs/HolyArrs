<script lang="ts">
	// Layout 4 · Lanes: one column per state (Downloading · Waiting its turn ·
	// Needs attention · Done today) with compact cards and a count in each
	// heading. On phones the lanes become a segmented control, one lane at a time.
	import { onMount } from 'svelte';
	import Poster from '../Poster.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import SegmentedControl from '../ui/SegmentedControl.svelte';
	import DlMore from './DlMore.svelte';
	import DlImports from './DlImports.svelte';
	import { to } from '../paths.ts';
	import { ago, bytes, duration, percent } from '../format.ts';
	import { rowName, manyWords, type DownloadRow } from '../downloaders.ts';
	import { doneToday, lanes as splitLanes, note, tone, type Lane } from './layouts.ts';
	import { poster, who, type LayoutProps } from './shared.ts';

	let { rows, imports, titles, busy, ondetails, onfail, onremove }: LayoutProps = $props();
	const split = $derived(splitLanes(rows));
	const today = $derived(doneToday(split.done));
	// Done today; when nothing finished today, the latest few, said as such.
	const doneShown = $derived(today.length ? today : split.done.slice(0, 6));
	const doneTitle = $derived(today.length || split.done.length === 0 ? 'Done today' : 'Done lately');
	const LANES = $derived<{ id: Lane; title: string; short: string; icon: IconName; rows: DownloadRow[]; empty: string }[]>([
		{ id: 'downloading', title: 'Downloading', short: 'Active', icon: 'download', rows: split.downloading, empty: 'Nothing is downloading.' },
		{ id: 'waiting', title: 'Waiting its turn', short: 'Waiting', icon: 'clock', rows: split.waiting, empty: 'Nothing is waiting.' },
		{ id: 'attention', title: 'Needs attention', short: 'Attention', icon: 'alert', rows: split.attention, empty: 'All good.' },
		{ id: 'done', title: doneTitle, short: 'Done', icon: 'check', rows: doneShown, empty: 'Nothing finished today.' }
	]);
	const total = $derived(split.downloading.length + split.waiting.length + split.attention.length);

	let phone = $state(false);
	let lane = $state<Lane>('downloading');
	let picked = false;
	$effect(() => {
		if (!picked && split.attention.length) lane = 'attention';
	});
	onMount(() => {
		const mq = window.matchMedia('(max-width: 767px)');
		const fit = () => (phone = mq.matches);
		fit();
		mq.addEventListener('change', fit);
		return () => mq.removeEventListener('change', fit);
	});
	const options = $derived(LANES.map((l) => ({ value: l.id, label: `${l.short} ${l.rows.length}` })));
	const shown = $derived(phone ? LANES.filter((l) => l.id === lane) : LANES);
</script>

{#snippet card(r: DownloadRow)}
	{@const pct = percent(r.progress)}
	{@const t = tone(r)}
	{@const n = note(r)}
	<li class="lcard {t}" data-testid="dl-lane-card" data-phase={r.phase}>
		<button type="button" class="hit" aria-label="Details for {who(r)}" onclick={() => ondetails(r)}></button>
		<span class="post"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
		<div class="txt">
			<p class="nm">{rowName(r)}</p>
			{#if manyWords(r.kind) && r.covers}<p class="sub">{r.covers}</p>{/if}
			{#if r.phase === 'downloading' || r.phase === 'importing' || r.phase === 'waiting_to_import' || r.phase === 'paused' || r.phase === 'stalled'}
				<span class="lbar" aria-hidden="true"><i style="transform: translateX({pct - 100}%)"></i></span>
			{/if}
			<p class="line" data-testid="row-state">
				{#if r.phase === 'downloading'}<b data-testid="row-percent">{pct}%</b>{#if r.eta}{' · '}{duration(r.eta)} left{/if}{#if r.speed}{' · '}{bytes(r.speed)}/s{/if}
				{:else if r.phase === 'done'}<b>{r.label}</b>{' · '}{ago(r.finished_at ?? r.updated_at)}
				{:else if r.phase === 'waiting'}{r.size ? bytes(r.size) : r.label}
				{:else}<b>{r.label}</b>{#if r.phase === 'paused' || r.phase === 'stalled'}{' · '}{pct}%{/if}{/if}
			</p>
			{#if n && (t === 'fail' || t === 'idle')}<p class="why">{n}</p>{/if}
		</div>
		<div class="end"><DlMore {r} name={who(r)} {busy} {ondetails} {onfail} {onremove} /></div>
		{#if r.phase === 'needs_a_decision'}<div class="decide"><Button size="sm" variant="secondary" href={to('/downloads/decide')}>Decide</Button></div>{/if}
	</li>
{/snippet}

<div class="lanes-view">
	{#if phone}
		<div class="seg"><SegmentedControl {options} bind:value={lane} label="Lane" size="sm" testid="lane-switch" onchange={() => (picked = true)} /></div>
	{/if}
	{#if total === 0 && !phone}
		<p class="calm" data-testid="dl-empty"><Icon name="check" size={18} stroke={2.2} />Nothing is downloading or waiting. New picks land in the first lane.</p>
	{/if}
	<div class="board" class:phone>
		{#each shown as l (l.id)}
			<section class="lane {l.id}" aria-labelledby="lane-{l.id}" data-testid="dl-lane">
				<header class="lh">
					<span class="li"><Icon name={l.icon} size={16} stroke={2} /></span>
					<h2 id="lane-{l.id}">{l.title}</h2>
					<span class="count">{l.rows.length}</span>
				</header>
				{#if l.rows.length === 0}
					<div class="slot" data-testid={phone && total === 0 ? 'dl-empty' : undefined}>
						<p>{l.empty}</p>
						{#if l.id === 'downloading' && total === 0}<Button size="sm" variant="secondary" icon="search" href={to('/search')}>Add media</Button>{/if}
					</div>
				{:else}
					<ul class="cards">{#each l.rows as r (r.id)}{@render card(r)}{/each}</ul>
				{/if}
			</section>
		{/each}
	</div>
	<DlImports {imports} compact />
</div>

<style>
	.lanes-view { display: flex; flex-direction: column; gap: 28px; min-width: 0; }
	.board { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; align-items: start; }
	.lane { min-width: 0; padding: 10px; border-radius: var(--r-panel); background: var(--surface-2); }
	.lh { display: flex; align-items: center; gap: 8px; padding: 4px 4px 10px; }
	.lh h2 { font: var(--t-headline); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.li { display: grid; place-items: center; width: 26px; height: 26px; flex: none; border-radius: 8px; background: color-mix(in srgb, var(--info) 16%, transparent); color: var(--info-text); }
	.waiting .li { background: color-mix(in srgb, var(--wait) 16%, transparent); color: var(--wait-text); }
	.attention .li { background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail-text); }
	.done .li { background: color-mix(in srgb, var(--ok) 16%, transparent); color: var(--ok-text); }
	.count { margin-left: auto; min-width: 24px; height: 22px; padding: 0 7px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--surface-1); color: var(--text-muted); font: var(--t-footnote); font-weight: 700; font-variant-numeric: tabular-nums; }
	.attention .count:not(:empty) { color: var(--fail-text); }
	.cards { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.lcard { position: relative; display: grid; grid-template-columns: 38px minmax(0, 1fr) auto; gap: 10px; align-items: start; padding: 10px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: var(--shadow-card); transition: box-shadow var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.lcard:hover { box-shadow: var(--shadow-lift); transform: translateY(-1px); }
	.lcard.fail { box-shadow: inset 3px 0 0 var(--fail), var(--shadow-card); }
	.lcard.idle { box-shadow: inset 3px 0 0 var(--text-faint), var(--shadow-card); }
	.hit { position: absolute; inset: 0; z-index: 0; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.post, .txt { pointer-events: none; }
	.end { position: relative; z-index: 1; display: flex; flex-direction: column; align-items: flex-end; gap: 6px; }
	.post { width: 38px; }
	.decide { position: relative; z-index: 1; grid-column: 2 / -1; }
	.txt { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.nm { color: var(--text); font: var(--t-callout); font-weight: 600; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.sub { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lbar { display: block; height: 4px; margin: 4px 0 1px; border-radius: 2px; background: var(--surface-3); overflow: hidden; }
	.lbar i { display: block; width: 100%; height: 100%; border-radius: 2px; background: var(--info); transition: transform var(--dur-slow) var(--ease-out); }
	.fail .lbar i { background: var(--fail); }
	.idle .lbar i { background: var(--text-faint); }
	.line { color: var(--text-muted); font: var(--t-footnote); font-weight: 400; font-variant-numeric: tabular-nums; }
	.line b { color: var(--text); font-weight: 600; }
	.fail .line b { color: var(--fail-text); }
	.ok .line b { color: var(--ok-text); }
	.why { margin-top: 2px; color: var(--text-muted); font: var(--t-footnote); font-weight: 400; overflow-wrap: anywhere; }
	.slot { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; padding: 18px 12px; border-radius: var(--r-card); border: 1.5px dashed var(--line-strong); color: var(--text-muted); font: var(--t-callout); }
	.calm { display: flex; align-items: center; gap: 10px; padding: 12px 16px; border-radius: var(--r-card); background: color-mix(in srgb, var(--ok) 10%, transparent); color: var(--ok-text); font: var(--t-callout); font-weight: 500; }
	.seg { position: sticky; top: calc(var(--topbar) + 8px); z-index: 5; }
	.seg :global(.ui-seg) { display: grid; width: 100%; -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); background: color-mix(in srgb, var(--surface-2) 86%, transparent); }
	.seg :global(.ui-seg button) { padding: 0 4px; }

	@media (max-width: 1199px) { .board { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 767px) {
		.lanes-view { gap: 16px; }
		.seg { top: 8px; }
		.board, .board.phone { grid-template-columns: minmax(0, 1fr); }
		.lane { padding: 0; background: none; }
		.lh { padding: 0 2px 10px; }
		.lh h2 { font: var(--t-title3); }
		.lcard { grid-template-columns: 48px minmax(0, 1fr) auto; padding: 12px; }
		.post { width: 48px; }
		.nm { font: var(--t-headline); font-size: 15px; }
		.line, .sub, .why { font: var(--t-subhead); font-weight: 400; }
	}
	@media (prefers-reduced-motion: reduce) { .lcard, .lbar i { transition: none; } .lcard:hover { transform: none; } }
</style>
