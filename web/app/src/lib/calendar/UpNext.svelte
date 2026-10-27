<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { byDay, entryKey } from '#lib/calendar.ts';
	import { dayName } from '#lib/calendar-looks.ts';
	import CalCard from './CalCard.svelte';
	import CalEmpty from './CalEmpty.svelte';

	// 1 · Up Next: a vertical agenda of the days that have something, each
	// a row of 16:9 still cards. Quiet days fold away; Today always shows.
	let { entries, today, from, to: until, onlater }: { entries: CalendarEntry[]; today: string; from: string; to: string; onlater: () => void } = $props();

	const days = $derived.by(() => {
		const d = byDay(entries);
		if (today >= from && today < until && !d.some((x) => x.day === today)) {
			const i = d.findIndex((x) => x.day > today);
			d.splice(i < 0 ? d.length : i, 0, { day: today, items: [] });
		}
		return d;
	});
	const busy = $derived(days.some((d) => d.items.length));
	const count = (n: number) => (n === 1 ? '1 date' : `${n} dates`);
</script>

{#if !busy}
	<CalEmpty {onlater} />
{/if}
<ol class="up" data-testid="cal-look-1">
	{#each days as d (d.day)}
		{@const past = d.day < today}
		<li class="day" class:today={d.day === today} class:past>
			<header>
				<h3>{dayName(d.day, today)}</h3>
				{#if d.items.length}<span class="n">{count(d.items.length)}</span>{/if}
			</header>
			{#if d.items.length}
				<ul class="cards" class:solo={d.items.length === 1}>
					{#each d.items as e (entryKey(e))}<li><CalCard {e} {today} all={entries} /></li>{/each}
				</ul>
			{:else}
				<p class="quiet">Nothing new today.</p>
			{/if}
		</li>
	{/each}
</ol>

<style>
	.up { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 34px; }
	.day { display: grid; grid-template-columns: 180px minmax(0, 1fr); gap: 24px; align-items: start; }
	header { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; gap: 2px; padding-top: 4px; }
	h3 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.today h3 { color: var(--accent-text); }
	.past h3 { color: var(--text-muted); }
	.n { color: var(--text-muted); font: var(--t-footnote); }
	.cards { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 22px 18px; }
	.cards > li { min-width: 0; }
	.past .cards { opacity: 0.72; }
	.quiet { color: var(--text-muted); font: var(--t-callout); padding-top: 8px; }
	@media (max-width: 767px) {
		.up { gap: 26px; }
		.day { grid-template-columns: minmax(0, 1fr); gap: 10px; }
		header { position: static; flex-direction: row; align-items: baseline; gap: 10px; }
		h3 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
		/* Phones: a busy day is a sideways row of cards, one and a bit on screen. */
		.cards { display: flex; gap: 12px; overflow-x: auto; scroll-snap-type: x mandatory; scrollbar-width: none; margin: 0 calc(-1 * var(--gutter, 16px)); padding: 0 var(--gutter, 16px); scroll-padding: 0 var(--gutter, 16px); }
		.cards::-webkit-scrollbar { display: none; }
		.cards > li { flex: 0 0 82%; scroll-snap-align: start; }
		.cards.solo > li { flex-basis: 100%; }
		.quiet { padding-top: 0; }
	}
</style>
