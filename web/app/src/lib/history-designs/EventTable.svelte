<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import Poster from '#lib/Poster.svelte';
	import Icon from '#lib/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import HistoryToolbar from './HistoryToolbar.svelte';
	import EventSheet from './EventSheet.svelte';
	import VerbMark from './VerbMark.svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import { createLatestOnly, dateRangeProblem, details, groupDownloads, pageLine, type HistoryFilter, type HistoryRow } from '#lib/history.ts';
	import { artOf, byWhom, clock, dayKey, dayLabel, eventsQuery, headVerb, notRoutineKinds, pickedKinds, rowTier, settle, sortRows, titleHref, type SortKey } from '#lib/history-views.ts';
	import type { EventPage, EventRecord, TitleView } from '#lib/types.ts';

	// 5 Table: every Event in a dense, sortable table with a sticky header.
	// Arrow keys (or j and k) move through the rows, Enter opens the side
	// sheet, [ and ] turn the page. Routine rows are quiet and can be hidden.
	let { filter, titles, titleById, onfilter, phone }: { filter: HistoryFilter; titles: TitleView[]; titleById: Map<number, TitleView>; onfilter: (c: Partial<HistoryFilter>) => void; phone: boolean } = $props();

	const SIZES = [50, 100, 250];
	let size = $state(100);
	let offset = $state(0);
	let hideRoutine = $state(false);
	let events = $state<EventRecord[]>([]);
	let total = $state(0);
	let loaded = $state(false);
	let error = $state('');
	let sheet = $state<HistoryRow | null>(null);
	let sort = $state<{ key: SortKey; dir: 1 | -1 }>({ key: 'at', dir: -1 });
	let at = $state(0);
	let body = $state<HTMLElement>();
	const latest = createLatestOnly();
	const kinds = $derived(hideRoutine ? pickedKinds(filter.groups, notRoutineKinds) : pickedKinds(filter.groups));

	async function load() {
		const ok = latest();
		if (dateRangeProblem(filter)) { loaded = true; return; }
		if (kinds && !kinds.length) { events = []; total = 0; loaded = true; return; }
		try {
			const p = await api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit: size, offset }));
			if (!ok()) return;
			events = p.events;
			total = p.total;
			error = '';
		} catch (e) {
			if (ok()) error = message(e);
		} finally {
			if (ok()) loaded = true;
		}
	}
	$effect(() => {
		void [filter.title, filter.episode, filter.from, filter.to, filter.groups.join(), hideRoutine, size];
		untrack(() => { offset = 0; at = 0; void load(); });
	});
	const fresh = settle(() => offset === 0 && !sheet && load(), 2000);
	onMount(() => {
		const offs = [live.on('event', fresh.poke), live.on('resync', fresh.poke)];
		return () => { offs.forEach((o) => o()); fresh.stop(); };
	});
	function page(n: number) {
		if (n < 0 || n >= total) return;
		offset = n;
		at = 0;
		void load().then(() => focusRow(0));
	}

	const rows = $derived(sortRows(groupDownloads(events), sort.key, sort.dir));
	function sortBy(key: SortKey) {
		sort = sort.key === key ? { key, dir: sort.dir === 1 ? -1 : 1 } : { key, dir: key === 'at' ? -1 : 1 };
	}
	const ariaSort = (key: SortKey) => (sort.key === key ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none');

	async function focusRow(i: number) {
		at = Math.max(0, Math.min(rows.length - 1, i));
		await tick();
		body?.querySelector<HTMLElement>(`[data-i="${at}"]`)?.focus();
	}
	function key(e: KeyboardEvent) {
		if (e.target instanceof HTMLAnchorElement && e.key === 'Enter') return;
		const k = e.key;
		if (k === 'ArrowDown' || k === 'j') { e.preventDefault(); void focusRow(at + 1); }
		else if (k === 'ArrowUp' || k === 'k') { e.preventDefault(); void focusRow(at - 1); }
		else if (k === 'Home') { e.preventDefault(); void focusRow(0); }
		else if (k === 'End') { e.preventDefault(); void focusRow(rows.length - 1); }
		else if (k === 'Enter' || k === ' ') { e.preventDefault(); sheet = rows[at] ?? null; }
		else if (k === ']') { e.preventDefault(); page(offset + size); }
		else if (k === '[') { e.preventDefault(); page(offset - size); }
	}
	/** A short "why" for the Detail column: the first reason or path an Event carries. */
	function detail(r: HistoryRow): string {
		if (r.steps.length) return r.steps.map((s) => `${s.label}${s.state === 'done' ? ' ✓' : s.state === 'failed' ? ' ✕' : ''}`).join(' → ');
		return details(r.head)[0]?.[1] ?? '';
	}
	const filtered = $derived(!!(filter.title || filter.groups.length || filter.from || filter.to));
	const columns: { key: SortKey | 'detail'; label: string }[] = [
		{ key: 'at', label: 'When' },
		{ key: 'title', label: 'Title' },
		{ key: 'what', label: 'What happened' },
		{ key: 'detail', label: 'Detail' },
		{ key: 'by', label: 'By' }
	];
	const when = (at: string) => (dayKey(at) === dayKey(new Date().toISOString()) ? clock(at) : `${dayLabel(dayKey(at))}, ${clock(at)}`);
</script>

{#snippet tools()}
	<label class="opt"><input type="checkbox" bind:checked={hideRoutine} data-testid="hv-routine" /><span>Hide routine</span></label>
	<label class="opt size"><span>Rows</span><select class="input" bind:value={size} aria-label="Rows a page">{#each SIZES as n (n)}<option value={n}>{n}</option>{/each}</select></label>
{/snippet}

<div class="wrap">
	<HistoryToolbar {filter} {titles} {onfilter} {phone} extra={tools} />

	{#if error && !events.length}
		<LoadError hasData={false} message="History could not be loaded." onretry={load} />
	{:else if !loaded}
		<div class="hv-sk">{#each Array(10) as _, i (i)}<Skeleton height="34px" />{/each}</div>
	{:else if !events.length}
		<EmptyState icon="clock" title={filtered ? 'Nothing matches' : 'No history yet'} text={filtered ? 'No Event matches these filters.' : 'Every search, download, import, rename and delete writes an Event here.'} action={filtered ? { label: 'Show everything', onclick: () => onfilter({ title: undefined, groups: [], from: undefined, to: undefined }) } : undefined} />
	{:else}
		<div class="meta">
			<p class="count" data-testid="history-count">{pageLine(total, offset, events.length)} Events{sort.key !== 'at' || sort.dir !== -1 ? ', sorted on this page' : ''}</p>
			{#if !phone}<p class="keys" aria-hidden="true"><kbd>↑</kbd><kbd>↓</kbd> move · <kbd>Enter</kbd> open · <kbd>[</kbd><kbd>]</kbd> page</p>{/if}
		</div>
		{#if phone}
			<ol class="cards" data-testid="hv-table">
				{#each rows as r (r.key)}
					{@const e = r.head}
					{@const t = titleById.get(e.TitleID ?? 0)}
					<li class="pc" class:quiet={rowTier(r) === 'routine'}>
						<button type="button" class="pc-hit" onclick={() => (sheet = r)}>
							<span class="pc-art" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{/if}</span>
							<span class="pc-main">
								<span class="pc-top"><VerbMark verb={headVerb(r)} size="sm" /><time datetime={e.At}>{ago(e.At)}</time></span>
								<span class="pc-title">{e.title_name ?? 'HolyArrs'}{e.episode ? ` ${e.episode}` : ''}</span>
								<span class="pc-sum">{e.Summary}</span>
								<span class="pc-by">{byWhom(e)}</span>
							</span>
						</button>
					</li>
				{/each}
			</ol>
		{:else}
			<div class="tablebox">
				<table class="t" data-testid="hv-table">
					<caption class="sr-only">Events, newest first. Arrow keys move, Enter opens.</caption>
					<colgroup><col class="c-when" /><col class="c-title" /><col class="c-what" /><col class="c-detail" /><col class="c-by" /></colgroup>
					<thead>
						<tr>
							{#each columns as c (c.key)}
								<th scope="col" aria-sort={c.key === 'detail' ? undefined : ariaSort(c.key)}>
									{#if c.key === 'detail'}{c.label}{:else}
										{@const k = c.key}
										<button type="button" class="sort" class:on={sort.key === k} onclick={() => sortBy(k)}>{c.label}<Icon name={sort.key === k && sort.dir === 1 ? 'chevronUpDown' : 'chevronDown'} size={12} /></button>
									{/if}
								</th>
							{/each}
						</tr>
					</thead>
					<!-- The rows are the focus stops; their keys are read once, here. -->
					<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
					<tbody bind:this={body} onkeydown={key}>
						{#each rows as r, i (r.key)}
							{@const e = r.head}
							{@const href = titleHref(e.TitleID, titleById)}
							{@const t = titleById.get(e.TitleID ?? 0)}
							{@const d = detail(r)}
							<tr data-i={i} tabindex={i === at ? 0 : -1} class:quiet={rowTier(r) === 'routine'} class:sel={sheet?.key === r.key} aria-selected={i === at} onfocus={() => (at = i)} onclick={(ev) => { if (!(ev.target as HTMLElement).closest('a')) { at = i; sheet = r; } }}>
								<td class="when"><time datetime={e.At} title={dateTime(e.At)}>{when(e.At)}</time></td>
								<td class="title">
									<span class="tt">
										<span class="t-art" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{/if}</span>
										<span class="t-name">{#if href}<a href={to(href)} tabindex="-1">{e.title_name}</a>{:else}{e.title_name ?? '—'}{/if}{#if e.episode}<small>{e.episode}</small>{/if}</span>
									</span>
								</td>
								<td class="what"><VerbMark verb={headVerb(r)} size="sm" /></td>
								<td class="detail"><span class="sum">{e.Summary}</span>{#if d}<span class="d">{d}</span>{/if}</td>
								<td class="by">{byWhom(e)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
		<nav class="pages" aria-label="Pages">
			<Button variant="secondary" icon="chevronLeft" disabled={offset === 0} onclick={() => page(Math.max(0, offset - size))}>Newer</Button>
			<span class="pg">Page {Math.floor(offset / size) + 1} of {Math.max(1, Math.ceil(total / size))}</span>
			<Button variant="secondary" iconAfter="chevronRight" disabled={offset + events.length >= total} onclick={() => page(offset + size)}>Older</Button>
		</nav>
	{/if}
</div>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.wrap { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.opt { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 0 12px; border-radius: var(--r-pill); background: var(--surface-2); font: var(--t-footnote); font-weight: 600; cursor: pointer; }
	.opt input[type='checkbox'] { width: 16px; height: 16px; accent-color: var(--accent); }
	.opt.size { padding-right: 4px; }
	.opt.size select { width: auto; min-height: 28px; padding: 0 6px; border-radius: var(--r-pill); font: var(--t-footnote); }
	.meta { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: baseline; gap: 6px 16px; }
	.count { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.keys { color: var(--text-faint); font: var(--t-caption); }
	kbd { display: inline-block; min-width: 18px; margin: 0 1px; padding: 0 4px; border-radius: 4px; background: var(--surface-2); box-shadow: inset 0 -1px 0 var(--line-strong); font: 600 11px/18px var(--font-mono); text-align: center; color: var(--text-muted); }
	.tablebox { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), var(--shadow-card); }
	.t { width: 100%; border-collapse: separate; border-spacing: 0; table-layout: fixed; font: var(--t-footnote); }
	.c-when { width: 132px; } .c-title { width: 22%; } .c-what { width: 150px; } .c-by { width: 130px; }
	thead th { position: sticky; top: var(--topbar, 0px); z-index: 2; height: 36px; padding: 0 10px; background: color-mix(in srgb, var(--surface-1) 90%, transparent); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); box-shadow: inset 0 -1px 0 var(--line-strong); color: var(--text-muted); font: var(--t-caption); font-weight: 600; letter-spacing: .04em; text-transform: uppercase; text-align: left; }
	thead th:first-child { border-top-left-radius: var(--r-panel); }
	thead th:last-child { border-top-right-radius: var(--r-panel); }
	.sort { display: inline-flex; align-items: center; gap: 4px; min-height: 28px; color: inherit; font: inherit; letter-spacing: inherit; text-transform: inherit; }
	.sort.on { color: var(--text); }
	.sort:hover { color: var(--text); }
	tbody tr { cursor: pointer; }
	tbody td { height: 38px; padding: 4px 10px; box-shadow: inset 0 -1px 0 var(--line); vertical-align: middle; }
	tbody tr:last-child td { box-shadow: none; }
	tbody tr:hover td { background: var(--surface-2); }
	tbody tr:focus-visible { outline: none; }
	tbody tr:focus-visible td, tbody tr.sel td { background: var(--accent-soft); }
	tbody tr:focus-visible td:first-child { box-shadow: inset 3px 0 0 var(--accent), inset 0 -1px 0 var(--line); }
	tr.quiet td { color: var(--text-muted); }
	tr.quiet .t-name a { color: var(--text-muted); font-weight: 500; }
	.when { color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.tt { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.t-art { flex: none; width: 20px; --poster-ratio: 2 / 3; border-radius: 3px; overflow: hidden; }
	.t-art.square { --poster-ratio: 1; border-radius: 50%; }
	.t-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.t-name a { color: var(--text); font-weight: 600; }
	.t-name a:hover { text-decoration: underline; }
	.t-name small { margin-left: 6px; color: var(--text-muted); font: inherit; }
	.what { overflow: hidden; }
	.detail { overflow: hidden; }
	.detail .sum { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.detail .d { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-faint); font-family: var(--font-mono); font-size: 11px; }
	.by { color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pages { display: flex; align-items: center; justify-content: flex-end; gap: 10px; }
	.pg { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.hv-sk { display: flex; flex-direction: column; gap: 6px; }
	.cards { display: flex; flex-direction: column; margin: 0; padding: 2px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.pc + .pc { box-shadow: 0 -1px 0 var(--line); }
	.pc-hit { display: flex; gap: 10px; width: 100%; padding: 10px; text-align: left; border-radius: var(--r-btn); }
	.pc-hit:active { background: var(--surface-2); }
	.pc-art { flex: none; width: 34px; --poster-ratio: 2 / 3; border-radius: 4px; overflow: hidden; align-self: flex-start; }
	.pc-art.square { --poster-ratio: 1; border-radius: 50%; }
	.pc-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.pc-top { display: flex; justify-content: space-between; align-items: center; gap: 8px; color: var(--text-muted); font: var(--t-footnote); }
	.pc-title { font: var(--t-callout); font-weight: 600; overflow-wrap: anywhere; }
	.pc-sum { color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.pc-by { color: var(--text-muted); font: var(--t-caption); }
	.pc.quiet .pc-title { color: var(--text-muted); font-weight: 500; }
	@media (max-width: 1100px) { .c-by { width: 0; } .by, thead th:last-child { display: none; } thead th:nth-last-child(2) { border-top-right-radius: var(--r-panel); } }
	@media (max-width: 767px) {
		.opt { min-height: 44px; }
		.pages { justify-content: space-between; }
		.pages > :global(button) { min-height: 44px; }
	}
</style>
