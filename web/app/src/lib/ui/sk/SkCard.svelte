<script lang="ts">
	// A 2:3 poster card placeholder with the two text lines under it (the PosterCard size).
	let { text = true, round = false }: { text?: boolean; round?: boolean } = $props();
</script>

<div class="sk-card" aria-hidden="true">
	<span class="sk art" class:round></span>
	{#if text}<span class="sk sk-line a"></span><span class="sk sk-line b"></span>{/if}
</div>

<style>
	.sk-card { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.art { aspect-ratio: var(--poster-ratio); width: 100%; }
	.round { aspect-ratio: 1; border-radius: 50%; }
	.a { width: 80%; height: 13px; }
	.b { width: 50%; height: 11px; }
</style>
