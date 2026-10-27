<script lang="ts">
	import { onMount } from 'svelte';
	import type { CalendarEntry } from '#lib/types.ts';
	import { entryKey, monthGrid } from '#lib/calendar.ts';
	import { dayDots, dayName, entryTitle } from '#lib/calendar-looks.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import CalCard from './CalCard.svelte';

	// 5 · Month: a full month like Apple Calendar. Each day shows up to two
	// tiny posters and "+3" (dots on phones); today is ringed and past days
	// step back. A day opens its dates as cards: a side panel on wide
	// screens, a bottom sheet on phones.
	let { entries, today, year, month }: { entries: CalendarEntry[]; today: string; year: number; month: number } = $props();

	const weeks = $derived(monthGrid(year, month, entries, today));
	const cells = $derived(weeks.flat());
	const heads = $derived(Array.from({ length: 7 }, (_, i) => new Date(2024, 0, 1 + i).toLocaleDateString(undefined, { weekday: 'short' })));
	const monthKey = $derived(`${year}-${String(month + 1).padStart(2, '0')}`);
	const busy = $derived(cells.some((c) => c.inMonth && c.items.length));

	let picked = $state<string | null>(null);
	// A new month starts on today when it is in it, else its first busy day.
	$effect(() => {
		if (picked && picked.startsWith(monthKey)) return;
		picked = cells.find((c) => c.today && c.inMonth)?.day ?? cells.find((c) => c.inMonth && c.items.length)?.day ?? null;
	});
	const day = $derived(cells.find((c) => c.day === picked) ?? null);

	let phone = $state(false);
	let sheet = $state(false);
	onMount(() => {
		const mq = matchMedia('(max-width: 899px)');
		const set = () => (phone = mq.matches);
		set();
		mq.addEventListener('change', set);
		return () => mq.removeEventListener('change', set);
	});
	function open(d: string) {
		picked = d;
		if (phone) sheet = true;
	}
	const count = (k: number) => (k === 1 ? '1 date' : `${k} dates`);
	const thumb = (e: CalendarEntry) => e.poster || e.backdrop || e.still || null;
</script>

