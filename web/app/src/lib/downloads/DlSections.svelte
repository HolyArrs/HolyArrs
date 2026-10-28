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
	// kept in this browser. The Title takes what is left (at least MIN_TITLE).
	type Col = 'bar' | 'st' | 'size' | 'time';
	const DEF: Record<Col, number> = { bar: 140, st: 120, size: 150, time: 64 };
	const LIM: Record<Col, [number, number]> = { bar: [80, 420], st: [80, 240], size: [90, 240], time: [48, 120] };
	const KEY = 'holyarrs.downloads.columns';
	const MIN_TITLE = 200;
	const COLS: Col[] = ['bar', 'st', 'size', 'time'];
	const DATA: Record<Col, string> = { bar: 'bar', st: 'status', size: 'size', time: 'time' };
	const clamp = (k: Col, v: number) => Math.round(Math.max(LIM[k][0], Math.min(LIM[k][1], v)));
	/** The saved widths, or null: none saved, so the columns fit their content. */
	function stored(): Record<Col, number> | null {
		try {
			const v = JSON.parse(localStorage.getItem(KEY) ?? 'null');
			if (!v || typeof v !== 'object') return null;
			return Object.fromEntries(COLS.map((k) => [k, typeof v[k] === 'number' ? clamp(k, v[k]) : DEF[k]])) as Record<Col, number>;
		} catch {
			return null;
		}
	}
	const saved = typeof localStorage === 'undefined' ? null : stored();
	let widths = $state<Record<Col, number>>(saved ?? { ...DEF });
	// Auto-fit runs once after the rows render when nothing is saved (and again after Reset columns).
	let fitted = $state(!!saved);
	let grid = $state<HTMLElement>();
	function save() {
		try {
			localStorage.setItem(KEY, JSON.stringify(widths));
		} catch {
			/* private window: the widths last for this visit */
		}
	}
	/** room is the most the four columns may add up to, leaving the Title MIN_TITLE. */
	const room = () => (grid?.querySelector('.dlrows, .chead')?.clientWidth ?? 1200) - (24 + 32 + 32 + 7 * 10) - MIN_TITLE;
	/** fitOf is the widest cell of a column (the header label too) plus padding. */
	function fitOf(k: Col): number {
		if (!grid) return DEF[k];
		if (k === 'bar') return DEF.bar; // a bar has no content width of its own
		let w = 0;
		for (const el of grid.querySelectorAll<HTMLElement>(`.dline [data-col="${DATA[k]}"]`)) {
			if (el.classList.contains('hs')) continue; // History's status spans two columns
			const tx = el.classList.contains('ch') ? el.firstChild : el.querySelector('.tx');
			let cw = el.scrollWidth;
			if (tx) {
				const r = document.createRange();
				r.selectNodeContents(tx);
				cw = r.getBoundingClientRect().width + (el.classList.contains('st') ? 14 : 0);
			}
			w = Math.max(w, cw);
		}
		return clamp(k, Math.ceil(w) + 12);
	}
	function fitAll() {
		const next = { ...widths };
		for (const k of COLS) next[k] = fitOf(k);
		// Too wide for the Title's minimum: the Size column (the widest) gives way.
		const over = COLS.reduce((n, k) => n + next[k], 0) - room();
		if (over > 0) next.size = clamp('size', next.size - over);
		widths = next;
		fitted = true;
	}
	$effect(() => {
		if (fitted || !grid || rows.length === 0) return;
		void tick().then(fitAll);
	});
	// A grip sits on the right edge of its column. Dragging it moves that edge
	// with the pointer: the column grows by what its right-hand neighbour
	// gives up, so nothing else moves. Time, the last one, grows into the
	// Title instead (the bin stays at the far right).
	function resize(k: Col, w0: Record<Col, number>, d: number) {
		const right = COLS.slice(COLS.indexOf(k) + 1);
		const w = { ...w0 };
		if (d >= 0) {
			// Growing: the columns to the right give up space in turn (down to
			// their minimum), then the Title (down to MIN_TITLE).
			let need = Math.min(Math.round(d), LIM[k][1] - w0[k]);
			let got = 0;
			for (const x of right) {
				const take = Math.min(need - got, w[x] - LIM[x][0]);
				w[x] -= take;
				got += take;
			}
			const used = COLS.reduce((n, x) => n + w[x], 0);
			got += Math.max(0, Math.min(need - got, room() - used));
			w[k] = w0[k] + got;
		} else {
			// Shrinking: the right-hand neighbour takes the space, so the edges
			// to its right stay put; the last column gives it to the Title.
			const give = Math.min(Math.round(-d), w0[k] - LIM[k][0]);
			w[k] = w0[k] - give;
			const next = right[0];
			if (next) w[next] = clamp(next, w0[next] + give);
		}
		widths = w;
	}
	function grab(e: PointerEvent, k: Col) {
		if (e.button !== 0) return;
		e.preventDefault();
		const x0 = e.clientX;
		const w0 = { ...widths };
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
		resize(k, { ...widths }, d * (e.shiftKey ? 40 : 10));
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
	// Title is the flexible column and has no grip; it takes what is left.
	const HEADS: { key: string; label: string; size?: Col; num?: boolean }[] = [
		{ key: 'title', label: 'Title' },
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

<div class="dlgrid" bind:this={grid} style="--c-bar: {widths.bar}px; --c-st: {widths.st}px; --c-size: {widths.size}px; --c-time: {widths.time}px">
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
			<div class="dline chead" data-testid="dl-colhead">
				<span></span><span></span>
				{#each HEADS as h (h.key)}
					<span class="ch" class:end={h.num} data-col={h.key}>{h.label}{#if h.size}{@const k = h.size}<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions --><span class="grip" role="separator" aria-orientation="vertical" aria-label="Resize {h.label}" aria-valuenow={widths[k]} aria-valuemin={LIM[k][0]} aria-valuemax={LIM[k][1]} tabindex="0" title="Drag to resize, double-click to fit" data-testid="dl-grip" data-grip={k} onpointerdown={(e) => grab(e, k)} ondblclick={() => fitOne(k)} onkeydown={(e) => nudge(e, k)}></span>{/if}</span>
				{/each}
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
		{/if}
	</section>
{/if}

{#if on === ALL || history.length}
	<section class="sec" aria-labelledby="dl-history">
		<h2 id="dl-history" class="sh">History <span class="n">{history.length}</span></h2>
		{#if found.length === 0}
			<p class="none">{q.trim() ? 'Nothing matches.' : 'Nothing finished yet.'}</p>
		{:else}
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
	.dlgrid { --gap: 10px; }
	.dlrows { container-type: inline-size; list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--line); }
	.drow { display: block; border-bottom: 1px solid var(--line); }
	.drow.open { background: var(--surface-2); }
	.dline { display: grid; grid-template-columns: 24px 32px minmax(0, 1fr) var(--c-bar) var(--c-st) var(--c-size) var(--c-time) 32px; align-items: center; column-gap: var(--gap); height: 48px; }
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
