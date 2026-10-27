<script lang="ts">
	// Layout 5 · Table: every download in one sortable table with tight rows,
	// small posters and a mini progress bar. A row opens its details in a side
	// sheet (a bottom sheet on phones). For people with many downloads.
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Menu from '../ui/Menu.svelte';
	import SegmentedControl from '../ui/SegmentedControl.svelte';
	import DlMore from './DlMore.svelte';
	import DlImports from './DlImports.svelte';
	import { to } from '../paths.ts';
	import { bytes, duration, percent } from '../format.ts';
	import { rowName, manyWords, type DownloadRow } from '../downloaders.ts';
	import { finished, needsAttention, queue, sortRows, tone, type SortKey } from './layouts.ts';
	import { poster, who, type LayoutProps } from './shared.ts';

	let { rows, imports, titles, busy, ondetails, onfail, onremove }: LayoutProps = $props();
	type Filter = 'now' | 'attention' | 'done' | 'all';
	let filter = $state<Filter>('now');
	let key = $state<SortKey>('state');
	let dir = $state<'asc' | 'desc'>('asc');

	const now = $derived(queue(rows));
	const attention = $derived(now.filter(needsAttention));
	const done = $derived(finished(rows));
	const everything = $derived([...now, ...done]);
	const base = $derived(filter === 'now' ? now : filter === 'attention' ? attention : filter === 'done' ? done : everything);
	const shown = $derived(sortRows(base, key, dir, rowName));
	const filters = $derived([
		{ value: 'now' as const, label: `Now ${now.length}` },
		{ value: 'attention' as const, label: `Attention ${attention.length}` },
		{ value: 'done' as const, label: `Done ${done.length}` },
		{ value: 'all' as const, label: `All ${everything.length}` }
	]);

	const COLS: { key: SortKey; label: string; num?: boolean; cls: string }[] = [
		{ key: 'title', label: 'Title', cls: 'c-title' },
		{ key: 'state', label: 'State', cls: 'c-state' },
		{ key: 'progress', label: 'Progress', cls: 'c-prog' },
		{ key: 'speed', label: 'Speed', num: true, cls: 'c-speed' },
		{ key: 'eta', label: 'Time left', num: true, cls: 'c-eta' },
		{ key: 'size', label: 'Size', num: true, cls: 'c-size' },
		{ key: 'downloader', label: 'Downloader', cls: 'c-dl' }
	];
	function sortBy(k: SortKey) {
		if (key === k) dir = dir === 'asc' ? 'desc' : 'asc';
		else {
			key = k;
			// Numbers read biggest first; words and states read from the top.
			dir = k === 'progress' || k === 'speed' || k === 'size' ? 'desc' : 'asc';
		}
	}
	const sortItems = $derived(COLS.map((c) => ({ label: c.label, checked: key === c.key, onselect: () => sortBy(c.key) })));
	const sortLabel = $derived(COLS.find((c) => c.key === key)?.label ?? '');
	function rowKey(e: KeyboardEvent, r: DownloadRow) {
		if (e.target !== e.currentTarget) return;
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			ondetails(r);
		}
	}
	const going = (r: DownloadRow) => r.active && r.phase === 'downloading';
</script>

