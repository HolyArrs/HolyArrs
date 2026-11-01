<script lang="ts">
	// Look B (?look=table): a dense table like Sonarr's History. One row per
	// download story or Event; click a header to sort, tick rows for bulk
	// steps, and the status tabs work as preset filters. Under 768px the rows
	// become stacked cards, so the page never scrolls sideways.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount, untrack } from 'svelte';
	import Icon from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import DirBar from './DirBar.svelte';
	import LookFrame from './LookFrame.svelte';
	import RowActions from './RowActions.svelte';
	import StateDot from './StateDot.svelte';
	import EventSheet from '../history-designs/EventSheet.svelte';
	import { EventFeed, TabCounts } from './feed.svelte.ts';
	import { ALL, lookTabs, tabKinds, whenShort } from '../history-looks.ts';
	import { groupDownloads, type HistoryRow } from '../history.ts';
	import { byWhom, headVerb } from '../history-views.ts';
	import { dateTime } from '../format.ts';
	import { toasts } from '../toasts.svelte.ts';
	import { dirQuery, partOf, qualityOf, readDir, sizeOf, sizeText, sourceOf, whoMatches } from './dirs.ts';
	import type { TitleView } from '../types.ts';

	let { titles, titleById, people, me }: { titles: TitleView[]; titleById: Map<number, TitleView>; people: Map<number, string>; me?: number } = $props();

	const st = $derived(readDir(page.url.searchParams));
	const kinds = $derived(tabKinds(st.show, st.sub));
	const tab = $derived(lookTabs.find((t) => t.key === st.show) ?? lookTabs[0]);
	const empty = $derived({ title: st.filter.title || st.filter.from || st.filter.to || st.who ? 'Nothing matches' : tab.empty, text: tab.hint });

	const SIZE = 200;
	const feed = new EventFeed(SIZE);
	const reload = () => feed.load(st.filter, kinds);
	$effect(() => {
		void [st.filter, kinds];
		untrack(reload);
	});
	onMount(() => feed.watch(reload));
	const counts = new TabCounts();
	$effect(() => {
		void counts.load(st.filter, st.show);
	});

	type Col = 'title' | 'part' | 'quality' | 'source' | 'size' | 'date' | 'who';
	const cols: { key: Col; label: string }[] = [
		{ key: 'title', label: 'Title' },
		{ key: 'part', label: 'Episode or album' },
		{ key: 'quality', label: 'Quality' },
		{ key: 'source', label: 'Source' },
		{ key: 'size', label: 'Size' },
		{ key: 'date', label: 'Date' },
		{ key: 'who', label: 'Who' }
	];
	let sortBy = $state<Col>('date');
	let desc = $state(true);
	function sortOn(c: Col) {
		if (sortBy === c) desc = !desc;
		else {
			sortBy = c;
			desc = c === 'date' || c === 'size';
		}
	}
	const cell = (r: HistoryRow, c: Col): string | number => {
		switch (c) {
			case 'title': return r.head.title_name ?? 'HolyArrs';
			case 'part': return partOf(r);
			case 'quality': return qualityOf(r);
			case 'source': return sourceOf(r);
			case 'size': return sizeOf(r);
			case 'date': return r.head.At;
			case 'who': return byWhom(r.head);
		}
	};
	const rows = $derived.by(() => {
		const list = groupDownloads(st.who ? feed.events.filter((e) => whoMatches(e, st.who, me)) : feed.events);
		const dir = desc ? -1 : 1;
		return [...list].sort((a, b) => {
			const x = cell(a, sortBy);
			const y = cell(b, sortBy);
			if (typeof x === 'number' && typeof y === 'number') return (x - y) * dir;
			// Empty cells go last whichever way it is sorted.
			if (!x !== !y) return x ? -1 : 1;
			return String(x).localeCompare(String(y)) * dir;
		});
	});

	let picked = $state<Set<string>>(new Set());
	const pickedRows = $derived(rows.filter((r) => picked.has(r.key)));
	const allOn = $derived(rows.length > 0 && pickedRows.length === rows.length);
	function tick(key: string, on: boolean) {
		const next = new Set(picked);
		if (on) next.add(key);
		else next.delete(key);
		picked = next;
	}
	const tickAll = (on: boolean) => (picked = on ? new Set(rows.map((r) => r.key)) : new Set());
	const soon = (what: string) => toasts.push(`${what} for ${pickedRows.length} ${pickedRows.length === 1 ? 'row' : 'rows'}: coming soon.`, 'info');

	let sheet = $state<HistoryRow | null>(null);
	const go = (change: Record<string, string | undefined>) => void goto(page.url.pathname + dirQuery(page.url.searchParams, change), { replace: true, reset: false });
