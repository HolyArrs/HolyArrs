<script lang="ts">
	// A table or list row placeholder: optional thumb, a title with a sub line, then columns.
	let { cols = 3, thumb = true, height = 64 }: { cols?: number; thumb?: boolean; height?: number } = $props();
</script>

<div class="sk-tr" style:min-height="{height}px" aria-hidden="true">
	{#if thumb}<span class="sk th"></span>{/if}
	<span class="main"><span class="sk sk-line a"></span><span class="sk sk-line b"></span></span>
	{#each Array.from({ length: cols }, (_, i) => i) as i (i)}<span class="sk sk-line c"></span>{/each}
</div>

<style>
	.sk-tr { display: flex; align-items: center; gap: 16px; padding: 8px 12px; border-bottom: 1px solid var(--line); }
	.th { width: 32px; aspect-ratio: var(--poster-ratio); border-radius: 6px; flex: none; }
	.main { flex: 1; display: flex; flex-direction: column; gap: 7px; min-width: 0; }
	.a { width: min(280px, 70%); height: 13px; }
	.b { width: min(160px, 45%); height: 10px; }
	.c { width: 72px; flex: none; }
	@media (max-width: 767px) { .c:nth-of-type(n + 3) { display: none; } }
</style>
