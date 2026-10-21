<script lang="ts">
	import SkCard from './SkCard.svelte';
	import SkPerson from './SkPerson.svelte';
	import SkRowTitle from './SkRowTitle.svelte';
	import SkWide from './SkWide.svelte';

	// A whole shelf placeholder: heading plus a row of cards laid out like Shelf (same
	// column width, gap and padding), clipped at the right edge.
	let {
		kind = 'poster',
		item,
		count = 8,
		title = true,
		titleWidth
	}: { kind?: 'poster' | 'wide' | 'person'; item?: string; count?: number; title?: boolean; titleWidth?: string } = $props();
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
	.row { display: grid; grid-auto-flow: column; grid-auto-columns: var(--item); gap: var(--shelf-gap); overflow: hidden; padding: 6px 0 14px; }
</style>
