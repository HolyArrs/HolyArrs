<script lang="ts">
	import { onMount, tick } from 'svelte';
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryHref, entryKey, monthGrid } from '#lib/calendar.ts';
	import { dayDots, dayName, entryFacts, entryTitle, fitThumbs } from '#lib/calendar-looks.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import CalCard from './CalCard.svelte';

	// 2 · Coming Soon, Month: the whole month on one screen. Each day holds as
	// many small posters (album covers square) as its tile fits, then "+N",
	// which opens the day in the day-by-day list, as does the date. Empty days
	// keep only a quiet number; today is filled in. On wide screens the grid
	// fills the window height, so the page never scrolls. Narrow screens get
	// a compact grid of kind dots: tapping a day lists its dates under it.
	let {
		entries,
		today,
		year,
		month,
		dir = 0,
		onday
	}: {
		entries: CalendarEntry[];
		today: string;
		year: number;
		month: number;
		/** The last step: -1 back, 1 on, 0 none; the new month slides in from that side. */
		dir?: number;
		/** Open a day in the day-by-day list. */
		onday: (day: string) => void;
	} = $props();

	const weeks = $derived(monthGrid(year, month, entries, today));
	const cells = $derived(weeks.flat());
	const heads = $derived(Array.from({ length: 7 }, (_, i) => new Date(2024, 0, 1 + i).toLocaleDateString(undefined, { weekday: 'short' })));
	const monthKey = $derived(`${year}-${String(month + 1).padStart(2, '0')}`);
	const title = $derived(new Date(year, month, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }));
	const busy = $derived(cells.some((c) => c.inMonth && c.items.length));
	const count = (k: number) => (k === 1 ? '1 date' : `${k} dates`);
	const thumb = (e: CalendarEntry) => e.poster || e.backdrop || e.still || null;

	// The wrapper's width picks the layout; the first tile's size sets the poster height.
	let width = $state(0);
	const compact = $derived(width > 0 && width < 700);
	let wrap = $state<HTMLDivElement>();
	let tile = $state<HTMLElement>();
	let fitH = $state<number | null>(null);
	let tileW = $state(0);
	let tileH = $state(0);
	// The grid ends this far above the window's bottom: room for the layout pill bar.
	const ROOM = 80;
	const PAD = 6;
	const NUM = 28;
	const GAP = 4;
	const MORE = 30;
	const posterH = $derived(Math.max(26, Math.min(132, tileH - PAD * 2 - NUM - GAP)));
	function shownCount(items: CalendarEntry[]): number {
		const widths = items.map((e) => (e.kind === 'album' ? posterH : Math.round((posterH * 2) / 3)));
		return fitThumbs(widths, tileW - PAD * 2, GAP, MORE);
	}

	function fit() {
		if (!wrap || compact) {
			fitH = null;
			return;
		}
		const top = wrap.getBoundingClientRect().top + window.scrollY;
		fitH = Math.max(420, Math.floor(window.innerHeight - top - ROOM));
	}
	onMount(() => {
		const again = () => void tick().then(fit);
		again();
		window.addEventListener('resize', again);
		const ro = new ResizeObserver(again);
		if (wrap?.parentElement) ro.observe(wrap.parentElement);
		return () => {
			window.removeEventListener('resize', again);
			ro.disconnect();
		};
	});
	$effect(() => {
		void compact;
		void weeks.length;
		void tick().then(fit);
	});
	$effect(() => {
		if (!tile) return;
		const el = tile;
		const ro = new ResizeObserver(() => {
			tileW = el.clientWidth;
			tileH = el.clientHeight;
		});
		ro.observe(el);
		return () => ro.disconnect();
	});

	// Narrow screens: the picked day's dates show under the grid.
	let picked = $state<string | null>(null);
	$effect(() => {
		if (picked && picked.startsWith(monthKey)) return;
		// Today when it has dates, else the next day that does, else the month's first busy day.
		const mine = cells.filter((c) => c.inMonth);
		const now = mine.find((c) => c.today);
		picked = (now?.items.length ? now.day : null) ?? mine.find((c) => c.day > today && c.items.length)?.day ?? now?.day ?? mine.find((c) => c.items.length)?.day ?? null;
	});
	const day = $derived(cells.find((c) => c.day === picked) ?? null);
</script>

