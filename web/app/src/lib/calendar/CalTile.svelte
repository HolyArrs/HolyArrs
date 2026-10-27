<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryArt, entryHref, itemDot } from '#lib/calendar.ts';
	import { dayChip, entryLine, entryTitle, groupFacts, groupTag, type CalGroup } from '#lib/calendar-looks.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import Badge from '#lib/ui/Badge.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';

	// Coming Soon's card for rows: the Day view's big backdrop card made row
	// size, with the name on the art and its badges. Several episodes of one
	// show on one day are one card, drawn as a small stack ("3 episodes").
	let { g, today, chip = false }: { g: CalGroup; today: string; /** Put the day ("Wed 28") on the art. */ chip?: boolean } = $props();

	const e = $derived<CalendarEntry>(g.lead);
	const n = $derived(g.entries.length);
	const src = $derived(e.kind === 'album' ? (e.poster ?? null) : entryArt(e));
	const badges = $derived([chip ? dayChip(g.day, today) : '', groupTag(g)].filter(Boolean));
	// A movie's badge already says which date it is, so its facts line is just its year.
	const movie = $derived(e.kind !== 'episode' && e.kind !== 'album');
	const facts = $derived(movie ? entryLine(e) : groupFacts(g));
	const line = $derived(n === 1 && !movie ? entryLine(e) : '');
	const have = $derived(g.entries.every((x) => itemDot(x, today) === 'have'));
	let failed = $state(false);
</script>

<article class="ct" class:album={e.kind === 'album'} class:stacked={n > 1} class:past={g.day < today} data-testid="cal-tile">
	<div class="stack">
		{#if n > 1}<span class="layer l2" aria-hidden="true"></span><span class="layer l1" aria-hidden="true"></span>{/if}
		<a class="art" href={to(entryHref(e))} aria-label="{entryTitle(e)}, {groupTag(g)}, {groupFacts(g)}">
			{#if src && !failed}
				<img class="bg" src={sized(src, e.kind === 'album' ? 'poster' : 'backdrop', 400)} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />
				{#if e.kind === 'album'}<img class="cover" src={sized(src, 'poster', 140)} alt="" loading="lazy" decoding="async" />{/if}
			{:else}
				<ArtPlaceholder name={entryTitle(e)} shape="wide" />
			{/if}
			<span class="scrim" aria-hidden="true"></span>
			<span class="on-art" aria-hidden="true">
				<span class="tags">{#each badges as b (b)}<Badge label={b} on="art" />{/each}</span>
				<span class="name">{entryTitle(e)}</span>
			</span>
		</a>
	</div>
	<p class="facts">{facts}{#if have}<span class="have"> · In the Library</span>{/if}</p>
	{#if line}<p class="line">{line}</p>{/if}
</article>

<style>
	.ct { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.ct.past { opacity: 0.75; }
	/* Room above every card for a stack's layers, so cards in one row line up. */
	.stack { position: relative; padding-top: 8px; }
	.layer { position: absolute; border-radius: var(--r-card); background: var(--surface-3); box-shadow: var(--shadow-card); }
	.l1 { top: 4px; left: 4%; right: 4%; height: 40px; opacity: 0.9; }
	.l2 { top: 0; left: 8%; right: 8%; height: 40px; opacity: 0.55; }
	.art { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); box-shadow: var(--shadow-card); transition: transform var(--dur-slow) var(--ease-out), box-shadow var(--dur-slow) var(--ease-out); }
	.art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.art > :global(.ui-art-ph) { position: absolute; inset: 0; }
	.bg { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.album .bg { filter: blur(22px) saturate(140%) brightness(0.7); transform: scale(1.2); }
	.cover { position: absolute; right: 6%; top: 50%; translate: 0 -50%; height: 70%; aspect-ratio: 1; object-fit: cover; border-radius: 6px; box-shadow: 0 8px 24px rgb(0 0 0 / 0.5); }
	.scrim { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 0.75), rgb(0 0 0 / 0.08) 60%, transparent); pointer-events: none; }
	.on-art { position: absolute; left: 6%; right: 6%; bottom: 8%; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
	.album .on-art { right: 46%; }
	.tags { display: flex; flex-wrap: wrap; gap: 4px; }
	.name { color: #fff; font: 700 clamp(17px, 1.6vw, 22px)/1.08 var(--font-display); letter-spacing: -0.02em; text-shadow: 0 2px 12px rgb(0 0 0 / 0.5); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.facts { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.have { color: var(--ok-text, var(--ok)); }
	.line { margin-top: -4px; color: var(--text); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.art:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	}
</style>