{#snippet dayCards()}
	{#if day}
		{#if day.items.length}
			<ul class="cards">
				{#each day.items as e (entryKey(e))}<li><CalCard {e} {today} all={entries} /></li>{/each}
			</ul>
		{:else}
			<p class="quiet">Nothing on this day.</p>
		{/if}
	{/if}
{/snippet}

<div class="mg" class:has-side={!phone} data-testid="cal-look-5">
	<div class="grid-wrap">
		<div class="grid" role="grid" aria-label={new Date(year, month, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' })}>
			<div class="row head" role="row">
				{#each heads as h, i (i)}<span role="columnheader">{h}</span>{/each}
			</div>
			{#each weeks as w (w[0].day)}
				<div class="row" role="row">
					{#each w as c (c.day)}
						<button
							type="button"
							role="gridcell"
							class="cell"
							class:out={!c.inMonth}
							class:past={c.day < today}
							class:today={c.today}
							class:on={picked === c.day}
							aria-selected={picked === c.day}
							aria-label="{dayName(c.day, today)}{c.items.length ? `, ${count(c.items.length)}: ${c.items.map(entryTitle).join(', ')}` : ', nothing'}"
							onclick={() => open(c.day)}
						>
							<span class="num">{c.date}</span>
							{#if c.items.length}
								<span class="posters" aria-hidden="true">
									{#each c.items.slice(0, 2) as e (entryKey(e))}
										{@const src = thumb(e)}
										<span class="p" class:sq={e.kind === 'album'}>{#if src}<img src={sized(src, 'poster', 60)} alt="" loading="lazy" decoding="async" />{/if}</span>
									{/each}
									{#if c.items.length > 2}<span class="more">+{c.items.length - 2}</span>{/if}
								</span>
								<span class="dots" aria-hidden="true">{#each dayDots(c.items) as k (k)}<i class="k-{k}"></i>{/each}</span>
							{/if}
						</button>
					{/each}
				</div>
			{/each}
		</div>
		{#if !busy}<p class="none">Nothing this month with the filters picked.</p>{/if}
	</div>
	{#if !phone}
		<aside class="side" aria-live="polite" aria-label={day ? dayName(day.day, today) : 'Pick a day'}>
			{#if day}
				<header><h3>{dayName(day.day, today)}</h3>{#if day.items.length}<span class="n">{count(day.items.length)}</span>{/if}</header>
				{@render dayCards()}
			{:else}
				<p class="quiet">Pick a day to see its dates.</p>
			{/if}
		</aside>
	{/if}
</div>

{#if phone && day}
	<Sheet bind:open={sheet} title={dayName(day.day, today)} description={day.items.length ? count(day.items.length) : undefined} placement="bottom">
		<div class="sheet-cards">{@render dayCards()}</div>
	</Sheet>
{/if}

<style>
	.mg { display: grid; grid-template-columns: minmax(0, 1fr); gap: 20px; align-items: start; }
	.mg.has-side { grid-template-columns: minmax(0, 1fr) clamp(300px, 26vw, 380px); }
	.grid { border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); outline-offset: -1px; box-shadow: var(--shadow-card); overflow: hidden; }
	.row { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); }
	.row + .row { border-top: 1px solid var(--line); }
	.head span { padding: 10px 10px 8px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.cell { position: relative; min-width: 0; min-height: 118px; padding: 8px; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; text-align: left; color: var(--text); transition: background var(--dur-fast) var(--ease-out); }
	.cell + .cell { border-left: 1px solid var(--line); }
	.cell:hover { background: var(--surface-2); }
	.cell:focus-visible { outline: none; box-shadow: inset var(--focus-ring); z-index: 1; }
	.cell.on { background: var(--accent-soft); }
	.num { display: inline-grid; place-items: center; min-width: 28px; height: 28px; padding: 0 5px; border-radius: 999px; font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.today .num { box-shadow: inset 0 0 0 2px var(--accent-fill); color: var(--accent-text); }
	.today.on .num, .cell.on.today .num { background: var(--accent-fill); color: var(--text-on-red); box-shadow: none; }
	.out { background: color-mix(in srgb, var(--surface-2) 40%, transparent); }
	.out .num { color: var(--text-faint); }
	.past:not(.today) .num { color: var(--text-muted); }
	.past:not(.today) .posters { opacity: 0.5; filter: saturate(0.6); }
	.posters { display: flex; align-items: flex-end; gap: 4px; }
	.p { position: relative; width: 34px; aspect-ratio: 2 / 3; border-radius: 5px; overflow: hidden; background: var(--surface-3); box-shadow: 0 2px 6px rgb(0 0 0 / 0.25); }
	.p.sq { aspect-ratio: 1; }
	.p img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.more { align-self: center; padding: 2px 6px; border-radius: 999px; background: var(--surface-3); color: var(--text); font: var(--t-caption); font-weight: 600; font-variant-numeric: tabular-nums; }
	.dots { display: none; gap: 3px; }
	i { display: inline-block; width: 6px; height: 6px; border-radius: 50%; }
	.k-episode { background: var(--ok); }
	.k-movie { background: var(--info); }
	.k-album { background: var(--wait); }
	.none { margin-top: 12px; color: var(--text-muted); font: var(--t-callout); }

	.side { position: sticky; top: calc(var(--topbar) + 16px); max-height: calc(100vh - var(--topbar) - 120px); overflow-y: auto; display: flex; flex-direction: column; gap: 14px; padding: 16px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	header { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
	h3 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.n { color: var(--text-muted); font: var(--t-footnote); }
	.cards { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 18px; }
	.cards > li { min-width: 0; }
	.quiet { color: var(--text-muted); font: var(--t-callout); }
	.sheet-cards { padding-bottom: 8px; }

	@media (max-width: 1180px) {
		.p { width: 26px; }
		.p:nth-child(2) { display: none; }
	}
	/* Phones and narrow windows: compact cells with kind dots, the day opens in a sheet. */
	@media (max-width: 899px) {
		.cell { min-height: 64px; padding: 6px 2px; align-items: center; gap: 4px; }
		.posters { display: none; }
		.dots { display: flex; }
		.head span { padding: 8px 0; text-align: center; font-size: 11px; }
	}
	@media (max-width: 400px) {
		.cell { min-height: 54px; }
		.num { min-width: 26px; height: 26px; font-size: 13px; }
	}
</style>
