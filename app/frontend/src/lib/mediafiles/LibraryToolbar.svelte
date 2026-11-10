<script lang="ts">
	import { tick, type Snippet } from 'svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import { densityLabels, posterLabels, type Density, type PosterSize } from '#lib/ui/library-art.ts';
	import ColumnChooser from './ColumnChooser.svelte';
	import LibraryViewSwitch from './LibraryViewSwitch.svelte';
	import type { Kind } from './library.ts';
	import { columnsFor, type Column, type LibKind } from './table.ts';
	import type { View } from './library-view.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	type Props = {
		phone: boolean;
		kind: Kind;
		kindTitle: string;
		loaded: boolean;
		error: string;
		countText: string;
		shelfItems: MenuItem[];
		sidebarCollapsed: boolean;
		sidebarButton: Snippet;
		find: Snippet;
		sortFilter: Snippet;
		view: View;
		setView: (v: View) => void;
		sizeItems: MenuItem[];
		posterSize: PosterSize;
		density: Density;
		listWidth: number;
		filtersOn: number;
		sortOpen: boolean;
		findOpen: boolean;
		findWords: string;
		q: string;
		syncUrl: () => void;
		columns: Column[];
		libKind: LibKind;
		setColumns: (c: Column[]) => void;
		canManage: boolean;
		shownCount: number;
		selecting: boolean;
		startSelecting: () => void;
		stopSelecting: () => void;
		addItems: MenuItem[];
	};
	let { phone, kind, kindTitle, loaded, error, countText, shelfItems, sidebarCollapsed, sidebarButton, find, sortFilter, view, setView, sizeItems, posterSize, density, listWidth, filtersOn, sortOpen = $bindable(), findOpen = $bindable(), findWords, q = $bindable(), syncUrl, columns, libKind, setColumns, canManage, shownCount, selecting, startSelecting, stopSelecting, addItems }: Props = $props();

	let toolbarEl = $state<HTMLElement>();
	let stuck = $state(false);
	function checkStuck() {
		if (!toolbarEl) return;
		const bar = parseFloat(getComputedStyle(toolbarEl).top) || 0;
		stuck = window.scrollY > 0 && getComputedStyle(toolbarEl).position === 'sticky' && toolbarEl.getBoundingClientRect().top <= bar + 0.5;
	}
</script>

<svelte:window onscroll={checkStuck} onresize={checkStuck} />

