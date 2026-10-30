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
	import { COL, COLUMNS, IDS, KEY, MAX_W, MIN_W, OLD_KEY, defaults, dump, load, move, moveVisible, quality, toggle as toggleCol, visible, type ColId, type Layout } from './columns.ts';
	import { replaceState } from '$app/navigation';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Chip from '../ui/Chip.svelte';
	import Menu from '../ui/Menu.svelte';
	import TopMenu from './TopMenu.svelte';
	import { to } from '../paths.ts';
	import DlBar from './DlBar.svelte';
	import DecisionPanel from './DecisionPanel.svelte';
	import SpeedGraph from './SpeedGraph.svelte';
	import Popover from '../ui/Popover.svelte';
	import { tip } from './tip.ts';
	import { dlScope } from '../search-field.svelte.ts';
	import { dlTop } from '../dl-top.svelte.ts';
	import { toasts } from '../toasts.svelte.ts';
	import { demoDetails, fileExtras, isProblem, sampleFiles, sampleHeader, sampleLog, samplePeers, sampleRatio, sampleServers, sampleStory, sampleSwarm, sampleTrackers, fileTree } from './sampleDetails.ts';
	import { ago, dateTime, duration, percent } from '../format.ts';
	import { size as bytes, protocolOf, KIND_TABS, KIND_WORD, type KindKey } from './size.ts';
	import { kindWords } from '../downloaders.ts';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { manyWords, rowName, type Decision, type DownloadRow } from '../downloaders.ts';
	import type { ImportRecord, TitleView } from '../types.ts';
	import { ALL, counts, pick, showOf, stageOf, statusWord, subCounts, tabsFor } from './status-tabs.ts';
	import { statusDetail } from './imports.ts';
	import { poster, who } from './shared.ts';
	import { barTone, facts, historyStatus, historyTotals, inHistory, matches, moving, sizeLine, totals } from './sections.ts';

	let {
		rows: everyRow,
		importOf,
		titles,
		titlesReady = true,
		busy,
		samples,
		history = [],
		torrent = false,
		decisionsOf,
		openId = $bindable(0),
		onfail,
		onremove,
		ondone,
		onact,
		onall,
		onlimit,
		watch = false
	}: {
		rows: DownloadRow[];
		importOf: Map<number, ImportRecord>;
		titles: Map<number, TitleView>;
		/** False while the Library titles (the posters) are still on their way: posters show a plain tile, not initials. */
		titlesReady?: boolean;
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
		/** A client action on one download; the new state comes back from the client. */
		onact?: (r: DownloadRow, action: ClientAction) => void;
		/** Pause (true) or resume (false) everything in every client. */
		onall?: (pause: boolean) => void;
		/** Opens the speed limit. */
		onlimit?: () => void;
		/** Watch only: someone who may not manage the Library sees their own downloads with no actions. */
		watch?: boolean;
	} = $props();

	// The Type filter (All, Movies, TV shows, Music) narrows every tab and total.
	let kindOn = $state<KindKey>('');
	const typed = $derived(kindOn ? everyRow.filter((r) => (r.kind ?? 'movie') === kindOn) : everyRow);
	// The search narrows every tab too, so the tab counts follow it.
	// The search is the top bar's field, scoped to this page (search-field.svelte.ts).
	const q = $derived(dlScope.term);
	const searched = $derived(q.trim() ? typed.filter((r) => matches(r, q)) : typed);
	// The left rail: a status, a source (Usenet or Torrent); the Type filter is its Categories.
	let srcOn = $state<'' | 'usenet' | 'torrent'>('');
	const bySource = $derived(srcOn ? searched.filter((r) => protocolOf(r) === srcOn) : searched);
	const rows = $derived(bySource);
	const kindCount = (k: string) => everyRow.filter((r) => (r.kind ?? 'movie') === k).length;
	const srcCount = (k: string) => everyRow.filter((r) => protocolOf(r) === k).length;

	// The picked tab (and sub-tab) lives in the address: &show=tab or tab.sub.
	let menuId = $state(0);
	type ClientAction = 'pause' | 'resume' | 'top' | 'recheck' | 'reannounce';
	const ACTIONS: { label: string; action: ClientAction; torrent?: boolean }[] = [
		{ label: 'Move to top', action: 'top' }, { label: 'Check again', action: 'recheck', torrent: true }, { label: 'Ask trackers again', action: 'reannounce', torrent: true }
	];
	const isPaused = (r: DownloadRow | undefined) => r?.state === 'paused';
	const mmss = (n: number) => `${Math.floor(n / 60)}:${String(n % 60).padStart(2, '0')}`;
	const TABS = ['Info', 'Files', 'Peers', 'Trackers', 'Log'] as const;
	// Sample state for the Files, Peers and Trackers tables (the page only; nothing is sent anywhere).
	let sel = $state<Record<string, string>>({});
	let prio = $state<Record<string, string>>({});
	let closed = $state<Record<string, boolean>>({});
	const PRIOS = ['Skip', 'Normal', 'High', 'Max'];
	const prioOf = (r: DownloadRow, key: string, i: number) => prio[`${r.id}|${key}`] ?? fileExtras(r, i).priority;
	const savePath = (r: DownloadRow, im: ImportRecord | undefined) => im?.Plan?.find((x) => x.target_path)?.target_path || `/data/downloads/${r.kind === 'show' ? 'shows' : r.kind === 'artist' ? 'music' : 'movies'}/${(r.name || r.title || 'download').replace(/\.(mkv|nzb|torrent)$/i, '')}`;
	const flagsOf = (p: { progress: number; down: number; up: number }) => `${p.progress >= 1 ? 'S ' : ''}${p.down > 0 ? 'D ' : ''}${p.up > 0 ? 'U ' : ''}E`.trim();
	const tabNow = (r: DownloadRow) => {
		const t = tabOf(r.id);
		return protocolOf(r) === 'torrent' || (t !== 'Peers' && t !== 'Trackers') ? t : 'Info';
	};
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
		// On a narrow row the picked pill, once open, is scrolled into view inside the row.
		setTimeout(() => document.querySelector('.pw.on')?.scrollIntoView({ block: 'nearest', inline: 'nearest' }), 220);
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
	const openRow = $derived(everyRow.find((r) => r.id === openId));
	// The toolbar's Start and Pause act on the open row; its state comes from the client.
	const canStart = $derived(!!openRow?.can_act && isPaused(openRow) && busy !== openRow?.id);
	const canPause = $derived(!!openRow?.can_act && !isPaused(openRow) && busy !== openRow?.id);
	const problems = $derived(allQueue.filter((r) => barTone(r) === 'bad').length);
	const done = $derived(historyTotals(typed.filter(inHistory)));

	function toggle(id: number) {
		openId = id;
		paneOpen = true;
	}
	// The detail pane under the list: its height is kept in this browser.
	const PANE_KEY = 'holyarrs.downloads.pane';
	let paneH = $state(260);
	let paneOpen = $state(true);
	try {
		const v = typeof localStorage === 'undefined' ? null : Number(localStorage.getItem(PANE_KEY));
		if (v && Number.isFinite(v)) paneH = Math.max(120, Math.min(900, Math.round(v)));
		else if (typeof innerHeight !== 'undefined') paneH = Math.max(150, Math.round(innerHeight * 0.3));
	} catch {
		/* private window: the default height */
	}
	function savePane() {
		try {
			localStorage.setItem(PANE_KEY, String(Math.round(paneH)));
		} catch {
			/* private window: the height lasts for this visit */
		}
	}
	let colEl = $state<HTMLElement>();
	// Phones have no top bar: the tab strip carries a magnifier that opens the same scoped search.
	let psOpen = $state(false);
	let psEl = $state<HTMLInputElement>();
	const barActs = $derived([
		{ key: 'start', icon: 'play' as const, label: 'Start', hint: canStart ? 'Start' : 'Start (select a paused download)', disabled: !canStart, run: () => openRow && onact?.(openRow, 'resume'), more: [{ label: 'Start all', onselect: () => onall?.(false) }] },
		{ key: 'pause', icon: 'pause' as const, label: 'Pause', hint: canPause ? 'Pause' : 'Pause (select a download)', disabled: !canPause, run: () => openRow && onact?.(openRow, 'pause'), more: [{ label: 'Pause all', onselect: () => onall?.(true) }, { label: 'Speed limit…', onselect: () => onlimit?.() }] },
		{ key: 'remove', icon: 'trash' as const, label: 'Remove', hint: openRow?.can_act ? 'Remove' : 'Remove (select a download first)', disabled: !openRow?.can_act || busy === openRow?.id, run: () => openRow && onremove(openRow) },
		{ key: 'turned', icon: 'shield' as const, label: 'Turned down', hint: 'Turned down copies', disabled: false, href: to('/downloads/turned-down') }
	]);
	const maxPane = () => Math.max(160, (colEl?.clientHeight ?? 600) - 140);
	function dragDown(e: PointerEvent) {
		if (e.button !== 0 || !paneOpen) return;
		e.preventDefault();
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		const y0 = e.clientY;
		const h0 = paneH;
		const mv = (m: PointerEvent) => (paneH = Math.max(120, Math.min(maxPane(), Math.round(h0 + (y0 - m.clientY)))));
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			savePane();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function dividerKey(e: KeyboardEvent) {
		if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
		e.preventDefault();
		e.stopPropagation();
		paneH = Math.max(120, Math.min(maxPane(), paneH + (e.key === 'ArrowUp' ? 24 : -24)));
		savePane();
	}
	/** Up and Down move the selected row; the pane follows. Escape clears it. */
	/** The row that has the focus (or holds it inside), if any. */
	const focusedRow = () => {
		const el = (document.activeElement as HTMLElement | null)?.closest<HTMLElement>('[data-dl-id]');
		return el ? everyRow.find((r) => r.id === Number(el.dataset.dlId)) : undefined;
	};
	let rowFocus = $state(0);
	/** One tab stop for the whole list: the row that was focused last, else the open one, else the first. */
	const tabRow = $derived(list.some((r) => r.id === rowFocus) ? rowFocus : list.some((r) => r.id === openId) ? openId : (list[0]?.id ?? 0));
	/** Left and Right move across the pills and the sub-options that are open, and pick as they go. */
	function navKey(e: KeyboardEvent) {
		// Escape clears a picked sub-option back to its parent pill.
		if (e.key === 'Escape' && subOn) {
			e.preventDefault();
			chooseSub(subOn);
			(e.currentTarget as HTMLElement).querySelector<HTMLElement>('.pw.on .tab')?.focus();
			return;
		}
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
		const root = e.currentTarget as HTMLElement;
		const list = [...root.querySelectorAll<HTMLElement>('[data-nav]')].filter((el) => !el.closest('[inert]'));
		let i = list.indexOf(document.activeElement as HTMLElement);
		if (i < 0) i = list.findIndex((el) => el.matches('[role="tab"][aria-selected="true"]'));
		const next = list[i + (e.key === 'ArrowRight' ? 1 : -1)];
		if (!next) return;
		e.preventDefault();
		next.focus();
		next.click();
	}
	function slashKey(e: KeyboardEvent) {
		const mod = e.ctrlKey || e.metaKey;
		const ck = mod && !e.altKey && (e.key === 'k' || e.key === 'K');
		if (!ck && (e.key !== '/' || mod || e.altKey)) return;
		const t = e.target as HTMLElement | null;
		if (!ck && (!t || t.closest('input, textarea, select, [contenteditable="true"]'))) return;
		e.preventDefault();
		e.stopImmediatePropagation();
		if (matchMedia('(max-width: 767px)').matches) {
			psOpen = true;
			tick().then(() => psEl?.focus());
		} else {
			const f = document.querySelector<HTMLInputElement>('[data-dl-search]');
			f?.focus();
			f?.select();
		}
	}
	$effect(() => {
		window.addEventListener('keydown', slashKey, true);
		return () => window.removeEventListener('keydown', slashKey, true);
	});
	function trNav(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		const tr = e.currentTarget as HTMLElement;
		const next = (e.key === 'ArrowDown' ? tr.nextElementSibling : tr.previousElementSibling) as HTMLElement | null;
		if (!next) return;
		e.preventDefault();
		e.stopPropagation();
		next.focus();
		next.click();
	}
	function listKey(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp' && e.key !== 'Escape' && e.key !== 'Delete') return;
		const t = e.target as HTMLElement | null;
		if (!t || t.closest('input, select, textarea, [contenteditable="true"], [role="tab"], [role="separator"], [role="dialog"], .mn')) return;
		if (e.key === 'Escape') {
			if (openId) {
				openId = 0;
				e.preventDefault();
			}
			return;
		}
		if (e.key === 'Delete') {
			if (t.closest('button, a')) return;
			const target = focusedRow();
			if (!target) return;
			e.preventDefault();
			if (target.can_act) onremove(target);
			else toasts.push('This download cannot be removed from here.', 'info');
			return;
		}
		if (!list.length) return;
		e.preventDefault();
		const from = focusedRow()?.id ?? openId;
		const i = list.findIndex((r) => r.id === from);
		const n = i < 0 ? 0 : Math.max(0, Math.min(list.length - 1, i + (e.key === 'ArrowDown' ? 1 : -1)));
		const r = list[n];
		openId = r.id;
		rowFocus = r.id;
		tick().then(() => {
			const row = grid?.querySelector<HTMLElement>(`[data-dl-id="${r.id}"] > .dline`);
			row?.focus({ preventScroll: true });
			row?.scrollIntoView({ block: 'nearest' });
		});
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
	let appEl = $state<HTMLElement>();
	let listEl = $state<HTMLElement>();
	let appH = $state(0);
	let phone = $state(false);
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
	// A layout nobody has changed shows more columns when the screen is wide (they drop out again when it is not).
	const customised = $derived(layout.hidden.length !== COLUMNS.filter((c) => !c.on).length || COLUMNS.some((c) => !c.on && !layout.hidden.includes(c.id)) || layout.order.some((id, i) => id !== IDS[i]));
	const WIDE: ColId[] = ['seeds', 'peers', 'added', 'downloader'];
	const visAll = $derived.by(() => {
		const v = visible(layout);
		return customised ? v : [...v, ...WIDE.filter((id) => !v.includes(id) && !COL[id].missing)];
	});
	/** The least width each number column needs to show its text whole. */
	const MINW: Partial<Record<ColId, number>> = { poster: 44, kind: 84, proto: 52, progress: 66, speed: 84, time: 76, size: 140, seeds: 56, peers: 56, ratio: 56, added: 90, status: 110 };
	/** Columns drop out, least important first, when the list is too narrow for all of them. */
	const DROP: ColId[] = ['quality', 'downloader', 'added', 'age', 'seeds', 'peers', 'ratio', 'requested', 'source', 'category', 'speed', 'time', 'size'];
	const TITLE_MIN = 220;
	const wMin = (id: ColId) => Math.max(layout.widths[id], MINW[id] ?? 0);
	const vis = $derived.by(() => {
		if (phone || room <= 0) return visAll;
		const need = (c: ColId[]) => 48 + FIXED + (c.length + 3) * gap + c.reduce((n, id) => n + (id === 'title' ? TITLE_MIN : wMin(id)), 0);
		let cur = [...visAll];
		for (const d of DROP) {
			if (need(cur) <= room) break;
			cur = cur.filter((x) => x !== d);
		}
		return cur;
	});
	/** A phone shows a fixed card (title, bar, status, time) whatever is picked. */
	const PHONE: ColId[] = ['poster', 'title', 'progress', 'status', 'time'];
	const cells = $derived([...vis.map((id) => ({ id, ph: false })), ...PHONE.filter((id) => !vis.includes(id)).map((id) => ({ id, ph: true }))]);
	const FIXED = 8 + 32;
	let room = $state(0);
	let titleMin = $state(MIN_W);
	/** wOf is a column's width on screen (an unset Title gives way when room is short). */
	const tW = $derived.by(() => {
		if (!vis.includes('title')) return 0;
		if (room <= 0) return layout.widths.title;
		// A Title nobody has sized takes what is left (with a little slack); a sized one only gives way when the room is short.
		const rest = vis.reduce((n, id) => (id === 'title' ? n : n + wMin(id)), (layout.titleSet ? 0 : 48) + FIXED + (vis.length + 3) * gap);
		return Math.max(TITLE_MIN, Math.min(layout.widths.title, Math.floor(room - rest)));
	});
	const wOf = (id: ColId) => (id === 'title' ? tW : wMin(id));
	// Until a person sets the Title width, it flexes and takes whatever room is left.
	const tpl = $derived(`8px ${vis.map((id) => (id === 'title' && !layout.titleSet ? `minmax(${wOf(id)}px, 1fr)` : `${wOf(id)}px`)).join(' ')} ${layout.titleSet || !vis.includes('title') ? 'minmax(0, 1fr)' : '0px'} 32px`);
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
	const clamp = (id: ColId, v: number) => Math.round(Math.max(minOf(id), MINW[id] ?? 0, Math.min(MAX_W, v)));
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
			room = (listEl ?? g).clientWidth;
			phone = matchMedia('(max-width: 640px)').matches;
			titleMin = minOf('title');
		});
		ro.observe(g);
		if (listEl) ro.observe(listEl);
		return () => ro.disconnect();
	});
	// The app fills the column down to the bottom of the screen; the list scrolls inside it.
	$effect(() => {
		if (!appEl) return;
		const fit = () => {
			const small = innerWidth <= 767;
			const tab = small ? (parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--tabbar')) || 56) : 0;
			appH = Math.max(420, Math.floor(innerHeight - appEl!.getBoundingClientRect().top - tab));
		};
		fit();
		const ro = new ResizeObserver(fit);
		ro.observe(document.body);
		window.addEventListener('resize', fit);
		return () => {
			ro.disconnect();
			window.removeEventListener('resize', fit);
		};
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
		if (e.target !== e.currentTarget) return;
		if (e.key === ' ') {
			// Until pausing exists, Space opens and closes the pane.
			e.preventDefault();
			if (openId === id) openId = 0;
			else toggle(id);
			return;
		}
		if (e.key !== 'Enter') return;
		e.preventDefault();
		toggle(id);
		tick().then(() => document.querySelector<HTMLElement>('.dpane [role="tab"][aria-selected="true"]')?.focus());
	}	const dash = '—';
	/** The Downloader's own configured name (as in the details); its kind goes in the tooltip. */
	const dlName = (r: DownloadRow) => r.downloader || (r.downloader_kind && kindWords[r.downloader_kind as keyof typeof kindWords]) || dash;
	const dlKind = (r: DownloadRow) => (r.downloader_kind && kindWords[r.downloader_kind as keyof typeof kindWords]) || '';
	const sub2 = (r: DownloadRow) => (manyWords(r.kind) && r.covers ? r.covers : r.name);
</script>

{#snippet lead(r: DownloadRow)}
	<span class="tg" aria-hidden="true"></span>
{/snippet}

{#snippet cell(r: DownloadRow, id: ColId, ph: boolean, hist: boolean)}
	{#if id === 'poster'}
		<span class="post" class:square={r.kind === 'artist'} class:ph data-col="poster"><Poster src={poster(r, titles)} name={titlesReady ? r.title || r.name : ''} /></span>
	{:else if id === 'kind'}
		<span class="kcell" class:ph data-col="kind"><span class="kt" data-testid="dl-kind">{KIND_WORD[r.kind ?? 'movie']}</span></span>
	{:else if id === 'proto'}
		{@const p = protocolOf(r)}
		<span class="pcell" class:ph data-col="proto">{#if p}<span class="pm" data-testid="dl-protocol" data-protocol={p} use:tip={`${p === 'torrent' ? 'Torrent' : 'Usenet'} · ${dlName(r)}`} role="img" aria-label="{p === 'torrent' ? 'Torrent' : 'Usenet'} download"><Icon name={p === 'torrent' ? 'torrent' : 'usenet'} size={14} /></span>{/if}</span>
	{:else if id === 'title'}
		<span class="nm" class:ph data-col="title">
			<span class="tt" title={rowName(r)}>{rowName(r)}{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
			<span class="rel" title={r.name}>{sub2(r)}</span>
		</span>
	{:else if id === 'progress'}
		{#if hist && stageOf(r) !== 'failed'}
			{@const s = historyStatus(r, importOf.get(r.id))}
			<span class="hic" class:ph class:bad={s.bad} data-col="progress"><span class="ic" aria-hidden="true">{s.bad ? '!' : '✓'}</span></span>
		{:else}
			{@const raw = percent(r.progress)}
			{@const pct = stageOf(r) === 'importing' ? raw : Math.min(99, raw)}
			{@const tone = stageOf(r) === 'paused' ? 'paused' : barTone(r)}
			<span class="bar {tone}" class:ph data-col="progress" role="progressbar" aria-label="Progress of {who(r)}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.floor(pct)} data-testid="dl-bar"><i style="transform: translateX({pct - 100}%)"></i><span class="bl">{Math.floor(pct)}%</span></span>
		{/if}
	{:else if id === 'status'}
		{#if hist}
			{@const s = historyStatus(r, importOf.get(r.id))}
			<span class="st" class:ph class:bad={s.bad} data-col="status" title={s.text}><span class="dot" aria-hidden="true"></span><span class="tx">{s.text}</span></span>
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
		{#if !watch}<button type="button" class="bin" tabindex={tabRow === r.id ? 0 : -1} aria-label="Remove {who(r)}" title={r.can_act ? 'Remove' : 'HolyArrs cannot remove this one'} disabled={!r.can_act || busy === r.id} onclick={() => onremove(r)} data-testid="dl-remove"><Icon name="trash" size={16} /></button>{/if}
	</span>
{/snippet}

{#snippet row(r: DownloadRow, hist: boolean)}
	<li class="drow" class:sel={openId === r.id} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions, a11y_no_static_element_interactions -->
		<div class="dline" class:hist tabindex={tabRow === r.id ? 0 : -1} onfocus={() => (rowFocus = r.id)} aria-current={openId === r.id} aria-label="{who(r)}: show details" data-testid="dl-line" onclick={(e) => rowClick(e, r.id)} onkeydown={(e) => rowKey(e, r.id)}>
			{@render lead(r)}
			{#each cells as c (c.id)}{@render cell(r, c.id, c.ph, hist)}{/each}
			<span class="spare"></span>
			{@render bin(r)}
		</div>
	</li>
{/snippet}

{#snippet header()}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dline chead" data-testid="dl-colhead" oncontextmenu={headMenu}>
		<span></span>
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
		{#if !watch}<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} onclick={() => onact?.(r, isPaused(r) ? 'resume' : 'pause')} data-testid="dl-panel-pause">{isPaused(r) ? 'Resume' : 'Pause'}</button>
		<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} title={r.can_act ? 'Remove' : 'HolyArrs cannot remove this one'} onclick={() => onremove(r)} data-testid="dl-panel-remove">Remove</button>{/if}
		{#if files}<button type="button" class="xbn" aria-expanded={filesOpen.has(r.id)} onclick={() => toggleFiles(r.id)} data-testid="dl-show-files">{filesOpen.has(r.id) ? 'Hide files' : 'Show files'}</button>{/if}
		{#if !watch}<button type="button" class="xbn more" aria-expanded={menuId === r.id} aria-label="More actions" onclick={() => (menuId = menuId === r.id ? 0 : r.id)} data-testid="dl-more">⋯</button>{/if}
	</div>
	{#if menuId === r.id && !watch}
		<ul class="mn" data-testid="dl-more-menu">
			{#each ACTIONS.filter((a) => !a.torrent || protocolOf(r) === 'torrent') as a (a.label)}
				<li><button type="button" class="mi" disabled={!r.can_act || busy === r.id} onclick={() => { menuId = 0; onact?.(r, a.action); }} data-testid="dl-act-{a.action}">{a.label}</button></li>
			{/each}
		</ul>
	{/if}
	{#if files && filesOpen.has(r.id)}<p class="nr" data-testid="dl-files">Not reported yet</p>{/if}
{/snippet}

{#snippet depth(r: DownloadRow)}
	{#if demoDetails}
		{#if isProblem(r)}
			{@const st = sampleStory(r)}
			<section class="dep story" data-testid="dl-story" aria-label="What happened">
				<h4>What happened</h4>
				<dl class="kv">{#each st.facts as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}</dl>
				<ol class="lg">{#each st.steps as x, i (i)}<li class:bad={x.bad}>{x.text}</li>{/each}</ol>
			</section>
		{/if}
		{#if protocolOf(r) !== 'torrent'}
			<section class="dep" data-testid="dl-servers" aria-label="Servers">
				<h4>Supplied by</h4>
				<ul class="fl">{#each sampleServers(r) as sv (sv.name)}<li><span class="fn">{sv.name}</span><span class="fs">{bytes(sv.bytes)} · {sv.missing} missing</span></li>{/each}</ul>
			</section>
		{/if}
	{/if}
{/snippet}

{#snippet tabStrip(r: DownloadRow)}
	{@const tor = protocolOf(r) === 'torrent'}
	{@const tab = tabNow(r)}
	<div class="xtabs" role="tablist" aria-label="Details">
		{#each TABS as t (t)}
			{#if tor || (t !== 'Peers' && t !== 'Trackers')}
				<button type="button" role="tab" class="xtab" class:on={tab === t} aria-selected={tab === t} onclick={() => { xtabOn[r.id] = t; paneOpen = true; }} data-testid="dl-tab-{t.toLowerCase()}">{t}</button>
			{/if}
		{/each}
	</div>
{/snippet}

{#snippet pills()}
	{#each tally as c (c.key)}
		{@const t = tabs.find((x) => x.key === c.key)}
		{@const so = t ? subCounts(rows, t) : []}
		{@const open = on === c.key && so.length > 0}
		<div class="pw" class:on={on === c.key} class:open class:leaf={on === c.key && !subOn} data-key={c.key}>
			<button type="button" role="tab" class="tab" class:zero={c.count === 0} aria-selected={on === c.key} tabindex={on === c.key ? 0 : -1} data-nav data-testid="rail-item" data-key={c.key} data-count={c.count} onclick={() => choose(c.key)}><span class="lb">{c.label}</span><b class="n">{c.count}</b></button>
			{#if so.length}
				<div class="subwrap" inert={!open}>
					<div class="subin" role="group" aria-label="{c.label}: show" data-testid={open ? 'dl-subseg' : undefined}>
						{#each so as x (x.key)}
							<button type="button" class="stab" class:on={open && subOn === x.key} aria-pressed={open && subOn === x.key} tabindex="-1" data-nav onclick={() => { if (on !== c.key) choose(c.key); chooseSub(x.key); }} data-testid={open ? 'dl-subseg-item' : undefined} data-key={x.key}>{x.label}<b>{x.count}</b></button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/each}
{/snippet}

{#snippet tabsView(r: DownloadRow, simple: { label: string; value: string }[], im: ImportRecord | undefined)}
	{@const tor = protocolOf(r) === 'torrent'}
	{@const tab = tabNow(r)}
	{#if tab === 'Info'}
		{#if demoDetails}
			{@const sw = sampleSwarm(r)}
			<div class="gen" data-testid="dl-info">
				<dl class="kv">
					<dt class="gh">Transfer</dt><dd class="gh"></dd>
					<dt>Downloaded</dt><dd>{bytes((r.size || 0) * percent(r.progress) / 100)} of {bytes(r.size || 0)}</dd>
					<dt>Speed</dt><dd>{r.speed ? `${bytes(r.speed)}/s down` : '–'}{r.up_speed ? ` · ${bytes(r.up_speed)}/s up` : ''}</dd>
					<dt>Time left</dt><dd>{r.eta ? duration(r.eta) : '∞'}</dd>
					{#if tor}
						<dt>Share ratio</dt><dd>{sampleRatio(r)}</dd>
						<dt>Seeds</dt><dd>{sw.seedsOn} connected of {sw.seedsOf}</dd>
						<dt>Peers</dt><dd>{sw.peersOn} connected of {sw.peersOf}</dd>
						<dt>Wasted</dt><dd>{bytes(sw.wasted)}</dd>
					{/if}
				</dl>
				<dl class="kv">
					<dt class="gh">Properties</dt><dd class="gh"></dd>
					<dt>Size</dt><dd>{bytes(r.size || 0)}</dd>
					<dt>Added</dt><dd>{dateTime(r.started_at) || '–'}</dd>
					<dt>Done</dt><dd>{dateTime(r.finished_at) || '–'}</dd>
					<dt>Save path</dt><dd>{savePath(r, im)}</dd>
					{#if tor}
						<dt>Next tracker check</dt><dd>in {mmss(sw.nextCheck)}</dd>
						<dt>Private</dt><dd>{sw.private ? 'Yes' : 'No'}</dd>
					{/if}
					{#each simple.filter((f) => f.label === 'Downloader' || f.label === 'Requested by') as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}
				</dl>
			</div>
		{:else}
			<dl class="kv">{#each simple as f (f.label)}<dt>{f.label}</dt><dd>{f.value}</dd>{/each}</dl>
		{/if}
	{:else if !demoDetails}
		<p class="nr">Not reported yet</p>
	{:else if tab === 'Files'}
		{@const rowsT = fileTree(sampleFiles(r), (k) => !!closed[`${r.id}|${k}`])}
		<div class="tblw" data-testid="dl-filetable"><table class="t2">
			<thead><tr><th class="c-chk"><span class="sr">Download</span></th><th class="c-name">Name</th><th class="c-num c-size">Total size</th><th class="c-prog">Progress</th><th class="c-prio">Priority</th><th class="c-num c-rem">Remaining</th><th class="c-num c-av">Availability</th></tr></thead>
			<tbody>
				{#each rowsT as t, ri (t.key)}
					{@const pc = t.size ? Math.floor((t.done / t.size) * 100) : 0}
					{@const pr = t.dir ? '' : prioOf(r, t.key, t.i)}
					<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
					<tr class:sel={sel[`${r.id}:f`] === t.key} class:off={pr === 'Skip'} tabindex={ri === 0 ? 0 : -1} onclick={() => (sel[`${r.id}:f`] = t.key)} onkeydown={(e) => { trNav(e); if (e.key === 'Enter') sel[`${r.id}:f`] = t.key; }} data-testid="dl-filerow">
						<td class="c-chk">{#if !t.dir}<input type="checkbox" checked={pr !== 'Skip'} aria-label="Download {t.name}" onclick={(e) => e.stopPropagation()} onchange={(e) => (prio[`${r.id}|${t.key}`] = e.currentTarget.checked ? 'Normal' : 'Skip')} />{/if}</td>
						<td class="c-name"><span class="fnm" style="padding-left: {t.depth * 14}px">{#if t.dir}<button type="button" class="fold" class:shut={closed[`${r.id}|${t.key}`]} aria-label="{closed[`${r.id}|${t.key}`] ? 'Expand' : 'Collapse'} {t.name}" aria-expanded={!closed[`${r.id}|${t.key}`]} onclick={(e) => { e.stopPropagation(); closed[`${r.id}|${t.key}`] = !closed[`${r.id}|${t.key}`]; }}><Icon name="chevronDown" size={14} stroke={2.2} /></button>{:else}<span class="fold sp0"></span>{/if}<span class="fn" title={t.key}>{t.name}</span></span></td>
						<td class="c-num c-size">{bytes(t.size)}</td>
						<td class="c-prog"><span class="pbar"><i style="width: {pc}%"></i></span><span class="pv">{pc}%</span></td>
						<td class="c-prio">{#if !t.dir}<select class="pick" aria-label="Priority of {t.name}" value={pr} onclick={(e) => e.stopPropagation()} onchange={(e) => (prio[`${r.id}|${t.key}`] = e.currentTarget.value)}>{#each PRIOS as o (o)}<option value={o}>{o}</option>{/each}</select>{/if}</td>
						<td class="c-num c-rem">{bytes(Math.max(0, t.size - t.done))}</td>
						<td class="c-num c-av">{t.dir ? '' : fileExtras(r, t.i).availability.toFixed(2)}</td>
					</tr>
				{/each}
			</tbody>
		</table></div>
	{:else if tab === 'Peers'}
		<div class="tblw" data-testid="dl-peertable"><table class="t2">
			<thead><tr><th class="c-name">Address</th><th class="c-cl">Client</th><th class="c-prog">Progress</th><th class="c-num">↓</th><th class="c-num">↑</th><th class="c-fl">Flags</th></tr></thead>
			<tbody>
				{#each samplePeers(r) as p, ri (p.ip + p.client)}
					<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
					<tr class:sel={sel[`${r.id}:p`] === p.ip} tabindex={ri === 0 ? 0 : -1} onclick={() => (sel[`${r.id}:p`] = p.ip)} onkeydown={(e) => { trNav(e); if (e.key === 'Enter') sel[`${r.id}:p`] = p.ip; }} data-testid="dl-peerrow">
						<td class="c-name"><span class="fn">{p.ip}</span></td>
						<td class="c-cl"><span class="fn">{p.client}</span></td>
						<td class="c-prog"><span class="pbar"><i style="width: {Math.floor(p.progress * 100)}%"></i></span><span class="pv">{Math.floor(p.progress * 100)}%</span></td>
						<td class="c-num">{bytes(p.down)}/s</td>
						<td class="c-num">{bytes(p.up)}/s</td>
						<td class="c-fl">{flagsOf(p)}</td>
					</tr>
				{/each}
				{#if !samplePeers(r).length}<tr><td colspan="6" class="nr">No peers connected</td></tr>{/if}
			</tbody>
		</table></div>
	{:else if tab === 'Trackers'}
		<div class="tblw" data-testid="dl-trackertable"><table class="t2">
			<thead><tr><th class="c-name">Source</th><th>Status</th><th class="c-num">Peers</th><th class="c-num">Seeds</th><th class="c-msg">Message</th><th class="c-num">Next check</th></tr></thead>
			<tbody>
				{#each sampleTrackers(r) as t, ri (t.source)}
					<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
					<tr class:sel={sel[`${r.id}:t`] === t.source} tabindex={ri === 0 ? 0 : -1} onclick={() => (sel[`${r.id}:t`] = t.source)} onkeydown={(e) => { trNav(e); if (e.key === 'Enter') sel[`${r.id}:t`] = t.source; }} data-testid="dl-trackerrow">
						<td class="c-name"><span class="fn">{t.source}</span></td>
						<td><span class="st-{t.state}">{t.status}</span></td>
						<td class="c-num">{t.peers}</td>
						<td class="c-num">{t.seeds}</td>
						<td class="c-msg"><span class="fn">{t.message}</span></td>
						<td class="c-num">{t.next ? mmss(t.next) : '–'}</td>
					</tr>
				{/each}
			</tbody>
		</table></div>
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
	<section class="dpane" class:shut={!paneOpen} style={paneOpen ? `height: ${paneH}px` : ''} id="dlx-{r.id}" data-testid="dl-pane" aria-label="Details of {who(r)}">
		<div class="ph">
			<p class="xt pt"><b>{who(r)}</b><span>{statusWord(r)} · {Math.floor(percent(r.progress))}% · {sizeWord}</span></p>
			{@render tabStrip(r)}
			<button type="button" class="pcl" aria-expanded={paneOpen} aria-label={paneOpen ? 'Collapse details' : 'Expand details'} onclick={() => (paneOpen = !paneOpen)} data-testid="dl-pane-toggle"><Icon name="chevronDown" size={14} stroke={2.2} /></button>
		</div>
		{#if paneOpen}
		<div class="pb" data-testid="dl-panel" data-card="d">
			<span class="big" class:square={r.kind === 'artist'} data-testid="dl-big-poster"><Poster src={poster(r, titles)} name={titlesReady ? r.title || r.name : ''} /></span>
			<div class="xb">
				{@render tabsView(r, simple, im)}
				{@render depth(r)}
				{#if ds.length && !watch}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
				{@render btns(r, false)}
				{#if r.can_act && !watch && !inHistory(r)}
					<p class="xa"><button type="button" class="lk" disabled={busy === r.id} onclick={() => onfail(r)}>Mark failed</button></p>
				{/if}
			</div>
		</div>
		{/if}
	</section>
{/snippet}

<svelte:window onpointerdown={outside} onkeydown={listKey} />

<div class="dlgrid" bind:this={grid} style="--tpl: {tpl}; --c-in: {inner}px">
<div class="app" data-testid="dl-app" bind:this={appEl} style="--app-h: {appH}px">
<h1 class="sr">Downloads</h1>
<div class="strip hdr" data-testid="dl-strip">
	<div class="psearch" class:open={psOpen || !!dlScope.term} data-testid="dl-psearch">
		<button type="button" class="psb" aria-label="Search downloads" aria-expanded={psOpen || !!dlScope.term} onclick={() => { psOpen = true; tick().then(() => psEl?.focus()); }} data-testid="dl-psearch-btn"><Icon name="search" size={16} /></button>
		{#if psOpen || dlScope.term}
			<input type="search" bind:this={psEl} bind:value={dlScope.term} placeholder="Search downloads" aria-label="Search downloads" onblur={() => { if (!dlScope.term) psOpen = false; }} onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); dlScope.term = ''; psOpen = false; } }} data-testid="dl-psearch-input" />
			{#if dlScope.term}<button type="button" class="psx" aria-label="Clear search" onclick={() => { dlScope.term = ''; psEl?.focus(); }}><Icon name="x" size={14} /></button>{/if}
		{/if}
	</div>
	<DlBar bind:kindOn bind:srcOn {kindCount} {srcCount} actions={barActs} {watch} bind:term={dlScope.term} shown={searched.length} total={typed.length} side={dlTop.collapsed} sideControls={dlTop.controls} onside={() => dlTop.toggle()} searchKey={dlTop.searchKey} />
</div>
<div class="body">
<div class="col" bind:this={colEl}>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="tabrow" data-testid="dl-tabrow" onkeydown={navKey}><div class="tabs" role="tablist" aria-label="Status">{@render pills()}</div></div>
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="listbox" bind:this={listEl} data-testid="dl-listbox">
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
</div>
{#if openRow}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div class="dv" role="separator" aria-orientation="horizontal" aria-label="Resize details" aria-valuenow={paneOpen ? paneH : 0} tabindex="0" class:fixed={!paneOpen} onpointerdown={dragDown} onkeydown={dividerKey} data-testid="dl-divider"><i aria-hidden="true"></i></div>
	{@render panel(openRow)}
{/if}
</div>
</div>
<div class="sbar" data-testid="dl-statusbar"><div class="sbi">
	<span class="health" data-testid="sb-health"><i class="dot" class:ok={demoDetails} aria-hidden="true"></i><b>{demoDetails ? 'Connected' : 'Not reported'}</b></span>
	<span class="s-free" data-testid="sb-free"><span class="k">Free</span><b>{demoDetails ? bytes(sampleHeader.free) : '–'}</b></span>
	<span class="sd" data-testid="sb-down"><b data-testid="dl-speed">↓ {bytes(sum.speed)}/s</b></span>
	<span class="sd up" data-testid="sb-up"><b data-testid="dl-up">↑ {bytes(sum.up)}/s</b></span>
	<span class="gr s-gr"><SpeedGraph speed={sum.speed} up={sum.up} {history} width={84} height={14} /></span>
	<span class="s-tl"><span class="k">Left</span><b>{sum.eta ? duration(sum.eta) : '–'}</b></span>
	{#if problems}<button type="button" class="need" onclick={() => choose('problems')} data-testid="dl-need">⚠ {problems}<span class="nw"> need{problems === 1 ? 's' : ''} you</span></button>{/if}
	<span class="grow"></span>
	<span class="smore" data-testid="sb-more"><Popover label="More status" icon="more" placement="above" align="end" width={280}>
		{#snippet children()}
			<ul class="mo">
				<li class="m-free"><span class="k">Free space</span> {demoDetails ? bytes(sampleHeader.free) : '–'}</li>
				<li class="m-gr"><SpeedGraph speed={sum.speed} up={sum.up} {history} width={120} height={22} /></li>
				<li class="m-tl"><span class="k">Time left</span> {sum.eta ? duration(sum.eta) : '–'}</li>
				{#if demoDetails}
					<li data-testid="sb-session"><span class="k">This session</span> ↓ {bytes(sampleHeader.sessionDown)} · ↑ {bytes(sampleHeader.sessionUp)}</li>
					<li data-testid="sb-totals"><span class="k">Today</span> {bytes(sampleHeader.today)} · <span class="k">Month</span> {bytes(sampleHeader.month)} · <span class="k">All time</span> {bytes(sampleHeader.all)}</li>
					<li class="note" data-testid="sb-sample" title="The Downloader is not connected yet; these are made-up values.">Sample data</li>
				{/if}
			</ul>
		{/snippet}
	</Popover></span>
</div></div>
</div>

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
	.dline { display: grid; grid-template-columns: var(--tpl); align-items: center; column-gap: var(--gap); height: 44px; }
	.drow > .dline { cursor: pointer; }
	.drow > .dline:hover { background: var(--surface-2); }
	.drow > .dline:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }

	.bin:focus-visible, .lk:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.post { width: 26px; height: 39px; justify-self: center; overflow: hidden; border-radius: 3px; --poster-ratio: 2 / 3; }
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
	.st .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.st.paused .dot { background: var(--text-muted); }
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
	.grip { position: absolute; top: 4px; bottom: 4px; right: calc(-1 * var(--gap) / 2 - 6px); width: 12px; cursor: col-resize; touch-action: none; border-radius: 3px; z-index: 3; }
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
	.xb { min-width: 0; }
	.big { width: 72px; height: 108px; overflow: hidden; border-radius: 6px; box-shadow: 0 4px 14px rgb(0 0 0 / 0.18); --poster-ratio: 2 / 3; }
	.big.square { height: 72px; --poster-ratio: 1; }
	.big :global(.poster) { border-radius: 6px; }
	.post { transition: opacity var(--dur-fast) var(--ease-out); }
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
	.nr { margin: 8px 0 0; color: var(--text-muted); font: var(--t-footnote); }
	.xtabs { display: flex; flex-wrap: wrap; gap: 2px; padding: 2px; width: max-content; max-width: 100%; border-radius: var(--r-btn); background: var(--surface-2); }
	.xtab { min-height: 28px; padding: 0 12px; border: 0; border-radius: calc(var(--r-btn) - 2px); background: none; color: var(--text-muted); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xtab.on { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
	.xtab:focus-visible, .xbn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.fl { list-style: none; padding: 0; margin: 0; display: grid; gap: 4px; font: var(--t-footnote); min-width: 0; }
	.fl li { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; min-width: 0; }
	.fn { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fs { flex: none; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.lg { list-style: none; padding: 0; margin: 0; display: grid; gap: 3px; font: var(--t-caption); min-width: 0; max-height: 220px; overflow: auto; }
	.lg li { overflow-wrap: anywhere; }
	.lg time { color: var(--text-muted); font-variant-numeric: tabular-nums; margin-right: 6px; }
	.lg .bad { color: var(--fail); }
	.app { display: flex; flex-direction: column; height: var(--app-h, auto); min-height: 420px; min-width: 0; overflow: clip; }
	.body { display: grid; grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); flex: 1 1 auto; min-height: 0; }
	.listbox { min-width: 0; min-height: 0; overflow: auto; overscroll-behavior: contain; }
	.listbox .tw { overflow: visible; }
	.listbox :global(.chead) { position: sticky; top: 0; z-index: 4; background: var(--bg); }
	.sbar { flex: none; border-top: 1px solid var(--line); font-variant-numeric: tabular-nums; }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--wait); }
	.dot.ok { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 25%, transparent); }
	.gen { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 24px; margin-top: 10px; }
	.gen .kv { margin: 0; }
	.tblw { margin-top: 10px; max-width: 100%; overflow-x: auto; border: 1px solid var(--line); border-radius: 8px; }
	.fnm { display: inline-flex; align-items: center; gap: 4px; max-width: 100%; min-width: 0; }
	.fold { flex: none; display: inline-grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--text-muted); cursor: pointer; }
	.fold.sp0 { cursor: default; }
	.fold :global(svg) { transition: transform var(--dur-fast) var(--ease-out); }
	.fold.shut :global(svg) { transform: rotate(-90deg); }
	.pbar { display: inline-block; vertical-align: middle; width: 56px; height: 5px; border-radius: 3px; background: var(--surface-3); overflow: hidden; }
	.pbar i { display: block; height: 100%; background: var(--ok); }
	.pv { display: inline-block; min-width: 34px; margin-left: 6px; text-align: right; }
	.pick { max-width: 88px; padding: 2px 4px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface-1); color: var(--text); font: var(--t-footnote); }
	.pick:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hdr { flex: none; display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; margin: 0; padding: 3px var(--dl-gutter); border-bottom: 1px solid var(--line); background: none; }
	.col { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.col > .listbox { flex: 1 1 auto; }
	.drow.sel > .dline { position: relative; z-index: 1; background: color-mix(in srgb, var(--accent) 14%, var(--surface-1)); box-shadow: inset 3px 0 0 var(--accent), 0 5px 10px -6px rgb(0 0 0 / 0.4), 0 -5px 10px -6px rgb(0 0 0 / 0.22); }
	.drow.sel > .dline:hover { background: color-mix(in srgb, var(--accent) 18%, var(--surface-1)); }
	.drow.sel .tt { color: var(--accent-text, var(--text)); }
	.dv { flex: none; position: relative; height: 9px; border-top: 1px solid var(--line); cursor: row-resize; touch-action: none; }
	.dv.fixed { cursor: default; }
	.dv i { position: absolute; left: 50%; top: 3px; width: 38px; height: 3px; margin-left: -19px; border-radius: 2px; background: var(--line-strong, var(--text-muted)); opacity: 0.6; }
	.dv:hover i, .dv:focus-visible i { opacity: 1; }
	.dv:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dpane { flex: none; display: flex; flex-direction: column; min-height: 0; background: none; }
	.dpane.shut { height: auto; }
	.ph { display: flex; align-items: center; gap: 2px 14px; flex-wrap: wrap; padding: 3px var(--dl-gutter); border-bottom: 1px solid var(--line); min-width: 0; }
	.dpane.shut .ph { border-bottom: 0; }
	.pt { flex: 0 1 300px; min-width: 0; margin: 0; }
	.pt b, .pt span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ph .xtabs { margin: 0; background: none; padding: 0; }
	.pcl { margin-left: auto; display: grid; place-items: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.pcl :global(svg) { transition: transform var(--dur-fast) var(--ease-out); }
	.dpane.shut .pcl :global(svg) { transform: rotate(180deg); }
	.pcl:hover { background: var(--surface-3); color: var(--text); }
	.pcl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pb { display: grid; grid-template-columns: 56px minmax(0, 1fr); align-items: start; gap: 12px; flex: 1 1 auto; min-height: 0; padding: 8px var(--dl-gutter); overflow: auto; }
	.pb .big { width: 56px; height: 84px; }
	.pb .big.square { height: 56px; }
	.pb .kv { gap: 2px 12px; font: var(--t-caption); }
	.pb .xtab { min-height: 24px; padding: 0 9px; font: var(--t-caption); font-weight: 500; }
	.pb .t2 { font: var(--t-caption); }
	.pb .t2 th { padding: 3px 8px; }
	.pb .t2 td { padding: 2px 8px; }
	.pb .tblw { margin-top: 6px; }
	.pb .gen { gap: 6px 20px; margin-top: 6px; }
	.pb .dep { margin-top: 8px; padding: 6px 10px; }
	.pb .dep h4 { margin-bottom: 4px; }
	.pb .lg { font: var(--t-caption); }
	.pb .xbt { margin-top: 6px; }
	.pb .xbn { min-height: 26px; padding: 0 10px; font: var(--t-caption); font-weight: 500; }
	.note { padding: 1px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--text-muted); font: var(--t-caption); }
	.sbar { container: sb / inline-size; }
	.sbi { display: flex; flex-wrap: nowrap; align-items: center; gap: 2px 14px; padding: 3px var(--dl-gutter); min-width: 0; overflow: hidden; white-space: nowrap; font-size: 11px; line-height: 1.3; }
	.sbi > span { display: inline-flex; align-items: center; gap: 6px; }
	.sbi .grow { flex: 1 1 0; }
	.sbi .health { order: -1; }
	.sbi .need { margin-left: 0; }
	.sbi > .smore { display: inline-flex; }
	.mo { list-style: none; margin: 0; padding: 4px; display: grid; gap: 6px; font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.m-free, .m-gr, .m-tl { display: none; }
	@container sb (max-width: 900px) { .sbi > .s-free { display: none; } .m-free { display: block; } }
	@container sb (max-width: 720px) { .sbi > .s-gr { display: none; } .m-gr { display: block; } }
	@container sb (max-width: 560px) { .sbi > .s-tl { display: none; } .m-tl { display: block; } }
	@container sb (max-width: 560px) { .sbi > .s-free { display: none; } .m-free { display: block; } }
	@container sb (max-width: 520px) { .sbi > .s-gr { display: none; } .m-gr { display: block; } }
	@container sb (max-width: 460px) { .sbi > .s-tl { display: none; } .m-tl { display: block; } .need .nw { display: none; } }
	.sbi .smore :global(button) { width: 22px; height: 22px; min-width: 0; padding: 0; }
	.sbi .smore :global(svg) { width: 14px; height: 14px; }
	.sbi .need { min-height: 20px; padding: 0 6px; font: inherit; font-weight: 600; }
	.sbi .k { font: inherit; }
	.mo li.note { color: var(--text-muted); }
	.kcell, .pcell { display: flex; align-items: center; min-width: 0; }
	.kcell .kt { width: 100%; margin: 0; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pcell { justify-content: center; }
	.pm { display: inline-grid; place-items: center; width: 22px; height: 22px; margin: 0; border-radius: 6px; background: var(--surface-3); color: var(--text-muted); }
	.bar.paused i { background: var(--text-muted); }
	.dline :global(.num), .num { text-align: right; font-variant-numeric: tabular-nums; }
	.app { --dl-gutter: 10px; --dl-icon: 16px; --dl-icon-sm: 14px; }
	.app :global(svg.icon), .app :global(.ui-ibtn svg) { width: var(--dl-icon); height: var(--dl-icon); }
	.psearch { display: none; }
	.grip:active::after { background: var(--accent); width: 2px; }
	.dv:active i { opacity: 1; }
	.psb, .psx { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--text); cursor: pointer; }
	.psb:hover, .psx:hover { background: var(--surface-3); }
	.psb:focus-visible, .psx:focus-visible, .psearch input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.psearch input { width: 150px; height: 30px; padding: 0 8px; border: 1px solid var(--line); border-radius: 8px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); }
	/* The status pills: the picked pill carries its own background and opens to hold its sub-options. */
	.tabrow { display: flex; flex: none; align-items: center; height: 44px; min-width: 0; padding: 0 var(--dl-gutter); border-bottom: 1px solid var(--line); overflow-x: auto; overflow-y: hidden; overscroll-behavior-x: contain; scrollbar-width: none; }
	.tabrow::-webkit-scrollbar { display: none; }
	.tabs { display: flex; flex: none; align-items: center; gap: 2px; width: max-content; padding: 2px; border-radius: 999px; background: var(--surface-2); }
	.pw { display: flex; align-items: center; flex: none; border-radius: 999px; background: transparent; transition: background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.pw.on { background: var(--hl-bg); box-shadow: var(--hl-shadow, none); }
	.tab { display: inline-flex; align-items: center; flex: none; gap: 6px; height: 28px; padding: 0 12px; border: 0; border-radius: 999px; background: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-footnote); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out); }
	.tab .n { color: inherit; opacity: 0.75; font-weight: 600; font-variant-numeric: tabular-nums; }
	.tab:hover { color: var(--text); }
	.pw.on .tab { color: var(--hl-fg); }
	.tab.zero:not([aria-selected='true']) { opacity: 0.72; }
	.tab:focus-visible, .stab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.subwrap { display: grid; grid-template-columns: 0fr; opacity: 0; transition: grid-template-columns 200ms var(--ease-out), opacity 200ms var(--ease-out); }
	.pw.open .subwrap { grid-template-columns: 1fr; opacity: 1; }
	.subin { display: flex; align-items: center; gap: 2px; min-width: 0; overflow: hidden; white-space: nowrap; }
	.pw.open .subin { padding: 0 2px 0 6px; margin-right: 2px; border-left: 1px solid var(--hl-line); }
	.stab { display: inline-flex; align-items: center; flex: none; gap: 6px; height: 24px; padding: 0 10px; border: 0; border-radius: 999px; background: none; color: var(--hl-sub); font: var(--t-footnote); white-space: nowrap; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.stab b { color: inherit; opacity: 0.8; font-weight: 600; font-variant-numeric: tabular-nums; }
	.stab:hover { color: var(--hl-fg); }
	.stab.on { background: var(--hl-sub-on-bg); color: var(--hl-sub-on-fg); }
	/* No sub-option picked: the parent itself carries the red inner highlight. */
	.pw.leaf .tab { background: var(--hl-sub-on-bg); color: var(--hl-sub-on-fg); box-shadow: inset 0 0 0 2px var(--hl-bg); }
	/* The picked pill is raised on the track like a segmented control; its picked sub-option is tinted red. */
	.tabs { --hl-bg: var(--seg-thumb); --hl-fg: var(--text); --hl-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); --hl-line: var(--line-strong); --hl-sub: color-mix(in srgb, var(--text) 72%, var(--hl-bg)); --hl-sub-on-bg: var(--accent-soft); --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 85%, var(--text)); }
	/* Dark: the red words are lightened so they read on the grey pill (4.5:1 or more). */
	:global(:root[data-theme='dark']) .tabs { --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 25%, var(--text-on-red)); }
	@media (prefers-color-scheme: dark) { :global(:root:not([data-theme='light'])) .tabs { --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 25%, var(--text-on-red)); } }
	@media (prefers-reduced-motion: reduce) { .pw, .tab, .stab, .subwrap { transition: none; } }
	.dep { margin-top: 12px; padding: 10px 12px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); box-shadow: 0 1px 2px rgb(0 0 0 / 0.06), 0 6px 16px rgb(0 0 0 / 0.06); min-width: 0; }
	.dep h4 { margin: 0 0 6px; color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.dep .lg { max-height: none; margin-top: 8px; padding-left: 16px; list-style: decimal; }
	.story { border-color: color-mix(in srgb, var(--fail) 40%, var(--line)); }
	.src { margin-top: 8px; }
	.st-working { color: var(--ok); }
	.st-updating { color: var(--info); }
	.st-error { color: var(--fail); }
	.more { min-width: 36px; padding: 0 10px; }
	.mn { list-style: none; margin: 6px 0 0; padding: 4px; display: grid; gap: 2px; width: max-content; max-width: 100%; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); box-shadow: 0 6px 18px rgb(0 0 0 / 0.14); }
	.mi { display: flex; align-items: baseline; justify-content: space-between; gap: 14px; width: 100%; padding: 6px 10px; border: 0; border-radius: 6px; background: none; color: var(--text); font: var(--t-footnote); text-align: left; }
	.mi:disabled { opacity: 0.6; cursor: not-allowed; }
	.strip.sub { margin-top: -4px; font: var(--t-footnote); color: var(--text-muted); }
	.lim { display: inline-flex; align-items: center; gap: 8px; }
	.xbt { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
	.xbn { min-height: 32px; padding: 0 14px; border: 1px solid var(--surface-3); border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xbn:hover:not(:disabled) { background: var(--surface-3); }
	.xbn:disabled { opacity: 0.5; cursor: not-allowed; }
	.xa { margin: 8px 0 0; }
	.lk { min-height: 28px; padding: 0 4px; border: 0; border-radius: 6px; background: none; color: var(--accent-text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }

	.foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; margin-top: 10px; }
	.tr { display: inline-flex; align-items: center; gap: 8px; margin-bottom: 12px; }
	.tot { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }

	/* Phones: a fixed card per row (title, then bar, status and time); no picker. */
	@media (max-width: 640px) {
		.dlgrid { --gap: 8px; }
		.chead { display: none; }
		.tin { min-width: 0; }
		.dline { grid-template-columns: 8px 28px minmax(0, 1fr) 88px 52px 32px; grid-template-areas: 'tg post nm nm nm del' '. post bar st time del'; row-gap: 4px; height: auto; min-height: 62px; padding: 6px 0; }
		.dline > [data-col] { display: none; }
		.dline > .ph { display: none; }
		.dline > [data-col='poster'] { display: block; grid-area: post; align-self: start; }
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
		.mi { gap: 8px; }
		.fl li { flex-direction: column; gap: 0; }
		.fs { white-space: normal; }
		/* Phones: a search icon that opens into the field. */
		.strip { gap: 4px 14px; }
		.picker { display: none; }
	}
	@media (max-width: 767px) {
		.psearch { display: flex; flex: none; align-items: center; gap: 2px; }
		.dv { display: none; }
		.dpane { background: var(--surface-1); }
		.dpane { position: fixed; left: 0; right: 0; bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom)); z-index: 40; height: min(64dvh, 520px) !important; border-radius: 16px 16px 0 0; box-shadow: 0 -12px 32px rgb(0 0 0 / 0.32); }
		.dpane.shut { height: auto !important; }
		.pb { grid-template-columns: 56px minmax(0, 1fr); gap: 12px; }
		.pt { flex-basis: 100%; }
		.hdr { grid-template-columns: minmax(0, 1fr); }
			.gen { grid-template-columns: minmax(0, 1fr); }
	}
	@media (prefers-reduced-motion: reduce) {
		.bar i, .tg :global(svg), .bin, .cw, .post { transition: none; }
	}
</style>
