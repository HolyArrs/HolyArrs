<script lang="ts">
	import SkCard from './SkCard.svelte';
	import SkPerson from './SkPerson.svelte';
	import SkWide from './SkWide.svelte';

	// A grid of card placeholders that fills at least one viewport, with the same
	// column rule the real grids use.
	let { kind = 'poster', min = '150px', count = 18 }: { kind?: 'poster' | 'wide' | 'person'; min?: string; count?: number } = $props();
</script>

<div class="sk-grid" style="--min: {min}" aria-hidden="true">
	{#each Array.from({ length: count }, (_, i) => i) as i (i)}
		{#if kind === 'wide'}<SkWide text />{:else if kind === 'person'}<SkPerson />{:else}<SkCard />{/if}
	{/each}
</div>

<style>
	.sk-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--min), 1fr)); gap: 20px var(--shelf-gap); justify-items: stretch; }
</style>
