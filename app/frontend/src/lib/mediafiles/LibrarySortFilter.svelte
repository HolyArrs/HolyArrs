<script lang="ts">
	import type { Snippet } from 'svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { to } from '../core/paths.ts';
	import { codecLabel } from './copies.ts';
	import type { MovieCollection } from './collections.ts';
	import { filters, type Kind } from './library.ts';
	import { filterCounts, sortLabel, sortsFor, startDir, type FilterState, type SortDir, type SortKey } from './table.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	type Props = {
		kind: Kind;
		sort: { key: SortKey; dir: SortDir };
		filter: FilterState;
		counts: ReturnType<typeof filterCounts>;
		askers: { id: number; name: string }[];
		series: MovieCollection[];
		subsTitles: Set<number> | null;
		loaded: boolean;
		myId: number | undefined;
		canManage: boolean;
		anyPart: boolean;
		deletedCount: number | null;
		groupSets: boolean;
		phone: boolean;
		view: string;
		sizeItems: MenuItem[];
		foot: Snippet;
		setSort: (key: SortKey, dir?: SortDir) => void;
		setFilter: (patch: Partial<FilterState>) => void;
		syncUrl: () => void;
		onclose: () => void;
	};
	let { kind, sort, filter, counts, askers, series, subsTitles, loaded, myId, canManage, anyPart, deletedCount, groupSets = $bindable(), phone, view, sizeItems, foot, setSort, setFilter, syncUrl, onclose }: Props = $props();

	// Counts stay blank until the Library has loaded, so a 0 only ever means 0.
	const countHint = (n: number) => (loaded ? String(n) : undefined);
</script>

