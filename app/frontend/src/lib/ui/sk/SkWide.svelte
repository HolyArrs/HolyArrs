<script lang="ts">
	// A 16:9 wide card placeholder (the WideCard size).
	let { text = false }: { text?: boolean } = $props();
</script>

<div class="sk-wide" aria-hidden="true">
	<span class="sk art"></span>
	{#if text}<span class="sk sk-line a"></span>{/if}
</div>

<style>
	.sk-wide { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.art { aspect-ratio: var(--wide-ratio); width: 100%; }
	.a { width: 60%; height: 13px; }
</style>
