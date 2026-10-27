<script lang="ts">
	// The Downloads table, like a Downloader's own list: every download in one
	// sortable table with tight rows, small posters and a mini progress bar.
	// Status chips on top filter it; five chip sets are being compared
	// (?set=1…5, see status-sets.ts). A row opens its details in a side sheet
	// (a bottom sheet on phones).
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { untrack } from 'svelte';
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
	import { ALL, SETS, chipsFor, counts, pick, seedTag, showOf, statusWord, subCounts, type SetId } from './status-sets.ts';
	import { byDownload, statusDetail } from './imports.ts';
	import { poster, who, type LayoutProps } from './shared.ts';

	let { rows, imports, titles, busy, ondetails, onfail, onremove, set, torrent }: LayoutProps & { set: SetId; torrent: boolean } = $props();
	let key = $state<SortKey>('state');
	let dir = $state<'asc' | 'desc'>('asc');

	// The picked chip, kept per set for the session and in the address (&show=).
	const KEEP = 'holyarrs.downloads-show';
	let show = $state(ALL);
	let sub = $state('');
	function kept(): Record<string, string> {
		try {
			return JSON.parse(sessionStorage.getItem(KEEP) ?? '{}') ?? {};
		} catch {
			return {};
		}
	}
	function remember() {
		const v = sub ? `${show}.${sub}` : show;
		try {
			sessionStorage.setItem(KEEP, JSON.stringify({ ...kept(), [set]: v }));
		} catch {
			/* storage blocked: the address alone keeps it */
		}
		try {
			const u = new URL(page.url.href);
			if (show === ALL) u.searchParams.delete('show');
			else u.searchParams.set('show', v);
			if (u.href !== page.url.href) replaceState(u, page.state);
		} catch {
			/* the router is not ready yet; the next pick writes it */
		}
	}
	let first = true;
	$effect(() => {
		const s = set;
		untrack(() => {
			// The address decides on the first visit; after that each set
			// comes back to the chip last picked in it.
			const wasFirst = first;
			first = false;
			const v = wasFirst ? (page.url.searchParams.get('show') ?? kept()[s]) : kept()[s];
			({ show, sub } = showOf(v, SETS[s].chips));
			remember();
		});
	});

	const chips = $derived(chipsFor(set, rows, torrent));
	const tally = $derived(counts(rows, chips));
	// A chip that is hidden (Seeding without a torrent Downloader) reads as All.
	const on = $derived(chips.some((c) => c.key === show) ? show : ALL);
	const onChip = $derived(chips.find((c) => c.key === on));
	const subs = $derived(subCounts(rows, onChip));
	const subOn = $derived(subs.some((x) => x.key === sub) ? sub : '');
	const base = $derived(on === ALL ? rows : pick(rows, chips, on, subOn));
	const shown = $derived(sortRows(base, key, dir, rowName));

	function choose(k: string) {
		show = k;
		sub = '';
		remember();
	}
	function chooseSub(k: string) {
		sub = k;
		remember();
	}

	// The sub-row slides open under the chips and folds away quicker than it came.
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

	// What an empty chip says.
	const EMPTY: Record<string, { title: string; text: string; icon: IconName }> = {
		all: { title: 'Nothing is downloading.', text: 'New picks from HolyArrs appear here as rows, with speed and time left.', icon: 'download' },
		waiting: { title: 'Nothing is waiting.', text: 'Downloads the Downloader has not started yet show here.', icon: 'download' },
		queued: { title: 'Nothing is queued.', text: 'Downloads the Downloader has not started yet show here.', icon: 'download' },
		downloading: { title: 'Nothing is downloading.', text: 'Downloads taking in data show here, with speed and time left.', icon: 'download' },
		importing: { title: 'Nothing is importing.', text: 'Finished downloads show here while they are checked, unpacked and moved into the Library.', icon: 'folder' },
		done: { title: 'Nothing finished yet.', text: 'Downloads that reached the Library show here.', icon: 'check' },
		finished: { title: 'Nothing finished yet.', text: 'Downloads that reached the Library show here.', icon: 'check' },
		seeding: { title: 'Nothing is seeding.', text: 'Finished torrents your Downloader still shares show here.', icon: 'check' },
		active: { title: 'Nothing is active.', text: 'Downloads your Downloader is working on right now show here.', icon: 'download' },
		paused: { title: 'Nothing is paused.', text: 'Downloads paused in the Downloader show here.', icon: 'check' },
		stalled: { title: 'Nothing is stalled.', text: 'Downloads that stopped taking in data show here.', icon: 'check' },
		failed: { title: 'Nothing failed.', text: 'Downloads that failed or wait for a decision show here.', icon: 'check' },
		problems: { title: 'No problems.', text: 'Stalled, paused and failed downloads show here.', icon: 'check' },
		needs: { title: 'Nothing needs you.', text: 'Paused, stalled and failed downloads show here.', icon: 'check' },
		stopped: { title: 'Nothing is stopped.', text: 'Waiting, paused, stalled and failed downloads show here.', icon: 'check' },
		progress: { title: 'Nothing is in progress.', text: 'Downloads that are waiting, downloading or importing show here.', icon: 'download' }
	};
	const empty = $derived(EMPTY[subOn || on] ?? EMPTY.all);

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
	const ratio = (r: DownloadRow) => (typeof r.ratio === 'number' ? r.ratio.toFixed(2) : '');