<div class="cm-wrap" class:fitted={!compact && fitH} bind:this={wrap} bind:clientWidth={width} style:height={!compact && fitH ? `${fitH}px` : null} data-testid="cal-span-month">
	{#key monthKey}
		{#if !compact}
			<div class="cm" style:--dir={dir} style:--weeks={weeks.length} style:--ph="{posterH}px" role="grid" aria-label={title}>
				<div class="head" role="row">
					{#each heads as h, i (i)}<span role="columnheader">{h}</span>{/each}
				</div>
				{#each weeks as w, wi (w[0].day)}
					<div class="wk" role="row">
						{#each w as c, ci (c.day)}
							{@const n = c.items.length ? shownCount(c.items) : 0}
							{#if wi === 0 && ci === 0}
								<div class="tile" class:busy={c.items.length} class:out={!c.inMonth} class:past={c.day < today} class:today={c.today} role="gridcell" data-day={c.day} bind:this={tile}>
									{@render tileBody(c, n)}
								</div>
							{:else}
								<div class="tile" class:busy={c.items.length} class:out={!c.inMonth} class:past={c.day < today} class:today={c.today} role="gridcell" data-day={c.day}>
									{@render tileBody(c, n)}
								</div>
							{/if}
						{/each}
					</div>
				{/each}
			</div>
		{:else}
			<div class="dots-grid" style:--dir={dir} role="grid" aria-label={title}>
				<div class="head" role="row">
					{#each heads as h, i (i)}<span role="columnheader">{h.slice(0, 1)}</span>{/each}
				</div>
				{#each weeks as w (w[0].day)}
					<div class="wk" role="row">
						{#each w as c (c.day)}
							<button
								type="button"
								role="gridcell"
								class="dcell"
								class:out={!c.inMonth}
								class:past={c.day < today}
								class:today={c.today}
								class:on={picked === c.day}
								aria-selected={picked === c.day}
								aria-label="{dayName(c.day, today)}{c.items.length ? `, ${count(c.items.length)}` : ', nothing'}"
								onclick={() => (picked = c.day)}
							>
								<span class="num">{c.date}</span>
								<span class="kdots" aria-hidden="true">{#each dayDots(c.items) as k (k)}<i class="k-{k}"></i>{/each}</span>
							</button>
						{/each}
					</div>
				{/each}
			</div>
		{/if}
	{/key}
</div>

{#snippet tileBody(c: { day: string; date: number; items: CalendarEntry[] }, n: number)}
	<button type="button" class="num" aria-label="{dayName(c.day, today)}{c.items.length ? `, ${count(c.items.length)}` : ''}: show it in the day-by-day list" onclick={() => onday(c.day)}>{c.date}</button>
	{#if c.items.length}
		<div class="thumbs">
			{#each c.items.slice(0, n) as e (entryKey(e))}
				{@const src = thumb(e)}
				<a class="p" class:sq={e.kind === 'album'} href={to(entryHref(e))} title={entryTitle(e)} aria-label="{entryTitle(e)}, {entryFacts(e)}">
					{#if src}<img src={sized(src, 'poster', Math.round((posterH * 2) / 3))} alt="" loading="lazy" decoding="async" />{:else}<span class="ini" aria-hidden="true">{entryTitle(e).slice(0, 1)}</span>{/if}
				</a>
			{/each}
			{#if n < c.items.length}
				<button type="button" class="more" aria-label="{c.items.length - n} more on {dayName(c.day, today)}: show the day" onclick={() => onday(c.day)}>+{c.items.length - n}</button>
			{/if}
		</div>
	{/if}
{/snippet}

{#if !busy}<p class="quiet">Nothing this month with the filters picked.</p>{/if}

{#if compact && day}
	<section class="picked" aria-live="polite" aria-label={dayName(day.day, today)}>
		<header><h3>{dayName(day.day, today)}</h3>{#if day.items.length}<span class="n">{count(day.items.length)}</span>{/if}</header>
		{#if day.items.length}
			<ul class="cards">
				{#each day.items as e (entryKey(e))}<li><CalCard {e} {today} all={entries} /></li>{/each}
			</ul>
		{:else}
			<p class="quiet">Nothing on this day.</p>
		{/if}
	</section>
{/if}

<style>
	.cm-wrap { min-width: 0; }
	/* Fitted to the window: swallow the room the page keeps under itself, so nothing scrolls. */
	.cm-wrap.fitted { margin-bottom: calc(var(--s-6) - var(--s-16) - 84px); }
	.cm { height: 100%; display: grid; grid-template-rows: auto repeat(var(--weeks), minmax(0, 1fr)); gap: 6px; animation: cm-in var(--dur-slow) var(--ease-out) both; }
	@keyframes cm-in { from { opacity: 0; transform: translateX(calc(var(--dir, 0) * 28px)); } }
	.head, .wk { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 6px; min-height: 0; }
	.head span { padding: 0 8px 2px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; }
	.tile { position: relative; min-width: 0; min-height: 0; padding: 6px; display: flex; flex-direction: column; gap: 4px; border-radius: var(--r-card); overflow: hidden; }
	.tile.busy { background: var(--surface-1); box-shadow: var(--shadow-card); outline: 1px solid var(--line); outline-offset: -1px; }
	.tile.today { background: color-mix(in srgb, var(--accent-fill) 9%, var(--surface-1)); outline: 2px solid var(--accent-fill); outline-offset: -2px; }
	.num { align-self: flex-start; flex: none; display: inline-grid; place-items: center; min-width: 28px; height: 28px; padding: 0 6px; border-radius: 999px; color: var(--text); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; transition: background var(--dur-fast) var(--ease-out); }
	.num:hover { background: var(--surface-3); }
	.num:focus-visible, .p:focus-visible, .more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tile:not(.busy) .num { color: var(--text-muted); }
	.tile.out .num { color: var(--text-faint); }
	.tile.today .num { background: var(--accent-fill); color: var(--text-on-red); }
	.thumbs { flex: 1; min-height: 0; display: flex; align-items: flex-end; gap: 4px; }
	.p { position: relative; flex: none; height: var(--ph); aspect-ratio: 2 / 3; border-radius: 6px; overflow: hidden; background: var(--surface-3); box-shadow: 0 2px 8px rgb(0 0 0 / 0.28); transition: transform var(--dur-slow) var(--ease-out); }
	.p.sq { aspect-ratio: 1; }
	.p img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.ini { position: absolute; inset: 0; display: grid; place-items: center; color: var(--text-muted); font: var(--t-headline); }
	.past:not(.today) .p { opacity: 0.55; filter: saturate(0.6); }
	.out .p { opacity: 0.45; }
	.more { flex: none; align-self: flex-end; min-width: 30px; height: 26px; padding: 0 7px; border-radius: 999px; background: var(--surface-3); color: var(--text); font: var(--t-caption); font-weight: 700; font-variant-numeric: tabular-nums; transition: background var(--dur-fast) var(--ease-out); }
	.more:hover { background: var(--accent-soft); }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.p:hover { transform: translateY(var(--lift)); }
	}
	.quiet { margin-top: 12px; color: var(--text-muted); font: var(--t-callout); }

	/* Narrow: a compact grid of dates with kind dots; the picked day lists below. */
	.dots-grid { display: flex; flex-direction: column; gap: 2px; animation: cm-in var(--dur-slow) var(--ease-out) both; }
	.dots-grid .head { gap: 0; }
	.dots-grid .head span { padding: 6px 0; text-align: center; font-size: 11px; }
	.dots-grid .wk { gap: 0; }
	.dcell { min-width: 0; min-height: 50px; padding: 4px 0; display: flex; flex-direction: column; align-items: center; gap: 4px; border-radius: var(--r-card); color: var(--text); transition: background var(--dur-fast) var(--ease-out); }
	.dcell .num { min-width: 30px; height: 30px; padding: 0 4px; font-size: 14px; }
	.dcell.out .num { color: var(--text-faint); }
	.dcell.past:not(.today) .num { color: var(--text-muted); }
	.dcell.today .num { background: var(--accent-fill); color: var(--text-on-red); }
	.dcell.on { background: var(--surface-2); }
	.dcell:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.kdots { display: flex; gap: 3px; min-height: 6px; }
	i { display: inline-block; width: 6px; height: 6px; border-radius: 50%; }
	.k-episode { background: var(--ok); }
	.k-movie { background: var(--info); }
	.k-album { background: var(--wait); }
	.picked { margin-top: 18px; display: flex; flex-direction: column; gap: 14px; }
	.picked header { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
	.picked h3 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.n { color: var(--text-muted); font: var(--t-footnote); }
	.cards { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 18px; }
	.cards > li { min-width: 0; }
	@media (prefers-reduced-motion: reduce) {
		.cm, .dots-grid { animation: none; }
	}
</style>