{#snippet sfRow(label: string, on: boolean, pick: () => void, hint = '', extra = '')}
<button type="button" class="sf-o" class:on aria-pressed={on} onclick={pick}>
	<span class="sf-tick" aria-hidden="true">{#if on}<Icon name="check" size={14} stroke={2.4} />{/if}</span><span class="sf-l">{label}</span>{#if extra}<span class="sf-x" aria-hidden="true">{extra}</span>{/if}{#if hint}<span class="sf-n">{hint}</span>{/if}
</button>
{/snippet}

<div class="sf">
	<section class="sf-col sf-sort" aria-label={t('mediafiles.sortFilter.sort')}>
		<h3 class="sf-h">{t('mediafiles.sortFilter.sortBy')}</h3>
		{#each sortsFor(kind === 'all' ? 'all' : kind) as k (k)}
			{@render sfRow(sortLabel(k, kind === 'all' ? 'all' : kind), sort.key === k, () => setSort(k, sort.key === k ? (sort.dir === 'asc' ? 'desc' : 'asc') : startDir(k)), '', sort.key === k ? (sort.dir === 'asc' ? '↑' : '↓') : '')}
		{/each}
		<div class="sf-dir"><SegmentedControl label={t('mediafiles.sortFilter.order')} size="sm" value={sort.dir} options={[{ value: 'asc', label: t('mediafiles.sortFilter.asc') }, { value: 'desc', label: t('mediafiles.sortFilter.desc') }]} onchange={(d) => setSort(sort.key, d)} /></div>
		{#if kind === 'movie'}
			<span class="sf-sep" aria-hidden="true"></span>
			{@render sfRow(t('mediafiles.sortFilter.group'), groupSets, () => { groupSets = !groupSets; syncUrl(); })}
		{/if}
	</section>
	<section class="sf-col sf-filter" aria-label={t('mediafiles.sortFilter.filter')}>
		<h3 class="sf-h">{t('mediafiles.col.status')}</h3>
		<div class="sf-chips">
			{#each filters as [f, label] (f)}<span class="sf-chip t-{f}"><Chip {label} count={loaded ? counts.status[f] : undefined} selected={filter.status === f} onclick={() => setFilter({ status: f })} /></span>{/each}
		</div>
		<h3 class="sf-h">{t('mediafiles.select.keep')}</h3>
		<div class="sf-chips">
			<Chip label={t('mediafiles.sortFilter.kept')} icon="bellOn" count={loaded ? counts.kept.on : undefined} selected={filter.kept === 'on'} onclick={() => setFilter({ kept: filter.kept === 'on' ? 'any' : 'on' })} />
			<Chip label={t('mediafiles.state.notKept')} icon="bell" count={loaded ? counts.kept.off : undefined} selected={filter.kept === 'off'} onclick={() => setFilter({ kept: filter.kept === 'off' ? 'any' : 'off' })} />
		</div>
		{#if askers.length || myId}
			<h3 class="sf-h">{t('mediafiles.sortFilter.askedBy')}</h3>
			<div class="sf-chips">
				<Chip label={t('mediafiles.sortFilter.anyone')} selected={filter.asker === 'anyone'} onclick={() => setFilter({ asker: 'anyone' })} />
				{#if myId}<Chip label={t('mediafiles.sortFilter.me')} count={loaded ? (counts.askers.get(myId) ?? 0) : undefined} selected={filter.asker === 'me'} onclick={() => setFilter({ asker: 'me' })} />{/if}
				{#each askers.filter((p) => p.id !== myId) as p (p.id)}<Chip label={p.name} count={loaded ? (counts.askers.get(p.id) ?? 0) : undefined} selected={filter.asker === String(p.id)} onclick={() => setFilter({ asker: String(p.id) })} />{/each}
			</div>
		{/if}
		{#if kind === 'movie' && counts.codecs.size}
			<h3 class="sf-h">{t('mediafiles.col.codec')}</h3>
			<div class="sf-chips">
				{#each [...counts.codecs].toSorted((a, b) => b[1] - a[1]) as [c, n] (c)}<Chip label={codecLabel(c)} count={n} selected={filter.codec === c} onclick={() => setFilter({ codec: filter.codec === c ? '' : c })} />{/each}
			</div>
		{/if}
		{#if (kind === 'movie' || kind === 'show') && subsTitles && (counts.subs || filter.subs)}
			<h3 class="sf-h">{t('mediafiles.sheet.subtitles')}</h3>
			<div class="sf-chips">
				<Chip label={t('mediafiles.filter.missing')} icon="subtitle" count={counts.subs} selected={!!filter.subs} onclick={() => setFilter({ subs: !filter.subs })} />
			</div>
		{/if}
		{#if (kind === 'movie' || kind === 'all') && series.length}
			<h3 class="sf-h">{t('mediafiles.col.series')}</h3>
			<div class="sf-well">
				{@render sfRow(t('mediafiles.sortFilter.everyMovie'), !filter.series, () => setFilter({ series: 0 }))}
				{#each series.toSorted((x, y) => x.name.localeCompare(y.name)) as sr (sr.id)}
					{@render sfRow(sr.name, filter.series === sr.id, () => setFilter({ series: sr.id }), countHint(counts.series.get(sr.id) ?? 0))}
				{/each}
			</div>
		{/if}
		{#if canManage && anyPart}
			<span class="sf-sep" aria-hidden="true"></span>
			<a class="sf-o sf-link" href={to('/library/deleted')} onclick={onclose} data-testid="library-deleted"><span class="sf-tick" aria-hidden="true"><Icon name="trash" size={14} /></span><span class="sf-l">{t('mediafiles.sortFilter.recentlyDeleted')}</span>{#if deletedCount}<span class="sf-n">{deletedCount}</span>{/if}</a>
		{/if}
	</section>
	{#if phone && view === 'posters'}
		<section class="sf-col sf-size" aria-label={t('mediafiles.toolbar.posters')}>
			{#each sizeItems as it, i (i)}
				{#if it.kind === 'heading'}<h3 class="sf-h">{it.label}</h3>
				{:else if it.kind !== 'separator'}{@render sfRow(it.label, !!it.checked, () => it.onselect?.())}{/if}
			{/each}
		</section>
	{/if}
	{#if !phone}{@render foot()}{/if}
</div>

<style>
	/* Sort and filter: one panel in the app's own material. Sort is a ticked list (as in every menu); filters are the same chips the page shows once they are on. */
	.sf { display: grid; grid-template-columns: 212px minmax(0, 1fr); padding: 6px; }
	.sf-col { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.sf-sort { padding-right: 12px; }
	.sf-filter { padding-left: 16px; border-left: 1px solid var(--line); }
	.sf-h { margin: 10px 8px 6px; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.sf-col > .sf-h:first-child { margin-top: 6px; }
	.sf-filter .sf-h { margin-inline: 2px; }
	.sf-sep { height: 1px; margin: 6px 8px; background: var(--line); }
	.sf-o { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 32px; padding: 0 8px; border-radius: 8px; color: var(--text); font: 500 13.5px/1.2 var(--font-ui); text-align: left; transition: background-color 120ms ease; }
	.sf-o + .sf-o { margin-top: 2px; }
	.sf-link { text-decoration: none; }
	.sf-link .sf-tick { color: var(--text-muted); }
	.sf-o:hover { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.sf-o:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sf-o.on { font-weight: 650; }
	.sf-tick { flex: none; display: grid; place-items: center; width: 14px; color: var(--accent-text, var(--accent)); }
	.sf-l { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sf-x { flex: none; color: var(--accent-text, var(--accent)); font-weight: 700; }
	.sf-n { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.sf-dir { margin: 10px 4px 4px; }
	.sf-dir :global(.ui-seg) { width: 100%; }
	.sf-chips { display: flex; flex-wrap: wrap; gap: 6px; }
	/* A status chip carries its status colour as a dot, the same colours the rows use. */
	.sf-chip { display: inline-flex; }
	.sf-chip:not(.t-all) :global(.main)::before { content: ''; flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--dot, var(--text-faint)); }
	.sf-chip.t-missing { --dot: var(--warn, #d9822b); }
	.sf-chip.t-not_out_yet { --dot: var(--text-muted); }
	.sf-chip.t-better { --dot: var(--info, #409cff); }
	.sf-chip.t-have { --dot: var(--ok, #2fa36b); }
	/* The series list is long: it scrolls inside its own well and takes the height the Sort column leaves. */
	.sf-well { flex: 1 1 0; min-height: 104px; overflow-y: auto; overscroll-behavior: contain; padding: 4px; border-radius: 10px; background: color-mix(in srgb, var(--text) 4%, transparent); box-shadow: inset 0 0 0 1px var(--line); scrollbar-width: thin; scrollbar-color: var(--line-strong, var(--text-faint)) transparent; }
	@media (pointer: coarse) { .sf-o { min-height: 44px; } }
	@media (max-width: 639px) {
		.sf { grid-template-columns: 1fr; padding: 0; }
		.sf-sort { padding: 0; }
		.sf-filter { margin-top: 10px; padding: 4px 0 0; border-left: 0; border-top: 1px solid var(--line); }
		.sf-well { flex: none; max-height: 232px; }
		.sf-size { margin-top: 10px; padding-top: 4px; border-top: 1px solid var(--line); }
	}
</style>
