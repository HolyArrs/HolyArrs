<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { pageLine } from './history.ts';
	import type { EventFeed } from './history-feed.svelte.ts';
	import type { Snippet } from 'svelte';

	// What History shows around its rows: a placeholder while the first page loads, the error, the empty tab, the count line and Show more.
	let {
		feed,
		empty,
		onretry,
		onmore,
		onall,
		count = true,
		held = false,
		children
	}: {
		feed: EventFeed;
		empty: { title: string; text: string };
		onretry: () => void;
		onmore: () => void;
		/** Shown on an empty, filtered list; clears the filters. */
		onall?: () => void;
		/** Hidden when a page filter leaves nothing to count. */
		count?: boolean;
		/** Rows already loaded but held back so the first view fits the screen; Show more reveals them. */
		held?: boolean;
		children: Snippet;
	} = $props();
</script>

{#if feed.error && !feed.events.length}
	<div class="empty" role="alert" data-testid="look-error">
		<span class="eic fail"><Icon name="alert" size={22} stroke={2} /></span>
		<p class="et">{t('downloads.hf.couldNot')}</p>
		<Button variant="secondary" size="sm" icon="refresh" busy={feed.busy} onclick={onretry}>{t('health.sections.tryAgain')}</Button>
	</div>
{:else if !feed.loaded}
	<div class="sk" aria-busy="true" aria-label={t('downloads.hf.loading')}>{#each Array(8) as _, i (i)}<Skeleton height="44px" />{/each}</div>
{:else if !feed.events.length}
	<div class="empty" data-testid="look-empty">
		<span class="eic"><Icon name="clock" size={22} stroke={2} /></span>
		<p class="et">{empty.title}</p>
		{#if empty.text}<p class="es">{empty.text}</p>{/if}
		{#if onall}<button type="button" class="elink" onclick={onall} data-testid="history-clear-empty">{t('downloads.bar.clear')}</button>{/if}
	</div>
{:else}
	{#if feed.error}<LoadError hasData message={t('downloads.hf.couldNot')} onretry={onretry} />{/if}
	<p class="count" class:gone={!count} aria-live="polite" data-testid="history-count">{t('downloads.hf.events', { range: pageLine(feed.total, 0, feed.events.length) })}</p>
	{@render children()}
	{#if held || feed.events.length < feed.total}
		<div class="more"><Button variant="secondary" busy={feed.busy} disabled={feed.busy} onclick={onmore}>{t('downloads.hf.showMore')}</Button></div>
	{/if}
{/if}

<style>
	.sk { display: grid; gap: 6px; }
	.count.gone { display: none; }
	.count { margin: 0 0 8px; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.more { display: flex; justify-content: center; margin-top: 16px; }
	.empty { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 40px 20px; text-align: center; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); }
	.eic { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 12px; background: var(--surface-2); color: var(--text-muted); }
	.empty > :global(.ui-btn) { margin-top: 6px; }
	.eic.fail { background: color-mix(in srgb, var(--fail) 12%, transparent); color: var(--fail-text); }
	.et { font: var(--t-headline); }
	.es { color: var(--text-muted); font: var(--t-callout); max-width: 360px; }
	.elink { display: inline-flex; align-items: center; min-height: 44px; padding: 0 4px; border: 0; background: none; color: var(--accent-text); font: var(--t-callout); font-weight: 600; cursor: pointer; border-radius: 6px; }
	.elink:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) { .more > :global(*) { flex: 1; min-height: 44px; } }
</style>
