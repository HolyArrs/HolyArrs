<script lang="ts">
	// Loading stand-in for a whole shelf row.
	import SkCard from './SkCard.svelte';
	import SkPerson from './SkPerson.svelte';
	import SkRowTitle from './SkRowTitle.svelte';
	import SkWide from './SkWide.svelte';

	let {
		kind = 'poster',
		item,
		count = 8,
		title = true,
		titleWidth
	}: { kind?: 'poster' | 'wide' | 'person'; item?: string; count?: number; title?: boolean; titleWidth?: string } = $props();
	// Column widths copy the real shelf so cards line up once loaded.
	const col = $derived(item ?? (kind === 'wide' ? 'var(--title-item, 279px)' : kind === 'person' ? '112px' : 'var(--poster-item, 168px)'));
</script>

<div class="sk-rail" aria-hidden="true">
	{#if title}<SkRowTitle width={titleWidth} />{/if}
	<div class="row" style="--item: {col}">
		{#each Array.from({ length: count }, (_, i) => i) as i (i)}
			{#if kind === 'wide'}<SkWide />{:else if kind === 'person'}<SkPerson />{:else}<SkCard />{/if}
		{/each}
	</div>
</div>

<style>
	.sk-rail { min-width: 0; }
	/* Clipped on the right so extra cards never add a scrollbar. */
	.row { display: grid; grid-auto-flow: column; grid-auto-columns: var(--item); gap: var(--shelf-gap); overflow: hidden; padding: 6px 0 14px; }
</style>
