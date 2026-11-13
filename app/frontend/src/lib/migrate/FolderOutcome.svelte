<script lang="ts">
	import { ltr, t } from '#lib/core/i18n/i18n.svelte.ts';
	import { folderOutcome, type Line } from './move.ts';

	let { lines, after = false }: { lines: Line[]; after?: boolean } = $props();
	const o = $derived(folderOutcome(lines));
</script>

{#if o.attention > 0 || o.unchecked > 0}
	<ul class="fo" data-testid="folder-outcome" aria-label={t('migrate.titleFolders')}>
		<li class="fo-row" data-testid="folder-outcome-came"><b>{after ? t('migrate.cameOverN', { n: o.came }) : t('migrate.willComeOverN', { n: o.came })}</b></li>
		{#if o.attention > 0}
			<li class="fo-row" data-testid="folder-outcome-attention">
				<b>{t('migrate.needAttentionN', { n: o.attention })}</b>
				<span class="fo-sub">{t('migrate.needAttentionWhy', { list: o.attentionFolders.map(ltr).join(', ') })}</span>
			</li>
		{/if}
		{#if o.unchecked > 0}
			<li class="fo-row" data-testid="folder-outcome-unchecked">
				<b>{t('migrate.leftOutUncheckedN', { n: o.unchecked })}</b>
				{#each o.uncheckedFolders as f (f)}
					<span class="fo-sub">{t('migrate.leftOutUncheckedWhy', { folder: ltr(f) })}</span>
				{/each}
			</li>
		{/if}
	</ul>
{/if}

<style>
	.fo { list-style: none; margin: 0; padding: 0 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.fo-row { display: flex; flex-direction: column; gap: 2px; padding: 12px 0; font: var(--t-callout); color: var(--text); min-width: 0; overflow-wrap: anywhere; }
	.fo-row + .fo-row { border-top: 1px solid var(--line); }
	.fo-row b { font-weight: 600; }
	.fo-sub { color: var(--text-muted); font: var(--t-footnote); }
</style>