<div class="toolbar" class:stuck class:topbar={!phone} role="toolbar" aria-label={t('mediafiles.toolbar.label')} bind:this={toolbarEl}>
	{#if phone}
		{#if shelfItems.length > 1}
			<span class="count shelf"><Menu label={t('mediafiles.toolbar.shelf', { now: kindTitle })} variant="button" icon={kind === 'show' ? 'tv' : kind === 'artist' ? 'music' : 'film'} text={loaded && !error ? countText : kindTitle} items={shelfItems} width={200} align="start" /></span>
		{:else}
			<p class="count" aria-live="polite">{#if loaded && !error}{countText}{/if}</p>
		{/if}
	{:else}
		{#if sidebarCollapsed}<span class="cap">{@render sidebarButton()}</span>{/if}
		{@render find()}
	{/if}
	<span class="t-view cap"><LibraryViewSwitch value={view} options={[{ value: 'rows', label: t('mediafiles.toolbar.rows'), icon: 'menu' }, { value: 'table', label: t('mediafiles.toolbar.banners'), icon: 'image' }, { value: 'posters', label: t('mediafiles.toolbar.posters'), icon: 'grid' }, { value: 'overview', label: t('mediafiles.toolbar.overview'), icon: 'info' }]} onchange={(v) => setView(v)} sizeItems={!phone && (view === 'table' || view === 'posters') ? sizeItems : null} sizeLabel={view === 'posters' ? posterLabels[posterSize] : densityLabels[density]} compact={phone || listWidth < 1100} /></span>
	<span class="filter-btn cap" class:on={filtersOn > 0}>
		{#if phone}
			<Button size="md" icon="sliders" aria-haspopup="dialog" onclick={() => (sortOpen = true)} data-testid="library-sort-filter">{filtersOn ? t('mediafiles.toolbar.sortFilterN', { n: filtersOn }) : t('mediafiles.toolbar.sortFilter')}</Button>
		{:else}
			<Popover label={t('mediafiles.toolbar.sortAndFilter')} variant="button" icon="sliders" text={filtersOn ? t('mediafiles.toolbar.sortFilterN', { n: filtersOn }) : t('mediafiles.toolbar.sortFilter')} width={620} bind:open={sortOpen}>
				{#snippet children()}{@render sortFilter()}{/snippet}
			</Popover>
		{/if}
	</span>
	{#if phone}
		<span class="t-find"><IconButton icon="search" label={findWords} aria-pressed={findOpen || !!q} onclick={() => { findOpen = !findOpen || !!q; if (findOpen) void tick().then(() => document.getElementById('find')?.focus()); }} /></span>
	{/if}
	<span class="tool-gap"></span>
	<span class="t-tools cap">
	{#if view !== 'posters' && kind !== 'all' && !phone}<span class="t-cols"><ColumnChooser {columns} onchange={setColumns} onreset={() => setColumns(columnsFor(libKind))} /></span>{/if}
	{#if canManage && shownCount}<span class="t-sel">{#if phone}<IconButton icon="check" variant={selecting ? 'filled' : 'tinted'} label={selecting ? t('mediafiles.toolbar.done') : t('mediafiles.toolbar.select')} onclick={() => (selecting ? stopSelecting() : startSelecting())} data-testid="library-select" />{:else}<Button variant={selecting ? 'primary' : 'secondary'} size="md" icon="check" onclick={() => (selecting ? stopSelecting() : startSelecting())} data-testid="library-select">{selecting ? t('mediafiles.toolbar.done') : t('mediafiles.toolbar.select')}</Button>{/if}</span>{/if}
	{#if canManage}
		<span class="t-add"><Menu label={t('mediafiles.toolbar.addTo')} variant={phone ? 'icon' : 'button'} text={phone ? undefined : t('mediafiles.toolbar.add')} icon="plus" items={addItems} width={190} align="end" /></span>
	{/if}
	</span>
	{#if phone && (findOpen || q)}
		<div class="find-box rise">
			<span class="find-ic" aria-hidden="true"><Icon name="search" size={16} /></span>
			<label class="sr-only" for="find">{findWords}</label>
			<input id="find" class="input find" type="search" bind:value={q} oninput={(event) => { q = event.currentTarget.value; syncUrl(); }} onblur={() => { if (!q) findOpen = false; }} placeholder={findWords} autocomplete="off" maxlength="256" />
		</div>
	{/if}
</div>

<style>
	/* One row, and the page has no top bar above it (as on Downloads): search, view and size, sort and filter, columns, select, add. Each group sits in the same glass capsule the Downloads row uses. Pinned, it is one solid band. */
	.toolbar { position: sticky; top: 0; z-index: 6; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: calc(-1 * var(--s-6, 24px)) calc(-1 * var(--gutter, 0px)) 0; padding: 10px var(--gutter, 0px); background: var(--bg); border-bottom: 1px solid transparent; transition: border-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.toolbar.stuck { border-bottom-color: var(--line); box-shadow: 0 10px 24px rgb(0 0 0 / .08); }
	.toolbar.topbar .cap { display: flex; flex: none; align-items: center; gap: 2px; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	/* Over the poster wall the bar is the same light glass as the app's top bar. */
	:global(html:has([data-poster-wall])) .toolbar { background: color-mix(in srgb, var(--bg) 62%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); }
	.toolbar.topbar .cap:empty { display: none; }
	.toolbar.topbar .tool-gap { display: none; }
	/* Inside a capsule every control is flat and 44px tall; the capsule is its frame. */
	.toolbar.topbar .cap :global(.ui-btn.secondary), .toolbar.topbar .cap :global(.trigger.button) { height: 44px; min-height: 44px; padding-inline: 12px; border-radius: 10px; background: none; box-shadow: none; }
	.toolbar.topbar .cap :global(.ui-btn.primary) { height: 44px; min-height: 44px; border-radius: 10px; }
	.toolbar.topbar .cap :global(.ui-btn.secondary:hover), .toolbar.topbar .cap :global(.trigger.button:hover), .toolbar.topbar .cap :global(.trigger.button[aria-expanded='true']) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.toolbar.topbar .cap :global(.ui-ibtn) { width: 44px; height: 44px; border-radius: 10px; }
	.toolbar.topbar .t-view { --vs-bg: transparent; }
	.toolbar.topbar .t-view :global(.vs) { padding: 0; gap: 2px; }
	.toolbar.topbar .t-view :global(.vs button[role='radio']) { min-height: 44px; min-width: 44px; }
	.toolbar.topbar .t-view :global(.vs .seg) { border-radius: 10px; }
	.toolbar.topbar .filter-btn.on :global(.trigger.button) { box-shadow: inset 0 0 0 1.5px var(--accent); color: var(--accent-text); }
	.t-tools, .t-sel, .t-add, .t-cols { display: contents; }
	.toolbar.topbar .t-tools { display: flex; margin-left: auto; }
	/* A narrower page keeps the one row: first Columns and Select fold to their icons, then Tools and Add too. A folded name is only hidden from the eye, so the button keeps its name for screen readers. */
	.toolbar.topbar { container-type: inline-size; }
	.toolbar.topbar :global(.dlfind) { flex-basis: 200px; min-width: 180px; }
	@container (max-width: 1000px) {
		.toolbar.topbar .t-cols :global(.trigger.button > span), .toolbar.topbar .t-cols :global(.trigger.button > svg:last-child) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
		.toolbar.topbar .t-sel :global(.ui-btn) { font-size: 0; gap: 0; }
		.toolbar.topbar .t-sel :global(.ui-btn), .toolbar.topbar .t-cols :global(.trigger.button) { width: 44px; padding-inline: 0; justify-content: center; }
	}
	@container (max-width: 780px) {
		.toolbar.topbar :global(.dlfind) { flex-basis: 150px; min-width: 150px; }
		.toolbar.topbar :global(.dlfind .kb) { display: none; }
	}
	@container (max-width: 1000px) {
		.toolbar.topbar .t-tools :global(.trigger.button > span), .toolbar.topbar .t-tools :global(.trigger.button > svg:last-child), .toolbar.topbar .filter-btn :global(.trigger.button > span), .toolbar.topbar .filter-btn :global(.trigger.button > svg:last-child) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
		.toolbar.topbar .t-tools :global(.trigger.button), .toolbar.topbar .filter-btn :global(.trigger.button) { width: 44px; padding-inline: 0; justify-content: center; }
	}
	.toolbar .count { margin: 0 8px 0 0; min-width: 0; font: var(--t-headline); font-weight: 650; color: var(--text); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.toolbar .count:empty { display: none; }
	.toolbar .count.shelf { display: inline-flex; min-width: 0; }
	.count.shelf :global(button[aria-haspopup]) { min-width: 0; max-width: 100%; padding-inline: 0 8px; background: none; box-shadow: none; font: var(--t-headline); font-weight: 650; color: var(--text); }
	.count.shelf :global(button[aria-haspopup] span) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.find-box { position: relative; flex: 1 1 220px; max-width: 360px; min-width: 0; }
	.find-ic { position: absolute; left: 11px; top: 50%; translate: 0 -50%; color: var(--text-muted); display: grid; pointer-events: none; }
	.find { width: 100%; min-height: 40px; padding-left: 34px; border-radius: var(--r-btn); background: var(--surface-2); border-color: transparent; }
	/* Touch phones get the full 44px tap height; desktop keeps the slimmer field. */
	@media (pointer: coarse) and (max-width: 767px) { .find { min-height: 44px; } }
	.tool-gap { flex: 1 1 auto; }
	.filter-btn.on :global(button[aria-haspopup]) { box-shadow: inset 0 0 0 1.5px var(--accent); color: var(--accent-text); }
	.toolbar.topbar .t-view { order: 2; }
	.toolbar.topbar .filter-btn { order: 3; }
	.toolbar.topbar .tool-gap { order: 4; }
	.toolbar.topbar .t-tools { order: 5; }
	@media (max-width: 639px) {
		.toolbar { position: static; margin: 0; padding: 8px 0; }
		.t-sel, .t-add, .t-view, .filter-btn { display: inline-flex; }
			/* Two rows: count, search, select, add; then view and size, sort and filter. */
		.toolbar .count { order: 0; flex: 1 1 auto; margin: 0; overflow: hidden; text-overflow: ellipsis; }
		.t-find { order: 1; }
		.t-add { order: 3; }
		.tool-gap { display: block; order: 4; flex-basis: 100%; height: 0; }
		.t-view { order: 5; }
		.filter-btn { order: 6; }
		.t-sel { order: 2; }
		.find-box { flex-basis: 100%; max-width: none; order: 4; }
	}
</style>
