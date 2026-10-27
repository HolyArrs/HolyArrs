<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { byDay, shiftWeek, ymd } from '#lib/calendar.ts';
	import { groupDates, thingCount, weekTitle, type CalGroup } from '#lib/calendar-looks.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import CalTile from './CalTile.svelte';

	// 2 · Coming Soon, Week: the Day view grouped by week. Each week is one
	// row of the Day view's cards in date order, each card carrying its day
	// ("Wed 28"); a busy week scrolls sideways. Weeks follow one another down
	// the page, and more load as you reach the end.
	let {
		entries,
		today,
		start,
		weeks
	}: {
		entries: CalendarEntry[];
		today: string;
		/** The Monday of the first week. */
		start: Date;
		/** How many weeks are loaded. */
		weeks: number;
	} = $props();

	const rows = $derived.by(() => {
		const days = byDay(entries);
		return Array.from({ length: weeks }, (_, i) => {
			const from = shiftWeek(start, i);
			const a = ymd(from);
			const b = ymd(shiftWeek(start, i + 1));
			const items = days.filter((d) => d.day >= a && d.day < b).flatMap((d) => d.items);
			return { key: a, title: weekTitle(from, today), count: items.length, groups: groupDates(items) };
		});
	});
</script>

<div class="cw" data-testid="cal-span-week">
	{#each rows as r (r.key)}
		{#if r.groups.length}
			<div class="wk" data-week={r.key}>
				<Shelf title={r.title} subtitle={thingCount(r.count)} item="clamp(270px, 25vw, 340px)" items={r.groups} key={(g: CalGroup) => g.key} placeholder="wide" lines={2}>
					{#snippet card(g: CalGroup)}<CalTile {g} {today} chip />{/snippet}
				</Shelf>
			</div>
		{:else}
			<section class="wk quiet" data-week={r.key} aria-label={r.title}>
				<h2>{r.title}</h2>
				<p>Nothing this week</p>
			</section>
		{/if}
	{/each}
</div>

<style>
	.cw { display: flex; flex-direction: column; gap: 34px; min-width: 0; }
	.quiet h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); color: var(--text-muted); }
	.quiet p { margin-top: 2px; color: var(--text-faint); font: var(--t-callout); }
	@media (max-width: 767px) {
		.cw { gap: 26px; }
	}
</style>