</script>

{#snippet seed(r: DownloadRow)}
	{#if seedTag(r, set)}<span class="seed" data-testid="row-seeding">Seeding{#if ratio(r)}<span class="ratio"> · {ratio(r)}</span>{/if}</span>{/if}
{/snippet}

<div class="tbl-view">
	<div class="tools">
		<div class="chips" role="group" aria-label="Show" data-testid="table-filter" data-set={set}>
			{#each tally as c (c.key)}
				<span class="cw" class:zero={c.count === 0} data-testid="status-chip" data-key={c.key} data-count={c.count}>
					<Chip label={c.label} count={c.count} note={c.seeding ? `${c.seeding} seeding` : undefined} selected={on === c.key} onclick={() => choose(c.key)} />
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
				<div class="subs" role="group" aria-label="{onChip?.label ?? ''}: show" data-testid="table-subfilter" in:swapIn>
					<span class="cw sm" data-testid="status-subchip" data-key="all" data-count={tally.find((t) => t.key === on)?.count ?? 0}>
						<Chip label="All" selected={subOn === ''} onclick={() => chooseSub('')} />
					</span>
					{#each subs as s (s.key)}
						<span class="cw sm" class:zero={s.count === 0} data-testid="status-subchip" data-key={s.key} data-count={s.count}>
							<Chip label={s.label} count={s.count} selected={subOn === s.key} onclick={() => chooseSub(s.key)} />
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
						{@const word = statusWord(r, set)}
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
							<td class="c-state"><span class="st {t}" data-testid="row-state" title={more ? `${word} · ${more}` : word}><i aria-hidden="true"></i><span class="sw">{word}{#if more}<span class="sd">{` · ${more}`}</span>{/if}</span></span>{@render seed(r)}</td>
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
							<span class="pl"><span class="st {t}"><i aria-hidden="true"></i><span class="sw">{statusWord(r, set)}{#if more}<span class="sd">{` · ${more}`}</span>{/if}</span></span>{@render seed(r)}{#if manyWords(r.kind) && r.covers}<span class="cv">{r.covers}</span>{/if}</span>
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
	.cw { display: inline-flex; transition: opacity var(--dur-fast) var(--ease-out); }
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
	.seed { display: inline-flex; align-items: center; flex: none; margin-left: 8px; padding: 1px 6px; border-radius: 4px; background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; white-space: nowrap; }
	.ratio { font-weight: 400; font-variant-numeric: tabular-nums; }
	.pl .seed { margin-left: 0; }
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
		.chips { width: 100%; }
	}
	@media (prefers-reduced-motion: reduce) { .arrow, tbody tr, .cw { transition: none; } }
</style>
