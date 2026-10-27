<script lang="ts">
	// The Downloads table, like a Downloader's own list: every download in one
	// sortable table with tight rows, small posters and a mini progress bar.
	// Five status tabs on top filter it (All · Waiting · In progress · Done ·
	// Problems), with small toggle sub-tabs under the picked one (see
	// status-tabs.ts). A row opens its details in a side sheet (a bottom
	// sheet on phones).
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { onMount } from 'svelte';
	import { fade, slide, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import Menu from '../ui/Menu.svelte';
	import Chip from '../ui/Chip.svelte';
	import DlMore from './DlMore.svelte';
	import { to } from '../paths.ts';
	import { bytes, duration, percent } from '../format.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { rowName, manyWords, type DownloadRow } from '../downloaders.ts';
	import { sortRows, tone, type SortKey } from './layouts.ts';
	import { ALL, ALL_HINT, counts, pick, showOf, statusWord, subCounts, tabsFor } from './status-tabs.ts';
	import { byDownload, statusDetail } from './imports.ts';
	import { poster, who, type LayoutProps } from './shared.ts';

	let { rows, imports, titles, busy, ondetails, onfail, onremove, torrent }: LayoutProps & { torrent: boolean } = $props();
	let key = $state<SortKey>('state');
	let dir = $state<'asc' | 'desc'>('asc');

	// The picked tab (and sub-tab) lives in the address: &show=tab or tab.sub.
	const start = showOf(page.url.searchParams.get('show'));
	let show = $state(start.show);
	let sub = $state(start.sub);
	function remember() {
		try {
			const u = new URL(page.url.href);
			if (show === ALL) u.searchParams.delete('show');
			else u.searchParams.set('show', sub ? `${show}.${sub}` : show);
			if (u.href !== page.url.href) replaceState(u, page.state);
		} catch {
			/* the router is not ready yet; the next pick writes it */
		}
	}
	// Old links with ?set= (the word sets that were compared) land here too; the parameter is dropped.
	onMount(() => {
		if (!page.url.searchParams.has('set')) return;
		try {
			const u = new URL(page.url.href);
			u.searchParams.delete('set');
			replaceState(u, page.state);
		} catch {
			/* harmless: the parameter is ignored */
		}
	});

	const tabs = $derived(tabsFor(rows, torrent));
	const tally = $derived(counts(rows, tabs));
	const on = $derived(tabs.some((c) => c.key === show) ? show : ALL);
	const onTab = $derived(tabs.find((c) => c.key === on));
	const subs = $derived(subCounts(rows, onTab));
	// A sub-tab that is hidden (Seeding without a torrent Downloader) reads as none.
	const subOn = $derived(subs.some((x) => x.key === sub) ? sub : '');
	const base = $derived(on === ALL ? rows : pick(rows, tabs, on, subOn));
	const shown = $derived(sortRows(base, key, dir, rowName));

	function choose(k: string) {
		show = k;
		sub = '';
		remember();
	}
	// A sub-tab is a toggle: it narrows the big tab's list, and a second click
	// (or a click on the big tab) clears it.
	function chooseSub(k: string) {
		sub = sub === k ? '' : k;
		remember();
	}

	// The sub-row slides open under the tabs and folds away quicker than it came.
	function subIn(node: Element): TransitionConfig {
		if (reducedMotion()) return fade(node, { duration: DUR.fast });
		return slide(node, { duration: DUR.base, easing: easeOut });
	}
	function subOut(node: Element): TransitionConfig {
		if (reducedMotion()) return fade(node, { duration: DUR.fast });
		return slide(node, { duration: DUR.fast, easing: easeIn });
	}
	function swapIn(node: Element): TransitionConfig {
		return fade(node, { duration: reducedMotion() ? 0 : DUR.fast, easing: easeOut });
	}

	// What an empty tab says: its title, and its hint as the line under it.
	const ICON: Record<string, IconName> = { all: 'download', waiting: 'download', queued: 'download', progress: 'download', downloading: 'download', importing: 'folder' };
	const empty = $derived.by(() => {
		const s = subOn ? onTab?.subs.find((x) => x.key === subOn) : undefined;
		const item = s ?? onTab;
		return {
			title: item?.empty ?? 'Nothing is downloading.',
			text: item?.hint ?? ALL_HINT,
			icon: ICON[subOn || on] ?? 'check'
		};
	});

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
	// Each row carries what its import is doing or why it failed; there is no separate Imports list.
	const importOf = $derived(byDownload(imports));
	const going = (r: DownloadRow) => r.active && r.phase === 'downloading';
</script>

<div class="tbl-view">
	<div class="tools">
		<div class="chips" role="group" aria-label="Show" data-testid="table-filter">
			{#each tally as c (c.key)}
				<span class="cw" class:zero={c.count === 0} data-testid="status-chip" data-key={c.key} data-count={c.count} data-tip={c.hint}>
					<Chip label={c.label} count={c.count} note={c.seeding ? `${c.seeding} seeding` : undefined} selected={on === c.key} describedby="dl-hint-{c.key}" onclick={() => choose(c.key)} />
					<span class="sr-only" id="dl-hint-{c.key}">{c.hint}</span>
				</span>
			{/each}
		</div>
		<span class="phone-sort">
			<Menu label="Sort by" items={sortItems} variant="button" text="Sort: {sortLabel}" icon="chevronUpDown" />
			<button type="button" class="dirbtn" aria-label={dir === 'asc' ? 'Ascending, switch to descending' : 'Descending, switch to ascending'} onclick={() => (dir = dir === 'asc' ? 'desc' : 'asc')}><span class="dirarr" class:up={dir === 'asc'}><Icon name="chevronDown" size={18} /></span></button>
		</span>
	</div>

	{#if subs.length}
		<div class="subrow" in:subIn out:subOut>
			{#key on}
				<div class="subs" role="group" aria-label="{onTab?.label ?? ''}: show" data-testid="table-subfilter" in:swapIn>
					{#each subs as s (s.key)}
						<span class="cw sm" class:zero={s.count === 0} data-testid="status-subchip" data-key={s.key} data-count={s.count} data-tip={s.hint}>
							<Chip label={s.label} count={s.count} selected={subOn === s.key} describedby="dl-subhint-{s.key}" onclick={() => chooseSub(s.key)} />
							<span class="sr-only" id="dl-subhint-{s.key}">{s.hint}</span>
						</span>
					{/each}
				</div>
			{/key}
		</div>
	{/if}

	{#if shown.length === 0}
		<div class="empty" data-testid="dl-empty">
			<span class="eic"><Icon name={empty.icon} size={22} stroke={2} /></span>
			<p class="et">{empty.title}</p>
			<p class="es">{empty.text}</p>
			{#if on === ALL}<a class="elink" href={to('/search')}>Add media</a>{:else if rows.length}<button type="button" class="elink" onclick={() => choose(ALL)}>Show all {rows.length}</button>{/if}
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
						{@const word = statusWord(r)}
						{@const more = statusDetail(r, importOf.get(r.id))}
						<tr class={t} data-testid="dl-table-row" data-phase={r.phase} tabindex="0" aria-label="{who(r)}, {word}{more ? `, ${more}` : ''}. Open details" onclick={() => ondetails(r)} onkeydown={(e) => rowKey(e, r)}>
							<td class="c-title">
								<span class="tc">
									<span class="post"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
									<span class="tn">
										<span class="nm" title={rowName(r)}>{rowName(r)}{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
										<span class="rel" title={r.name}>{manyWords(r.kind) && r.covers ? r.covers : r.name}</span>
									</span>
								</span>
							</td>
							<td class="c-state"><span class="st {t}" data-testid="row-state" title={more ? `${word} · ${more}` : word}><i aria-hidden="true"></i><span class="sw">{word}{#if more}<span class="sd">{` · ${more}`}</span>{/if}</span></span></td>
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
				{@const more = statusDetail(r, importOf.get(r.id))}
				<li class="prow {t}" data-phase={r.phase}>
					<button type="button" class="phit" aria-label="Details for {who(r)}" onclick={() => ondetails(r)}>
						<span class="post"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
						<span class="tn">
							<span class="nm">{rowName(r)}</span>
							<span class="pl"><span class="st {t}"><i aria-hidden="true"></i><span class="sw">{statusWord(r)}{#if more}<span class="sd">{` · ${more}`}</span>{/if}</span></span>{#if manyWords(r.kind) && r.covers}<span class="cv">{r.covers}</span>{/if}</span>
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
</div>

<style>
	.tbl-view { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	.tools { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
	.chips, .subs { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.subrow { margin-top: -6px; }
	.subs { gap: 6px; }
	/* A chip with nothing under it stays in place, quieter. */
	.cw { position: relative; display: inline-flex; transition: opacity var(--dur-fast) var(--ease-out); }
	/* The chip's hint: a tooltip on hover or keyboard focus, after a short wait
	   (the same look as the icon buttons' tips). Not drawn at rest, so it never
	   widens the page; touch screens read it as the empty-state line instead. */
	.cw[data-tip]::after {
		content: attr(data-tip); position: absolute; z-index: 60; left: 0; top: calc(100% + 6px);
		display: none; width: max-content; max-width: min(280px, calc(100vw - 32px)); padding: 6px 10px; border-radius: 8px;
		background: var(--text); color: var(--bg); font: var(--t-footnote); white-space: normal; pointer-events: none;
	}
	.cw[data-tip]:hover::after, .cw[data-tip]:has(:focus-visible)::after { display: block; animation: tip-in var(--dur-fast) var(--ease-out) 500ms backwards; }
	/* Chips on the right half open their tip towards the left. */
	.chips .cw:nth-last-child(-n + 2)[data-tip]::after { left: auto; right: 0; }
	@keyframes tip-in { from { opacity: 0; translate: 0 -2px; } }
	@media (hover: none) { .cw[data-tip]:hover::after { display: none; } }
	.cw.zero { opacity: 0.5; }
	.cw.zero:hover, .cw.zero:focus-within { opacity: 0.8; }
	.cw.sm :global(.main) { min-height: 28px; padding: 0 11px; font-size: 13px; }
	.phone-sort { display: none; align-items: center; gap: 4px; }
	.dirbtn { display: grid; place-items: center; width: 40px; height: 40px; border: 0; border-radius: 10px; background: var(--surface-2); color: var(--text-muted); cursor: pointer; }
	.dirarr { display: grid; transition: transform var(--dur-fast) var(--ease-out); }
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
	.arrow { display: inline-grid; opacity: 0; transform: rotate(180deg); transition: transform var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.sort.on .arrow { opacity: 1; }
	.sort:hover .arrow { opacity: 0.5; }
	.sort.on:hover .arrow { opacity: 1; }
	.arrow.desc { transform: none; }
	td { height: 52px; padding: 0 12px; vertical-align: middle; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	td.num { text-align: right; color: var(--text-muted); }
	tbody tr { cursor: pointer; transition: background var(--dur-instant) var(--ease-out); }
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
	.sw { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sd { color: var(--text-muted); font-weight: 400; }
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
	.elink { display: inline-flex; align-items: center; min-height: 44px; padding: 0 4px; border: 0; background: none; color: var(--accent-text); font: var(--t-callout); font-weight: 600; cursor: pointer; border-radius: 6px; }
	.elink:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	@media (max-width: 1199px) { .c-dl { display: none; } .c-state { width: 150px; } }
	@media (max-width: 960px) { .c-size, .c-speed { display: none; } }
	@media (max-width: 767px) {
		.scroller { display: none; }
		.plist { display: block; }
		.phone-sort { display: inline-flex; }
		/* The five tabs share one row: the word above its count, smaller,
		   wrapping onto two lines when a word needs it. */
		.chips { width: 100%; display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); align-items: stretch; gap: 4px; }
		.chips .cw, .chips .cw :global(.ui-chip) { width: 100%; min-width: 0; }
		.chips .cw :global(.ui-chip), .chips .cw :global(.main) { height: 100%; }
		.chips .cw :global(.main) { width: 100%; flex-direction: column; justify-content: center; gap: 1px; min-height: 48px; padding: 4px 2px; font-size: 12.5px; line-height: 1.15; text-align: center; white-space: normal; overflow-wrap: anywhere; }
		/* The seeding sub-count lives on the Seeding sub-tab here. */
		.chips .cw :global(.n + .n) { display: none; }
	}
	@media (prefers-reduced-motion: reduce) { .arrow, tbody tr, .cw { transition: none; } .cw[data-tip]::after { animation-name: none; } }
</style>
