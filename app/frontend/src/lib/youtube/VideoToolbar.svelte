<script lang="ts">
	// The YouTube page's one row, laid out like the Library's: search, grid or list, sort and filter, and Add at the end.
	import { tick, type Snippet } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { DlFind } from '#lib/downloads/index.ts';
	import { LibraryViewSwitch } from '#lib/mediafiles/index.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import type { YtView } from './list.ts';

	type Props = {
		phone: boolean;
		loaded: boolean;
		countText: string;
		shown: number;
		total: number;
		view: YtView;
		setView: (v: YtView) => void;
		filtersOn: number;
		sortOpen: boolean;
		addOpen: boolean;
		findOpen: boolean;
		q: string;
		searchKey: string;
		sidebarCollapsed: boolean;
		onsidebar: () => void;
		/** The Sort and filter panel (the page shows it in a sheet on a phone). */
		sortFilter: Snippet;
		/** The link box (the page shows it in a sheet on a phone). */
		add: Snippet;
	};
	let { phone, loaded, countText, shown, total, view, setView, filtersOn, sortOpen = $bindable(), addOpen = $bindable(), findOpen = $bindable(), q = $bindable(), searchKey, sidebarCollapsed, onsidebar, sortFilter, add }: Props = $props();

	const findWords = $derived(t('youtube.find'));
	const sortWords = $derived(filtersOn ? t('mediafiles.toolbar.sortFilterN', { n: filtersOn }) : t('mediafiles.toolbar.sortFilter'));

	let toolbarEl = $state<HTMLElement>();
	let stuck = $state(false);
	function checkStuck() {
		if (!toolbarEl) return;
		const bar = parseFloat(getComputedStyle(toolbarEl).top) || 0;
		stuck = window.scrollY > 0 && getComputedStyle(toolbarEl).position === 'sticky' && toolbarEl.getBoundingClientRect().top <= bar + 0.5;
	}
</script>

<svelte:window onscroll={checkStuck} onresize={checkStuck} />

