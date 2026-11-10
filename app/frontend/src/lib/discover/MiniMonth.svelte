<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { monthGrid } from './calendar.ts';
	import { dayName, thingCount } from './calendar-groups.ts';
	import type { CalendarEntry } from '#lib/core/types.ts';

	// The small month beside the calendar (a sheet on phones). Only days with something are buttons, marked with a dot; empty days are dimmed and skipped by the keyboard. Today is ringed and the day in view is filled.
	let {
		year,
		month,
		entries,
		today,
		active = null,
		onpick,
		onpage
	}: {
		year: number;
		month: number;
		entries: CalendarEntry[];
		today: string;
		/** The day at the top of the page now. */
		active?: string | null;
		onpick: (day: string) => void;
		onpage: (n: number) => void;
	} = $props();

	const weeks = $derived(monthGrid(year, month, entries, today));
	const name = $derived(new Date(year, month, 1).toLocaleDateString(appLocale(), { month: 'long', year: 'numeric' }));
	const heads = $derived(Array.from({ length: 7 }, (_, i) => new Date(2024, 0, 1 + i).toLocaleDateString(appLocale(), { weekday: 'narrow' })));
</script>

<div class="mm" data-testid="cal-picker">
	<div class="head">
		<IconButton icon="chevronLeft" size="sm" label={t('discover.miniMonth.monthBefore')} onclick={() => onpage(-1)} />
		<b aria-live="polite">{name}</b>
		<IconButton icon="chevronRight" size="sm" label={t('discover.miniMonth.monthAfter')} onclick={() => onpage(1)} />
	</div>
	<div class="grid" role="group" aria-label={name}>
		{#each heads as h, i (i)}<span class="wd" aria-hidden="true">{h}</span>{/each}
		{#each weeks.flat() as d (d.day)}
			{#if d.items.length}
				<button
					type="button"
					class="pd busy"
					class:out={!d.inMonth}
					class:today={d.today}
					class:on={active === d.day}
					data-pick={d.day}
					aria-current={active === d.day ? 'date' : undefined}
					aria-label={t('discover.miniMonth.goToThisDay', { v: dayName(d.day, today), v2: thingCount(d.items.length) })}
					onclick={() => onpick(d.day)}
				>
					<span class="n">{d.date}</span><span class="dot" aria-hidden="true"></span>
				</button>
			{:else}
				<span class="pd" class:out={!d.inMonth} class:today={d.today} data-pick={d.day}><span class="n">{d.date}</span></span>
			{/if}
		{/each}
	</div>
</div>

<style>
	.mm { display: flex; flex-direction: column; gap: 8px; }
	.head { display: flex; align-items: center; justify-content: space-between; gap: 6px; font: var(--t-subhead); }
	.grid { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 2px; text-align: center; }
	.wd { padding: 4px 0; color: var(--text-muted); font: var(--t-caption); }
	.pd { position: relative; height: 34px; display: grid; place-items: center; border-radius: 999px; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.pd .n { display: inline-grid; place-items: center; min-width: 28px; height: 28px; border-radius: 999px; }
	.busy { color: var(--text); font-weight: 600; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.busy:hover { background: var(--surface-3); }
	.busy:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.busy.out { color: var(--text-muted); }
	.dot { position: absolute; bottom: 2px; left: 50%; translate: -50% 0; width: 4px; height: 4px; border-radius: 50%; background: var(--accent-fill); }
	.today .n { box-shadow: inset 0 0 0 2px var(--accent-fill); color: var(--accent-text); }
	.on .n { background: var(--text); color: var(--bg); box-shadow: none; }
	.on.today .n { background: var(--accent-fill); color: var(--text-on-red); }
	@media (max-width: 1023px) {
		.pd { height: 44px; font-size: 15px; }
		.pd .n { min-width: 34px; height: 34px; }
	}
</style>
