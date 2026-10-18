<script lang="ts">
	// Placeholder while content loads: one subtle sweep every 1.4s.
	let {
		shape = 'line',
		width,
		height,
		lines = 1
	}: { shape?: 'line' | 'block' | 'poster' | 'wide' | 'circle'; width?: string; height?: string; lines?: number } = $props();
</script>

{#if shape === 'line' && lines > 1}
	<span class="stack" aria-hidden="true">
		{#each Array.from({ length: lines }, (_, i) => i) as i (i)}<span class="ui-sk line" style:width={i === lines - 1 ? '62%' : width}></span>{/each}
	</span>
{:else}
	<span class="ui-sk {shape}" style:width style:height aria-hidden="true"></span>
{/if}

<style>
	.stack { display: flex; flex-direction: column; gap: 8px; }
	.ui-sk { position: relative; display: block; overflow: hidden; background: var(--surface-2); }
	.ui-sk::after { content: ''; position: absolute; inset: 0; background: linear-gradient(100deg, transparent 25%, var(--surface-3) 50%, transparent 75%); transform: translateX(-100%); animation: sweep 1.4s ease-in-out infinite; }
	.line { height: 12px; width: 100%; border-radius: 6px; }
	.block { height: 80px; width: 100%; border-radius: var(--r-card); }
	.poster { aspect-ratio: var(--poster-ratio); width: 100%; border-radius: var(--r-card); }
	.wide { aspect-ratio: var(--wide-ratio); width: 100%; border-radius: var(--r-card); }
	.circle { width: 64px; height: 64px; border-radius: 50%; }
	@keyframes sweep { to { transform: translateX(100%); } }
	@media (prefers-reduced-motion: reduce) { .ui-sk::after { animation: none; display: none; } }
</style>
