<script lang="ts">
	// Look 1, Table: every Event in one dense, sortable table, the same build
	// as the Downloads table (tight rows, small posters, a state dot). A
	// download's steps fold into one row that says how far it got. A row opens
	// its details in the side sheet; phones get the same rows as a tight list.
	import { onMount, untrack } from 'svelte';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import EventSheet from '../history-designs/EventSheet.svelte';
	import StateDot from './StateDot.svelte';
	import LookFrame from './LookFrame.svelte';
	import { EventFeed } from './feed.svelte.ts';
	import { groupDownloads, type HistoryFilter, type HistoryRow } from '../history.ts';
	import { artOf, byWhom, headVerb, rowTier, sortRows, type SortKey } from '../history-views.ts';
	import { rowWhy, whenShort } from '../history-looks.ts';
	import type { TitleView } from '../types.ts';

	let { filter, kinds, titleById, empty, onall }: { filter: HistoryFilter; kinds: string[] | undefined; titleById: Map<number, TitleView>; empty: { title: string; text: string }; onall?: () => void } = $props();

	const SIZE = 100;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(filter, kinds);
	$effect(() => {
		void [filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));

	let sort = $state<{ key: SortKey; dir: 1 | -1 }>({ key: 'at', dir: -1 });
	const rows = $derived(sortRows(groupDownloads(feed.events), sort.key, sort.dir));
	let sheet = $state<HistoryRow | null>(null);

	const COLS: { key: SortKey | 'why'; label: string; cls: string }[] = [
		{ key: 'title', label: 'Title', cls: 'c-title' },
		{ key: 'what', label: 'What happened', cls: 'c-what' },
		{ key: 'why', label: 'Detail', cls: 'c-why' },
		{ key: 'by', label: 'By', cls: 'c-by' },
		{ key: 'at', label: 'When', cls: 'c-at' }
	];
	function sortBy(k: SortKey) {
		sort = sort.key === k ? { key: k, dir: sort.dir === 1 ? -1 : 1 } : { key: k, dir: k === 'at' ? -1 : 1 };
	}
	const ariaSort = (k: string) => (sort.key === k ? (sort.dir === 1 ? 'ascending' : 'descending') : undefined);
	function rowKey(e: KeyboardEvent, r: HistoryRow) {
		if (e.target !== e.currentTarget) return;
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			sheet = r;
		}
	}
	const sub = (r: HistoryRow) => [r.head.episode, r.head.version].filter(Boolean).join(' · ');
</script>

