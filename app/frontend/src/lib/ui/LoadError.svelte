<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Error state: a compact banner when old data is still shown, otherwise a full block.
	import Icon from './Icon.svelte';
	import Button from './Button.svelte';

	let {
		hasData,
		message = t('ui.loadError.couldNotLoad'),
		onretry,
		busy = false,
		network = false
	}: { hasData: boolean; message?: string; onretry?: () => void; busy?: boolean; network?: boolean } = $props();
</script>

{#if hasData}
	<div class="ui-loaderr inline" role="status">
		<Icon name="alert" size={17} />
		<span class="grow">{t('ui.loadError.showingWhatWasLoaded', { message })}</span>
		{#if onretry}<Button size="sm" variant="plain" {busy} onclick={onretry}>{t('ui.loadError.tryAgain')}</Button>{/if}
	</div>
{:else}
	<div class="ui-loaderr full" role="alert">
		<span class="ic"><Icon name="alert" size={26} /></span>
		<h2>{message}</h2>
		<p>{network ? t('ui.loadError.checkConnection') : t('ui.loadError.tryAgainSoon')}</p>
		{#if onretry}<Button variant="secondary" icon="refresh" {busy} onclick={onretry}>{t('ui.loadError.tryAgain')}</Button>{/if}
	</div>
{/if}

<style>
	.inline { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 4px 6px 4px 14px; border-radius: var(--r-btn); background: color-mix(in srgb, var(--wait) 12%, var(--surface-1)); color: var(--text); font: var(--t-callout); }
	.inline > :global(.icon) { color: var(--wait-text); }
	.grow { flex: 1; min-width: 0; }
	.inline :global(.ui-btn.plain) { color: var(--text); text-decoration: underline; }
	.full { display: flex; flex-direction: column; align-items: center; text-align: center; gap: 8px; padding: 48px 24px; }
	.ic { width: 60px; height: 60px; display: grid; place-items: center; border-radius: 50%; background: color-mix(in srgb, var(--fail) 12%, transparent); color: var(--fail-text); margin-bottom: 6px; }
	h2 { font: var(--t-title3); }
	p { color: var(--text-muted); margin-bottom: 8px; }
</style>
