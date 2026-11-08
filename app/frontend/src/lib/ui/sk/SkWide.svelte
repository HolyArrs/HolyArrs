<script lang="ts">
	// Loading stand-in for a wide card, sized to match the real one.
	let { text = false }: { text?: boolean } = $props();
</script>

<div class="sk-wide" aria-hidden="true">
	<span class="sk art"></span>
	{#if text}<span class="sk sk-line a"></span>{/if}
</div>

<style>
	.sk-wide { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	/* Wide ratio keeps the box the same height as the loaded card. */
	.art { aspect-ratio: var(--wide-ratio); width: 100%; }
	.a { width: 60%; height: 13px; }
</style>
