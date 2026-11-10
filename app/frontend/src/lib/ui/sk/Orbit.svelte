<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Small spinning ring with an optional status line, for a region that is slow to load.
	let { status = '', size = 24 }: { status?: string; size?: number } = $props();
</script>

<span class="orbit" role="status" aria-live="polite">
	<svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
		<circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-opacity=".18" stroke-width="2" />
		<circle class="ring" cx="12" cy="12" r="10" fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-dasharray="16 47" />
	</svg>
	{#if status}<span class="t">{status}</span>{:else}<span class="sr-only">{t('ui.skOrbit.loading')}</span>{/if}
</span>

<style>
	.orbit { display: inline-flex; align-items: center; gap: 10px; color: var(--text-muted); font: var(--t-callout); }
	svg { flex: none; overflow: visible; }
	.ring { transform-origin: 12px 12px; animation: orbit-turn 1.1s linear infinite; }
	@keyframes orbit-turn { to { transform: rotate(360deg); } }
	/* Screen reader only text. */
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	/* Reduced motion swaps the spin for a pulse; !important beats the global motion reset. */
	@media (prefers-reduced-motion: reduce) { .ring { animation: orbit-pulse 1.6s ease-in-out infinite !important; animation-duration: 1.6s !important; animation-iteration-count: infinite !important; } @keyframes orbit-pulse { 50% { opacity: .35; } } }
</style>
