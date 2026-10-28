<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryArt, entryHref, entryKey, itemDot, type Follows } from '#lib/calendar.ts';
	import { entryLine, entryTitle, groupDates, groupFacts, groupTag, monthGroups, movieDates, thingCount, type CalGroup } from '#lib/calendar-looks.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import CalTile from './CalTile.svelte';
	import { sized } from '#lib/tmdb-image.ts';
	import Badge from '#lib/ui/Badge.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import CalEmpty from './CalEmpty.svelte';
	import FollowWho from './FollowWho.svelte';

	// Cards: one day per row. A big date stamp on the left (the
	// day name over a large number, once per day) and a large backdrop card
	// beside it with the name on the art, a short line and an Open button.
	// Month names divide the list.
	// mode says which follows are shown: Everyone's follows names who follows each title;
	// onfollow follows or stops following a title from its card.
	let {
		entries,
		today,
		onlater,
		mode = 'library',
		onfollow
	}: { entries: CalendarEntry[]; today: string; onlater: () => void; mode?: Follows; onfollow?: (e: CalendarEntry, on: boolean) => void } = $props();

	const months = $derived(monthGroups(entries));
	const openWord = (e: CalendarEntry) => (e.kind === 'episode' ? 'Open show' : e.kind === 'album' ? 'Open album' : 'Open movie');
	const at = (day: string) => new Date(day + 'T00:00:00');
	let failed = $state<Record<string, boolean>>({});
</script>

