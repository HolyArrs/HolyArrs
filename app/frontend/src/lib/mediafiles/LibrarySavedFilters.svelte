<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import type { FilterState, SavedFilter } from './table.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	type Props = {
		sheet?: boolean;
		saved: SavedFilter[];
		filter: FilterState;
		filtersOn: number;
		setFilter: (patch: Partial<FilterState>) => void;
		forget: (name: string) => void;
		clearFilters: () => void;
		openSave: () => void;
	};
	let { sheet = false, saved, filter, filtersOn, setFilter, forget, clearFilters, openSave }: Props = $props();

	const sameFilter = (a: FilterState, b: FilterState) => a.status === b.status && a.kept === b.kept && a.asker === b.asker && a.series === b.series && (a.codec ?? '') === (b.codec ?? '') && !!a.subs === !!b.subs;
</script>

{#if sheet}
	<div class="sf-foot sf-foot-sheet">{@render body()}</div>
{:else}
	<footer class="sf-foot">{@render body()}</footer>
{/if}

{#snippet body()}
<div class="sf-saved">
	{#each saved as sv (sv.name)}<Chip label={sv.name} icon="star" selected={sameFilter(sv.filter, filter)} onclick={() => setFilter(sv.filter)} onremove={() => forget(sv.name)} />{/each}
	<Button variant="plain" size="sm" icon="plus" disabled={!filtersOn} onclick={openSave}>{t('mediafiles.dialogs.saveFilter')}</Button>
</div>
<Button variant="plain" size="sm" disabled={!filtersOn} onclick={clearFilters}>{t('mediafiles.saved.clear')}</Button>
{/snippet}

<style>
	/* The footer stays at the panel's edge while the lists scroll under it (the panel has 6px padding). */
	.sf-foot { grid-column: 1 / -1; position: sticky; bottom: -6px; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 8px; margin: 8px -6px -6px; padding: 8px 8px 6px; background: var(--surface-1); border-top: 1px solid var(--line); }
	.sf-foot-sheet { position: static; margin: 0; padding: 0; border: 0; background: none; width: 100%; }
	.sf-saved { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; min-width: 0; }
	@media (max-width: 639px) {
		.sf-foot { flex-wrap: wrap; }
	}
</style>
