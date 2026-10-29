<script lang="ts">
	// The Downloads page body, laid out like a Downloader's own web page: a
	// header strip (speed with a small graph, time left), the status tabs
	// (Active · Waiting · In progress · Problems · Done, with sub-tabs; see
	// status-tabs.ts), then one list filtered by them. Every row shares one
	// column grid: ▾ · poster · the picked columns · bin (columns.ts
	// holds which columns exist and the saved layout). ▾ opens a row's details in place; the bin removes it
	// (through the page's Remove dialog).
	import { tick } from 'svelte';
	import { page } from '$app/state';
	import { COL, KEY, MAX_W, MIN_W, OLD_KEY, defaults, dump, load, move, moveVisible, quality, fitTitle, toggle as toggleCol, visible, type ColId, type Layout } from './columns.ts';
	import { replaceState } from '$app/navigation';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Chip from '../ui/Chip.svelte';
	import Menu from '../ui/Menu.svelte';
	import DecisionPanel from './DecisionPanel.svelte';
	import SpeedGraph from './SpeedGraph.svelte';
	import { cardLook } from './cardLook.svelte.ts';
	import { demoDetails, sampleFiles, sampleLog, samplePeers, sampleRatio } from './sampleDetails.ts';
	import { ago, dateTime, duration, percent } from '../format.ts';
	import { size as bytes, protocolOf, KIND_TABS, KIND_WORD, type KindKey } from './size.ts';
	import { kindWords } from '../downloaders.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { manyWords, rowName, type Decision, type DownloadRow } from '../downloaders.ts';
	import type { ImportRecord, TitleView } from '../types.ts';
	import { ALL, counts, pick, showOf, statusWord, subCounts, tabsFor } from './status-tabs.ts';
	import { statusDetail } from './imports.ts';
	import { poster, who } from './shared.ts';
	import { barTone, facts, historyStatus, historyTotals, inHistory, matches, moving, sizeLine, totals } from './sections.ts';

	let {
		rows: everyRow,
		importOf,
		titles,
		busy,
		samples,
		history = [],
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
		/** The server's speed readings since the app started. */
		history?: { down: number; up: number }[];
		/** A torrent Downloader brings the Seeding sub-tab under Done. */
		torrent?: boolean;
		decisionsOf: (r: DownloadRow) => Decision[];
		/** The row whose details are open (0: none). */
		openId?: number;
		onfail: (r: DownloadRow) => void;
		onremove: (r: DownloadRow) => void;
		ondone: () => void;
	} = $props();

	// The Type filter (All, Movies, TV shows, Music) narrows every tab and total.
	let kindOn = $state<KindKey>('');
	const typed = $derived(kindOn ? everyRow.filter((r) => (r.kind ?? 'movie') === kindOn) : everyRow);
	// The search narrows every tab too, so the tab counts follow it.
	let q = $state('');
	let findEl = $state<HTMLInputElement>();
	let findOpen = $state(false);
	const rows = $derived(q.trim() ? typed.filter((r) => matches(r, q)) : typed);
	function slashKey(e: KeyboardEvent) {
		if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return;
		const t = e.target as HTMLElement;
		if (t.closest('input, textarea, select, [contenteditable="true"]')) return;
		e.preventDefault();
		e.stopImmediatePropagation(); // on this page "/" is the Downloads search, not the app search
		findOpen = true;
		tick().then(() => findEl?.focus());
	}
	$effect(() => {
		window.addEventListener('keydown', slashKey, true);
		return () => window.removeEventListener('keydown', slashKey, true);
	});
	function clearFind() {
		q = '';
		findEl?.focus();
	}

	// The picked tab (and sub-tab) lives in the address: &show=tab or tab.sub.
	// ?card=a|b|c picks the expanded row's layout (c, the big poster, is the default).
	const card = $derived(cardLook.value);
	const TABS = ['Info', 'Files', 'Peers', 'Log'] as const;
	let xtabOn = $state<Record<number, (typeof TABS)[number]>>({});
	const tabOf = (id: number) => xtabOn[id] ?? 'Info';
	let filesOpen = $state(new Set<number>());
	function toggleFiles(id: number) {
		const n = new Set(filesOpen);
		if (n.has(id)) n.delete(id);
		else n.add(id);
		filesOpen = n;
	}
	const start = showOf(page.url.searchParams.get('show'));
	let show = $state(start.show);
	let sub = $state(start.sub);
	function remember() {
		try {
			const u = new URL(location.href);
			if (show === ALL) u.searchParams.delete('show');
			else u.searchParams.set('show', sub ? `${show}.${sub}` : show);
			if (u.href !== location.href) replaceState(u, page.state);
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
	const shown = $derived(pick(rows, tabs, on, subOn));
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
	const allQueue = $derived(typed.filter((r) => !inHistory(r)));
	// One list, filtered by the tabs. Done reads newest first and can be searched.
	const isDone = $derived(on === 'done');
	const newest = (a: DownloadRow, b: DownloadRow) => (b.finished_at || b.updated_at).localeCompare(a.finished_at || a.updated_at);
	const list = $derived(isDone ? [...shown].sort(newest) : shown);
	const sum = $derived(totals(allQueue));
	const problems = $derived(allQueue.filter((r) => barTone(r) === 'bad').length);
	const done = $derived(historyTotals(typed.filter(inHistory)));

	function toggle(id: number) {
		openId = openId === id ? 0 : id;
	}
	function xIn(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.base, easing: easeOut });
	}
	// The open row's poster grows from the row's small one into the details
	// (FLIP: start at the small poster's box, end at its own).
	function grow(node: Element, opts: { from: () => Element | null | undefined; dur: number; ease: (t: number) => number }): TransitionConfig {
		const a = opts.from()?.getBoundingClientRect();
		const b = node.getBoundingClientRect();
		if (!a || !b.width || reducedMotion()) return { duration: reducedMotion() ? DUR.fast : 0, css: (t) => `opacity: ${t}` };
		const dx = a.left - b.left, dy = a.top - b.top, sx = a.width / b.width, sy = a.height / b.height;
		return { duration: opts.dur, easing: opts.ease, css: (t, u) => `transform-origin: 0 0; transform: translate(${dx * u}px, ${dy * u}px) scale(${sx + (1 - sx) * t}, ${sy + (1 - sy) * t})` };
	}
	const small = (id: number) => () => grid?.querySelector(`[data-dl-id="${id}"] > .dline .post`);
	function xOut(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
	// Columns: which ones show, their order and their widths are each person's
	// own, kept in this browser (columns.ts). Every row of both sections shares
	// one grid: ▾ · poster · the shown columns in order · spare room · bin.
	// Dragging a column's right edge changes that column only; dragging its name
	// moves it; right-click a name, or ⋯ → Columns, opens the picker.
	let grid = $state<HTMLElement>();
	let gap = $state(10);
	let layout = $state<Layout>(typeof localStorage === 'undefined' ? defaults() : readLayout());
	function readLayout(): Layout {
		try {
			return load(localStorage.getItem(KEY), localStorage.getItem(OLD_KEY));
		} catch {
			return defaults();
		}
	}
	function save() {
		try {
			localStorage.setItem(KEY, dump(layout));
		} catch {
			/* private window: the layout lasts for this visit */
		}
	}
	const vis = $derived(visible(layout));
	/** A phone shows a fixed card (title, bar, status, time) whatever is picked. */
	const PHONE: ColId[] = ['title', 'progress', 'status', 'time'];
	const cells = $derived([...vis.map((id) => ({ id, ph: false })), ...PHONE.filter((id) => !vis.includes(id)).map((id) => ({ id, ph: true }))]);
	const FIXED = 24 + 32 + 32;
	let room = $state(0);
	let titleMin = $state(MIN_W);
	/** wOf is a column's width on screen (an unset Title gives way when room is short). */
	const tW = $derived(vis.includes('title') ? fitTitle(layout, room, FIXED + (vis.length + 3) * gap, titleMin) : 0);
	const wOf = (id: ColId) => (id === 'title' ? tW : layout.widths[id]);
	const tpl = $derived(`24px 32px ${vis.map((id) => `${wOf(id)}px`).join(' ')} minmax(0, 1fr) 32px`);
	/** inner is the full width of the columns; the table box is at least this wide. */
	const inner = $derived(vis.reduce((n, id) => n + wOf(id), FIXED) + (vis.length + 3) * gap);
	const textW = (n: Node) => {
		const r = document.createRange();
		r.selectNodeContents(n);
		return r.getBoundingClientRect().width;
	};
	/** minOf is a column's header label plus padding: any column shrinks to its own name. */
	function minOf(id: ColId): number {
		const l = grid?.querySelector<HTMLElement>(`.chead [data-col="${id}"] .lbl`);
		const w = l ? textW(l) : 0;
		return w > 0 ? Math.ceil(w) + 14 : MIN_W;
	}
	const clamp = (id: ColId, v: number) => Math.round(Math.max(minOf(id), Math.min(MAX_W, v)));
	/** fitOf is the widest cell of a column (the header label too) plus padding. */
	function fitOf(id: ColId): number {
		if (!grid) return COL[id].width;
		if (id === 'progress') return clamp(id, COL.progress.width); // a bar has no content width of its own
		let w = 0;
		for (const el of grid.querySelectorAll<HTMLElement>(`.dline [data-col="${id}"]`)) {
			if (id === 'title' && !el.classList.contains('ch')) {
				for (const t of el.querySelectorAll('.tt, .rel')) w = Math.max(w, textW(t));
				continue;
			}
			const tx = el.classList.contains('ch') ? el.querySelector('.lbl') : (el.querySelector('.tx') ?? el);
			w = Math.max(w, tx ? textW(tx) + (el.classList.contains('st') ? 14 : 0) : 0);
		}
		return clamp(id, Math.ceil(w) + 14);
	}
	$effect(() => {
		if (!grid) return;
		gap = parseFloat(getComputedStyle(grid).getPropertyValue('--gap')) || 10;
		const g = grid;
		const ro = new ResizeObserver(() => {
			room = g.clientWidth;
			titleMin = minOf('title');
		});
		ro.observe(g);
		return () => ro.disconnect();
	});
	// Every table box scrolls sideways together, so the columns stay lined up.
	function sync(e: Event) {
		const x = (e.currentTarget as HTMLElement).scrollLeft;
		for (const t of grid?.querySelectorAll<HTMLElement>('.tw') ?? []) if (t.scrollLeft !== x) t.scrollLeft = x;
	}
	function resize(id: ColId, w0: number, d: number) {
		if (id === 'title') layout.titleSet = true;
		layout.widths[id] = clamp(id, w0 + d);
	}
	function grab(e: PointerEvent, id: ColId) {
		if (e.button !== 0) return;
		e.preventDefault();
		e.stopPropagation();
		const x0 = e.clientX;
		const w0 = wOf(id);
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		el.classList.add('drag');
		const mv = (m: PointerEvent) => resize(id, w0, m.clientX - x0);
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			el.classList.remove('drag');
			save();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function nudge(e: KeyboardEvent, id: ColId) {
		const d = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
		if (!d) return;
		e.preventDefault();
		resize(id, wOf(id), d * (e.shiftKey ? 40 : 10));
		save();
	}
	// Double-click auto-fits that column, as in a spreadsheet.
	function fitOne(id: ColId) {
		if (id === 'title') layout.titleSet = true;
		layout.widths[id] = fitOf(id);
		save();
	}
	function resetAll() {
		try {
			localStorage.removeItem(KEY);
			localStorage.removeItem(OLD_KEY);
		} catch {
			/* nothing saved */
		}
		layout = defaults();
	}

	// Dragging a column's name left or right moves it: a ghost follows the
	// pointer and a line shows where it will land.
	let ghost = $state<{ id: ColId; x: number; y: number } | null>(null);
	let lineX = $state<number | null>(null);
	let slot = 0;
	function slotAt(head: HTMLElement, x: number): { slot: number; line: number } {
		const box = head.getBoundingClientRect();
		const hs = [...head.querySelectorAll<HTMLElement>('.ch')].map((h) => h.getBoundingClientRect());
		let i = hs.findIndex((r) => x < r.left + r.width / 2);
		if (i < 0) i = hs.length;
		const at = i < hs.length ? hs[i].left - gap / 2 : hs[hs.length - 1].right + gap / 2;
		return { slot: i, line: at - box.left };
	}
	function headDown(e: PointerEvent, id: ColId) {
		if (e.button !== 0) return;
		const el = e.currentTarget as HTMLElement;
		const head = el.closest<HTMLElement>('.chead');
		if (!head) return;
		const x0 = e.clientX;
		let on = false;
		el.setPointerCapture(e.pointerId);
		const mv = (m: PointerEvent) => {
			if (!on && Math.abs(m.clientX - x0) < 5) return;
			on = true;
			ghost = { id, x: m.clientX, y: m.clientY };
			const s = slotAt(head, m.clientX);
			slot = s.slot;
			lineX = s.line;
		};
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', stop);
			if (on) {
				const from = vis.indexOf(id);
				const to = slot > from ? slot - 1 : slot;
				if (to !== from) {
					layout.order = moveVisible(layout, id, to);
					save();
				}
			}
			ghost = null;
			lineX = null;
		};
		const stop = () => {
			on = false;
			up();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', stop);
	}

	// The column picker: every column with a tick and a handle to reorder.
	let pop = $state<{ x: number; y: number } | null>(null);
	let pickEl = $state<HTMLElement>();
	let held = $state<ColId | null>(null);
	let heldFrom: ColId[] = [];
	let said = $state('');
	function openPicker(x: number, y: number) {
		const w = 260;
		pop = { x: Math.max(8, Math.min(x, innerWidth - w - 8)), y: Math.max(8, Math.min(y, innerHeight - 440)) };
		held = null;
		tick().then(() => pickEl?.querySelector<HTMLElement>('input, button')?.focus());
	}
	function closePicker() {
		if (held) layout.order = heldFrom;
		held = null;
		pop = null;
		save();
	}
	function headMenu(e: MouseEvent) {
		e.preventDefault();
		openPicker(e.clientX, e.clientY);
	}
	function fromMenu() {
		const b = grid?.querySelector<HTMLElement>('.cm button');
		const r = b?.getBoundingClientRect();
		openPicker(r ? r.right - 260 : innerWidth / 2, r ? r.bottom + 6 : 80);
	}
	function outside(e: PointerEvent) {
		if (pop && pickEl && !pickEl.contains(e.target as Node)) closePicker();
	}
	function flip(id: ColId) {
		layout.hidden = toggleCol(layout, id);
		save();
	}
	async function refocus(id: ColId) {
		await tick();
		pickEl?.querySelector<HTMLElement>(`[data-handle="${id}"]`)?.focus();
	}
	function handleKey(e: KeyboardEvent, id: ColId) {
		const i = layout.order.indexOf(id);
		if (e.key === ' ' || e.key === 'Enter') {
			e.preventDefault();
			if (held === id) {
				held = null;
				said = `${COL[id].label} dropped at position ${i + 1}.`;
				save();
			} else if (e.key === ' ') {
				heldFrom = [...layout.order];
				held = id;
				said = `${COL[id].label} picked up. Use the arrow keys to move it, Enter to drop.`;
			}
		} else if (held === id && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
			e.preventDefault();
			const to = i + (e.key === 'ArrowUp' ? -1 : 1);
			if (to < 0 || to >= layout.order.length) return;
			layout.order = move(layout.order, id, to);
			said = `${COL[id].label}: position ${to + 1} of ${layout.order.length}.`;
			refocus(id);
		} else if (e.key === 'Escape' && held) {
			e.preventDefault();
			e.stopPropagation();
			layout.order = heldFrom;
			held = null;
			said = 'Move cancelled.';
			refocus(id);
		}
	}
	function handleDown(e: PointerEvent, id: ColId) {
		if (e.button !== 0) return;
		e.preventDefault();
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		held = id;
		const mv = (m: PointerEvent) => {
			const lis = [...(pickEl?.querySelectorAll<HTMLElement>('li[data-id]') ?? [])];
			let to = lis.findIndex((li) => m.clientY < li.getBoundingClientRect().top + li.offsetHeight / 2);
			if (to < 0) to = lis.length - 1;
			const from = layout.order.indexOf(id);
			if (to > from) to -= 1;
			if (to !== from) layout.order = move(layout.order, id, to);
		};
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			held = null;
			save();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function pickerKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && !held) {
			e.preventDefault();
			closePicker();
			grid?.querySelector<HTMLElement>('.cm button')?.focus();
		}
	}
	// A click anywhere on a row opens or closes its details, as the ▾ does;
	// not on the bin, a link, a control, or while selecting text.
	function rowClick(e: MouseEvent, id: number) {
		const t = e.target as HTMLElement;
		if (t.closest('button, a, input, label, .grip, [data-col="bin"]')) return;
		if ((getSelection()?.toString() ?? '').length > 0) return;
		toggle(id);
	}
	function rowKey(e: KeyboardEvent, id: number) {
		if (e.target !== e.currentTarget || (e.key !== 'Enter' && e.key !== ' ')) return;
		e.preventDefault();
		toggle(id);
	}
	const dash = '—';
	/** The Downloader's own configured name (as in the details); its kind goes in the tooltip. */
	const dlName = (r: DownloadRow) => r.downloader || (r.downloader_kind && kindWords[r.downloader_kind as keyof typeof kindWords]) || dash;
	const dlKind = (r: DownloadRow) => (r.downloader_kind && kindWords[r.downloader_kind as keyof typeof kindWords]) || '';
	const sub2 = (r: DownloadRow) => (manyWords(r.kind) && r.covers ? r.covers : r.name);
</script>

{#snippet lead(r: DownloadRow)}
	<button type="button" class="tg" class:on={openId === r.id} aria-expanded={openId === r.id} aria-controls="dlx-{r.id}" aria-label="{openId === r.id ? 'Hide' : 'Show'} details for {who(r)}" onclick={() => toggle(r.id)} data-testid="dl-toggle">
		<Icon name="chevronDown" size={14} stroke={2.2} />
	</button>
	<span class="post" class:square={r.kind === 'artist'}><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
{/snippet}

{#snippet proto(r: DownloadRow)}
	{@const p = protocolOf(r)}
	{#if p}<span class="pm" data-testid="dl-protocol" data-protocol={p} title="{p === 'torrent' ? 'Torrent' : 'Usenet'} · {dlName(r)}{dlKind(r) && dlKind(r) !== dlName(r) ? ` (${dlKind(r)})` : ''}" aria-label="{p === 'torrent' ? 'Torrent' : 'Usenet'} download">{p === 'torrent' ? 'T' : 'U'}</span>{/if}
{/snippet}

{#snippet cell(r: DownloadRow, id: ColId, ph: boolean, hist: boolean)}
	{#if id === 'title'}
		<span class="nm" class:ph data-col="title">
			<span class="tt" title={rowName(r)}>{@render proto(r)}{rowName(r)}{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
			<span class="rel" title={r.name}><span class="kt" data-testid="dl-kind">{KIND_WORD[r.kind ?? 'movie']}</span>{sub2(r)}</span>
		</span>
	{:else if id === 'progress'}
		{#if hist}
			{@const s = historyStatus(r, importOf.get(r.id))}
			<span class="hic" class:ph class:bad={s.bad} data-col="progress"><span class="ic" aria-hidden="true">{s.bad ? '!' : '✓'}</span></span>
		{:else}
			{@const pct = percent(r.progress)}
			<span class="bar {barTone(r)}" class:ph data-col="progress" role="progressbar" aria-label="Progress of {who(r)}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.floor(pct)} data-testid="dl-bar"><i style="transform: translateX({pct - 100}%)"></i><span class="bl">{Math.floor(pct)}%</span></span>
		{/if}
	{:else if id === 'status'}
		{#if hist}
			{@const s = historyStatus(r, importOf.get(r.id))}
			<span class="st" class:ph class:bad={s.bad} data-col="status" title={s.text}><span class="tx">{s.text}</span></span>
		{:else}
			{@const t = barTone(r)}
			{@const more = statusDetail(r, importOf.get(r.id))}
			<span class="st {t}" class:ph data-col="status" title={more ? `${statusWord(r)} · ${more}` : statusWord(r)}><span class="dot" aria-hidden="true"></span><span class="tx">{statusWord(r)}</span></span>
		{/if}
	{:else if id === 'speed'}
		<span class="num" class:ph data-col="speed">{!hist && moving(r) ? `${bytes(r.speed)}/s` : dash}</span>
	{:else if id === 'time'}
		{#if hist}
			<span class="num" class:ph data-col="time" title={r.finished_at || r.updated_at}>{ago(r.finished_at || r.updated_at)}</span>
		{:else}
			<span class="num" class:ph data-col="time">{r.active && r.eta ? duration(r.eta) : dash}</span>
		{/if}
	{:else if id === 'size'}
		<span class="num" class:ph data-col="size">{hist ? (r.size ? bytes(r.size) : dash) : sizeLine(r) || dash}</span>
	{:else if id === 'quality'}
		<span class="txt" class:ph data-col="quality" title={r.name}>{quality(r.name) || dash}</span>
	{:else if id === 'added'}
		<span class="num" class:ph data-col="added" title={r.started_at ?? ''}>{r.started_at ? ago(r.started_at) : dash}</span>
	{:else if id === 'seeds' || id === 'peers'}
		{@const n = id === 'seeds' ? r.seeds : r.peers}
		<span class="num" class:ph data-col={id}>{!hist && r.protocol === 'torrent' && (r.phase === 'downloading' || r.phase === 'stalled') && n != null ? n : dash}</span>
	{:else if id === 'downloader'}
		<span class="txt" class:ph data-col="downloader" title={dlKind(r) || dlName(r)}>{dlName(r)}</span>
	{:else}
		<span class="num" class:ph data-col={id}>{dash}</span>
	{/if}
{/snippet}

{#snippet bin(r: DownloadRow)}
	<span class="del" data-col="bin">
		<button type="button" class="bin" aria-label="Remove {who(r)}" title={r.can_act ? 'Remove' : 'HolyArrs cannot remove this one'} disabled={!r.can_act || busy === r.id} onclick={() => onremove(r)} data-testid="dl-remove"><Icon name="trash" size={16} /></button>
	</span>
{/snippet}

{#snippet row(r: DownloadRow, hist: boolean)}
	<li class="drow" class:open={openId === r.id} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions, a11y_no_static_element_interactions -->
		<div class="dline" class:hist tabindex="0" aria-label="{who(r)}: {openId === r.id ? 'hide' : 'show'} details" data-testid="dl-line" onclick={(e) => rowClick(e, r.id)} onkeydown={(e) => rowKey(e, r.id)}>
			{@render lead(r)}
			{#each cells as c (c.id)}{@render cell(r, c.id, c.ph, hist)}{/each}
			<span class="spare"></span>
			{@render bin(r)}
		</div>
		{#if openId === r.id}{@render panel(r)}{/if}
	</li>
{/snippet}

{#snippet header()}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dline chead" data-testid="dl-colhead" oncontextmenu={headMenu}>
		<span></span><span></span>
		{#each vis as id (id)}
			{@const c = COL[id]}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<span class="ch" class:end={c.num} class:moving={ghost?.id === id} data-col={id} data-testid="dl-head" onpointerdown={(e) => headDown(e, id)} title="Drag to move · right-click for columns"><span class="lbl">{c.label}</span><!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions --><span class="grip" role="separator" aria-orientation="vertical" aria-label="Resize {c.label}" aria-valuenow={wOf(id)} tabindex="0" title="Drag to resize, double-click to fit" data-testid="dl-grip" data-grip={id} onpointerdown={(e) => grab(e, id)} ondblclick={() => fitOne(id)} onkeydown={(e) => nudge(e, id)}></span></span>
		{/each}
		<span></span>
		<span class="cm"><Menu label="Columns" icon="more" width={180} items={[{ label: 'Columns…', icon: 'more', onselect: fromMenu }, { label: 'Reset columns', icon: 'refresh', onselect: resetAll }]} /></span>
		{#if lineX !== null}<span class="ins" style="left: {lineX}px" data-testid="dl-insert" aria-hidden="true"></span>{/if}
	</div>
{/snippet}

{#snippet btns(r: DownloadRow, files: boolean)}
	<div class="xbt" data-testid="dl-actions">
		<button type="button" class="xbn" disabled title="Coming soon">Pause</button>
		<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} title={r.can_act ? 'Remove' : 'HolyArrs cannot remove this one'} onclick={() => onremove(r)} data-testid="dl-panel-remove">Remove</button>
		{#if files}<button type="button" class="xbn" aria-expanded={filesOpen.has(r.id)} onclick={() => toggleFiles(r.id)} data-testid="dl-show-files">{filesOpen.has(r.id) ? 'Hide files' : 'Show files'}</button>{/if}
	</div>
	{#if files && filesOpen.has(r.id)}<p class="nr" data-testid="dl-files">Not reported yet</p>{/if}
{/snippet}

{#snippet tabsView(r: DownloadRow, simple: { label: string; value: string }[])}
	<div class="xtabs" role="tablist" aria-label="Details">
		{#each TABS as t (t)}
			{#if t !== 'Peers' || protocolOf(r) === 'torrent'}
				<button type="button" role="tab" class="xtab" class:on={tabOf(r.id) === t} aria-selected={tabOf(r.id) === t} onclick={() => (xtabOn[r.id] = t)}>{t}</button>
			{/if}
		{/each}
	</div>
	{#if tabOf(r.id) === 'Info'}
		<dl class="kv">
			{#each simple as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}
			{#if demoDetails}
				{#if r.speed}<dt>Speed</dt><dd>{bytes(r.speed)}/s{r.up_speed ? ` down · ${bytes(r.up_speed)}/s up` : ''}</dd>{/if}
				{#if r.eta}<dt>Time left</dt><dd>{duration(r.eta)}</dd>{/if}
				{#if protocolOf(r) === 'torrent'}<dt>Seeds / peers</dt><dd>{r.seeds ?? 0} / {r.peers ?? 0}</dd><dt>Share ratio</dt><dd>{sampleRatio(r)}</dd>{/if}
			{/if}
		</dl>
	{:else if !demoDetails}
		<p class="nr">Not reported yet</p>
	{:else if tabOf(r.id) === 'Files'}
		<ul class="fl" data-testid="dl-filelist">
			{#each sampleFiles(r) as f (f.name)}
				<li><span class="fn" title={f.name}>{f.name}</span><span class="fs">{bytes(f.size)} · {Math.floor(f.size ? (f.done / f.size) * 100 : 0)}%</span></li>
			{/each}
		</ul>
	{:else if tabOf(r.id) === 'Peers'}
		<ul class="fl pr" data-testid="dl-peerlist">
			{#each samplePeers(r) as p (p.ip + p.client)}
				<li><span class="fn">{p.ip} · {p.client}</span><span class="fs">{Math.floor(p.progress * 100)}% · ↓ {bytes(p.down)}/s · ↑ {bytes(p.up)}/s</span></li>
			{/each}
			{#if !samplePeers(r).length}<li class="nr">No peers connected</li>{/if}
		</ul>
	{:else}
		<ol class="lg" data-testid="dl-log">
			{#each sampleLog(r) as l, i (i)}<li class:bad={l.bad}><time>{l.at}</time> {l.text}</li>{/each}
		</ol>
	{/if}
{/snippet}

{#snippet panel(r: DownloadRow)}
	{@const ds = decisionsOf(r)}
	{@const sizeWord = r.size ? bytes(r.size) : 'Not reported yet'}
	{@const im = importOf.get(r.id)}
	{@const simple = [
		{ label: 'Source', value: im?.Plan?.[0]?.source_file || 'Not reported yet' },
		{ label: 'Downloader', value: r.downloader || 'Not reported yet' },
		{ label: 'Folder', value: im?.Plan?.find((p) => p.target_path)?.target_path || 'Not reported yet' },
		{ label: 'Added', value: dateTime(r.started_at) || 'Not reported yet' },
		{ label: 'Requested by', value: 'Not reported yet' }
	]}
	{#if card === 'c'}
	<div class="x" id="dlx-{r.id}" data-testid="dl-panel" data-card="c" in:fade={{ duration: DUR.base }} out:fade={{ duration: DUR.fast }}>
		<span class="big" class:square={r.kind === 'artist'} data-testid="dl-big-poster" in:grow={{ from: small(r.id), dur: DUR.base, ease: easeOut }} out:grow={{ from: small(r.id), dur: DUR.fast, ease: easeIn }}><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
		<div class="xb">
		<dl class="kv">
			{#each facts(r, im) as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}
		</dl>
		{#if ds.length}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
		{@render btns(r, false)}
		{#if r.can_act && !inHistory(r)}
			<p class="xa"><button type="button" class="lk" disabled={busy === r.id} onclick={() => onfail(r)}>Mark failed</button></p>
		{/if}
		</div>
	</div>
	{:else if card === 'd'}
	<div class="x" id="dlx-{r.id}" data-testid="dl-panel" data-card="d" in:fade={{ duration: DUR.base }} out:fade={{ duration: DUR.fast }}>
		<span class="big" class:square={r.kind === 'artist'} data-testid="dl-big-poster"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
		<div class="xb">
		<p class="xt"><b>{who(r)}</b><span>{statusWord(r)} · {Math.floor(percent(r.progress))}% · {sizeWord}</span></p>
		<div class="xd">{@render tabsView(r, simple)}</div>
		{#if ds.length}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
		{@render btns(r, false)}
		{#if r.can_act && !inHistory(r)}
			<p class="xa"><button type="button" class="lk" disabled={busy === r.id} onclick={() => onfail(r)}>Mark failed</button></p>
		{/if}
		</div>
	</div>
	{:else}
	<div class="x cd" id="dlx-{r.id}" data-testid="dl-panel" data-card={card} in:fade={{ duration: DUR.base }} out:fade={{ duration: DUR.fast }}>
		<div class="xh">
			{#if card === 'a'}<span class="thumb" class:square={r.kind === 'artist'}><Poster src={poster(r, titles)} name={r.title || r.name} /></span>{/if}
			<p class="xt"><b>{who(r)}</b><span>{statusWord(r)} · {Math.floor(percent(r.progress))}% · {sizeWord}</span></p>
		</div>
		{#if card === 'b'}
			{@render tabsView(r, simple)}
		{:else}
			<dl class="kv">{#each simple as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}</dl>
		{/if}
		{#if ds.length}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
		{@render btns(r, card === 'a')}
		{#if r.can_act && !inHistory(r)}
			<p class="xa"><button type="button" class="lk" disabled={busy === r.id} onclick={() => onfail(r)}>Mark failed</button></p>
		{/if}
	</div>
	{/if}
{/snippet}

<svelte:window onpointerdown={outside} />

<div class="dlgrid" bind:this={grid} style="--tpl: {tpl}; --c-in: {inner}px">
<div class="strip" data-testid="dl-strip">
	<span class="sp"><b data-testid="dl-speed">↓ {bytes(sum.speed)}/s</b><b class="up" data-testid="dl-up">↑ {bytes(sum.up)}/s</b><SpeedGraph speed={sum.speed} up={sum.up} {history} /></span>
	<span><span class="k">Time left</span><b>{sum.eta ? duration(sum.eta) : '–'}</b></span>
	<span data-testid="dl-togo"><b>{bytes(sum.left)}</b><span class="k">to go</span></span>
	{#if problems}<button type="button" class="need" onclick={() => choose('problems')} data-testid="dl-need">⚠ {problems} need{problems === 1 ? 's' : ''} you</button>{/if}
</div>

<div class="tops">
<div class="chips" role="group" aria-label="Show" data-testid="table-filter">
	{#each tally as c (c.key)}
		<span class="cw" class:zero={c.count === 0} data-testid="status-chip" data-key={c.key} data-count={c.count} title={c.hint}>
			<Chip label={c.label} count={c.count} note={c.seeding ? `${c.seeding} seeding` : undefined} selected={on === c.key} onclick={() => choose(c.key)} />
		</span>
	{/each}
</div>
<div class="tr">
<div class="types" role="group" aria-label="Type" data-testid="dl-type-filter">
	{#each KIND_TABS as t (t.key)}
		<button type="button" class="ty" class:on={kindOn === t.key} aria-pressed={kindOn === t.key} data-key={t.key || 'all'} onclick={() => (kindOn = t.key)}>{t.label}</button>
	{/each}
</div>
</div>
<div class="find" class:open={findOpen || !!q} data-testid="dl-find">
	<button type="button" class="fi" aria-label="Search downloads" aria-expanded={findOpen || !!q} onclick={() => { findOpen = true; tick().then(() => findEl?.focus()); }}><Icon name="search" size={14} /></button>
	<input type="search" bind:this={findEl} bind:value={q} placeholder="Search downloads" aria-label="Search downloads" aria-keyshortcuts="/" onblur={() => (findOpen = false)} onkeydown={(e) => { if (e.key === 'Escape' && q) { e.preventDefault(); q = ''; } }} data-testid="dl-search" />
	{#if q}<button type="button" class="fx" aria-label="Clear search" onclick={clearFind} data-testid="dl-search-clear">✕</button>{:else}<kbd class="kb" aria-hidden="true">/</kbd>{/if}
</div>
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

{#if list.length}
	<div class="tw" onscroll={sync} data-testid="dl-tablebox"><div class="tin">
		{@render header()}
		<ul class="dlrows" data-testid="dl-list" data-tab={on}>
			{#each list as r (r.id)}{@render row(r, inHistory(r))}{/each}
		</ul>
	</div></div>
{:else}
	<p class="none" data-testid="dl-empty">{q.trim() ? 'Nothing matches.' : ((subOn ? onTab?.subs.find((x) => x.key === subOn)?.empty : onTab?.empty) ?? 'Nothing here.')}</p>
{/if}
{#if isDone}
	<div class="foot">
		<span class="tot" data-testid="dl-totals">{bytes(done.today)} today · {bytes(done.month)} this month</span>
	</div>
{/if}

{#if ghost}<div class="ghost" style="left: {ghost.x}px; top: {ghost.y}px" aria-hidden="true" data-testid="dl-ghost">{COL[ghost.id].label}</div>{/if}

{#if pop}
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div class="picker" role="dialog" aria-label="Columns" tabindex="-1" bind:this={pickEl} style="left: {pop.x}px; top: {pop.y}px" data-testid="dl-picker" onkeydown={pickerKey}>
		<p class="ph-t">Columns</p>
		<ul class="pl">
			{#each layout.order as id (id)}
				{@const c = COL[id]}
				<li data-id={id} class:held={held === id} class:off={c.missing}>
					<button type="button" class="hd" data-handle={id} aria-label="Move {c.label}" aria-pressed={held === id} aria-describedby="dl-pick-help" disabled={c.missing} onkeydown={(e) => handleKey(e, id)} onpointerdown={(e) => handleDown(e, id)}><svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true"><g fill="currentColor"><circle cx="5" cy="3" r="1.2"/><circle cx="9" cy="3" r="1.2"/><circle cx="5" cy="7" r="1.2"/><circle cx="9" cy="7" r="1.2"/><circle cx="5" cy="11" r="1.2"/><circle cx="9" cy="11" r="1.2"/></g></svg></button>
					<label class="pk"><input type="checkbox" checked={!c.missing && !layout.hidden.includes(id)} disabled={c.missing} onchange={() => flip(id)} data-testid="dl-pick-{id}" /><span>{c.label}</span>{#if c.missing}<span class="nr">Not reported yet</span>{/if}</label>
				</li>
			{/each}
		</ul>
		<p id="dl-pick-help" class="help">Drag a handle, or press Space on it, then ↑/↓ and Enter.</p>
		<div class="pf"><button type="button" class="lk" onclick={resetAll} data-testid="dl-pick-reset">Reset to default</button></div>
		<span class="sr" aria-live="polite">{said}</span>
	</div>
{/if}

</div>
<style>
	.strip { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 22px; padding: 8px 0; margin-bottom: 10px; border-bottom: 1px solid var(--line); font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.strip > span { display: inline-flex; align-items: center; gap: 6px; }
	.k { color: var(--text-muted); font: var(--t-footnote); }
	.strip b { font-weight: 600; }
	.sp { gap: 8px; }
	.up { color: var(--info); }
	.need { min-height: 28px; padding: 0 8px; margin-left: -8px; border: 0; border-radius: var(--r-btn); background: none; color: var(--fail); font: var(--t-callout); font-weight: 600; cursor: pointer; }
	.need:hover { background: var(--surface-2); }
	.need:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.tops { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 0 16px; }
	.types { display: inline-flex; gap: 2px; padding: 2px; margin-bottom: 12px; border-radius: var(--r-btn); background: var(--surface-2); }
	.ty { min-height: 28px; padding: 0 10px; border: 0; border-radius: calc(var(--r-btn) - 2px); background: none; color: var(--text-muted); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.ty.on { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
	.ty:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pm { display: inline-grid; place-items: center; width: 14px; height: 14px; margin-right: 6px; vertical-align: 1px; border-radius: 3px; background: var(--surface-3); color: var(--text-muted); font-size: 9px; font-weight: 700; line-height: 1; }
	.kt { display: inline-block; box-sizing: border-box; height: 16px; margin-right: 6px; padding: 0 5px; vertical-align: 1px; border: 1px solid var(--line); border-radius: var(--r-sm); background: var(--surface-3); color: var(--text); font: var(--t-caption); font-size: 11px; line-height: 14px; font-weight: 600; white-space: nowrap; }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-bottom: 12px; }
	.subs { gap: 6px; margin-top: -4px; }
	.cw { display: inline-flex; transition: opacity var(--dur-fast) var(--ease-out); }
	.cw.zero { opacity: 0.72; }
	.cw.zero:hover, .cw.zero:focus-within { opacity: 0.8; }
	.cw.sm :global(.main) { min-height: 28px; padding: 0 11px; font-size: 13px; }

	.none { padding: 10px 0; color: var(--text-muted); font: var(--t-callout); }

	/* One column grid for every row of both sections. */
	.dlgrid { --gap: 10px; min-width: 0; }
	.tw { overflow-x: auto; overflow-y: hidden; max-width: 100%; }
	.tin { min-width: var(--c-in); }
	.dlrows { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--line); }
	.drow { display: block; border-bottom: 1px solid var(--line); }
	.drow.open { background: var(--surface-2); }
	.dline { display: grid; grid-template-columns: var(--tpl); align-items: center; column-gap: var(--gap); height: 48px; }
	.drow > .dline { cursor: pointer; }
	.drow > .dline:hover { background: var(--surface-2); }
	.drow > .dline:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }

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
	.bar i { position: absolute; inset: 0; background: var(--ok); opacity: 0.5; transition: transform var(--dur-slow) var(--ease-out); }
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
	.st.bad { color: var(--fail); }
	.hic { display: flex; align-items: center; }
	.ic { display: grid; place-items: center; flex: none; width: 16px; height: 16px; border-radius: 50%; background: var(--ok); color: var(--surface-1); font-size: 10px; font-weight: 700; }
	.hic.bad .ic { background: var(--fail); }

	.dline > .ph { display: none; }
	.txt { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.num { text-align: right; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

	.chead { position: relative; height: 28px; border-bottom: 1px solid var(--line); color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.chead + .dlrows { border-top: 0; }
	.chead:hover { background: none; }
	.ch { position: relative; align-self: stretch; display: flex; align-items: center; min-width: 0; }
	.ch.end { justify-content: flex-end; }
	.ch { cursor: grab; user-select: none; touch-action: none; }
	.ch.moving { opacity: 0.4; }
	.ins { position: absolute; top: 2px; bottom: 2px; width: 2px; margin-left: -1px; border-radius: 1px; background: var(--accent); pointer-events: none; z-index: 4; }
	.ghost { position: fixed; z-index: 1000; transform: translate(-50%, -130%); padding: 3px 10px; border-radius: var(--r-sm); background: var(--surface-1); box-shadow: 0 4px 14px rgb(0 0 0 / 0.25); color: var(--text); font: var(--t-caption); font-weight: 600; pointer-events: none; white-space: nowrap; }
	.picker { position: fixed; z-index: 1000; width: 260px; max-height: min(440px, calc(100vh - 16px)); overflow: auto; padding: 8px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28); }
	.ph-t { margin: 2px 6px 6px; color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.pl { list-style: none; margin: 0; padding: 0; }
	.pl li { display: flex; align-items: center; gap: 4px; height: 30px; border-radius: 6px; }
	.pl li.held { background: var(--surface-3); }
	.hd { display: grid; place-items: center; width: 24px; height: 26px; padding: 0; border: 0; border-radius: 5px; background: none; color: var(--text-muted); cursor: grab; touch-action: none; }
	.hd:disabled { opacity: 0.3; cursor: default; }
	.hd:focus-visible, .pk input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pk { display: flex; flex: 1; align-items: center; gap: 8px; min-width: 0; font: var(--t-footnote); cursor: pointer; }
	.off .pk { color: var(--text-muted); cursor: default; }
	.nr { margin-left: auto; padding-right: 4px; color: var(--text-muted); font: var(--t-caption); }
	.help { margin: 6px 6px 0; color: var(--text-muted); font: var(--t-caption); }
	.pf { display: flex; justify-content: flex-end; margin-top: 4px; border-top: 1px solid var(--line); padding-top: 4px; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
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
	/* Details: the grown poster under the ▾ and poster columns, the facts to its right. */
	.x { display: grid; grid-template-columns: 72px minmax(0, 1fr); align-items: start; gap: 16px; padding: 6px 12px 14px calc(24px + var(--gap)); }
	.xb { min-width: 0; }
	.big { width: 72px; height: 108px; overflow: hidden; border-radius: 6px; box-shadow: 0 4px 14px rgb(0 0 0 / 0.18); --poster-ratio: 2 / 3; }
	.big.square { height: 72px; --poster-ratio: 1; }
	.big :global(.poster) { border-radius: 6px; }
	.post { transition: opacity var(--dur-fast) var(--ease-out); }
	.drow.open > .dline .post { opacity: 0; }
	.kv { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 3px 16px; margin: 0; font: var(--t-footnote); }
	.kv dt { color: var(--text-muted); }
	.kv dd { margin: 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
	.dp { margin-top: 10px; }
	.cd { display: block; }
	.xh { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.thumb { flex: none; width: 32px; height: 48px; overflow: hidden; border-radius: 4px; --poster-ratio: 2 / 3; }
	.thumb.square { height: 32px; --poster-ratio: 1; }
	.thumb :global(.poster) { border-radius: 4px; }
	.xt { display: grid; gap: 2px; margin: 0; min-width: 0; font: var(--t-footnote); }
	.xt b { font-weight: 600; overflow-wrap: anywhere; }
	.xt span { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.cd .kv, .cd .xtabs { margin-top: 10px; }
	.nr { margin: 8px 0 0; color: var(--text-muted); font: var(--t-footnote); }
	.xtabs { display: flex; flex-wrap: wrap; gap: 2px; padding: 2px; width: max-content; max-width: 100%; border-radius: var(--r-btn); background: var(--surface-2); }
	.xtab { min-height: 28px; padding: 0 12px; border: 0; border-radius: calc(var(--r-btn) - 2px); background: none; color: var(--text-muted); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xtab.on { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
	.xtab:focus-visible, .xbn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.xb > .xt { margin-bottom: 8px; }
	.xd .xtabs, .xd .kv, .xd .fl, .xd .lg, .xd .nr { margin-top: 8px; }
	.xd .xtabs { margin-top: 0; }
	.fl { list-style: none; padding: 0; margin: 0; display: grid; gap: 4px; font: var(--t-footnote); min-width: 0; }
	.fl li { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; min-width: 0; }
	.fn { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fs { flex: none; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.pr .fs { white-space: nowrap; }
	.lg { list-style: none; padding: 0; margin: 0; display: grid; gap: 3px; font: var(--t-caption); min-width: 0; max-height: 220px; overflow: auto; }
	.lg li { overflow-wrap: anywhere; }
	.lg time { color: var(--text-muted); font-variant-numeric: tabular-nums; margin-right: 6px; }
	.lg .bad { color: var(--fail); }
	.xbt { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
	.xbn { min-height: 32px; padding: 0 14px; border: 1px solid var(--surface-3); border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xbn:hover:not(:disabled) { background: var(--surface-3); }
	.xbn:disabled { opacity: 0.5; cursor: not-allowed; }
	.xa { margin: 8px 0 0; }
	.lk { min-height: 28px; padding: 0 4px; border: 0; border-radius: 6px; background: none; color: var(--accent-text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }

	.foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; margin-top: 10px; }
	.tr { display: inline-flex; align-items: center; gap: 8px; margin-bottom: 12px; }
	.tr .types { margin-bottom: 0; }
	.find { display: inline-flex; align-items: center; gap: 6px; height: 32px; padding: 0 8px 0 10px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text-muted); }
	.find:focus-within { box-shadow: var(--focus-ring); }
	.fi { display: grid; place-items: center; width: 18px; height: 24px; padding: 0; border: 0; background: none; color: inherit; cursor: default; }
	.find input { width: 170px; border: 0; background: none; color: var(--text); font: var(--t-footnote); outline: none; }
	.find input::-webkit-search-cancel-button { display: none; }
	.fx { display: grid; place-items: center; width: 20px; height: 20px; padding: 0; border: 0; border-radius: 50%; background: var(--surface-3); color: var(--text-muted); font-size: 10px; cursor: pointer; }
	.fx:hover { color: var(--text); }
	.fx:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.kb { display: grid; place-items: center; min-width: 18px; height: 18px; padding: 0 4px; border: 1px solid var(--line); border-radius: 4px; color: var(--text-muted); font: var(--t-caption); }
	.tot { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }

	/* Phones: a fixed card per row (title, then bar, status and time); no picker. */
	@media (max-width: 640px) {
		.dlgrid { --gap: 8px; }
		.chead { display: none; }
		.tin { min-width: 0; }
		.dline { grid-template-columns: 24px 28px minmax(0, 1fr) 88px 52px 32px; grid-template-areas: 'tg post nm nm nm del' '. post bar st time del'; row-gap: 4px; height: auto; min-height: 62px; padding: 6px 0; }
		.dline > [data-col] { display: none; }
		.dline > .ph { display: none; }
		.dline > [data-col='title'] { display: flex; grid-area: nm; }
		.dline > [data-col='progress'] { display: flex; grid-area: bar; }
		.dline > [data-col='status'] { display: flex; grid-area: st; }
		.dline > [data-col='time'] { display: block; grid-area: time; }
		.dline > [data-col='bin'] { display: flex; grid-area: del; }
		.dline > .spare { display: none; }
		.tg { grid-area: tg; }
		.post { grid-area: post; align-self: start; }
		.x { grid-template-columns: 56px minmax(0, 1fr); gap: 12px; padding: 4px 8px 12px calc(24px + var(--gap)); }
		.big { width: 56px; height: 84px; }
		.big.square { height: 56px; }
		.x[data-card='d'] { grid-template-columns: minmax(0, 1fr); gap: 10px; }
		.fl li { flex-direction: column; gap: 0; }
		.fs { white-space: normal; }
		.pr .fs { white-space: normal; }
		/* Phones: a search icon that opens into the field. */
		.find:not(.open) { padding: 0 7px; }
		.find:not(.open) input, .find:not(.open) .kb { display: none; }
		.find .fi { cursor: pointer; }
		.find.open input { width: 120px; }
		.kb { display: none; }
		.strip { gap: 4px 14px; }
		.picker { display: none; }
	}
	@media (prefers-reduced-motion: reduce) {
		.bar i, .tg :global(svg), .bin, .cw, .post { transition: none; }
	}
</style>
