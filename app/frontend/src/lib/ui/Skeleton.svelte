<script lang="ts">
	// Placeholder shapes shown while content loads.
	let {
		shape = 'line',
		width,
		height,
		lines = 1
	}: { shape?: 'line' | 'block' | 'poster' | 'wide' | 'circle'; width?: string; height?: string; lines?: number } = $props();
</script>

<!-- the last line of a stack is shorter so it reads like real text -->
{#if shape === 'line' && lines > 1}
	<span class="stack" aria-hidden="true">
		{#each Array.from({ length: lines }, (_, i) => i) as i (i)}<span class="sk ui-sk line" style:width={i === lines - 1 ? '62%' : width}></span>{/each}
	</span>
{:else}
	<span class="sk ui-sk {shape}" style:width style:height aria-hidden="true"></span>
{/if}

<style>
	/* fixed sizes keep the layout from jumping when real content lands */
	.stack { display: flex; flex-direction: column; gap: 8px; }
	.ui-sk { display: block; }
	.line { height: 12px; width: 100%; border-radius: 6px; }
	.block { height: 80px; width: 100%; border-radius: var(--r-card); }
	.poster { aspect-ratio: var(--poster-ratio); width: 100%; border-radius: var(--r-card); }
	.wide { aspect-ratio: var(--wide-ratio); width: 100%; border-radius: var(--r-card); }
	.circle { width: 64px; height: 64px; border-radius: 50%; }
</style>
