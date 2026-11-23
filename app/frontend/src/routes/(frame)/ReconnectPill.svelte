<script lang="ts">
	// Small top-of-page pill while live updates reconnect; the page underneath stays usable.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	let { show }: { show: boolean } = $props();
</script>

<div class="reconnect" aria-live="polite" data-testid="reconnect-pill">
	{#if show}<span class="ui-pill"><i aria-hidden="true"></i>{t('core.loader.reconnecting')}</span>{/if}
</div>

<style>
	.reconnect { position: fixed; top: calc(env(safe-area-inset-top, 0px) + 12px); left: 0; right: 0; display: flex; justify-content: center; pointer-events: none; z-index: 60; padding: 0 16px; }
	/* same look as StatusPill (wait tone), with a soft shadow so it reads over any page */
	.ui-pill { display: inline-flex; align-items: center; gap: 6px; height: 24px; padding: 0 10px 0 8px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); font: var(--t-subhead); white-space: nowrap; max-width: 100%; overflow: hidden; text-overflow: ellipsis; box-shadow: 0 4px 16px rgb(0 0 0 / 0.25); animation: in var(--dur, 200ms) var(--ease-out, ease-out); }
	i { width: 7px; height: 7px; border-radius: 50%; background: var(--wait); flex: none; animation: pulse 1.4s ease-in-out infinite; }
	@keyframes in { from { opacity: 0; transform: translateY(-6px); } }
	@keyframes pulse { 50% { opacity: 0.35; } }
	@media (prefers-reduced-motion: reduce) { .ui-pill, i { animation: none; } }
</style>
