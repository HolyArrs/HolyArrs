<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { dragScroll } from '#lib/ui/drag-scroll.ts';
	import { dayOffset, railLabel, type RailDay, type RailItem } from './popular.ts';
	import type { Snippet } from 'svelte';

	// This week: a date rail on New & Popular, from 3 days back to 7 ahead, each with a day word and its titles beside it. The cards are the page's own, passed in as `card`.
	let { days, card }: { days: RailDay[]; card: Snippet<[RailItem]> } = $props();

	const today = new Date();
	const keyOf = (it: RailItem) => `${it.kind}:${it.tmdb_id}`;
</script>

{#if days.length > 0}
	<section class="rail" aria-labelledby="rail-title" data-testid="new-rail">
		<h2 id="rail-title" class="rail-title">{t('discover.popularRail.thisWeek')}</h2>
		<div class="track" use:dragScroll role="list">
			{#each days as d (d.day)}
				{@const label = railLabel(d.day, today)}
				{@const off = dayOffset(d.day, today)}
				<div class="day" class:past={off < 0} class:now={off === 0} role="listitem" data-day={d.day}>
					<div class="when" aria-label="{label.big}, {label.small}">
						<span class="big">{label.big}</span>
						<span class="small">{label.small}</span>
					</div>
					<ul class="titles">
						{#each d.items as it (keyOf(it))}
							<li class="t">
								{@render card(it)}
								<span class="what">{it.what}{#if it.upcoming && it.in_library} {t('discover.popularRail.coming')}{/if}</span>
							</li>
						{/each}
					</ul>
				</div>
			{/each}
		</div>
	</section>
{/if}

<style>
	.rail {
		--rail-line: color-mix(in srgb, var(--text, #fff) 10%, transparent);
		position: relative;
		border-radius: 20px;
		background: color-mix(in srgb, var(--surface-2, #1c1c1e) 72%, transparent);
		-webkit-backdrop-filter: saturate(160%) blur(24px);
		backdrop-filter: saturate(160%) blur(24px);
		box-shadow: 0 0 0 0.5px var(--rail-line), 0 18px 40px rgb(0 0 0 / 0.18);
		padding: 16px 0 14px;
	}
	.rail-title { margin: 0 0 10px; padding: 0 20px; font: var(--weight-semibold, 600) 13px/1 var(--font-ui); letter-spacing: .14em; text-transform: uppercase; color: var(--text-muted); }
	.track { display: flex; overflow-x: auto; scroll-snap-type: x proximity; overscroll-behavior-x: contain; scrollbar-width: none; padding: 0 12px; }
	.track::-webkit-scrollbar { display: none; }
	.day { flex: none; display: flex; gap: 14px; padding: 4px 16px 4px 8px; scroll-snap-align: start; border-right: 0.5px solid var(--rail-line); }
	.day:last-child { border-right: 0; }
	.when { flex: none; width: 74px; display: flex; flex-direction: column; gap: 6px; padding-top: 2px; }
	.big { font: 400 30px/.92 Anton, var(--font-display, var(--font-ui)), sans-serif; letter-spacing: .01em; color: var(--text, #fff); text-transform: uppercase; }
	.now .big { color: #ff4a52; }
	.past .big { color: color-mix(in srgb, var(--text, #fff) 62%, transparent); }
	.small { font: 600 11px/1.2 var(--font-ui); color: var(--text-muted); letter-spacing: .02em; }
	.titles { list-style: none; margin: 0; padding: 0; display: flex; gap: var(--shelf-gap, 12px); }
	/* Same width as a Top 10 card. */
	.t { width: var(--rail-item, 240px); flex: none; display: flex; flex-direction: column; gap: 6px; }
	.what { font: 500 12px/1.25 var(--font-ui); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	@media (max-width: 767px) {
		.rail { border-radius: 16px; padding: 12px 0 10px; }
		.rail-title { padding: 0 16px; }
		.day { flex-direction: column; gap: 8px; }
		.when { width: auto; flex-direction: row; align-items: baseline; gap: 8px; }
		.big { font-size: 24px; }
		.track { scroll-snap-type: x mandatory; }
	}
</style>
