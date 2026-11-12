<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { checkMetadata } from './health.svelte.ts';
	import { sourceOff } from '../core/source-off.svelte.ts';

	// An outage shows as a toast; rows that are off on this install show one calm note here.
	// Counted, so a page's load error stays quiet while this note is shown.
	onMount(() => {
		void checkMetadata();
		sourceOff.notices++;
		return () => sourceOff.notices--;
	});
</script>

{#if sourceOff.seen}
	<p class="notice off-note" role="status" data-testid="source-off"><Icon name="info" size={18} /><span>{t('core.api.sourceOff')}</span></p>
{/if}

<style>
	.off-note { margin-block: var(--s-3); }
	/* On phones the sticky kind bar of Explore and New & Popular sits over the first lines: start below it. */
	@media (max-width: 767px) { :global(.types) + .off-note { margin-top: 44px; } }
</style>
