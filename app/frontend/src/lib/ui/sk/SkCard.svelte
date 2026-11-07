<script lang="ts">
	// Loading stand-in for a poster card, sized to match the real one.
	let { text = true, round = false }: { text?: boolean; round?: boolean } = $props();
</script>

<div class="sk-card" aria-hidden="true">
	<span class="sk art" class:round></span>
	{#if text}<span class="sk sk-line a"></span><span class="sk sk-line b"></span>{/if}
</div>

<style>
	.sk-card { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	/* Poster ratio keeps the box the same height as the loaded card. */
	.art { aspect-ratio: var(--poster-ratio); width: 100%; }
	.round { aspect-ratio: 1; border-radius: 50%; }
	.a { width: 80%; height: 13px; }
	.b { width: 50%; height: 11px; }
</style>
