<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryArt, entryDay, entryHref, entryKey, itemDot } from '#lib/calendar.ts';
	import { entryFacts, entryLine, entryTag, entryTitle, monthGroups, movieDates } from '#lib/calendar-looks.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import Badge from '#lib/ui/Badge.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import CalEmpty from './CalEmpty.svelte';

	// 2 · Coming Soon: one date per row. A big date stamp on the left (the
	// day name over a large number, once per day) and a large backdrop card
	// beside it with the name on the art, a short line and an Open button.
	// Month names divide the list.
	let {
		entries,
		today,
		onlater,
		focus = null,
		onfocused
	}: {
		entries: CalendarEntry[];
		today: string;
		onlater: () => void;
		/** A day to bring into view once its dates are shown (from Week or Month); the first row on or after it. */
		focus?: string | null;
		onfocused?: () => void;
	} = $props();
	let list = $state<HTMLDivElement>();
	$effect(() => {
		if (!focus || !list) return;
		const day = focus;
		const rows = [...list.querySelectorAll<HTMLElement>('.row[data-day]')];
		const row = rows.find((r) => (r.dataset.day ?? '') >= day) ?? rows.at(-1);
		const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
		requestAnimationFrame(() => row?.scrollIntoView({ block: 'start', behavior: still ? 'auto' : 'smooth' }));
		onfocused?.();
	});

	const months = $derived(monthGroups(entries));
	const openWord = (e: CalendarEntry) => (e.kind === 'episode' ? 'Open show' : e.kind === 'album' ? 'Open album' : 'Open movie');
	const at = (day: string) => new Date(day + 'T00:00:00');
	let failed = $state<Record<string, boolean>>({});
</script>

{#snippet card(e: CalendarEntry)}
	{@const src = e.kind === 'album' ? e.poster : entryArt(e)}
	{@const k = entryKey(e)}
	{@const line = entryLine(e)}
	{@const others = movieDates(e, entries)}
	{@const have = itemDot(e, today) === 'have'}
	<article class="cs-card" class:album={e.kind === 'album'}>
		<a class="art" href={to(entryHref(e))} aria-label={entryTitle(e)}>
			{#if src && !failed[k]}
				<img class="bg" src={sized(src, e.kind === 'album' ? 'poster' : 'backdrop', 780)} alt="" loading="lazy" decoding="async" onerror={() => (failed[k] = true)} />
				{#if e.kind === 'album'}<img class="cover" src={sized(src, 'poster', 220)} alt="" loading="lazy" decoding="async" />{/if}
			{:else}
				<ArtPlaceholder name={entryTitle(e)} shape="wide" />
			{/if}
			<span class="scrim" aria-hidden="true"></span>
			<span class="on-art" aria-hidden="true">
				<span class="tag"><Badge label={entryTag(e)} on="art" /></span>
				<span class="name">{entryTitle(e)}</span>
			</span>
		</a>
		<div class="text">
			<h4><a href={to(entryHref(e))}>{entryTitle(e)}</a></h4>
			<p class="facts">{entryFacts(e)}{#if have}<span class="have"> · In the Library</span>{/if}</p>
			{#if line}<p class="line">{line}</p>{/if}
			{#if others}<p class="also">Also {others}</p>{/if}
			<span class="open"><Button variant="secondary" size="sm" href={to(entryHref(e))} iconAfter="chevronRight">{openWord(e)}</Button></span>
		</div>
	</article>
{/snippet}

{#if !months.length}
	<CalEmpty {onlater} />
{:else}
	<div class="cs" data-testid="cal-look-2" bind:this={list}>
		{#each months as m (m.month)}
			<section class="month" aria-label={m.name}>
				<h3 class="divider"><span>{m.name}</span></h3>
				<ol class="rows">
					{#each m.days as d (d.day)}
						{#each d.items as e, i (entryKey(e))}
							<li class="row" data-day={d.day} class:today={d.day === today} class:past={entryDay(e) < today} class:more={i > 0}>
								<div class="stamp" aria-hidden={i > 0}>
									{#if i === 0}
										<span class="dow">{d.day === today ? 'Today' : at(d.day).toLocaleDateString(undefined, { weekday: 'short' })}</span>
										<span class="big">{at(d.day).getDate()}</span>
										<span class="mon">{at(d.day).toLocaleDateString(undefined, { month: 'short' })}</span>
									{/if}
								</div>
								{@render card(e)}
							</li>
						{/each}
					{/each}
				</ol>
			</section>
		{/each}
	</div>
{/if}

<style>
	.cs { display: flex; flex-direction: column; gap: 36px; max-width: 980px; }
	.divider { display: flex; align-items: center; gap: 14px; margin: 0 0 18px; font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.divider::after { content: ''; flex: 1; height: 1px; background: var(--line); }
	.rows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 34px; }
	.row { display: grid; grid-template-columns: 96px minmax(0, 1fr); gap: 20px; align-items: start; }
	.row.more { margin-top: -14px; }
	/* With the page's own scroll padding (the top bar), a focused day lands under the bar like the sticky stamp. */
	.row { scroll-margin-top: 8px; }
	.stamp { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; align-items: flex-start; line-height: 1; }
	.dow { color: var(--text-muted); font: var(--t-subhead); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; }
	.big { margin-top: 4px; font: 700 64px/0.95 var(--font-display); letter-spacing: -0.04em; font-variant-numeric: tabular-nums; }
	.mon { margin-top: 4px; color: var(--text-muted); font: var(--t-footnote); }
	.today .dow { color: var(--accent-text); }
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
	.open { margin-top: 8px; }
	h4 { min-width: 0; font: var(--t-title3); letter-spacing: var(--t-title3-track); overflow-wrap: anywhere; }
	h4 a { color: var(--text); }
	h4 a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.facts { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.have { color: var(--ok-text, var(--ok)); }
	.line { font: var(--t-callout); color: var(--text); overflow-wrap: anywhere; }
	.also { color: var(--text-muted); font: var(--t-footnote); }
	.past .cs-card { opacity: 0.75; }

	/* Wide screens: the art and its words side by side, so two or three rows fit a screen. */
	@media (min-width: 1100px) {
		.cs-card { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(0, 1fr); gap: 26px; align-items: center; }
		.text { gap: 6px; }
		.cs { max-width: 1120px; }
	}
	@media (max-width: 767px) {
		.cs { gap: 28px; }
		.rows { gap: 26px; }
		.row { grid-template-columns: 46px minmax(0, 1fr); gap: 12px; }
		.row.more { margin-top: -10px; }
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
