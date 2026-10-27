<script lang="ts">
	// Look 4, Cards: what happened to your media as cards with their art,
	// grouped by day, so "what came in today" reads at a glance. Routine work
	// stays out of the way: a single line per day says how much ran, and
	// opens it as quiet cards. Cards open the side sheet.
	import { onMount, untrack } from 'svelte';
	import { fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import EventSheet from '../history-designs/EventSheet.svelte';
	import VerbMark from '../history-designs/VerbMark.svelte';
	import LookFrame from './LookFrame.svelte';
	import { EventFeed } from './feed.svelte.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeOut } from '../ui/transitions.ts';
	import type { HistoryFilter, HistoryRow } from '../history.ts';
	import { artOf, burstLine, byWhom, clock, dayLabel, headVerb, rowTier } from '../history-views.ts';
	import { byDay, dayLine, rowWhy } from '../history-looks.ts';
	import type { TitleView } from '../types.ts';

	let { filter, kinds, titleById, empty, onall }: { filter: HistoryFilter; kinds: string[] | undefined; titleById: Map<number, TitleView>; empty: { title: string; text: string }; onall?: () => void } = $props();

	const SIZE = 150;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(filter, kinds);
	$effect(() => {
		void [filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));

	const days = $derived(
		byDay(feed.events).map((d) => {
			const loud = d.rows.filter((r) => rowTier(r) !== 'routine');
			// A page of routine only (the Routine tab) shows it all as cards.
			const allQuiet = loud.length === 0;
			return { ...d, cards: allQuiet ? d.rows : loud, quiet: allQuiet ? [] : d.rows.filter((r) => rowTier(r) === 'routine') };
		})
	);
	let sheet = $state<HistoryRow | null>(null);
	let open = $state<Record<string, boolean>>({});
	function cardIn(node: Element): TransitionConfig {
		return fade(node, { duration: reducedMotion() ? 0 : DUR.slow, easing: easeOut });
	}
</script>

{#snippet card(r: HistoryRow, quiet: boolean)}
	{@const e = r.head}
	{@const v = headVerb(r)}
	{@const t = titleById.get(e.TitleID ?? 0)}
	<li class="card {v.tone}" class:quiet in:cardIn>
		<button type="button" class="hit" data-testid="look-row" onclick={() => (sheet = r)}>
			<span class="art" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={18} /></span>{/if}</span>
			<span class="body">
				<span class="vrow"><VerbMark verb={v} size="sm" /><time datetime={e.At}>{clock(e.At)}</time></span>
				<span class="nm">{e.title_name ?? 'HolyArrs'}</span>
				{#if e.episode || e.version}<span class="sub">{[e.episode, e.version].filter(Boolean).join(' · ')}</span>{/if}
				<span class="sum">{rowWhy(r) || e.Summary}</span>
				<span class="by">{byWhom(e)}{r.events.length > 1 ? ` · ${r.events.length} steps` : ''}</span>
			</span>
		</button>
	</li>
{/snippet}

<LookFrame {feed} {empty} {onall} onretry={reload} onmore={() => feed.load(filter, kinds, feed.offset + SIZE, true)}>
	<div class="days" data-testid="look-cards">
		{#each days as d (d.key)}
			<section aria-labelledby="cd-{d.key}">
				<header class="dh"><h2 id="cd-{d.key}">{dayLabel(d.key)}</h2><p class="dl">{dayLine(d.tally)}</p></header>
				{#if d.cards.length}
					<ul class="grid">{#each d.cards as r (r.key)}{@render card(r, rowTier(r) === 'routine')}{/each}</ul>
				{/if}
				{#if d.quiet.length}
					{@const on = !!open[d.key]}
					<button type="button" class="quietline" aria-expanded={on} onclick={() => (open = { ...open, [d.key]: !on })}>
						<span class="qn">{d.quiet.length}</span>
						<span class="ql">{on ? 'Routine work' : burstLine(d.quiet.flatMap((r) => r.events), titleById)}</span>
						<span class="chev" class:on aria-hidden="true"><Icon name="chevronDown" size={16} /></span>
					</button>
					{#if on}<ul class="grid">{#each d.quiet as r (r.key)}{@render card(r, true)}{/each}</ul>{/if}
				{/if}
			</section>
		{/each}
	</div>
</LookFrame>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.days { display: flex; flex-direction: column; gap: 24px; }
	section { display: flex; flex-direction: column; gap: 10px; }
	.dh { display: flex; align-items: baseline; justify-content: space-between; gap: 4px 12px; flex-wrap: wrap; }
	.dh h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.dl { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr)); gap: 10px; margin: 0; padding: 0; list-style: none; }
	.card { min-width: 0; border-radius: var(--r-card); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); transition: box-shadow var(--dur-slow) var(--ease-out), translate var(--dur-slow) var(--ease-out); }
	.card:hover { box-shadow: var(--shadow-lift); translate: 0 -1px; }
	.card.fail { box-shadow: inset 3px 0 0 var(--fail), var(--shadow-card); }
	.card.fail:hover { box-shadow: inset 3px 0 0 var(--fail), var(--shadow-lift); }
	.hit { width: 100%; height: 100%; display: grid; grid-template-columns: 64px minmax(0, 1fr); gap: 12px; padding: 10px; border: 0; border-radius: inherit; background: none; color: inherit; text-align: left; font: var(--t-callout); cursor: pointer; }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.art { width: 64px; align-self: start; --poster-ratio: 2 / 3; }
	.art.square { --poster-ratio: 1; }
	.art :global(.poster) { border-radius: 6px; }
	.sys { display: grid; place-items: center; width: 64px; aspect-ratio: 2 / 3; border-radius: 6px; background: var(--surface-2); color: var(--text-muted); }
	.body { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.vrow { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; }
	.vrow time { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.nm { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: var(--t-headline); }
	.sub { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sum { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.by { margin-top: auto; padding-top: 2px; color: var(--text-faint); font: var(--t-caption); }
	.quiet .nm { font: var(--t-callout); font-weight: 600; color: var(--text-muted); }
	.quietline { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; width: 100%; min-height: 44px; padding: 0 12px; border: 0; border-radius: var(--r-card); background: var(--surface-2); color: var(--text-muted); text-align: left; font: var(--t-footnote); cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.quietline:hover { background: var(--surface-3); color: var(--text); }
	.quietline:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.qn { display: grid; place-items: center; min-width: 26px; height: 22px; padding: 0 6px; border-radius: 6px; background: var(--surface-1); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }
	.ql { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.chev { display: grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out); }
	.chev.on { transform: rotate(180deg); }
	@media (max-width: 767px) { .hit { grid-template-columns: 52px minmax(0, 1fr); } .art, .sys { width: 52px; } }
	@media (prefers-reduced-motion: reduce) { .card, .quietline, .chev { transition: none; } .card:hover { translate: none; } }
</style>
