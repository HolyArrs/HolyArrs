<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import type { Follows } from '#lib/calendar.ts';
	import { groupDates, monthGroups, thingCount } from '#lib/calendar-looks.ts';
	import { DEFAULT_LOOK, type CaptionLook } from '#lib/calendar-captions.ts';
	import CalTile from './CalTile.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import CalEmpty from './CalEmpty.svelte';

	// Cards: one day per row. A big date stamp on the left (the
	// day name over a large number, once per day) and a large backdrop card
	// beside it with the name on the art, a short line and an Open button.
	// Month names divide the list.
	// mode says which follows are shown: Everyone's follows names who follows each title;
	// onfollow follows or stops following a title from its card; look picks the caption lines.
	let {
		entries,
		today,
		onlater,
		mode = 'library',
		look = { ...DEFAULT_LOOK, region: 'global' },
		onfollow
	}: { entries: CalendarEntry[]; today: string; onlater: () => void; mode?: Follows; look?: CaptionLook; onfollow?: (e: CalendarEntry, on: boolean) => void } = $props();

	const months = $derived(monthGroups(entries));
	const at = (day: string) => new Date(day + 'T00:00:00');
</script>


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
						<li class="row" data-day={d.day} class:today={d.day === today} class:past={d.day < today}>
							<div class="stamp">
								<span class="dow">{at(d.day).toLocaleDateString(undefined, { weekday: 'short' })}</span>
								<span class="big">{at(d.day).getDate()}</span>
								<span class="mon">{at(d.day).toLocaleDateString(undefined, { month: 'short' })}</span>
								{#if d.day === today}<span class="today-tag"><Badge label="Today" /></span>{/if}
							</div>
							<!-- Every card of the day, all one size, wrapping onto more rows when a row is full. -->
							<div class="day">
								<h4 class="count">{thingCount(d.items.length)}</h4>
								<div class="grid" data-testid="cal-day-grid">
									{#each gs as g (g.key)}<CalTile {g} {today} {mode} {look} {onfollow} />{/each}
								</div>
							</div>
												</li>
					{/each}
				</ol>
			</section>
		{/each}
	</div>
{/if}

<style>
	.cs { display: flex; flex-direction: column; gap: 28px; container-type: inline-size; }
	.divider { display: flex; align-items: center; gap: 14px; margin: 0 0 16px; font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.divider::after { content: ''; flex: 1; height: 1px; background: var(--line); }
	.rows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 28px; }
	/* Each day after the first starts on a hairline across the full width. */
	.row { display: grid; grid-template-columns: 96px minmax(0, 1fr); gap: 20px; align-items: start; scroll-margin-top: 8px; }
	.row + .row { padding-top: 28px; border-top: 1px solid var(--line); }
	.day { min-width: 0; display: flex; flex-direction: column; gap: 16px; }
	.count { font: var(--t-title3); letter-spacing: var(--t-title3-track); color: var(--text-muted); line-height: 24px; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(300px, 100%), 1fr)); gap: 20px; }
	.stamp { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; align-items: flex-start; line-height: 1; }
	.dow { color: var(--text-muted); font: var(--t-subhead); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; }
	.big { margin-top: 4px; font: 700 64px/0.95 var(--font-display); letter-spacing: -0.04em; font-variant-numeric: tabular-nums; }
	.mon { margin-top: 4px; color: var(--text-muted); font: var(--t-footnote); }
	.today-tag { margin-top: 6px; display: inline-flex; }
	.past .big { color: var(--text-muted); }
	@media (max-width: 767px) {
		.cs, .rows { gap: 24px; }
		.row { grid-template-columns: 46px minmax(0, 1fr); gap: 12px; }
		.row + .row { padding-top: 24px; }
		.grid { gap: 16px; }
		.day { gap: 12px; }
		.stamp { top: calc(var(--topbar) + 8px); align-items: center; }
		.dow { font-size: 11px; letter-spacing: 0.04em; }
		.big { font-size: 36px; }
		.mon { font-size: 11px; }
	}
</style>
