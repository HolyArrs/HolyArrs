<script lang="ts">
	// Sort and filter for the YouTube page, in the Library's panel: a ticked sort list at the left, filter chips and lists at the right.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Chip from '#lib/ui/Chip.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { ytSorts, ytStartDir, ytStates, type YtFilter, type YtSort, type YtSortDir, type YtSortKey, type YtStateFilter } from './list.ts';
	import type { YtFollow } from './view.ts';

	type Props = {
		sort: YtSort;
		filter: YtFilter;
		channels: { id: string; name: string }[];
		playlists: YtFollow[];
		counts: Record<Exclude<YtStateFilter, 'any'>, number>;
		loaded: boolean;
		setSort: (key: YtSortKey, dir?: YtSortDir) => void;
		setFilter: (patch: Partial<YtFilter>) => void;
	};
	let { sort, filter, channels, playlists, counts, loaded, setSort, setFilter }: Props = $props();

	const sortWords = (k: YtSortKey) =>
		k === 'added' ? t('mediafiles.sort.added') : k === 'uploaded' ? t('youtube.sort.uploaded') : k === 'title' ? t('mediafiles.list.title') : k === 'channel' ? t('youtube.follows.channel') : k === 'duration' ? t('youtube.sort.duration') : t('mediafiles.col.size');
	const stateWords = (s: Exclude<YtStateFilter, 'any'>) =>
		s === 'done' ? t('youtube.state.done') : s === 'downloading' ? t('youtube.strip.title') : s === 'queued' ? t('youtube.state.queued') : t('youtube.state.failed');
</script>

{#snippet sfRow(label: string, on: boolean, pick: () => void, extra = '')}
	<button type="button" class="sf-o" class:on aria-pressed={on} onclick={pick}>
		<span class="sf-tick" aria-hidden="true">{#if on}<Icon name="check" size={14} stroke={2.4} />{/if}</span><span class="sf-l">{label}</span>{#if extra}<span class="sf-x" aria-hidden="true">{extra}</span>{/if}
	</button>
{/snippet}

<div class="sf" data-testid="youtube-sf">
	<section class="sf-col sf-sort" aria-label={t('mediafiles.sortFilter.sort')}>
		<h3 class="sf-h">{t('mediafiles.sortFilter.sortBy')}</h3>
		{#each ytSorts as k (k)}
			{@render sfRow(sortWords(k), sort.key === k, () => setSort(k, sort.key === k ? (sort.dir === 'asc' ? 'desc' : 'asc') : ytStartDir(k)), sort.key === k ? (sort.dir === 'asc' ? '↑' : '↓') : '')}
		{/each}
		<div class="sf-dir">
			<SegmentedControl
				label={t('mediafiles.sortFilter.order')}
				size="sm"
				value={sort.dir}
				options={[
					{ value: 'asc', label: t('mediafiles.sortFilter.asc') },
					{ value: 'desc', label: t('mediafiles.sortFilter.desc') }
				]}
				onchange={(d) => setSort(sort.key, d)}
			/>
		</div>
	</section>
	<section class="sf-col sf-filter" aria-label={t('mediafiles.sortFilter.filter')}>
		<h3 class="sf-h">{t('mediafiles.col.status')}</h3>
		<div class="sf-chips">
			{#each ytStates as s (s)}
				<span class="sf-chip t-{s}"><Chip label={stateWords(s)} count={loaded ? counts[s] : undefined} selected={filter.state === s} onclick={() => setFilter({ state: filter.state === s ? 'any' : s })} /></span>
			{/each}
		</div>
		<h3 class="sf-h">{t('youtube.watched')}</h3>
		<div class="sf-chips">
			<Chip label={t('youtube.filter.unwatched')} selected={filter.watched === 'unwatched'} onclick={() => setFilter({ watched: filter.watched === 'unwatched' ? 'any' : 'unwatched' })} />
			<Chip label={t('youtube.watched')} selected={filter.watched === 'watched'} onclick={() => setFilter({ watched: filter.watched === 'watched' ? 'any' : 'watched' })} />
		</div>
		{#if playlists.length}
			<h3 class="sf-h">{t('youtube.filter.playlists')}</h3>
			<div class="sf-well" data-testid="youtube-sf-playlists">
				{@render sfRow(t('youtube.filter.everything'), !filter.playlist, () => setFilter({ playlist: 0 }))}
				{#each playlists as p (p.id)}
					{@render sfRow(p.title, filter.playlist === p.id, () => setFilter({ playlist: filter.playlist === p.id ? 0 : p.id }))}
				{/each}
			</div>
		{/if}
		{#if channels.length}
			<h3 class="sf-h">{t('youtube.filter.channels')}</h3>
			<div class="sf-well" data-testid="youtube-sf-channels">
				{@render sfRow(t('youtube.filter.everything'), !filter.channel, () => setFilter({ channel: '' }))}
				{#each channels as c (c.id)}
					{@render sfRow(c.name, filter.channel === c.id, () => setFilter({ channel: filter.channel === c.id ? '' : c.id }))}
				{/each}
			</div>
		{/if}
	</section>
</div>

<style>
	/* The Library's Sort and filter panel: sort is a ticked list, filters are the chips the page shows once they are on. */
	.sf { display: grid; grid-template-columns: minmax(190px, max-content) minmax(0, 1fr); padding: 6px; }
	.sf-col { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.sf-sort { padding-right: 12px; }
	.sf-filter { padding-left: 16px; border-left: 1px solid var(--line); }
	.sf-h { margin: 10px 8px 6px; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.sf-col > .sf-h:first-child { margin-top: 6px; }
	.sf-filter .sf-h { margin-inline: 2px; }
	.sf-o { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 32px; padding: 0 8px; border-radius: 8px; color: var(--text); font: 500 13.5px/1.2 var(--font-ui); text-align: left; transition: background-color 120ms ease; }
	.sf-o + .sf-o { margin-top: 2px; }
	.sf-o:hover { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.sf-o:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sf-o.on { font-weight: 650; }
	.sf-tick { flex: none; display: grid; place-items: center; width: 14px; color: var(--accent-text, var(--accent)); }
	.sf-l { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sf-x { flex: none; color: var(--accent-text, var(--accent)); font-weight: 700; }
	.sf-dir { margin: 10px 8px 4px; }
	.sf-dir :global(.ui-seg) { width: 100%; }
	.sf-chips { display: flex; flex-wrap: wrap; gap: 6px; }
	/* A state chip carries its colour as a dot, the same colours the list uses. */
	.sf-chip { display: inline-flex; }
	.sf-chip :global(.main)::before { content: ''; flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--dot, var(--text-faint)); }
	.sf-chip.t-done { --dot: var(--ok, #2fa36b); }
	.sf-chip.t-downloading { --dot: var(--info, #409cff); }
	.sf-chip.t-queued { --dot: var(--wait, var(--text-muted)); }
	.sf-chip.t-failed { --dot: var(--fail, #d64545); }
	/* A long list of playlists or channels scrolls inside its own well. */
	.sf-well { max-height: 168px; overflow-y: auto; overscroll-behavior: contain; margin-inline: -6px; scrollbar-width: thin; scrollbar-color: var(--line-strong, var(--text-faint)) transparent; }
	@media (pointer: coarse) { .sf-o { min-height: 44px; } }
	@media (max-width: 639px) {
		.sf { grid-template-columns: 1fr; padding: 0; }
		.sf-sort { padding: 0; }
		.sf-filter { margin-top: 10px; padding: 4px 0 0; border-left: 0; border-top: 1px solid var(--line); }
	}
</style>
