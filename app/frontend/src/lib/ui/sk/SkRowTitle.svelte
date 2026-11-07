<script lang="ts">
	// Loading stand-in for a shelf heading, same box height as the real one.
	let { width = '180px', subtitle = false }: { width?: string; subtitle?: boolean } = $props();
</script>

<div class="sk-head" aria-hidden="true">
	<span class="sk sk-line t" style:width></span>
	{#if subtitle}<span class="sk sk-line s"></span>{/if}
</div>

<style>
	/* 44px matches the real header so the shelf does not shift. */
	.sk-head { display: flex; flex-direction: column; justify-content: flex-end; gap: 8px; min-height: 44px; margin-bottom: 12px; }
	.t { height: 20px; max-width: 70%; }
	.s { height: 12px; width: 120px; }
</style>