<div class="toolbar" class:stuck class:topbar={!phone} role="toolbar" aria-label={t('youtube.title')} bind:this={toolbarEl} data-testid="youtube-toolbar">
	{#if phone}
		<p class="count" aria-live="polite">{#if loaded}{countText}{/if}</p>
	{:else}
		{#if sidebarCollapsed}<span class="cap"><IconButton icon="sidebar" label={t('library.page.showSidebar')} onclick={onsidebar} /></span>{/if}
		<DlFind bind:term={q} {shown} {total} {searchKey} placeholder={findWords} note={loaded ? countText : ''} testid="youtube-find" />
	{/if}
	<span class="t-view cap">
		<LibraryViewSwitch
			value={view}
			options={[
				{ value: 'grid', label: t('youtube.view.grid'), icon: 'grid' },
				{ value: 'list', label: t('youtube.view.list'), icon: 'menu' }
			]}
			onchange={(v) => setView(v)}
			compact={phone}
		/>
	</span>
	<span class="filter-btn cap" class:on={filtersOn > 0} data-testid="youtube-sort-filter">
		{#if phone}
			<Button size="md" icon="sliders" aria-haspopup="dialog" onclick={() => (sortOpen = true)}>{sortWords}</Button>
		{:else}
			<Popover label={t('mediafiles.toolbar.sortAndFilter')} variant="button" icon="sliders" text={sortWords} width={560} panelClass="sf-pop" bind:open={sortOpen}>
				{#snippet children()}{@render sortFilter()}{/snippet}
			</Popover>
		{/if}
	</span>
	{#if phone}
		<span class="t-find"><IconButton icon="search" label={findWords} aria-pressed={findOpen || !!q} onclick={() => { findOpen = !findOpen || !!q; if (findOpen) void tick().then(() => document.getElementById('find')?.focus()); }} /></span>
	{/if}
	<span class="tool-gap"></span>
	<span class="t-tools cap">
		<span class="t-add" data-testid="youtube-add-open">
			{#if phone}
				<IconButton icon="plus" label={t('youtube.add.button')} aria-haspopup="dialog" onclick={() => (addOpen = true)} />
			{:else}
				<Popover label={t('youtube.add.button')} variant="button" icon="plus" text={t('youtube.add.button')} width={440} align="end" bind:open={addOpen}>
					{#snippet children()}{@render add()}{/snippet}
				</Popover>
			{/if}
		</span>
	</span>
	{#if phone && (findOpen || q)}
		<div class="find-box rise">
			<span class="find-ic" aria-hidden="true"><Icon name="search" size={16} /></span>
			<label class="sr-only" for="find">{findWords}</label>
			<input id="find" class="input find" type="search" bind:value={q} onblur={() => { if (!q) findOpen = false; }} placeholder={findWords} autocomplete="off" maxlength="256" />
		</div>
	{/if}
</div>

<style>
	/* The Library's toolbar, with the YouTube page's controls: one row, no top bar above it, each group in the same glass capsule. Pinned, it is one solid band. */
	:global(.panel.panel.sf-pop) { max-height: min(calc(100dvh - 96px), 760px); }
	.toolbar { position: sticky; top: 0; z-index: 6; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: calc(-1 * var(--s-6, 24px)) calc(-1 * var(--gutter, 0px)) 0; padding: 10px var(--gutter, 0px); background: var(--bg); border-bottom: 1px solid transparent; transition: border-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.toolbar.stuck { border-bottom-color: var(--line); box-shadow: 0 10px 24px rgb(0 0 0 / .08); }
	.toolbar.topbar .cap { display: flex; flex: none; align-items: center; gap: 2px; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.toolbar.topbar .tool-gap { display: none; }
	/* Inside a capsule every control is flat and 44px tall; the capsule is its frame. */
	.toolbar.topbar .cap :global(.ui-btn.secondary), .toolbar.topbar .cap :global(.trigger.button) { height: 44px; min-height: 44px; padding-inline: 12px; border-radius: 10px; background: none; box-shadow: none; }
	.toolbar.topbar .cap :global(.ui-btn.secondary:hover), .toolbar.topbar .cap :global(.trigger.button:hover), .toolbar.topbar .cap :global(.trigger.button[aria-expanded='true']) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.toolbar.topbar .cap :global(.ui-ibtn) { width: 44px; height: 44px; border-radius: 10px; }
	.toolbar.topbar .t-view { --vs-bg: transparent; }
	.toolbar.topbar .t-view :global(.vs) { padding: 0; gap: 2px; }
	.toolbar.topbar .t-view :global(.vs button[role='radio']) { min-height: 44px; min-width: 44px; }
	.toolbar.topbar .t-view :global(.vs .seg) { border-radius: 10px; }
	.toolbar.topbar .filter-btn.on :global(.trigger.button) { box-shadow: inset 0 0 0 1.5px var(--accent); color: var(--accent-text); }
	.t-tools, .t-add { display: contents; }
	.toolbar.topbar .t-tools { display: flex; margin-left: auto; }
	/* A narrower page keeps the one row: the button names fold away, hidden from the eye only. */
	.toolbar.topbar { container-type: inline-size; }
	.toolbar.topbar :global(.dlfind) { flex-basis: 200px; min-width: 180px; }
	@container (max-width: 780px) {
		.toolbar.topbar :global(.dlfind) { flex-basis: 150px; min-width: 150px; }
		.toolbar.topbar :global(.dlfind .kb) { display: none; }
		.toolbar.topbar .t-tools :global(.trigger.button > span), .toolbar.topbar .t-tools :global(.trigger.button > svg:last-child), .toolbar.topbar .filter-btn :global(.trigger.button > span), .toolbar.topbar .filter-btn :global(.trigger.button > svg:last-child) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
		.toolbar.topbar .t-tools :global(.trigger.button), .toolbar.topbar .filter-btn :global(.trigger.button) { width: 44px; padding-inline: 0; justify-content: center; }
	}
	.toolbar .count { margin: 0 8px 0 0; min-width: 0; font: var(--t-headline); font-weight: 650; color: var(--text); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.toolbar .count:empty { display: none; }
	.find-box { position: relative; flex: 1 1 220px; max-width: 360px; min-width: 0; }
	.find-ic { position: absolute; left: 11px; top: 50%; translate: 0 -50%; color: var(--text-muted); display: grid; pointer-events: none; }
	.find { width: 100%; min-height: 40px; padding-left: 34px; border-radius: var(--r-btn); background: var(--surface-2); border-color: transparent; }
	@media (pointer: coarse) and (max-width: 767px) { .find { min-height: 44px; } }
	.tool-gap { flex: 1 1 auto; }
	.filter-btn.on :global(button[aria-haspopup]) { box-shadow: inset 0 0 0 1.5px var(--accent); color: var(--accent-text); }
	.toolbar.topbar .t-view { order: 2; }
	.toolbar.topbar .filter-btn { order: 3; }
	.toolbar.topbar .tool-gap { order: 4; }
	.toolbar.topbar .t-tools { order: 5; }
	@media (max-width: 639px) {
		.toolbar { position: static; margin: 0; padding: 8px 0; }
		.t-add, .t-view, .filter-btn { display: inline-flex; }
		/* Two rows: count, search, add; then grid or list, sort and filter. */
		.toolbar .count { order: 0; flex: 1 1 auto; margin: 0; overflow: hidden; text-overflow: ellipsis; }
		.t-find { order: 1; }
		.t-add { order: 3; }
		.tool-gap { display: block; order: 4; flex-basis: 100%; height: 0; }
		.t-view { order: 5; }
		.filter-btn { order: 6; }
		.find-box { flex-basis: 100%; max-width: none; order: 4; }
	}
</style>
