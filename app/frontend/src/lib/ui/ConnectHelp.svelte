<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { connectHelpUrl } from '#lib/core/test-tag.ts';

	// Link to the Connect page row that explains a failed test's name line.
	let { text = '' }: { text?: string | null } = $props();

	const href = $derived(connectHelpUrl(text));
</script>

{#if href}<a class="connect-help" {href} target="_blank" rel="noopener" data-testid="connect-help">{t('ui.connectHelp.link')}</a>{/if}

<style>
	.connect-help { margin-left: 6px; white-space: nowrap; }
</style>