<div class="tbl-view">
	<div class="tools">
		<SegmentedControl options={filters} bind:value={filter} label="Show" size="sm" testid="table-filter" />
		<span class="phone-sort">
			<Menu label="Sort by" items={sortItems} variant="button" text="Sort: {sortLabel}" icon="chevronUpDown" />
			<button type="button" class="dirbtn" aria-label={dir === 'asc' ? 'Ascending, switch to descending' : 'Descending, switch to ascending'} onclick={() => (dir = dir === 'asc' ? 'desc' : 'asc')}><span class="dirarr" class:up={dir === 'asc'}><Icon name="chevronDown" size={18} /></span></button>
		</span>
	</div>

	{#if shown.length === 0}
		<div class="empty" data-testid="dl-empty">
			<span class="eic"><Icon name={filter === 'attention' ? 'check' : 'download'} size={22} stroke={2} /></span>
			<p class="et">{filter === 'attention' ? 'Nothing needs attention.' : filter === 'done' ? 'Nothing finished yet.' : 'Nothing is downloading.'}</p>
			<p class="es">{filter === 'now' ? 'New picks from HolyArrs appear here as rows, with speed and time left.' : 'Switch to All to see every download.'}</p>
			{#if filter === 'now'}<a class="elink" href={to('/search')}>Add media</a>{/if}
		</div>
	{:else}
		<div class="scroller">
			<table class="t" data-testid="dl-table">
				<thead>
					<tr>
						{#each COLS as c (c.key)}
							<th class={c.cls} class:num={c.num} aria-sort={key === c.key ? (dir === 'asc' ? 'ascending' : 'descending') : undefined} scope="col">
								<button type="button" class="sort" class:on={key === c.key} onclick={() => sortBy(c.key)}>
									{c.label}<span class="arrow" class:desc={key === c.key && dir === 'desc'} aria-hidden="true"><Icon name="chevronDown" size={14} stroke={2.2} /></span>
								</button>
							</th>
						{/each}
						<th class="c-act"><span class="sr-only">Actions</span></th>
					</tr>
				</thead>
				<tbody>
					{#each shown as r (r.id)}
						{@const pct = percent(r.progress)}
						{@const t = tone(r)}
						<tr class={t} data-testid="dl-table-row" data-phase={r.phase} tabindex="0" aria-label="{who(r)}, {r.label}. Open details" onclick={() => ondetails(r)} onkeydown={(e) => rowKey(e, r)}>
							<td class="c-title">
								<span class="tc">
									<span class="post"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
									<span class="tn">
										<span class="nm" title={rowName(r)}>{rowName(r)}{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
										<span class="rel" title={r.name}>{manyWords(r.kind) && r.covers ? r.covers : r.name}</span>
									</span>
								</span>
							</td>
							<td class="c-state"><span class="st {t}" data-testid="row-state"><i aria-hidden="true"></i>{r.label}</span></td>
							<td class="c-prog">
								{#if r.phase !== 'failed' && r.phase !== 'removed' && r.phase !== 'vanished'}
									<span class="pg"><span class="mini" aria-hidden="true"><i style="width: {pct}%"></i></span><span class="pv" data-testid="row-percent">{Math.floor(pct)}%</span></span>
								{:else}<span class="dash">–</span>{/if}
							</td>
							<td class="c-speed num">{going(r) && r.speed ? `${bytes(r.speed)}/s` : '–'}</td>
							<td class="c-eta num">{going(r) && r.eta ? duration(r.eta) : '–'}</td>
							<td class="c-size num">{r.size ? bytes(r.size) : '–'}</td>
							<td class="c-dl"><span class="dl" title={r.downloader}>{r.downloader || '–'}</span></td>
							<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
							<td class="c-act" onclick={(e) => e.stopPropagation()}><DlMore {r} name={who(r)} {busy} {ondetails} {onfail} {onremove} /></td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<!-- Phones: the same rows as a tight list; a tap opens the bottom sheet. -->
		<ul class="plist" data-testid="dl-table-list">
			{#each shown as r (r.id)}
				{@const pct = percent(r.progress)}
				{@const t = tone(r)}
				<li class="prow {t}" data-phase={r.phase}>
					<button type="button" class="phit" aria-label="Details for {who(r)}" onclick={() => ondetails(r)}>
						<span class="post"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
						<span class="tn">
							<span class="nm">{rowName(r)}</span>
							<span class="pl"><span class="st {t}"><i aria-hidden="true"></i>{r.label}</span>{#if manyWords(r.kind) && r.covers}<span class="cv">{r.covers}</span>{/if}</span>
							{#if r.phase !== 'failed' && r.phase !== 'removed' && r.phase !== 'vanished' && r.phase !== 'done'}<span class="mini" aria-hidden="true"><i style="width: {pct}%"></i></span>{/if}
						</span>
						<span class="pr">
							<b>{r.phase === 'done' ? (r.size ? bytes(r.size) : '') : `${Math.floor(pct)}%`}</b>
							{#if going(r) && r.eta}<small>{duration(r.eta)}</small>{:else if going(r) && r.speed}<small>{bytes(r.speed)}/s</small>{/if}
						</span>
					</button>
					<DlMore {r} name={who(r)} {busy} {ondetails} {onfail} {onremove} />
				</li>
			{/each}
		</ul>
	{/if}

	<DlImports {imports} compact />
</div>

<style>
	.tbl-view { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	.tools { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
	.phone-sort { display: none; align-items: center; gap: 4px; }
	.dirbtn { display: grid; place-items: center; width: 40px; height: 40px; border: 0; border-radius: 10px; background: var(--surface-2); color: var(--text-muted); cursor: pointer; }
	.dirarr { display: grid; transition: transform var(--dur-fast); }
	.dirarr.up { transform: rotate(180deg); }
	.dirbtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

	.scroller { overflow-x: auto; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.t { width: 100%; border-collapse: collapse; table-layout: fixed; font: var(--t-callout); font-variant-numeric: tabular-nums; }
	thead { position: sticky; top: 0; background: var(--surface-1); }
	th { height: 38px; padding: 0 12px; text-align: left; border-bottom: 1px solid var(--line); }
	th.num { text-align: right; }
	.sort { display: inline-flex; align-items: center; gap: 4px; min-height: 30px; padding: 0 4px; margin: 0 -4px; border: 0; border-radius: 6px; background: none; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.02em; cursor: pointer; white-space: nowrap; }
	.sort:hover, .sort.on { color: var(--text); }
	.sort:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.arrow { display: inline-grid; opacity: 0; transform: rotate(180deg); transition: transform var(--dur-fast), opacity var(--dur-fast); }
	.sort.on .arrow { opacity: 1; }
	.sort:hover .arrow { opacity: 0.5; }
	.sort.on:hover .arrow { opacity: 1; }
	.arrow.desc { transform: none; }
	td { height: 52px; padding: 0 12px; vertical-align: middle; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	td.num { text-align: right; color: var(--text-muted); }
	tbody tr { cursor: pointer; transition: background var(--dur-instant); }
	tbody tr + tr { border-top: 1px solid var(--line); }
	tbody tr:hover { background: var(--surface-2); }
	tbody tr:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	tbody tr.fail { box-shadow: inset 3px 0 0 var(--fail); }
	tbody tr.fail:focus-visible { box-shadow: inset 3px 0 0 var(--fail), inset 0 0 0 2px var(--accent); }
	.c-title { width: auto; }
	.c-state { width: 206px; }
	.c-prog { width: 150px; }
	.c-speed { width: 100px; }
	.c-eta { width: 92px; }
	.c-size { width: 86px; }
	.c-dl { width: 150px; }
	.c-act { width: 52px; padding: 0 6px; text-align: right; }
	.tc { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.post { flex: none; width: 26px; }
	.post :global(.poster) { border-radius: 4px; }
	.tn { display: flex; flex-direction: column; min-width: 0; }
	.nm { color: var(--text); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ver { margin-left: 6px; padding: 0 5px; border-radius: 4px; background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); }
	.rel { color: var(--text-faint); font: var(--t-footnote); font-weight: 400; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.st { display: inline-flex; align-items: center; gap: 7px; max-width: 100%; overflow: hidden; text-overflow: ellipsis; color: var(--text); font-weight: 500; }
	.st i { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--info); }
	.st.fail { color: var(--fail-text); }
	.st.fail i { background: var(--fail); }
	.st.ok i { background: var(--ok); }
	.st.wait i { background: var(--wait); }
	.st.idle i { background: var(--text-faint); }
	.st.idle { color: var(--text-muted); }
	.pg { display: flex; align-items: center; gap: 8px; }
	.mini { position: relative; display: block; flex: 1; height: 4px; border-radius: 2px; background: var(--surface-3); overflow: hidden; }
	.mini i { display: block; height: 100%; border-radius: 2px; background: var(--info); }
	.fail .mini i { background: var(--fail); }
	.ok .mini i { background: var(--ok); }
	.idle .mini i { background: var(--text-faint); }
	.pv { width: 34px; text-align: right; color: var(--text-muted); font: var(--t-footnote); }
	.dash { color: var(--text-faint); }
	.dl { display: block; overflow: hidden; text-overflow: ellipsis; color: var(--text-muted); }

	.plist { display: none; list-style: none; margin: 0; padding: 4px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); }
	.prow { display: flex; align-items: center; gap: 2px; border-radius: var(--r-btn); }
	.prow + .prow { box-shadow: 0 -1px 0 var(--line); }
	.prow.fail { box-shadow: inset 3px 0 0 var(--fail); }
	.prow + .prow.fail { box-shadow: inset 3px 0 0 var(--fail), 0 -1px 0 var(--line); }
	.phit { flex: 1; display: grid; grid-template-columns: 34px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 60px; min-width: 0; padding: 8px 4px 8px 10px; border: 0; border-radius: var(--r-btn); background: none; color: inherit; text-align: left; cursor: pointer; font: inherit; }
	.phit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.phit .post { width: 34px; }
	.pl { display: flex; align-items: center; gap: 8px; min-width: 0; font: var(--t-footnote); }
	.pl .st { font-weight: 600; }
	.cv { color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.phit .mini { margin-top: 5px; flex: none; }
	.pr { display: flex; flex-direction: column; align-items: flex-end; font-variant-numeric: tabular-nums; }
	.pr b { font: var(--t-callout); font-weight: 600; }
	.pr small { color: var(--text-muted); font: var(--t-footnote); }

	.empty { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 40px 20px; text-align: center; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); }
	.eic { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 12px; background: var(--surface-2); color: var(--text-muted); }
	.et { font: var(--t-headline); }
	.es { color: var(--text-muted); font: var(--t-callout); max-width: 360px; }
	.elink { display: inline-flex; align-items: center; min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 600; }

	@media (max-width: 1199px) { .c-dl { display: none; } .c-state { width: 150px; } }
	@media (max-width: 960px) { .c-size, .c-speed { display: none; } }
	@media (max-width: 767px) {
		.scroller { display: none; }
		.plist { display: block; }
		.phone-sort { display: inline-flex; }
		.tools :global(.ui-seg) { display: grid; width: 100%; }
		.tools :global(.ui-seg button) { padding: 0 4px; }
	}
	@media (prefers-reduced-motion: reduce) { .arrow, tbody tr { transition: none; } }
</style>
