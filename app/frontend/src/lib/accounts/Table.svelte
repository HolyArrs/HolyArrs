<script lang="ts" generics="Row extends Record<string, unknown>">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { Snippet } from 'svelte';

	// Dense data table with a sticky header. Pass `cell` to render a column yourself; otherwise the row value is shown as text.
	type Column = { key: string; label: string; align?: 'start' | 'end' | 'center'; width?: string; hideOnPhone?: boolean };
	let {
		columns,
		rows,
		caption,
		captionVisible = false,
		rowKey,
		cell,
		empty
	}: {
		columns: Column[];
		rows: Row[];
		caption: string;
		captionVisible?: boolean;
		rowKey: (row: Row, i: number) => string | number;
		cell?: Snippet<[Row, Column]>;
		empty?: string;
	} = $props();
	const emptyText = $derived(empty ?? t('accounts.table.empty'));
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="ui-table" tabindex="0" role="region" aria-label={caption}>
	<table>
		<caption class:sr-only={!captionVisible}>{caption}</caption>
		<thead>
			<tr>{#each columns as c (c.key)}<th scope="col" class="a-{c.align ?? 'start'}" class:phone-hide={c.hideOnPhone} style:width={c.width}>{c.label}</th>{/each}</tr>
		</thead>
		<tbody>
			{#each rows as row, i (rowKey(row, i))}
				<tr>
					{#each columns as c (c.key)}
						<td class="a-{c.align ?? 'start'}" class:phone-hide={c.hideOnPhone}>{#if cell}{@render cell(row, c)}{:else}{String(row[c.key] ?? '')}{/if}</td>
					{/each}
				</tr>
			{:else}
				<tr><td class="empty" colspan={columns.length}>{emptyText}</td></tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	.ui-table { overflow: auto; max-height: var(--table-max, none); border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.ui-table:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	table { width: 100%; border-collapse: separate; border-spacing: 0; font: var(--t-callout); font-variant-numeric: tabular-nums; }
	caption { text-align: left; padding: 12px 14px 4px; font: var(--t-headline); }
	th { position: sticky; top: 0; z-index: 1; padding: 9px 14px; background: var(--surface-1); color: var(--text-muted); font: var(--t-footnote); font-weight: 600; text-align: left; border-bottom: 1px solid var(--line); white-space: nowrap; }
	td { padding: 10px 14px; border-bottom: 1px solid var(--line); vertical-align: middle; }
	tbody tr:last-child td { border-bottom: 0; }
	tbody tr:hover td { background: var(--surface-2); }
	.a-end { text-align: right; }
	.a-center { text-align: center; }
	.empty { color: var(--text-muted); text-align: center; padding: 28px; }
	@media (max-width: 767px) { .phone-hide { display: none; } }
</style>
