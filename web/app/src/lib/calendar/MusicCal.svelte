<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { entryDay } from '#lib/calendar.ts';
	import AlbumCover from './AlbumCover.svelte';

	// The calendar for music only (Fridays): one heading per release day and a grid of square
	// covers, like new releases. Each day keeps data-day so the page's scrolling (today, the
	// arrows, the small month) still works.
	let { entries, today }: { entries: CalendarEntry[]; today: string } = $props();

	const days = $derived.by(() => {
		const by = new Map<string, CalendarEntry[]>();
		for (const e of entries) {
			const d = entryDay(e);
			by.set(d, [...(by.get(d) ?? []), e]);
		}
		return [...by.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([day, list]) => ({ day, list }));
	});
	const at = (day: string) => new Date(day + 'T00:00:00');
	const dayName = (day: string) => at(day).toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
	const title = (e: CalendarEntry) => e.album ?? e.name;
</script>

<div class="mc" data-testid="cal-music">
	{#each days as d (d.day)}
		<section class="day" class:today={d.day === today} class:past={d.day < today} data-day={d.day}>
			<header class="dh">
				<h3>{d.day === today ? 'Today' : dayName(d.day)}</h3>
				<span>{d.list.length} {d.list.length === 1 ? 'release' : 'releases'}</span>
			</header>
				<ul class="covers">
					{#each d.list as e (e.part_id ?? e.title_id + title(e))}
						<li><AlbumCover {e} {today} />
						</li>
					{/each}
				</ul>
		</section>
	{/each}
</div>

<style>
	.mc { display: flex; flex-direction: column; gap: 40px; }
	.dh { display: flex; align-items: baseline; gap: 12px; margin: 0 0 16px; padding-bottom: 10px; border-bottom: 1px solid var(--line); }
	.dh h3 { margin: 0; font: 800 26px/1.1 var(--font-display); letter-spacing: -.02em; }
	.dh span { color: var(--text-muted); font: 600 14px/1 var(--font-ui); }
	.today .dh h3 { color: var(--accent); }
	.past { opacity: .7; }

	/* Fridays: a grid of square covers. */
	.covers { display: grid; grid-template-columns: repeat(auto-fill, minmax(176px, 1fr)); gap: 24px 18px; margin: 0; padding: 0; list-style: none; }
	@media (max-width: 767px) {
		.mc { gap: 28px; }
		.dh h3 { font-size: 21px; }
		.covers { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 12px; }
	}
</style>
