<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { moreOf } from './status-tabs.ts';

	// Torrents HolyArrs did not add that the page has not loaded yet: a note under the tabs, or Show more at the end of the list.
	let { mo, filtered = false, loaded, capMax, onmore, end = false }: { mo: ReturnType<typeof moreOf>; filtered?: boolean; loaded: number; capMax: number; onmore?: () => void; end?: boolean } = $props();
</script>

{#if end}
	{#if mo.more}<p class="more"><button type="button" class="mn" onclick={() => onmore?.()} data-testid="dl-outside-more">{t('downloads.page.showMore')}</button></p>{/if}
{:else if mo.left > 0}
	<p class="morenote" id="dl-morenote" data-testid="dl-morenote">
		{#if mo.more}<span>{t(filtered ? 'downloads.page.partialFiltered' : 'downloads.page.partial', { shown: loaded.toLocaleString(), total: mo.total.toLocaleString() })}</span><button type="button" class="mn" onclick={() => onmore?.()} data-testid="dl-outside-more-top">{t('downloads.page.showMore')}</button>
		{:else}<span>{t('downloads.page.capped', { max: capMax.toLocaleString(), total: mo.total.toLocaleString() })}</span>{/if}
	</p>
{/if}

<style>
	.morenote { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 8px; margin: 0; padding: 4px var(--dl-gutter); border-bottom: 1px solid var(--line); color: var(--text-muted); font: 400 12px/16px var(--font-ui); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
	.more { display: flex; justify-content: center; margin: 0; padding: 12px var(--dl-gutter) 16px; color: var(--text-muted); font: 400 12px/16px var(--font-ui); text-align: center; }
	.mn { min-height: 24px; padding: 0 4px; border: 0; background: none; color: var(--accent-text); font: inherit; font-weight: 600; cursor: pointer; }
	.mn:hover { text-decoration: underline; }
	.mn:focus-visible { outline: none; border-radius: 4px; box-shadow: var(--focus-ring); }
	/* Desktop: the tab row has no line under it, so neither does the note. */
	@media (min-width: 641px) { .morenote { border-bottom: 0; } }
	/* Touch phones get a 44px target; the hit area grows with an invisible ::after so nothing moves. */
	@media (max-width: 767px) and (pointer: coarse) {
		.mn { position: relative; }
		.mn::after { content: ''; position: absolute; inset: -10px 0; }
	}
</style>
