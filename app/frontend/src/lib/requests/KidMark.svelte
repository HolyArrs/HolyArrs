<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// A person's age limit next to their name, so a Kid's ask is seen as one.
	let { label, size = 'md' }: { label: string; size?: 'sm' | 'md' } = $props();
</script>

{#if label}<span class="kid {size}" data-testid="ask-kid" title={t('requests.kidMark.title')}>{label}</span>{/if}

<style>
	.kid { display: inline-flex; align-items: center; gap: 4px; padding: 2px 8px; border-radius: var(--r-pill); background: color-mix(in srgb, var(--mark-ask) 24%, var(--surface-1)); color: color-mix(in srgb, var(--mark-ask) 45%, var(--text)); font: var(--t-footnote); font-weight: 600; white-space: nowrap; box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--mark-ask) 45%, transparent); }
	.kid.sm { padding: 1px 6px; font-size: 11px; }
</style>
