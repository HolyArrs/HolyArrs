<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { dropIn, dropOut } from '../core/transitions.ts';
	import type { Kind } from './library.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	type Props = {
		selecting: boolean;
		phone: boolean;
		kind: Kind;
		busy: boolean;
		pickedCount: number;
		pickedIds: number[];
		shownCount: number;
		allPicked: boolean;
		copyPicksCount: number;
		phoneActs: MenuItem[];
		moveItems: MenuItem[];
		rulesItems: MenuItem[];
		pickAll: () => void;
		setKeep: (ids: number[], keep: boolean) => void;
		searchTitles: (ids: number[]) => void;
		reviewCopies: () => void;
		askDelete: (ids: number[]) => void;
		stopSelecting: () => void;
	};
	let { selecting, phone, kind, busy, pickedCount, pickedIds, shownCount, allPicked, copyPicksCount, phoneActs, moveItems, rulesItems, pickAll, setKeep, searchTitles, reviewCopies, askDelete, stopSelecting }: Props = $props();
</script>

{#if selecting}
	<div class="select-bar" role="toolbar" aria-label={t('mediafiles.select.selected')} data-testid="select-bar" in:dropIn out:dropOut>
		<span class="count" aria-live="polite">{t('mediafiles.select.count', { n: pickedCount })}</span>
		{#if !phone}<Button variant="plain" size="md" onclick={pickAll}>{allPicked ? t('mediafiles.select.none') : t('mediafiles.select.all', { n: shownCount })}</Button>{/if}
		<span class="bar-gap"></span>
		{#if phone}
			<Menu label={t('mediafiles.select.actions')} icon="more" items={phoneActs} width={240} placement="above" align="end" />
		{:else}
			<Button size="md" icon="bellOn" disabled={busy || !pickedCount} onclick={() => setKeep(pickedIds, true)}>{t('mediafiles.select.keep')}</Button>
			<Button size="md" icon="bell" disabled={busy || !pickedCount} onclick={() => setKeep(pickedIds, false)}>{t('mediafiles.select.stopKeeping')}</Button>
			{#if moveItems.length > 1}<Menu label={t('mediafiles.select.moveLabel')} variant="button" text={t('mediafiles.select.move')} icon="folder" items={moveItems} width={260} placement="above" />{/if}
			{#if rulesItems.length}<Menu label={t('mediafiles.select.qualityLabel')} variant="button" text={t('mediafiles.select.quality')} icon="star" items={rulesItems} width={220} placement="above" />{/if}
			<Button size="md" icon="search" disabled={busy || !pickedCount} onclick={() => searchTitles(pickedIds)}>{t('mediafiles.select.searchNow')}</Button>
			{#if kind === 'movie'}<Button size="md" icon="copy" disabled={busy || !copyPicksCount} onclick={reviewCopies} data-testid="select-find-copy">{t('mediafiles.select.findCopy')}</Button>{/if}
		{/if}
		<Button variant="destructive" size="md" icon="trash" disabled={busy || !pickedCount} onclick={() => askDelete(pickedIds)} data-testid="select-delete">{t('mediafiles.select.remove')} {pickedCount || ''}</Button>
		<IconButton icon="x" label={t('mediafiles.select.stop')} onclick={stopSelecting} />
	</div>
{/if}

<style>
	.select-bar { position: sticky; bottom: 16px; z-index: 20; container-type: inline-size; display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 6px; margin-top: 8px; padding: 10px 12px; border-radius: var(--r-panel); background: color-mix(in srgb, var(--sheet-a) 90%, transparent); -webkit-backdrop-filter: blur(16px); backdrop-filter: blur(16px); box-shadow: var(--shadow-card), inset 0 1px 0 rgb(255 255 255 / 6%), 0 8px 28px rgb(0 0 0 / 25%); }
	/* Narrow: the bar's buttons fold to their icons (Remove keeps its word), the toolbar's own trick. */
	@container (max-width: 1000px) {
		.select-bar :global(.ui-btn.secondary > span), .select-bar :global(.trigger.button > span), .select-bar :global(.trigger.button > svg:last-child) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
		.select-bar :global(.ui-btn.secondary), .select-bar :global(.trigger.button) { width: 36px; padding-inline: 0; justify-content: center; }
	}
	/* Phones: the bar stays above the bottom tab bar and the safe area. */
	@media (max-width: 767px) { .select-bar { bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom, 0px) + 8px); } }
	@media (max-width: 639px) { .select-bar { flex-wrap: nowrap; } .select-bar .bar-gap { flex: 1 1 0; min-width: 0; } }
	.select-bar > :global(.ui-ibtn:last-child) { order: -1; }
	/* A little tighter, so the whole bar stays on one line beside an open side menu. */
	.select-bar :global(.ui-btn.md), .select-bar :global(.trigger.button) { padding-inline: 11px; }
	.select-bar :global(.ui-btn.plain) { padding-inline: 6px; }
	.select-bar .count { padding: 0 8px; font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.bar-gap { flex: 1 1 auto; }
</style>
