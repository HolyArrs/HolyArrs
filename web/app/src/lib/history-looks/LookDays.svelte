<script lang="ts">
	// Look 2, Days: a timeline read one day at a time. Each day has a sticky
	// header with what it held ("2 came in · 1 problem · 40 routine"); rows
	// sit on a rail with their clock time. Runs of routine work (refreshes,
	// scans, searches) fold into one quiet line that opens in place.
	import { onMount, untrack } from 'svelte';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import EventSheet from '../history-designs/EventSheet.svelte';
	import StateDot from './StateDot.svelte';
	import LookFrame from './LookFrame.svelte';
	import { EventFeed } from './feed.svelte.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import type { HistoryFilter, HistoryRow } from '../history.ts';
	import { artOf, burstLine, clock, dayLabel, headVerb, rowTier } from '../history-views.ts';
	import { byDay, dayItems, dayLine, rowWhy } from '../history-looks.ts';
	import type { TitleView } from '../types.ts';

	let { filter, kinds, titleById, empty, onall }: { filter: HistoryFilter; kinds: string[] | undefined; titleById: Map<number, TitleView>; empty: { title: string; text: string }; onall?: () => void } = $props();

	const SIZE = 200;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(filter, kinds);
	$effect(() => {
		void [filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));

	// A page of routine only (the Routine tab) is not folded: it is what was asked for.
	const days = $derived.by(() => {
		const list = byDay(feed.events);
		const allQuiet = list.every((d) => d.rows.every((r) => rowTier(r) === 'routine'));
		return list.map((d) => ({ ...d, items: allQuiet ? d.rows.map((row) => ({ type: 'row' as const, key: row.key, row, tier: 'routine' as const })) : dayItems(d.rows) }));
	});
	let sheet = $state<HistoryRow | null>(null);
	let open = $state<Record<string, boolean>>({});

	function unfold(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.base, easing: easeOut });
	}
	function fold(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
</script>

{#snippet line(r: HistoryRow)}
	{@const e = r.head}
	{@const v = headVerb(r)}
	{@const t = titleById.get(e.TitleID ?? 0)}
	<button type="button" class="hit {v.tone}" data-testid="look-row" onclick={() => (sheet = r)}>
		<time class="clk" datetime={e.At}>{clock(e.At)}</time>
		<span class="node" aria-hidden="true"><i></i></span>
		<span class="post" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={14} /></span>{/if}</span>
		<span class="tx">
			<span class="top"><span class="nm">{e.title_name ?? 'HolyArrs'}{#if e.episode}<span class="ep">{` ${e.episode}`}</span>{/if}</span><StateDot verb={v} /></span>
			<span class="sum">{rowWhy(r) || e.Summary}</span>
		</span>
	</button>
{/snippet}

<LookFrame {feed} {empty} {onall} onretry={reload} onmore={() => feed.load(filter, kinds, feed.offset + SIZE, true)}>
	<div class="days" data-testid="look-days">
		{#each days as d (d.key)}
			<section class="day" aria-labelledby="day-{d.key}">
				<header class="dh">
					<h2 id="day-{d.key}">{dayLabel(d.key)}</h2>
					<p class="dl">{dayLine(d.tally)}</p>
				</header>
				<ol class="rail">
					{#each d.items as it (it.key)}
						{#if it.type === 'row'}
							<li class="item" class:quiet={it.tier === 'routine'}>{@render line(it.row)}</li>
						{:else}
							{@const on = !!open[it.key]}
							<li class="item fold">
								<button type="button" class="hit q" aria-expanded={on} onclick={() => (open = { ...open, [it.key]: !on })}>
									<time class="clk" datetime={it.rows[0].head.At}>{clock(it.rows[0].head.At)}</time>
									<span class="node" aria-hidden="true"><i></i></span>
									<span class="qn">{it.rows.length}</span>
									<span class="tx"><span class="sum q">{burstLine(it.rows.flatMap((r) => r.events), titleById)}</span></span>
									<span class="chev" class:on aria-hidden="true"><Icon name="chevronDown" size={16} /></span>
								</button>
								{#if on}
									<ol class="inner" in:unfold out:fold>
										{#each it.rows as r (r.key)}<li class="item quiet">{@render line(r)}</li>{/each}
									</ol>
								{/if}
							</li>
						{/if}
					{/each}
				</ol>
			</section>
		{/each}
	</div>
</LookFrame>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.days { display: flex; flex-direction: column; gap: 20px; }
	.dh { position: sticky; top: var(--topbar); z-index: 2; display: flex; align-items: baseline; justify-content: space-between; gap: 4px 12px; flex-wrap: wrap; padding: 10px 0 8px; background: var(--bg); transition: top var(--bar-glide); }
	:global(html.bar-hidden) .dh { top: 0; }
	.dh h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.dl { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.rail { position: relative; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	/* The rail line runs through every dot. */
	.rail::before { content: ''; position: absolute; top: 18px; bottom: 18px; left: calc(4px + 10px + 64px + 10px + 5px); width: 2px; border-radius: 1px; background: var(--line); }
	.item + .item { box-shadow: 0 -1px 0 var(--line); }
	.hit { position: relative; width: 100%; display: grid; grid-template-columns: 64px 12px 26px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 52px; padding: 6px 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	.hit:hover { background: var(--surface-2); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.clk { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; text-align: right; white-space: nowrap; }
	.node { display: grid; place-items: center; width: 12px; height: 12px; }
	.node i { width: 10px; height: 10px; border-radius: 50%; background: var(--info); box-shadow: 0 0 0 3px var(--surface-1); }
	.ok .node i { background: var(--ok); }
	.fail .node i { background: var(--fail); }
	.wait .node i { background: var(--wait); }
	.idle .node i, .q .node i { width: 6px; height: 6px; background: var(--text-faint); }
	.post { width: 26px; --poster-ratio: 2 / 3; }
	.post.square { --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 4px; }
	.sys { display: grid; place-items: center; width: 26px; aspect-ratio: 2 / 3; border-radius: 4px; background: var(--surface-2); color: var(--text-muted); }
	.tx { display: flex; flex-direction: column; gap: 1px; min-width: 0; grid-column: 4 / -1; }
	.top { display: flex; align-items: center; gap: 10px; min-width: 0; font: var(--t-footnote); }
	.nm { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font: var(--t-callout); font-weight: 600; }
	.top :global(.st) { flex: none; }
	.ep { color: var(--text-muted); font-weight: 400; }
	.sum { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.quiet .nm { color: var(--text-muted); font-weight: 500; }
	/* A folded run: the count where the poster goes, the line, a chevron. */
	.hit.q { grid-template-columns: 64px 12px 26px minmax(0, 1fr) auto; }
	.hit.q .tx { grid-column: 4; }
	.qn { display: grid; place-items: center; height: 22px; border-radius: 6px; background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }
	.sum.q { color: var(--text-muted); font: var(--t-callout); }
	.chev { display: grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out); }
	.chev.on { transform: rotate(180deg); }
	.inner { margin: 0 0 4px; padding: 0; list-style: none; }
	.inner .item { box-shadow: 0 -1px 0 var(--line); }
	@media (max-width: 767px) {
		.rail::before { left: calc(4px + 8px + 54px + 8px + 5px); }
		.hit, .hit.q { grid-template-columns: 54px 12px 26px minmax(0, 1fr) auto; gap: 8px; padding: 6px 8px; min-height: 56px; }
		.top { flex-direction: column; align-items: stretch; gap: 1px; }
		.top :global(.st) { align-self: flex-start; }
		.top :global(.st) { flex: initial; }
	}
	@media (prefers-reduced-motion: reduce) { .hit, .chev, .dh { transition: none; } }
</style>