</script>

{#snippet kindMark(r: HistoryRow)}
	{@const v = headVerb(r)}
	<span class="km {v.tone}" title={v.word}><Icon name={v.icon} size={15} /><span class="sr-only">{v.word}</span></span>
{/snippet}

<DirBar counts={counts.tabs} {titles} />

{#if pickedRows.length}
	<div class="bulk" role="region" aria-label="Picked rows" data-testid="history-bulk">
		<span class="bn">{pickedRows.length} picked</span>
		<Button variant="secondary" size="sm" icon="declined" onclick={() => soon('Bad release, find another')}>Bad release, find another</Button>
		<Button variant="secondary" size="sm" icon="refresh" onclick={() => soon('Retry')}>Retry</Button>
		<Button variant="plain" size="sm" onclick={() => tickAll(false)}>Clear</Button>
	</div>
{/if}

<LookFrame {feed} {empty} onall={st.show !== ALL ? () => go({ show: undefined }) : undefined} onretry={reload} onmore={() => feed.load(st.filter, kinds, feed.offset + SIZE, true)}>
	<div class="wrap" data-testid="history-table">
		<table class="tbl">
			<thead>
				<tr>
					<th class="ck"><input type="checkbox" aria-label="Pick every row" checked={allOn} indeterminate={pickedRows.length > 0 && !allOn} onchange={(e) => tickAll(e.currentTarget.checked)} /></th>
					<th class="kc"><span class="sr-only">Kind</span></th>
					{#each cols as c (c.key)}
						<th class="c-{c.key}" aria-sort={sortBy === c.key ? (desc ? 'descending' : 'ascending') : undefined}>
							<button type="button" class="sort" onclick={() => sortOn(c.key)}>{c.label}<span class="ar" class:on={sortBy === c.key} aria-hidden="true"><Icon name={sortBy === c.key && !desc ? 'chevronUp' : 'chevronDown'} size={13} /></span></button>
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as r (r.key)}
					<tr class:on={picked.has(r.key)} data-testid="history-table-row">
						<td class="ck"><input type="checkbox" aria-label="Pick {r.head.title_name ?? 'this row'}" checked={picked.has(r.key)} onchange={(e) => tick(r.key, e.currentTarget.checked)} /></td>
						<td class="kc">{@render kindMark(r)}</td>
						<td class="c-title"><button type="button" class="open" onclick={() => (sheet = r)}>{cell(r, 'title')}</button></td>
						<td class="c-part">{partOf(r) || '—'}</td>
						<td class="c-quality">{qualityOf(r) || '—'}</td>
						<td class="c-source">{sourceOf(r) || '—'}</td>
						<td class="c-size num">{sizeText(r) || '—'}</td>
						<td class="c-date num"><time datetime={r.head.At} title={dateTime(r.head.At)}>{whenShort(r.head.At)}</time></td>
						<td class="c-who">{byWhom(r.head)}</td>
					</tr>
				{/each}
			</tbody>
		</table>

		<ul class="cards" aria-label="History">
			{#each rows as r (r.key)}
				{@const v = headVerb(r)}
				{@const facts = [partOf(r), qualityOf(r), sourceOf(r), sizeText(r)].filter(Boolean)}
				<li class="card" class:on={picked.has(r.key)}>
					<label class="cck"><input type="checkbox" aria-label="Pick {r.head.title_name ?? 'this row'}" checked={picked.has(r.key)} onchange={(e) => tick(r.key, e.currentTarget.checked)} /></label>
					<button type="button" class="cbody" onclick={() => (sheet = r)}>
						<span class="ct">{cell(r, 'title')}</span>
						<StateDot verb={v} />
						{#if facts.length}<span class="cf">{facts.join(' · ')}</span>{/if}
						<span class="cf"><time datetime={r.head.At}>{whenShort(r.head.At)}</time> · {byWhom(r.head)}</span>
					</button>
				</li>
			{/each}
		</ul>
	</div>
</LookFrame>

<EventSheet bind:row={sheet} {titleById} {people} {me}>
	{#snippet actions(row)}<RowActions {row} />{/snippet}
</EventSheet>

<style>
	.bulk { position: sticky; top: calc(var(--topbar) + 8px); z-index: 5; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 0 0 12px; padding: 6px 8px 6px 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); }
	.bn { margin-right: auto; font: var(--t-callout); font-weight: 600; }
	.wrap { min-width: 0; }
	.tbl { width: 100%; border-collapse: separate; border-spacing: 0; font: var(--t-footnote); }
	th { position: sticky; top: var(--topbar); z-index: 2; height: 40px; padding: 0 8px; border-bottom: 1px solid var(--line-strong); background: var(--bg); color: var(--text-muted); font-weight: 600; text-align: left; white-space: nowrap; }
	td { height: 44px; padding: 0 8px; border-bottom: 1px solid var(--line); color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 240px; }
	tr.on td { background: color-mix(in srgb, var(--info) 10%, transparent); }
	tbody tr:hover td { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.ck { width: 36px; padding-right: 0; }
	.kc { width: 32px; padding: 0 4px; }
	.num { font-variant-numeric: tabular-nums; }
	.c-date, .c-size { color: var(--text-muted); }
	.c-part, .c-quality, .c-source, .c-who { color: var(--text-muted); }
	input[type='checkbox'] { width: 16px; height: 16px; margin: 0; accent-color: var(--accent); cursor: pointer; }
	.sort { display: inline-flex; align-items: center; gap: 4px; height: 32px; padding: 0 4px; margin-left: -4px; border: 0; border-radius: 8px; background: none; color: inherit; font: inherit; cursor: pointer; }
	.sort:hover { color: var(--text); }
	.ar { display: inline-grid; opacity: 0; }
	.ar.on, .sort:hover .ar { opacity: 1; }
	.sort:focus-visible, .open:focus-visible, .cbody:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.open { max-width: 100%; padding: 0; border: 0; background: none; color: var(--text); font: inherit; font-weight: 600; text-align: left; overflow: hidden; text-overflow: ellipsis; cursor: pointer; }
	.open:hover { text-decoration: underline; }
	.km { display: grid; place-items: center; width: 24px; height: 24px; border-radius: 7px; color: var(--text-muted); background: color-mix(in srgb, var(--text) 6%, transparent); }
	.km.ok { color: var(--ok-text); background: color-mix(in srgb, var(--ok) 14%, transparent); }
	.km.fail { color: var(--fail-text); background: color-mix(in srgb, var(--fail) 14%, transparent); }
	.km.wait { color: var(--wait-text); background: color-mix(in srgb, var(--wait) 14%, transparent); }
	.km.info { color: var(--info-text); background: color-mix(in srgb, var(--info) 14%, transparent); }
	.cards { display: none; margin: 0; padding: 0; list-style: none; }
	@media (max-width: 1100px) { .c-who, .c-source { display: none; } }
	/* Phones: stacked cards, nothing wider than the screen. */
	@media (max-width: 767px) {
		.tbl { display: none; }
		.cards { display: flex; flex-direction: column; gap: 8px; }
		.card { position: relative; display: flex; align-items: flex-start; gap: 10px; padding: 12px; border: 1px solid var(--line); border-radius: 12px; background: var(--surface-1); }
		.card.on { border-color: color-mix(in srgb, var(--info) 50%, var(--line)); }
		.cck { flex: none; display: grid; place-items: center; width: 44px; height: 44px; margin: -12px -6px -12px -12px; cursor: pointer; }
		.cck input { width: 20px; height: 20px; }
		.cbody { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; flex: 1; min-width: 0; min-height: 44px; padding: 0; border: 0; background: none; color: var(--text); font: var(--t-callout); text-align: left; cursor: pointer; }
		.ct { max-width: 100%; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
		.cf { max-width: 100%; color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	}
</style>
