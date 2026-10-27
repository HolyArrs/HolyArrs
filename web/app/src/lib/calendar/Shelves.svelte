<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { entryKey } from '#lib/calendar.ts';
	import { shelves } from '#lib/calendar-looks.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import CalCard from './CalCard.svelte';
	import CalEmpty from './CalEmpty.svelte';

	// 4 · Shelves: Netflix-style rows by when (Today, This week, Next week,
	// Later this month…) and by kind (Coming to digital, Coming on disc, New
	// albums). Each card carries its date chip; a row with nothing is left out.
	let { entries, today, onlater }: { entries: CalendarEntry[]; today: string; onlater: () => void } = $props();

	const rows = $derived(shelves(entries, today));
	const width = { wide: 'clamp(260px, 26vw, 360px)', poster: 'clamp(138px, 12vw, 180px)', square: 'clamp(150px, 13vw, 196px)' } as const;
</script>

{#if !rows.length}
	<CalEmpty {onlater} />
{:else}
	<div class="sh" data-testid="cal-look-4">
		{#each rows as r (r.key)}
			<Shelf title={r.title} subtitle={r.items.length === 1 ? '1 date' : `${r.items.length} dates`} item={width[r.shape]} items={r.items} key={(e) => entryKey(e)} placeholder={r.shape === 'wide' ? 'wide' : r.shape} lines={2}>
				{#snippet card(e)}<CalCard {e} {today} all={entries} shape={r.shape} chip />{/snippet}
			</Shelf>
		{/each}
	</div>
{/if}

<style>
	.sh { display: flex; flex-direction: column; gap: 30px; min-width: 0; }
</style>
