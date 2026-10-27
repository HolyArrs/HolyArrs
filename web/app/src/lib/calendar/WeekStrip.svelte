<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { entryKey } from '#lib/calendar.ts';
	import { dayDots, dayName, nextBusy, startDay, stripDays } from '#lib/calendar-looks.ts';
	import Button from '#lib/ui/Button.svelte';
	import CalCard from './CalCard.svelte';

	// 3 · Week strip: every loaded day as a sideways strip of date buttons with
	// a dot per kind of date (episode, movie, album). Today is ringed and
	// picked first; the picked day's dates show below as large cards.
	let { entries, today, start, days: n, onlater }: { entries: CalendarEntry[]; today: string; start: Date; days: number; onlater: () => void } = $props();

	const days = $derived(stripDays(start, n, entries, today));
	let picked = $state<string | null>(null);
	// A new range (the arrows) starts again on today or its first busy day.
	$effect(() => {
		const first = startDay(days);
		if (!picked || !days.some((d) => d.day === picked)) picked = first;
	});
	const day = $derived(days.find((d) => d.day === picked) ?? null);
	const next = $derived(picked ? nextBusy(picked, days) : null);
	const busyDays = $derived(days.filter((d) => d.items.length).length);

	let strip = $state<HTMLElement>();
	// The picked day scrolls into the middle of the strip.
	$effect(() => {
		void picked;
		const on = strip?.querySelector<HTMLElement>('[aria-pressed="true"]');
		if (on && strip) strip.scrollTo({ left: on.offsetLeft - (strip.clientWidth - on.offsetWidth) / 2, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
	});
	function key(ev: KeyboardEvent) {
		if (ev.key !== 'ArrowRight' && ev.key !== 'ArrowLeft') return;
		const i = days.findIndex((d) => d.day === picked);
		const j = Math.max(0, Math.min(days.length - 1, i + (ev.key === 'ArrowRight' ? 1 : -1)));
		picked = days[j].day;
		ev.preventDefault();
		void Promise.resolve().then(() => strip?.querySelector<HTMLElement>('[aria-pressed="true"]')?.focus());
	}
	const count = (k: number) => (k === 1 ? '1 date' : `${k} dates`);
</script>

<div class="ws" data-testid="cal-look-3">
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div class="strip" role="group" aria-label="Pick a day" bind:this={strip} onkeydown={key}>
		{#each days as d (d.day)}
			<button
				type="button"
				class="d"
				class:today={d.today}
				class:past={d.past}
				class:busy={d.items.length > 0}
				aria-pressed={picked === d.day}
				aria-label="{dayName(d.day, today)}, {d.items.length ? count(d.items.length) : 'nothing'}"
				tabindex={picked === d.day ? 0 : -1}
				onclick={() => (picked = d.day)}
			>
				<span class="mon" aria-hidden="true">{d.first ? d.month : ''}</span>
				<span class="dow" aria-hidden="true">{d.dow}</span>
				<span class="num" aria-hidden="true">{d.date}</span>
				<span class="dots" aria-hidden="true">
					{#each dayDots(d.items) as k (k)}<i class="k-{k}"></i>{/each}
				</span>
			</button>
		{/each}
	</div>
	<p class="legend" aria-hidden="true"><span><i class="k-episode"></i>Episodes</span><span><i class="k-movie"></i>Movies</span><span><i class="k-album"></i>Albums</span></p>

	{#if day}
		<section class="picked" aria-live="polite" aria-label={dayName(day.day, today)}>
			<header>
				<h3>{dayName(day.day, today)}</h3>
				{#if day.items.length}<span class="n">{count(day.items.length)}</span>{/if}
			</header>
			{#if day.items.length}
				<ul class="cards">
					{#each day.items as e (entryKey(e))}<li><CalCard {e} {today} all={entries} /></li>{/each}
				</ul>
			{:else}
				<div class="quiet">
					<p>{busyDays ? `Nothing on ${dayName(day.day, today) === 'Today' ? 'today' : dayName(day.day, today)}.` : 'Nothing in these weeks with the filters picked.'}</p>
					{#if !busyDays}<Button variant="secondary" size="sm" iconAfter="chevronRight" onclick={onlater}>Show the weeks after</Button>{/if}
					{#if next}<Button variant="secondary" size="sm" iconAfter="chevronRight" onclick={() => (picked = next)}>Next: {dayName(next, today)}</Button>{/if}
				</div>
			{/if}
		</section>
	{/if}
</div>

<style>
	.ws { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.strip {
		position: relative; display: flex; gap: 6px; overflow-x: auto; padding: 6px 2px 10px;
		scroll-snap-type: x proximity; scrollbar-width: thin; overscroll-behavior-x: contain;
	}
	.d {
		flex: none; width: 60px; scroll-snap-align: center; display: flex; flex-direction: column; align-items: center; gap: 2px;
		padding: 8px 0 10px; border-radius: 16px; color: var(--text); background: var(--surface-1);
		box-shadow: inset 0 0 0 1px var(--line); transition: background var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out);
	}
	.d:hover { background: var(--surface-2); }
	.d:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.mon { height: 12px; color: var(--accent-text); font: 600 10px/12px var(--font-ui); text-transform: uppercase; letter-spacing: 0.06em; }
	.dow { color: var(--text-muted); font: var(--t-caption); }
	.num { font: 700 22px/1.1 var(--font-display); font-variant-numeric: tabular-nums; letter-spacing: -0.02em; }
	.dots { display: flex; gap: 3px; height: 6px; margin-top: 3px; }
	i { display: inline-block; width: 6px; height: 6px; border-radius: 50%; }
	.k-episode { background: var(--ok); }
	.k-movie { background: var(--info); }
	.k-album { background: var(--wait); }
	.past:not([aria-pressed='true']) { opacity: 0.55; }
	.today { box-shadow: inset 0 0 0 2px var(--accent-fill); }
	.today .dow { color: var(--accent-text); font-weight: 600; }
	.d[aria-pressed='true'] { background: var(--text); color: var(--bg); box-shadow: none; }
	.d[aria-pressed='true'] .dow, .d[aria-pressed='true'] .mon { color: var(--bg); }
	.d[aria-pressed='true'] i { box-shadow: 0 0 0 1.5px var(--text); }
	.legend { display: flex; flex-wrap: wrap; gap: 4px 14px; color: var(--text-muted); font: var(--t-caption); margin-top: -6px; }
	.legend span { display: inline-flex; align-items: center; gap: 6px; }

	.picked { display: flex; flex-direction: column; gap: 14px; padding-top: 6px; }
	header { display: flex; align-items: baseline; gap: 12px; }
	h3 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.n { color: var(--text-muted); font: var(--t-footnote); }
	.cards { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 24px 20px; }
	.cards > li { min-width: 0; }
	.quiet { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 22px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); color: var(--text-muted); font: var(--t-callout); }
	@media (max-width: 767px) {
		.strip { margin: 0 calc(-1 * var(--gutter)); padding: 6px var(--gutter) 10px; scroll-padding: 0 var(--gutter); scrollbar-width: none; }
		.strip::-webkit-scrollbar { display: none; }
		.d { width: 52px; border-radius: 14px; }
		.num { font-size: 20px; }
		h3 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
		.cards { grid-template-columns: minmax(0, 1fr); gap: 20px; }
		.quiet { padding: 16px; }
	}
</style>
