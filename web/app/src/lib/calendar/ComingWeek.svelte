<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryArt, entryHref, entryKey, itemDot, ymd } from '#lib/calendar.ts';
	import { dayName, entryFacts, entryTitle, stripDays } from '#lib/calendar-looks.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';

	// 2 · Coming Soon, Week: one week at a time, the seven days side by side
	// as columns of picture cards (episode stills, movie backdrops, album
	// covers). Empty days keep only their date. Narrow screens stack the days.
	let {
		entries,
		today,
		start,
		dir = 0,
		onday
	}: {
		entries: CalendarEntry[];
		today: string;
		/** The Monday of the week shown. */
		start: Date;
		/** The last step: -1 back, 1 on, 0 none; the new week slides in from that side. */
		dir?: number;
		/** Open a day in the day-by-day list. */
		onday: (day: string) => void;
	} = $props();

	const days = $derived(stripDays(start, 7, entries, today));
	const busy = $derived(days.some((d) => d.items.length));
	let failed = $state<Record<string, boolean>>({});
</script>

{#snippet card(e: CalendarEntry, past: boolean)}
	{@const src = e.kind === 'album' ? (e.poster ?? null) : entryArt(e)}
	{@const k = entryKey(e)}
	<li class="cw-card" class:past>
		<a href={to(entryHref(e))} class="art" class:album={e.kind === 'album'} aria-label="{entryTitle(e)}, {entryFacts(e)}">
			{#if src && !failed[k]}
				<img class="bg" src={sized(src, e.kind === 'album' ? 'poster' : 'backdrop', 240)} alt="" loading="lazy" decoding="async" onerror={() => (failed[k] = true)} />
				{#if e.kind === 'album'}<img class="cover" src={sized(src, 'poster', 90)} alt="" loading="lazy" decoding="async" />{/if}
			{:else}
				<ArtPlaceholder name={entryTitle(e)} shape="wide" />
			{/if}
			{#if itemDot(e, today) === 'have'}<span class="have" title="In the Library" aria-hidden="true"></span>{/if}
		</a>
		<a href={to(entryHref(e))} class="name" tabindex="-1" aria-hidden="true">{entryTitle(e)}</a>
		<span class="facts" aria-hidden="true">{entryFacts(e)}</span>
	</li>
{/snippet}

<div class="cw-wrap" data-testid="cal-span-week">
	{#key ymd(start)}
		<ol class="cw" style:--dir={dir} aria-label="Week of {dayName(ymd(start), today)}">
			{#each days as d (d.day)}
				<li class="col" class:today={d.today} class:empty={!d.items.length} class:past={d.past} data-day={d.day}>
					<button type="button" class="stamp" aria-label="{dayName(d.day, today)}: show it in the day-by-day list" onclick={() => onday(d.day)}>
						<span class="dow">{d.today ? 'Today' : d.dow}</span>
						<span class="big">{d.date}</span>
					</button>
					{#if d.items.length}
						<ul class="cards">{#each d.items as e (entryKey(e))}{@render card(e, d.past)}{/each}</ul>
					{:else}
						<p class="none">Nothing</p>
					{/if}
				</li>
			{/each}
		</ol>
	{/key}
	{#if !busy}<p class="quiet">Nothing this week with the filters picked.</p>{/if}
</div>

<style>
	.cw-wrap { container-type: inline-size; }
	.cw { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 12px; animation: cw-in var(--dur-slow) var(--ease-out) both; }
	@keyframes cw-in { from { opacity: 0; transform: translateX(calc(var(--dir, 0) * 28px)); } }
	.col { min-width: 0; display: flex; flex-direction: column; gap: 12px; padding: 10px 8px 14px; border-radius: var(--r-panel); }
	.col.today { background: color-mix(in srgb, var(--accent-fill) 9%, transparent); outline: 1px solid color-mix(in srgb, var(--accent-fill) 35%, transparent); outline-offset: -1px; }
	.today .big { color: var(--accent-text); }
	.stamp { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 2px 4px; border-radius: var(--r-sm); color: var(--text); text-align: left; line-height: 1; transition: background var(--dur-fast) var(--ease-out); }
	.stamp:hover { background: var(--surface-2); }
	.stamp:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dow { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; }
	.big { font: 700 34px/1 var(--font-display); letter-spacing: -0.03em; font-variant-numeric: tabular-nums; }
	.today .dow { color: var(--accent-text); }
	.empty:not(.today) .big, .past:not(.today) .big { color: var(--text-muted); }
	.none { padding: 0 4px; color: var(--text-faint); font: var(--t-footnote); }
	.cards { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 16px; }
	.cw-card { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.cw-card.past { opacity: 0.75; }
	.art { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur-slow) var(--ease-out), box-shadow var(--dur-slow) var(--ease-out); }
	.art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.art > :global(.ui-art-ph) { position: absolute; inset: 0; }
	.bg { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.album .bg { filter: blur(18px) saturate(140%) brightness(0.7); transform: scale(1.25); }
	.cover { position: absolute; left: 50%; top: 50%; translate: -50% -50%; height: 78%; aspect-ratio: 1; object-fit: cover; border-radius: 5px; box-shadow: 0 6px 18px rgb(0 0 0 / 0.45); }
	.have { position: absolute; right: 6px; top: 6px; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 2px rgb(0 0 0 / 0.35); }
	.name { color: var(--text); font: var(--t-subhead); font-weight: 600; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.name:hover { text-decoration: underline; text-underline-offset: 3px; }
	.facts { margin-top: -4px; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.quiet { margin-top: 12px; color: var(--text-muted); font: var(--t-callout); }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.art:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	}
	@media (prefers-reduced-motion: reduce) {
		.cw { animation: none; }
	}

	/* Too narrow for seven columns: the days stack, the date beside its cards. */
	@container (max-width: 859px) {
		.cw { grid-template-columns: minmax(0, 1fr); gap: 4px; }
		.col { display: grid; grid-template-columns: 52px minmax(0, 1fr); align-items: start; gap: 12px; padding: 10px 8px; }
		.stamp { align-items: center; }
		.big { font-size: 28px; }
		.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 14px 12px; }
		.none { padding-top: 14px; }
		.col.empty { padding-block: 4px; }
		.col.empty .big { font-size: 20px; }
		.col.empty .none { padding-top: 6px; }
	}
</style>
