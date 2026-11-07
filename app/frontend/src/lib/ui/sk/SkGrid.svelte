<script lang="ts">
	// Loading stand-in for a card grid.
	import SkCard from './SkCard.svelte';
	import SkPerson from './SkPerson.svelte';
	import SkWide from './SkWide.svelte';

	// Enough items to fill a tall screen so the page does not jump when data lands.
	let { kind = 'poster', min = '150px', count = 18 }: { kind?: 'poster' | 'wide' | 'person'; min?: string; count?: number } = $props();
</script>

<div class="sk-grid" style="--min: {min}" aria-hidden="true">
	{#each Array.from({ length: count }, (_, i) => i) as i (i)}
		{#if kind === 'wide'}<SkWide text />{:else if kind === 'person'}<SkPerson />{:else}<SkCard />{/if}
	{/each}
</div>

<style>
	/* Same column rule as the real grids so columns do not shift on load. */
	.sk-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--min), 1fr)); gap: 20px var(--shelf-gap); justify-items: stretch; }
</style>
