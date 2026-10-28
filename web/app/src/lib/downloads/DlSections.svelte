<script lang="ts">
	// The Downloads page body, laid out like a Downloader's own web page: a
	// header strip (speed with a small graph, time left), the status tabs
	// (All · Waiting · In progress · Done · Problems, with sub-tabs; see
	// status-tabs.ts), then the Queue and History. Every row of both sections
	// shares one fixed column grid: ▾ · poster · Title · bar · Status · Size ·
	// Time · bin. ▾ opens a row's details in place; the bin removes it
	// (through the page's Remove dialog).
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { tick } from 'svelte';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Chip from '../ui/Chip.svelte';
	import Menu from '../ui/Menu.svelte';
	import DecisionPanel from './DecisionPanel.svelte';
	import { ago, bytes, duration, percent } from '../format.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { manyWords, rowName, type Decision, type DownloadRow } from '../downloaders.ts';
	import type { ImportRecord, TitleView } from '../types.ts';
	import { ALL, counts, pick, showOf, statusWord, subCounts, tabsFor } from './status-tabs.ts';
	import { statusDetail } from './imports.ts';
	import { poster, who } from './shared.ts';
	import { barTone, facts, historyStatus, historyTotals, inHistory, matches, sizeLine, sparkPath, totals } from './sections.ts';

	let {
		rows,
		importOf,
		titles,
		busy,
		samples,
		torrent = false,
		decisionsOf,
		openId = $bindable(0),
		onfail,
		onremove,
		ondone
	}: {
		rows: DownloadRow[];
		importOf: Map<number, ImportRecord>;
		titles: Map<number, TitleView>;
		busy: number;
		/** Recent total-speed readings, oldest first. */
		samples: number[];
		/** A torrent Downloader brings the Seeding sub-tab under Done. */
		torrent?: boolean;
		decisionsOf: (r: DownloadRow) => Decision[];
		/** The row whose details are open (0: none). */
		openId?: number;
		onfail: (r: DownloadRow) => void;
		onremove: (r: DownloadRow) => void;
		ondone: () => void;
	} = $props();

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
	const tabs = $derived(tabsFor(rows, torrent));
	const tally = $derived(counts(rows, tabs));
	const on = $derived(tabs.some((c) => c.key === show) ? show : ALL);
	const onTab = $derived(tabs.find((c) => c.key === on));
	const subs = $derived(subCounts(rows, onTab));
	const subOn = $derived(subs.some((x) => x.key === sub) ? sub : '');
	const shown = $derived(on === ALL ? rows : pick(rows, tabs, on, subOn));
	function choose(k: string) {
		show = k;
		sub = '';
		remember();
	}
	// A sub-tab is a toggle: a second click clears it.
	function chooseSub(k: string) {
		sub = sub === k ? '' : k;
		remember();
	}

	// The header strip counts the whole Queue, whatever tab is picked.
	const allQueue = $derived(rows.filter((r) => !inHistory(r)));
	const queue = $derived(shown.filter((r) => !inHistory(r)));
	const history = $derived(shown.filter(inHistory).sort((a, b) => (b.finished_at || b.updated_at).localeCompare(a.finished_at || a.updated_at)));
	let q = $state('');
	const found = $derived(history.filter((r) => matches(r, q)));
	const sum = $derived(totals(allQueue));
	const problems = $derived(allQueue.filter((r) => barTone(r) === 'bad').length);
	const done = $derived(historyTotals(rows.filter(inHistory)));
	const spark = $derived(sparkPath(samples, 64, 18));

	function toggle(id: number) {
		openId = openId === id ? 0 : id;
	}
	function xIn(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.base, easing: easeOut });
	}
	function xOut(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
	// Column widths, the same for every row of both sections; each person's own,
	// kept in this browser. Every column (the Title too) has its own width, as in
	// a desktop table: dragging a column's right edge changes only that column and
	// moves the ones after it. The columns need not fill the table: room left over
	// sits before the bin; more than fits scrolls sideways inside the table box.
	type Col = 'title' | 'bar' | 'st' | 'size' | 'time';
	const COLS: Col[] = ['title', 'bar', 'st', 'size', 'time'];
	const DEF: Record<Col, number> = { title: 420, bar: 140, st: 120, size: 150, time: 64 };
	const MAX: Record<Col, number> = { title: 3000, bar: 1200, st: 1200, size: 1200, time: 800 };
	/** Fallback minimums (the label plus padding) when no header is on the page to measure. */
	const MINF: Record<Col, number> = { title: 44, bar: 70, st: 56, size: 42, time: 46 };
	const KEY = 'holyarrs.downloads.columns';
	const DATA: Record<Col, string> = { title: 'title', bar: 'bar', st: 'status', size: 'size', time: 'time' };
	/** FIXED is the toggle, poster and bin columns; GAPS the gaps between nine columns. */
	const FIXED = 24 + 32 + 32;
	const GAPS = 8;
	let grid = $state<HTMLElement>();
	let gap = $state(10);
	const textW = (n: Node) => {
		const r = document.createRange();
		r.selectNodeContents(n);
		return r.getBoundingClientRect().width;
	};
	/** minOf is a column's header label plus padding: any column shrinks to its own name. */
	function minOf(k: Col): number {
		const l = grid?.querySelector<HTMLElement>(`.chead [data-col="${DATA[k]}"] .lbl`);
		const w = l ? textW(l) : 0;
		return w > 0 ? Math.ceil(w) + 14 : MINF[k];
	}
	const clamp = (k: Col, v: number) => Math.round(Math.max(minOf(k), Math.min(MAX[k], v)));
	/** The saved widths, or null: none saved, so the columns fit their content. */
	function stored(): Record<Col, number> | null {
		try {
			const v = JSON.parse(localStorage.getItem(KEY) ?? 'null');
			if (!v || typeof v !== 'object' || typeof v.title !== 'number') return null;
			return Object.fromEntries(COLS.map((k) => [k, typeof v[k] === 'number' ? Math.max(MINF[k], Math.min(MAX[k], Math.round(v[k]))) : DEF[k]])) as Record<Col, number>;
		} catch {
			return null;
		}
	}
	const saved = typeof localStorage === 'undefined' ? null : stored();
	let widths = $state<Record<Col, number>>(saved ?? { ...DEF });
	// Auto-fit runs once after the rows render when nothing is saved (and again after Reset columns).
	let fitted = $state(!!saved);
	/** inner is the full width of the columns; the table box is at least this wide. */
	const inner = $derived(COLS.reduce((n, k) => n + widths[k], FIXED) + GAPS * gap);
	function save() {
		try {
			localStorage.setItem(KEY, JSON.stringify(widths));
		} catch {
			/* private window: the widths last for this visit */
		}
	}
	/** box is the visible width of the table box. */
	const box = () => grid?.querySelector<HTMLElement>('.tw')?.clientWidth ?? 1200;
	/** fitOf is the widest cell of a column (the header label too) plus padding. */
	function fitOf(k: Col): number {
		if (!grid) return DEF[k];
		if (k === 'bar') return clamp(k, DEF.bar); // a bar has no content width of its own
		let w = 0;
		for (const el of grid.querySelectorAll<HTMLElement>(`.dline [data-col="${DATA[k]}"]`)) {
			if (el.classList.contains('hs')) continue; // History's status spans two columns
			if (k === 'title' && !el.classList.contains('ch')) {
				for (const t of el.querySelectorAll('.tt, .rel')) w = Math.max(w, textW(t));
				continue;
			}
			const tx = el.classList.contains('ch') ? el.querySelector('.lbl') : el.querySelector('.tx');
			w = Math.max(w, tx ? textW(tx) + (el.classList.contains('st') ? 14 : 0) : el.scrollWidth);
		}
		return clamp(k, Math.ceil(w) + 14);
	}
	function fitAll() {
		const next = { ...widths };
		for (const k of COLS) if (k !== 'title') next[k] = fitOf(k);
		// The Title starts with the room the others leave.
		const rest = box() - FIXED - GAPS * gap - (next.bar + next.st + next.size + next.time);
		next.title = clamp('title', Math.max(rest, Math.min(fitOf('title'), 320)));
		widths = next;
		fitted = true;
	}
	$effect(() => {
		if (fitted || !grid || rows.length === 0) return;
		void tick().then(fitAll);
	});
	$effect(() => {
		if (!grid) return;
		gap = parseFloat(getComputedStyle(grid).getPropertyValue('--gap')) || 10;
	});
	// Queue and History scroll sideways together, so their columns stay lined up.
	function sync(e: Event) {
		const x = (e.currentTarget as HTMLElement).scrollLeft;
		for (const t of grid?.querySelectorAll<HTMLElement>('.tw') ?? []) if (t.scrollLeft !== x) t.scrollLeft = x;
	}
	// A grip sits on every column's right edge. Dragging it changes that column
	// only; the columns after it move along. It stops at the column's own name.
	function resize(k: Col, w0: number, d: number) {
		widths[k] = clamp(k, w0 + d);
	}
	function grab(e: PointerEvent, k: Col) {
		if (e.button !== 0) return;
		e.preventDefault();
		const x0 = e.clientX;
		const w0 = widths[k];
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		el.classList.add('drag');
		const move = (m: PointerEvent) => resize(k, w0, m.clientX - x0);
		const up = () => {
			el.removeEventListener('pointermove', move);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			el.classList.remove('drag');
			save();
		};
		el.addEventListener('pointermove', move);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function nudge(e: KeyboardEvent, k: Col) {
		const d = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
		if (!d) return;
		e.preventDefault();
		resize(k, widths[k], d * (e.shiftKey ? 40 : 10));
		save();
	}
	// Double-click auto-fits that column, as in a spreadsheet.
	function fitOne(k: Col) {
		widths[k] = fitOf(k);
		save();
	}
	function resetAll() {
		try {
			localStorage.removeItem(KEY);
		} catch {
			/* nothing saved */
		}
		fitAll();
	}
	const HEADS: { key: string; label: string; size: Col; num?: boolean }[] = [
		{ key: 'title', label: 'Title', size: 'title' },
		{ key: 'bar', label: 'Progress', size: 'bar' },
		{ key: 'status', label: 'Status', size: 'st' },
		{ key: 'size', label: 'Size', size: 'size', num: true },
		{ key: 'time', label: 'Time', size: 'time', num: true }
	];
	const sub2 = (r: DownloadRow) => (manyWords(r.kind) && r.covers ? r.covers : r.name);
</script>

{#snippet head(r: DownloadRow)}
	<button type="button" class="tg" class:on={openId === r.id} aria-expanded={openId === r.id} aria-controls="dlx-{r.id}" aria-label="{openId === r.id ? 'Hide' : 'Show'} details for {who(r)}" onclick={() => toggle(r.id)} data-testid="dl-toggle">
		<Icon name="chevronDown" size={14} stroke={2.2} />
	</button>
	<span class="post" class:square={r.kind === 'artist'}><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
	<span class="nm" data-col="title">
		<span class="tt" title={rowName(r)}>{rowName(r)}{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
		<span class="rel" title={r.name}>{sub2(r)}</span>
	</span>
{/snippet}

{#snippet bin(r: DownloadRow)}
	<span class="del" data-col="bin">
		<button type="button" class="bin" aria-label="Remove {who(r)}" title={r.can_act ? 'Remove' : 'HolyArrs cannot remove this one'} disabled={!r.can_act || busy === r.id} onclick={() => onremove(r)} data-testid="dl-remove"><Icon name="trash" size={16} /></button>
	</span>
{/snippet}

{#snippet panel(r: DownloadRow)}
	{@const ds = decisionsOf(r)}
	<div class="x" id="dlx-{r.id}" data-testid="dl-panel" in:xIn out:xOut>
		<dl class="kv">
			{#each facts(r, importOf.get(r.id)) as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}
		</dl>
		{#if ds.length}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
		{#if r.can_act && !inHistory(r)}
			<p class="xa"><button type="button" class="lk" disabled={busy === r.id} onclick={() => onfail(r)}>Mark failed</button></p>
		{/if}
	</div>
{/snippet}

<div class="dlgrid" bind:this={grid} style="--c-title: {widths.title}px; --c-bar: {widths.bar}px; --c-st: {widths.st}px; --c-size: {widths.size}px; --c-time: {widths.time}px; --c-in: {inner}px">
<div class="strip" data-testid="dl-strip">
	<span class="sp"><span class="k">Speed</span><b data-testid="dl-speed">{bytes(sum.speed)}/s</b>
		<svg class="spark" viewBox="0 0 64 18" width="64" height="18" aria-hidden="true">{#if spark}<path d={spark} />{/if}</svg>
	</span>
	<span><span class="k">Time left</span><b>{sum.eta ? duration(sum.eta) : '–'}</b></span>
	<span><span class="k">Left</span><b>{bytes(sum.left)}</b></span>
	<span><span class="k">In queue</span><b>{allQueue.length}</b></span>
</div>

<div class="chips" role="group" aria-label="Show" data-testid="table-filter">
	{#each tally as c (c.key)}
		<span class="cw" class:zero={c.count === 0} data-testid="status-chip" data-key={c.key} data-count={c.count} title={c.hint}>
			<Chip label={c.label} count={c.count} note={c.seeding ? `${c.seeding} seeding` : undefined} selected={on === c.key} onclick={() => choose(c.key)} />
		</span>
	{/each}
</div>
{#if subs.length}
	<div class="chips subs" role="group" aria-label="{onTab?.label ?? ''}: show" data-testid="table-subfilter" in:xIn out:xOut>
		{#each subs as s (s.key)}
			<span class="cw sm" class:zero={s.count === 0} data-testid="status-subchip" data-key={s.key} data-count={s.count} title={s.hint}>
				<Chip label={s.label} count={s.count} selected={subOn === s.key} onclick={() => chooseSub(s.key)} />
			</span>
		{/each}
	</div>
{/if}

{#if on === ALL || queue.length}
	<section class="sec" aria-labelledby="dl-queue">
		<div class="shr"><h2 id="dl-queue" class="sh">Queue <span class="n">{queue.length}</span></h2>
			<span class="qs">{#if problems}<span class="qerr" title="Problems"><Icon name="alert" size={13} />{problems}</span>{/if}<span>{bytes(sum.left)} left</span></span></div>
		{#if queue.length === 0}
			<p class="none">Nothing in the queue.</p>
		{:else}
			<div class="tw" onscroll={sync} data-testid="dl-tablebox"><div class="tin">
				<div class="dline chead" data-testid="dl-colhead">
				<span></span><span></span>
				{#each HEADS as h (h.key)}
					<span class="ch" class:end={h.num} data-col={h.key}><span class="lbl" title={h.label}>{h.label}</span><!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions --><span class="grip" role="separator" aria-orientation="vertical" aria-label="Resize {h.label}" aria-valuenow={widths[h.size]} tabindex="0" title="Drag to resize, double-click to fit" data-testid="dl-grip" data-grip={h.size} onpointerdown={(e) => grab(e, h.size)} ondblclick={() => fitOne(h.size)} onkeydown={(e) => nudge(e, h.size)}></span></span>
				{/each}
				<span></span>
				<span class="cm"><Menu label="Columns" icon="more" width={180} items={[{ label: 'Reset columns', icon: 'refresh', onselect: resetAll }]} /></span>
			</div>
			<ul class="dlrows" data-testid="dl-queue">
				{#each queue as r (r.id)}
					{@const pct = percent(r.progress)}
					{@const t = barTone(r)}
					{@const more = statusDetail(r, importOf.get(r.id))}
					<li class="drow" class:open={openId === r.id} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
						<div class="dline">
							{@render head(r)}
							<span class="bar {t}" data-col="bar" role="progressbar" aria-label="Progress of {who(r)}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.floor(pct)} data-testid="dl-bar"><i style="transform: translateX({pct - 100}%)"></i><span class="bl">{Math.floor(pct)}%</span></span>
							<span class="st {t}" data-col="status" title={more ? `${statusWord(r)} · ${more}` : statusWord(r)}><span class="dot" aria-hidden="true"></span><span class="tx">{statusWord(r)}</span></span>
							<span class="num" data-col="size">{sizeLine(r) || '–'}</span>
							<span class="num" data-col="time">{r.active && r.eta ? duration(r.eta) : '–'}</span>
							{@render bin(r)}
						</div>
						{#if openId === r.id}{@render panel(r)}{/if}
					</li>
				{/each}
			</ul>
			</div></div>
		{/if}
	</section>
{/if}

{#if on === ALL || history.length}
	<section class="sec" aria-labelledby="dl-history">
		<h2 id="dl-history" class="sh">History <span class="n">{history.length}</span></h2>
		{#if found.length === 0}
			<p class="none">{q.trim() ? 'Nothing matches.' : 'Nothing finished yet.'}</p>
		{:else}
			<div class="tw" onscroll={sync}><div class="tin">
				<ul class="dlrows" data-testid="dl-history">
				{#each found as r (r.id)}
					{@const s = historyStatus(r, importOf.get(r.id))}
					<li class="drow" class:open={openId === r.id} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
						<div class="dline">
							{@render head(r)}
							<span class="st hs" class:bad={s.bad} data-col="bar" title={s.text}><span class="ic" aria-hidden="true">{s.bad ? '!' : '✓'}</span><span class="tx">{s.text}</span></span>
							<span class="num" data-col="size">{r.size ? bytes(r.size) : '–'}</span>
							<span class="num" data-col="time" title={r.finished_at || r.updated_at}>{ago(r.finished_at || r.updated_at)}</span>
							{@render bin(r)}
						</div>
						{#if openId === r.id}{@render panel(r)}{/if}
					</li>
				{/each}
			</ul>
			</div></div>
		{/if}
		<div class="foot">
			<label class="find"><Icon name="search" size={14} /><input type="search" placeholder="Search History" aria-label="Search History" bind:value={q} /></label>
			<span class="tot" data-testid="dl-totals">{bytes(done.today)} today · {bytes(done.month)} this month</span>
		</div>
	</section>
{/if}

{#if queue.length === 0 && history.length === 0 && on !== ALL}
	<p class="none" data-testid="dl-empty">{(subOn ? onTab?.subs.find((x) => x.key === subOn)?.empty : onTab?.empty) ?? 'Nothing here.'} <button type="button" class="lk" onclick={() => choose(ALL)}>Show all {rows.length}</button></p>
{/if}

</div>

<style>
	.strip { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 22px; padding: 8px 0; margin-bottom: 10px; border-bottom: 1px solid var(--line); font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.strip > span { display: inline-flex; align-items: center; gap: 6px; }
	.k { color: var(--text-muted); font: var(--t-footnote); }
	.strip b { font-weight: 600; }
	.spark { flex: none; }
	.spark path { fill: none; stroke: var(--ok); stroke-width: 1.5; stroke-linejoin: round; stroke-linecap: round; }

	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-bottom: 12px; }
	.subs { gap: 6px; margin-top: -4px; }
	.cw { display: inline-flex; transition: opacity var(--dur-fast) var(--ease-out); }
	.cw.zero { opacity: 0.5; }
	.cw.zero:hover, .cw.zero:focus-within { opacity: 0.8; }
	.cw.sm :global(.main) { min-height: 28px; padding: 0 11px; font-size: 13px; }

	.sec { margin-bottom: 22px; }
	.sh { display: flex; align-items: baseline; gap: 8px; margin: 0 0 6px; font: var(--t-headline); }
	.shr { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
	.qs { display: inline-flex; align-items: center; gap: 12px; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.qerr { display: inline-flex; align-items: center; gap: 4px; color: var(--fail); font-weight: 600; }
	.n { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.none { padding: 10px 0; color: var(--text-muted); font: var(--t-callout); }

	/* One column grid for every row of both sections. */
	.dlgrid { --gap: 10px; min-width: 0; }
	.tw { overflow-x: auto; overflow-y: hidden; max-width: 100%; }
	.tin { min-width: var(--c-in); }
	.dlrows { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--line); }
	.drow { display: block; border-bottom: 1px solid var(--line); }
	.drow.open { background: var(--surface-2); }
	.dline { display: grid; grid-template-columns: 24px 32px var(--c-title) var(--c-bar) var(--c-st) var(--c-size) var(--c-time) minmax(0, 1fr) 32px; align-items: center; column-gap: var(--gap); height: 48px; }
	.dline:hover { background: var(--surface-2); }

	.tg { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.tg :global(svg) { transform: rotate(-90deg); transition: transform var(--dur-fast) var(--ease-out); }
	.tg.on :global(svg) { transform: none; }
	.tg:hover { color: var(--text); background: var(--surface-3); }
	.tg:focus-visible, .bin:focus-visible, .lk:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.post { width: 28px; height: 42px; justify-self: center; overflow: hidden; border-radius: 3px; --poster-ratio: 2 / 3; }
	.post.square { height: 28px; --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 3px; }

	.nm { display: flex; flex-direction: column; min-width: 0; line-height: 1.25; }
	.tt, .rel { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tt { font: var(--t-callout); font-weight: 500; }
	.rel { color: var(--text-muted); font: var(--t-caption); }
	.ver { margin-left: 6px; padding: 0 5px; border-radius: 4px; background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); }

	.bar { position: relative; display: flex; align-items: center; height: 14px; white-space: nowrap; padding: 0 6px; border-radius: 4px; overflow: hidden; background: var(--surface-3); color: var(--text); font-size: 10px; line-height: 14px; font-weight: 600; font-variant-numeric: tabular-nums; }
	.bar i { position: absolute; inset: 0; background: var(--ok); opacity: 0.85; transition: transform var(--dur-slow) var(--ease-out); }
	.bar.queued i { background: var(--info); }
	.bar.held i { background: var(--wait); }
	.bar.bad i { background: var(--fail); }
	.bl, .br { position: relative; overflow: hidden; text-overflow: ellipsis; }
	.bw { font-weight: 500; }
	.hcell { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; height: 24px; padding: 0 8px; }
	.hsz { flex: none; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }
	.bl { font-weight: 600; text-shadow: 0 0 3px var(--surface-1); }

	.st { display: flex; align-items: center; gap: 6px; min-width: 0; font: var(--t-footnote); }
	.st .tx { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.st.queued .dot { background: var(--info); }
	.st.held .dot { background: var(--wait); }
	.st.bad .dot { background: var(--fail); }
	.hs { grid-column: span 2; }
	.st.bad, .hs.bad { color: var(--fail); }
	.ic { display: grid; place-items: center; flex: none; width: 16px; height: 16px; border-radius: 50%; background: var(--ok); color: var(--surface-1); font-size: 10px; font-weight: 700; }
	.hs.bad .ic { background: var(--fail); }

	.num { text-align: right; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

	.chead { height: 28px; border-bottom: 1px solid var(--line); color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.chead + .dlrows { border-top: 0; }
	.chead:hover { background: none; }
	.ch { position: relative; align-self: stretch; display: flex; align-items: center; min-width: 0; }
	.ch.end { justify-content: flex-end; }
	.lbl { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.grip { position: absolute; top: 4px; bottom: 4px; right: calc(-1 * var(--gap) / 2 - 4px); width: 8px; cursor: col-resize; touch-action: none; border-radius: 3px; z-index: 3; }
	.grip::after { content: ''; position: absolute; left: 3px; top: 0; bottom: 0; width: 1px; background: var(--line-strong); transition: background var(--dur-fast) var(--ease-out); }
	.grip:hover::after, .grip:focus-visible::after, .grip:global(.drag)::after { background: var(--accent); width: 2px; }
	.grip:focus-visible { outline: none; }
	.cm { display: flex; justify-content: flex-end; }
	.del { display: flex; justify-content: flex-end; }
	.bin { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; transition: color var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.bin:hover:not(:disabled) { color: var(--fail); background: var(--surface-3); }
	.bin:disabled { opacity: 0.35; cursor: not-allowed; }

	/* The panel starts at the Title column: ▾ + poster + two gaps. */
	.x { padding: 4px 12px 12px calc(24px + 28px + 2 * var(--gap)); }
	.kv { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 3px 16px; margin: 0; font: var(--t-footnote); }
	.kv dt { color: var(--text-muted); }
	.kv dd { margin: 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
	.dp { margin-top: 10px; }
	.xa { margin: 8px 0 0; }
	.lk { min-height: 28px; padding: 0 4px; border: 0; border-radius: 6px; background: none; color: var(--accent-text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }

	.foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; margin-top: 10px; }
	.find { display: inline-flex; align-items: center; gap: 6px; height: 32px; padding: 0 10px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text-muted); }
	.find:focus-within { box-shadow: var(--focus-ring); }
	.find input { width: 180px; border: 0; background: none; color: var(--text); font: var(--t-footnote); outline: none; }
	.tot { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }

	/* Phones: name on top, the bar under it, then status, size and time. */
	@media (max-width: 767px) {
		.dlgrid { --gap: 8px; }
		.chead { display: none; }
		.tin { min-width: 0; }
		.dline { grid-template-columns: 24px 28px minmax(0, 1fr) 88px 52px 32px; grid-template-areas: 'tg post nm nm nm del' '. post bar st time del'; row-gap: 4px; height: auto; min-height: 62px; padding: 6px 0; }
		.tg { grid-area: tg; }
		.post { grid-area: post; align-self: start; }
		.nm { grid-area: nm; }
		.bar { grid-area: bar; }
		.hs { grid-area: bar / bar / bar / st; grid-column: 3 / 5; }
		.st:not(.hs) { grid-area: st; }
		[data-col='size'] { display: none; }
		[data-col='time'] { grid-area: time; }
		.del { grid-area: del; }
		.x { padding: 4px 8px 12px calc(24px + 28px + 2 * var(--gap)); }
		.find { flex: 1; }
		.find input { width: 100%; }
		.strip { gap: 4px 14px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.bar i, .tg :global(svg), .bin, .cw { transition: none; }
	}
</style>
