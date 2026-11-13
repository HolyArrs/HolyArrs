<!-- Toolbar of the scanned folders: search, the status filter and the Library's own view switch. -->
<script lang="ts">
	import { tick, type Snippet } from 'svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import LibraryViewSwitch from './LibraryViewSwitch.svelte';
	import type { ImportFilter, ImportView } from './import-view.ts';
	import { appLocale, t } from '#lib/core/i18n/i18n.svelte.ts';

	let {
		phone,
		compact,
		view,
		setView,
		sizeItems,
		sizeLabel,
		filters,
		filter = $bindable('all'),
		findLabel,
		findOpen = $bindable(false),
		hasWords,
		find
	}: {
		phone: boolean;
		compact: boolean;
		view: ImportView;
		setView: (v: ImportView) => void;
		sizeItems: MenuItem[] | null;
		sizeLabel: string;
		filters: { value: ImportFilter; label: string; n: number }[];
		filter?: ImportFilter;
		findLabel: string;
		findOpen?: boolean;
		hasWords: boolean;
		/** The search field; the page owns it. */
		find: Snippet;
	} = $props();

	const options = $derived<{ value: ImportView; label: string; icon: 'menu' | 'image' | 'grid' | 'info' }[]>([
		{ value: 'rows', label: t('mediafiles.toolbar.rows'), icon: 'menu' },
		{ value: 'banners', label: t('mediafiles.toolbar.banners'), icon: 'image' },
		{ value: 'posters', label: t('mediafiles.toolbar.posters'), icon: 'grid' },
		{ value: 'overview', label: t('mediafiles.toolbar.overview'), icon: 'info' }
	]);

	async function toggleFind() {
		findOpen = !findOpen || hasWords;
		if (!findOpen) return;
		await tick();
		document.querySelector<HTMLInputElement>('[data-testid="import-find-input"]')?.focus();
	}
</script>

{#snippet chips()}
	<div class="chips" class:many={filters.length > 3} role="group" aria-label={t('mediafiles.sortFilter.filter')} data-testid="import-filter">
		{#each filters as c (c.value)}
			<button type="button" aria-pressed={filter === c.value} onclick={() => (filter = c.value)}>{c.label} <span class="n">{c.n.toLocaleString(appLocale())}</span></button>
		{/each}
	</div>
{/snippet}

<div class="itb" class:phone data-testid="import-toolbar">
	{#if phone}
		<div class="line">
			<span class="pv"><LibraryViewSwitch value={view} {options} onchange={setView} compact /></span>
			<span class="gap"></span>
			<IconButton icon="search" label={findLabel} aria-pressed={findOpen || hasWords} aria-expanded={findOpen || hasWords} onclick={toggleFind} data-testid="import-find-toggle" />
		</div>
		{#if findOpen || hasWords}<div class="line">{@render find()}</div>{/if}
		<div class="line">{@render chips()}</div>
	{:else}
		<div class="line wide">
			{@render find()}
			<span class="cap">{@render chips()}</span>
			<span class="cap pv"><LibraryViewSwitch value={view} {options} onchange={setView} {sizeItems} {sizeLabel} {compact} /></span>
		</div>
	{/if}
</div>

<style>
	.itb { container-type: inline-size; display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.line { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.wide { flex-wrap: wrap; }
	.wide > :global(.dlfind) { flex: 1 1 220px; min-width: 180px; }
	.cap { display: flex; flex: none; align-items: center; gap: 2px; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.cap.pv { margin-inline-start: auto; --vs-bg: transparent; }
	.cap.pv :global(.vs) { padding: 0; gap: 2px; }
	.cap.pv :global(.vs button[role='radio']) { min-height: 44px; min-width: 44px; }
	.cap.pv :global(.vs .seg) { border-radius: 10px; }
	.chips { display: flex; gap: 2px; min-width: 0; }
	.chips button { display: inline-flex; align-items: center; gap: 6px; height: 44px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: var(--text); font: var(--t-subhead); font-size: 14px; font-weight: 600; white-space: nowrap; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.chips button:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.chips button[aria-pressed='true'] { background: color-mix(in srgb, var(--text) 8%, transparent); }
	.chips button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.n { color: var(--text-muted); font-weight: 500; font-variant-numeric: tabular-nums; }
	/* Narrow cards put the search on its own line, the filter and the views under it. */
	@container (max-width: 760px) {
		.wide > :global(.dlfind) { flex-basis: 100%; }
		.cap.pv { margin-inline-start: auto; }
	}

	/* Phone: the view switch and a search button, then the filter as one segmented control. */
	.phone .gap { flex: 1; }
	.phone .pv :global(.vs) { --vs-bg: var(--surface-2); }
	.phone .line > :global(.dlfind) { flex: 1 1 auto; min-width: 0; }
	.phone .chips { flex: 1; padding: 2px; border-radius: 10px; background: var(--surface-2); overflow-x: auto; scrollbar-width: none; }
	.phone .chips button { flex: 1 0 auto; justify-content: center; height: 40px; padding: 0 8px; border-radius: 8px; }
	.phone .chips button[aria-pressed='true'] { background: var(--seg-thumb); box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 0.5px rgb(0 0 0 / 4%); }
	@media (prefers-reduced-motion: reduce) { .chips button { transition: none; } }
</style>