{#snippet card(g: CalGroup)}
	{@const e = g.lead}
	{@const n = g.entries.length}
	{@const src = e.kind === 'album' ? e.poster : entryArt(e)}
	{@const k = entryKey(e)}
	{@const line = n === 1 ? entryLine(e) : ''}
	{@const others = movieDates(e, entries)}
	{@const have = g.entries.every((x) => itemDot(x, today) === 'have')}
	<article class="cs-card" class:album={e.kind === 'album'} class:stacked={n > 1}>
		<div class="stack">
		{#if n > 1}<span class="layer l2" aria-hidden="true"></span><span class="layer l1" aria-hidden="true"></span>{/if}
		<a class="art" href={to(entryHref(e))} aria-label={entryTitle(e)}>
			{#if src && !failed[k]}
				<img class="bg" src={sized(src, e.kind === 'album' ? 'poster' : 'backdrop', 780)} alt="" loading="lazy" decoding="async" onerror={() => (failed[k] = true)} />
				{#if e.kind === 'album'}<img class="cover" src={sized(src, 'poster', 220)} alt="" loading="lazy" decoding="async" />{/if}
			{:else}
				<ArtPlaceholder name={entryTitle(e)} shape="wide" />
			{/if}
			<span class="scrim" aria-hidden="true"></span>
			<span class="on-art" aria-hidden="true">
				<span class="tag"><Badge label={groupTag(g)} on="art" /></span>
				<span class="name">{entryTitle(e)}</span>
			</span>
		</a>
		</div>
		<div class="text">
			<h4><a href={to(entryHref(e))}>{entryTitle(e)}</a></h4>
			<p class="facts">{groupFacts(g)}{#if have}<span class="have"> · In the Library</span>{/if}</p>
			{#if line}<p class="line">{line}</p>{/if}
			{#if others}<p class="also">Also {others}</p>{/if}
			{#if mode === 'everyone'}<p class="who"><FollowWho {e} /></p>{/if}
			<span class="open">
				<Button variant="secondary" size="sm" href={to(entryHref(e))} iconAfter="chevronRight">{openWord(e)}</Button>
				{#if onfollow}<Button variant="plain" size="sm" icon={e.following ? 'bellOn' : 'bell'} class="cs-follow{e.following ? ' on' : ''}" aria-pressed={!!e.following} aria-label="{e.following ? 'Unfollow' : 'Follow'} {entryTitle(e)}" onclick={() => onfollow(e, !e.following)} data-testid="cal-follow">{e.following ? 'Following' : 'Follow'}</Button>{/if}
			</span>
		</div>
	</article>
{/snippet}

{#if !months.length}
	<CalEmpty {onlater} />
{:else}
	<div class="cs" data-testid="cal-cards">
		{#each months as m (m.month)}
			<section class="month" aria-label={m.name}>
				<h3 class="divider"><span>{m.name}</span></h3>
				<ol class="rows">
					{#each m.days as d (d.day)}
						{@const gs = groupDates(d.items)}
						<li class="row" class:busy={gs.length > 1} data-day={d.day} class:today={d.day === today} class:past={d.day < today}>
							<div class="stamp">
								<span class="dow">{at(d.day).toLocaleDateString(undefined, { weekday: 'short' })}</span>
								<span class="big">{at(d.day).getDate()}</span>
								<span class="mon">{at(d.day).toLocaleDateString(undefined, { month: 'short' })}</span>
								{#if d.day === today}<span class="today-tag"><Badge label="Today" /></span>{/if}
							</div>
							{#if gs.length > 1}
								<!-- A busy day: one row of cards that scrolls sideways, its count on top. -->
								<div class="many">
									<Shelf title={thingCount(d.items.length)} item="clamp(260px, 24vw, 320px)" items={gs} key={(g: CalGroup) => g.key} placeholder="wide" lines={2}>
										{#snippet card(g: CalGroup)}<CalTile {g} {today} {mode} {onfollow} />{/snippet}
									</Shelf>
								</div>
							{:else}
								{@render card(gs[0])}
							{/if}
						</li>
					{/each}
				</ol>
			</section>
		{/each}
	</div>
{/if}

<style>
	.cs { display: flex; flex-direction: column; gap: 36px; max-width: 1120px; container-type: inline-size; }
	.divider { display: flex; align-items: center; gap: 14px; margin: 0 0 18px; font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.divider::after { content: ''; flex: 1; height: 1px; background: var(--line); }
	.rows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 34px; }
	.row { display: grid; grid-template-columns: 96px minmax(0, 1fr); gap: 20px; align-items: start; }
	.many { min-width: 0; }
	/* The row stops at the column's right edge, so it never runs under the small month beside it. */
	.many :global(.ui-shelf .row) { margin-right: 0; padding-right: 0; }
	.many :global(.ui-shelf h2) { font: var(--t-title3); letter-spacing: var(--t-title3-track); color: var(--text-muted); }
	/* Several episodes of one show: the card sits on a small stack. */
	.stack { position: relative; }
	.stacked .stack { padding-top: 10px; }
	.layer { position: absolute; border-radius: var(--r-panel); background: var(--surface-3); box-shadow: var(--shadow-card); }
	.l1 { top: 5px; left: 3%; right: 3%; height: 60px; opacity: 0.9; }
	.l2 { top: 0; left: 6%; right: 6%; height: 60px; opacity: 0.55; }
	/* With the page's own scroll padding (the top bar), a day picked in the small month lands under the bar like the sticky stamp. */
	.row { scroll-margin-top: 8px; }
	.stamp { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; align-items: flex-start; line-height: 1; }
	.dow { color: var(--text-muted); font: var(--t-subhead); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; }
	.big { margin-top: 4px; font: 700 64px/0.95 var(--font-display); letter-spacing: -0.04em; font-variant-numeric: tabular-nums; }
	.mon { margin-top: 4px; color: var(--text-muted); font: var(--t-footnote); }
	/* Today is told by a small tag under the date, not by colour. */
	.today-tag { margin-top: 6px; display: inline-flex; }
	.past .big { color: var(--text-muted); }

	.cs-card { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.art { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: var(--r-panel); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	.art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.art :global(.ui-art-ph), .art > :global(*:first-child:not(img)) { position: absolute; inset: 0; }
	.bg { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.album .bg { filter: blur(28px) saturate(140%) brightness(0.7); transform: scale(1.2); }
	.cover { position: absolute; right: 6%; top: 50%; translate: 0 -50%; height: 72%; aspect-ratio: 1; object-fit: cover; border-radius: 8px; box-shadow: 0 12px 36px rgb(0 0 0 / 0.5); }
	.scrim { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 0.72), rgb(0 0 0 / 0.1) 55%, transparent); pointer-events: none; }
	.on-art { position: absolute; left: 5%; right: 5%; bottom: 7%; display: flex; flex-direction: column; align-items: flex-start; gap: 10px; }
	.album .on-art { right: 48%; }
	.name { color: #fff; font: 700 clamp(22px, 3.4vw, 40px)/1.02 var(--font-display); letter-spacing: -0.03em; text-shadow: 0 2px 14px rgb(0 0 0 / 0.5); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.art:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	}
	.text { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.open { margin-top: 8px; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
	.who { min-width: 0; display: flex; }
	.open :global(.cs-follow) { color: var(--text-muted); }
	.open :global(.cs-follow.on) { color: var(--accent-text); }
	h4 { min-width: 0; font: var(--t-title3); letter-spacing: var(--t-title3-track); overflow-wrap: anywhere; }
	h4 a { color: var(--text); }
	h4 a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.facts { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.have { color: var(--ok-text, var(--ok)); }
	.line { font: var(--t-callout); color: var(--text); overflow-wrap: anywhere; }
	.also { color: var(--text-muted); font: var(--t-footnote); }
	.past .cs-card { opacity: 0.75; }

	/* Wide screens: the art and its words side by side, so two or three rows fit a screen. */
	@container (min-width: 760px) {
		.cs-card { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(0, 1fr); gap: 26px; align-items: center; }
		.text { gap: 6px; }
	}
	@media (max-width: 767px) {
		.cs { gap: 28px; }
		.rows { gap: 26px; }
		.row { grid-template-columns: 46px minmax(0, 1fr); gap: 12px; }
		.stamp { top: calc(var(--topbar) + 8px); align-items: center; }
		.dow { font-size: 11px; letter-spacing: 0.04em; }
		.big { font-size: 36px; }
		.mon { font-size: 11px; }
		.cs-card { gap: 10px; }
		.art { border-radius: var(--r-card); }
		.name { font-size: clamp(18px, 6vw, 24px); }
		h4 { font: var(--t-headline); }
		.open { margin-top: 4px; }
	}
</style>
