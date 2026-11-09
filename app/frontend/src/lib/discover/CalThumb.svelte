<script lang="ts">
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import { entryArt } from './calendar.ts';
	import { entryTitle } from './calendar-groups.ts';
	import type { CalendarEntry } from '#lib/core/types.ts';
	import { sized } from '#lib/core/tmdb-image.ts';

	// Small 16:9 picture on a List row; an album cover fills it, cropped.
	let { e, w = 160 }: { e: CalendarEntry; w?: number } = $props();
	const src = $derived(e.kind === 'album' ? (e.poster ?? null) : entryArt(e));
	let failed = $state(false);
</script>

<span class="th" aria-hidden="true">
	{#if src && !failed}
		<img src={sized(src, e.kind === 'album' ? 'poster' : 'backdrop', w)} alt="" loading="lazy" decoding="async" onerror={() => (failed = true)} />
	{:else}
		<ArtPlaceholder name={entryTitle(e)} shape="wide" />
	{/if}
</span>

<style>
	.th { position: relative; display: block; width: 100%; height: 100%; overflow: hidden; border-radius: inherit; background: var(--surface-2); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
</style>
