<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import Badge from '#lib/ui/Badge.svelte';
	import type { Follows } from './calendar.ts';
	import { groupDates, monthGroups, thingCount } from './calendar-groups.ts';
	import { DEFAULT_LOOK, type CaptionLook } from './calendar-captions.ts';
	import CalTile from './CalTile.svelte';
	import CalEmpty from './CalEmpty.svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { onDestroy } from 'svelte';
	import type { CalendarEntry } from '#lib/core/types.ts';

	// Cards view: one day per row. A date stamp on the left (day name over a large number, once per day) and backdrop cards beside it with the name on the art, a short line and an Open button. Month names divide the list.
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

	// Only days near the screen get real cards; the rest hold plain boxes of exactly the same size, so the page keeps its height and scrolling never jumps while a year of days stays cheap.
	const drawn = new SvelteSet<string>();
	const io = typeof IntersectionObserver === 'undefined' ? null : new IntersectionObserver(
		(es) => {
			for (const e of es) if (e.isIntersecting) {
				const d = (e.target as HTMLElement).dataset.day;
				if (d) drawn.add(d);
				io?.unobserve(e.target);
			}
		},
		{ rootMargin: '1200px 0px' }
	);
	function near(node: HTMLElement) {
		if (!io) return void (node.dataset.day && drawn.add(node.dataset.day));
		io.observe(node);
		return { destroy: () => io.unobserve(node) };
	}
	onDestroy(() => io?.disconnect());
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
						<li class="row" use:near data-day={d.day} class:today={d.day === today} class:past={d.day < today}>
							<div class="stamp">
								<span class="dow">{at(d.day).toLocaleDateString(appLocale(), { weekday: 'short' })}</span>
								<span class="big">{at(d.day).getDate()}</span>
								<span class="mon">{at(d.day).toLocaleDateString(appLocale(), { month: 'short' })}</span>
								{#if d.day === today}<span class="today-tag"><Badge label={t('discover.comingSoon.today')} /></span>{/if}
							</div>
							<div class="day">
								<h4 class="count">{thingCount(d.items.length)}</h4>
								<div class="grid" data-testid="cal-day-grid">
									{#if drawn.has(d.day)}
										{#each gs as g (g.key)}<CalTile {g} {today} {mode} {look} {onfollow} />{/each}
									{:else}
										{#each gs as g (g.key)}<div class="ph" aria-hidden="true"><span class="pa"></span><span class="pc"></span></div>{/each}
									{/if}
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
	.row { display: grid; grid-template-columns: 96px minmax(0, 1fr); gap: 20px; align-items: start; scroll-margin-top: 8px; }
	.row + .row { padding-top: 28px; border-top: 1px solid var(--line); }
	.day { min-width: 0; display: flex; flex-direction: column; gap: 16px; }
	.count { font: var(--t-title3); letter-spacing: var(--t-title3-track); color: var(--text-muted); line-height: 24px; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(300px, 100%), 1fr)); gap: 20px; }
	/* The stand-in for an undrawn card has CalTile's exact box (picture, 8px foot, 6px gap, two caption lines). */
	.ph { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.pa { display: block; aspect-ratio: 16 / 9; margin-bottom: 8px; border-radius: var(--r-card); background: var(--surface-1); }
	.pc { display: block; height: 38px; }
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
