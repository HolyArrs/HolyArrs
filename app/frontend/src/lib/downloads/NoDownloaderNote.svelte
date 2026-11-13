<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { to } from '#lib/core/paths.ts';
	// Why a title's wanted copies are not sent: what the Sources find needs a kind of Downloader that is not set up.
	let { kind, manage = true }: { kind?: string; manage?: boolean } = $props();
</script>

{#if kind === 'usenet' || kind === 'torrent'}
	<p class="nodl" role="status" data-testid="no-downloader">
		<Icon name="alert" size={15} />
		<span>{kind === 'usenet' ? t('downloads.noDl.usenet') : t('downloads.noDl.torrent')}{#if manage}{' '}<a href={to('/settings/downloaders')}>{t('downloads.noDl.add')}</a>{/if}</span>
	</p>
{/if}

<style>
	.nodl { display: flex; align-items: flex-start; gap: 8px; margin-top: 8px; padding: 8px 12px; border-radius: var(--r-btn); background: color-mix(in srgb, var(--wait) 12%, var(--surface-1)); color: var(--text); font: var(--t-callout); overflow-wrap: anywhere; }
	.nodl > :global(.icon) { flex: none; margin-top: 2px; color: var(--wait-text); }
	a { color: inherit; text-decoration: underline; white-space: nowrap; }
</style>