<LookFrame {feed} {empty} {onall} onretry={reload} onmore={() => feed.load(filter, kinds, feed.offset + SIZE, true)}>
	<div class="scroller">
		<table class="t" data-testid="look-table">
			<thead>
				<tr>
					{#each COLS as c (c.key)}
						<th class={c.cls} scope="col" aria-sort={ariaSort(c.key)}>
							{#if c.key === 'why'}<span class="plain">{c.label}</span>
							{:else}
								{@const k = c.key}
								<button type="button" class="sort" class:on={sort.key === k} onclick={() => sortBy(k)}>{c.label}<span class="arrow" class:desc={sort.key === k && sort.dir === -1} aria-hidden="true"><Icon name="chevronDown" size={14} stroke={2.2} /></span></button>
							{/if}
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as r (r.key)}
					{@const e = r.head}
					{@const v = headVerb(r)}
					{@const t = titleById.get(e.TitleID ?? 0)}
					<tr class={v.tone} class:quiet={rowTier(r) === 'routine'} data-testid="look-row" tabindex="0" aria-label="{e.title_name ?? 'HolyArrs'}, {v.word}. Open details" onclick={() => (sheet = r)} onkeydown={(ev) => rowKey(ev, r)}>
						<td class="c-title">
							<span class="tc">
								<span class="post" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={14} /></span>{/if}</span>
								<span class="tn"><span class="nm">{e.title_name ?? 'HolyArrs'}</span>{#if sub(r)}<span class="rel">{sub(r)}</span>{/if}</span>
							</span>
						</td>
						<td class="c-what"><StateDot verb={v} detail={r.events.length > 1 ? `${r.events.length} steps` : ''} /></td>
						<td class="c-why"><span class="why" title={rowWhy(r) || e.Summary}>{rowWhy(r) || e.Summary}</span></td>
						<td class="c-by">{byWhom(e)}</td>
						<td class="c-at"><time datetime={e.At}>{whenShort(e.At)}</time></td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<ul class="plist" data-testid="look-list">
		{#each rows as r (r.key)}
			{@const e = r.head}
			{@const v = headVerb(r)}
			{@const t = titleById.get(e.TitleID ?? 0)}
			<li class="prow {v.tone}" class:quiet={rowTier(r) === 'routine'}>
				<button type="button" class="phit" data-testid="look-row" aria-label="Details for {e.title_name ?? 'HolyArrs'}, {v.word}" onclick={() => (sheet = r)}>
					<span class="post" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="sys"><Icon name="settings" size={14} /></span>{/if}</span>
					<span class="tn">
						<span class="nm">{e.title_name ?? 'HolyArrs'}{#if e.episode}<span class="ep">{` ${e.episode}`}</span>{/if}</span>
						<span class="pl"><StateDot verb={v} /></span>
						<span class="rel">{rowWhy(r) || e.Summary}</span>
					</span>
					<time class="pr" datetime={e.At}>{whenShort(e.At).replace(/^(Today|Yesterday), /, '')}</time>
				</button>
			</li>
		{/each}
	</ul>
</LookFrame>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.scroller { overflow-x: auto; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.t { width: 100%; border-collapse: collapse; table-layout: fixed; font: var(--t-callout); font-variant-numeric: tabular-nums; }
	thead { position: sticky; top: 0; background: var(--surface-1); }
	th { height: 38px; padding: 0 12px; text-align: left; border-bottom: 1px solid var(--line); }
	.plain, .sort { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.02em; white-space: nowrap; }
	.sort { display: inline-flex; align-items: center; gap: 4px; min-height: 30px; padding: 0 4px; margin: 0 -4px; border: 0; border-radius: 6px; background: none; cursor: pointer; }
	.sort:hover, .sort.on { color: var(--text); }
	.sort:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.arrow { display: inline-grid; opacity: 0; transform: rotate(180deg); transition: transform var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.sort.on .arrow { opacity: 1; }
	.sort:hover .arrow { opacity: 0.5; }
	.sort.on:hover .arrow { opacity: 1; }
	.arrow.desc { transform: none; }
	td { height: 52px; padding: 0 12px; vertical-align: middle; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	tbody tr { cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
	tbody tr + tr { border-top: 1px solid var(--line); }
	tbody tr:hover { background: var(--surface-2); }
	tbody tr:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	tbody tr.fail { box-shadow: inset 3px 0 0 var(--fail); }
	tbody tr.fail:focus-visible { box-shadow: inset 3px 0 0 var(--fail), inset 0 0 0 2px var(--accent); }
	tr.quiet .nm { font-weight: 500; color: var(--text-muted); }
	.c-title { width: 30%; }
	.c-what { width: 190px; }
	.c-why { width: auto; }
	.c-by { width: 140px; color: var(--text-muted); }
	.c-at { width: 150px; color: var(--text-muted); text-align: right; }
	th.c-at { text-align: right; }
	.tc { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.post { flex: none; width: 26px; --poster-ratio: 2 / 3; }
	.post.square { --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 4px; }
	.sys { display: grid; place-items: center; width: 26px; aspect-ratio: 2 / 3; border-radius: 4px; background: var(--surface-2); color: var(--text-muted); }
	.tn { display: flex; flex-direction: column; min-width: 0; }
	.nm { color: var(--text); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ep { color: var(--text-muted); font-weight: 400; }
	.rel { color: var(--text-faint); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.why { display: block; overflow: hidden; text-overflow: ellipsis; color: var(--text-muted); }

	.plist { display: none; list-style: none; margin: 0; padding: 4px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); }
	.prow { border-radius: var(--r-btn); }
	.prow + .prow { box-shadow: 0 -1px 0 var(--line); }
	.prow.fail { box-shadow: inset 3px 0 0 var(--fail); }
	.prow + .prow.fail { box-shadow: inset 3px 0 0 var(--fail), 0 -1px 0 var(--line); }
	.phit { width: 100%; display: grid; grid-template-columns: 34px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 64px; min-width: 0; padding: 8px 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; cursor: pointer; font: inherit; }
	.phit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.phit .post, .phit .sys { width: 34px; }
	.pl { display: flex; min-width: 0; font: var(--t-footnote); }
	.pr { align-self: start; padding-top: 2px; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }

	@media (max-width: 1199px) { .c-by { display: none; } }
	@media (max-width: 960px) { .c-at { width: 120px; } .c-what { width: 160px; } }
	@media (max-width: 767px) { .scroller { display: none; } .plist { display: block; } }
	@media (prefers-reduced-motion: reduce) { .arrow, tbody tr { transition: none; } }
</style>
