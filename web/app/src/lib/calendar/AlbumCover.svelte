<script lang="ts">
	import type { CalendarEntry } from '#lib/types.ts';
	import { to } from '#lib/paths.ts';
	import { entryHref, itemDot } from '#lib/calendar.ts';
	import { sized } from '#lib/tmdb-image.ts';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';

	// One album on the calendar: the square cover with its kind and a green dot when it is in
	// the Library, then the album and the artist (the Music calendar and the mixed All grid).
	let { e, today }: { e: CalendarEntry; today: string } = $props();
	const title = $derived(e.album ?? e.name);
	const kind = $derived(e.album_type ? e.album_type[0].toUpperCase() + e.album_type.slice(1).toLowerCase() : 'Album');
	const have = $derived(itemDot(e, today) === 'have');
	const src = $derived(e.poster ? sized(e.poster, 'poster', 400) : '');
	let failed = $state(false);
</script>

<a class="ac" href={to(entryHref(e))} data-testid="cal-album">
	<span class="pic">
		{#if src && !failed}<img {src} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />{:else}<ArtPlaceholder name={title} shape="square" />{/if}
		<i class="kind">{kind}</i>
		{#if have}<i class="have" title="In Library"></i>{/if}
	</span>
	<b>{title}</b>
	<small>{e.name}</small>
</a>

<style>
	.ac { display: flex; flex-direction: column; gap: 3px; min-width: 0; color: var(--text); text-decoration: none; }
	.pic { position: relative; display: block; aspect-ratio: 1; margin-bottom: 8px; overflow: hidden; border-radius: 10px; background: var(--surface-2); box-shadow: var(--shadow-card); }
	.pic img { width: 100%; height: 100%; object-fit: cover; transition: scale var(--dur-slow) var(--ease-out); }
	.ac:hover img { scale: 1.04; }
	.kind { position: absolute; left: 8px; bottom: 8px; padding: 3px 8px; border-radius: 999px; background: rgb(0 0 0 / .55); backdrop-filter: blur(10px); color: #fff; font: 700 11px/1.2 var(--font-ui); font-style: normal; }
	.have { position: absolute; top: 8px; right: 8px; width: 12px; height: 12px; border-radius: 50%; background: #34c759; box-shadow: 0 0 0 2px rgb(0 0 0 / .4); }
	b { font: 700 14px/1.25 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	small { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (prefers-reduced-motion: reduce) { .ac:hover img { scale: none; } }
</style>
