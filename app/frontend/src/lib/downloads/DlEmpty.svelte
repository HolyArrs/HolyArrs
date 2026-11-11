<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import GhostEmpty from './GhostEmpty.svelte';
	import type { Empty } from './status-tabs.ts';

	// The empty list's words and its one way on. A tab whose torrents are not loaded yet says so (left of them), not that there are none.
	let { em, left = 0, more = false, onclear, onall, onmore }: { em: Empty; left?: number; more?: boolean; onclear: () => void; onall: () => void; onmore?: () => void } = $props();
	const shown = $derived(left > 0 ? { title: t('downloads.page.notLoaded.title'), text: t('downloads.page.notLoaded.text', { count: left.toLocaleString() }), act: more ? 'more' : '', label: t('downloads.page.showMore') } : { title: em.title, text: em.text, act: em.action?.act ?? '', label: em.action?.label ?? '' });
	const run = () => (shown.act === 'clear' ? onclear() : shown.act === 'all' ? onall() : onmore?.());
</script>

{#key shown.title}
	<GhostEmpty title={shown.title} text={shown.text}>
		{#snippet action()}
			{#if shown.act}<button type="button" data-testid="dl-empty-act" onclick={run}>{shown.label}</button>{/if}
		{/snippet}
	</GhostEmpty>
{/key}
