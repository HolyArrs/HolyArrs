<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import GhostEmpty from './GhostEmpty.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import type { Empty } from './status-tabs.ts';

	// The empty list's words and its one way on. A tab whose torrents are not loaded yet says so (left of them), not that there are none. Before the first read the list shows the page loader instead.
	let { em, left = 0, loading = false, onclear, onall }: { em: Empty; left?: number; more?: boolean; loading?: boolean; onclear: () => void; onall: () => void } = $props();
	const shown = $derived(left > 0 ? { title: t('downloads.page.notLoaded.title'), text: t('downloads.page.notLoaded.text', { count: left.toLocaleString() }), act: '', label: '' } : { title: em.title, text: em.text, act: em.action?.act ?? '', label: em.action?.label ?? '' });
	const run = () => (shown.act === 'clear' ? onclear() : onall());
</script>

{#if loading}
	<div class="wait"><PageLoader loading label={t('downloads.page.loading')} /></div>
{:else}
	{#key shown.title}
		<GhostEmpty title={shown.title} text={shown.text}>
			{#snippet action()}
				{#if shown.act}<button type="button" data-testid="dl-empty-act" onclick={run}>{shown.label}</button>{/if}
			{/snippet}
		</GhostEmpty>
	{/key}
{/if}

<style>
	/* The loader sits inside the list box: a full-screen height would give the list a scrollbar while it waits. */
	.wait :global(.pl) { min-height: 240px; }
</style>
