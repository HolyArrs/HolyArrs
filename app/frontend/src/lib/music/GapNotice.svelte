<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { PageGap } from './music-pages.ts';

	// The metadata server is still getting this (fetching, or waiting in line); its own words stand in place of the content until it lands. Never "not found".
	let { gap }: { gap: PageGap } = $props();
</script>

<p class="gap" role="status" aria-live="polite" data-testid="gap-notice" data-state={gap.state}>
	<span class="dot" aria-hidden="true"></span>{gap.message || t('music.gettingThisFromThe')}
</p>

<style>
	.gap { display: flex; align-items: center; gap: 10px; margin: 0; padding: 12px 0; color: var(--text-muted); font: var(--t-subhead); }
	.dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: currentColor; animation: gap-pulse 1.4s ease-in-out infinite; }
	@keyframes gap-pulse { 50% { opacity: 0.25; } }
	@media (prefers-reduced-motion: reduce) { .dot { animation: none; } }
</style>
