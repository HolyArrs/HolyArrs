<script lang="ts">
	// The Downloads page body, laid out like a Downloader's own web page: a
	// header strip (speed with a small graph, time left), the status tabs
	// (Active · Waiting · In progress · Problems · Done, with sub-tabs; see
	// status-tabs.ts), then one list filtered by them. Every row shares one
	// column grid: ▾ · poster · the picked columns · bin (columns.ts
	// holds which columns exist and the saved layout). ▾ opens a row's details in place; the bin removes it
	// (through the page's Remove dialog).
	import { tick, untrack } from 'svelte';
	import { mergeDetail } from './detail-merge.ts';
	import { page } from '$app/state';
	import { COL, COLUMNS, IDS, KEY, MAX_W, MIN_W, OLD_KEY, defaults, dump, load, move, moveVisible, nextSort, quality, ratioText, shortAge, sortRows, swarmNum, toggle as toggleCol, visible, type ColId, type Layout, type SwarmFacet } from './columns.ts';
	import { goto, replaceState } from '$app/navigation';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import Poster from '../ui/Poster.svelte';
	import Icon from '../ui/Icon.svelte';
	import Chip from '../ui/Chip.svelte';
	import Menu, { type MenuItem } from '../ui/Menu.svelte';
	import { PRIORITIES, clientOk, paneExtras, rowItems, usenetQueued, type Priority, type RowItem } from './rowmenu.ts';
	import TopMenu from './TopMenu.svelte';
	import { to } from '../core/paths.ts';
	import DlBar from './DlBar.svelte';
	import DecisionPanel from './DecisionPanel.svelte';
	import DlStatus from './DlStatus.svelte';
	import DlTabsView from './DlTabsView.svelte';
	import DlMorePop from './DlMorePop.svelte';
	import { createColumns } from './columns-grid.svelte.ts';
	import { Downloaders } from './clients.svelte.ts';
	import { PaneDetail } from './pane-detail.svelte.ts';
	import { CONN_HINT, CONN_WORD, tabsOf, type PaneTab } from './pane-facts.ts';
	import GhostEmpty from './GhostEmpty.svelte';
	import { reviewCount, reviewWord } from './status-views.ts';
	import { listEsc, reviewShown, searchShortcut } from './keys.ts';
	import Popover from '../ui/Popover.svelte';
	import { tip } from './tip.ts';
	import { overlayScroll } from './overlay-scroll.ts';
	import { dlScope } from '../core/search-field.svelte.ts';
	import { dlTop } from './dl-top.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { fileTree } from './filetree.ts';
	import { api, message, ApiError } from '../core/api.ts';
	import Confirm from '../ui/Confirm.svelte';
	import { ago, dateTime, duration, percent } from '../core/format.ts';
	import { session } from '../core/session.svelte.ts';
	import { healthLine, quietPeers, logDays, badKind, clientMark, swarmLine, swarmTotal } from './panel-words.ts';
	// Each client's own app icon (design/assets/clients, licenses in its README).
	const CLIENT_ICONS = new Set(['qbittorrent', 'transmission', 'deluge', 'sabnzbd', 'nzbget']);
	import { size as bytes, categoryWords, protocolOf, KIND_TABS, KIND_WORD, type KindKey } from './size.ts';
	import { kindWords } from './downloaders.ts';
	import { reducedMotion } from '../core/motion.ts';
	import { DUR, easeIn, easeOut, sheetIn, sheetOut } from '../core/transitions.ts';
	import { kindOf, manyWords, rowName, rowPath } from './downloaders.ts';
	import { type Decision, type DownloadRow } from '../core/types.ts';
	import type { TitleView } from '../core/types.ts';
	import type { ImportRecord } from './types.ts';
	import { ALL, counts, emptyOf, pick, showOf, shortReason, stageOf, statusHint, statusWord, stepLine, subCounts, tabsFor } from './status-tabs.ts';
	import { statusDetail } from './imports.ts';
	import { controllable, pausable, repairSteps, retryable, serverRows, type UsenetView } from './usenet.ts';
	import { poster, who } from './row-art.ts';
	import { byTracker, countLabel, filterFiles, indexesUnder, limitsWords, pickRange, problemWords, splitTrackers, toolbarTargets, trackerOptions, type BulkAction, type SteerMode } from './manage.ts';
	import { longPress, phoneTitle, subLine } from './phone.ts';
	import { problem } from './problem.ts';
	import { barTone, facts, historyStatus, historyTotals, inHistory, matches, moving, sizeLine, totals } from './sections.ts';

	type ClientAction = 'pause' | 'resume' | 'top' | 'recheck' | 'reannounce' | 'retry';
	let {
		rows: everyRow,
		notes = [],
		importOf,
		titles,
		titlesReady = true,
		busy,
		torrent = false,
		decisionsOf,
		openId = $bindable(0),
		onfail,
		onremove,
		ondone,
		onact,
		onall,
		onlimit,
		onadd,
		onremovemany,
		onretry,
		onbulk,
		onsteer,
		changeOutside = true,
		batchBusy = false,
		watch = false,
		upTotal
	}: {
		rows: DownloadRow[];
		/** Calm notes from the page (a Downloader not answering, a side read that failed): shown in the status popover, with a dot on its trigger. */
		notes?: string[];
		importOf: Map<number, ImportRecord>;
		titles: Map<number, TitleView>;
		/** False while the Library titles (the posters) are still on their way: posters show a plain tile, not initials. */
		titlesReady?: boolean;
		busy: number;
		/** A torrent Downloader brings the Seeding sub-tab under Done. */
		torrent?: boolean;
		decisionsOf: (r: DownloadRow) => Decision[];
		/** The row whose details are open (0: none). */
		openId?: number;
		onfail: (r: DownloadRow) => void;
		onremove: (r: DownloadRow, withFiles?: boolean) => void;
		ondone: () => void;
		/** A client action on one download; the new state comes back from the client. */
		onact?: (r: DownloadRow, action: ClientAction) => void;
		/** Pause (true) or resume (false) everything in every client. */
		onall?: (pause: boolean) => void;
		/** Opens the speed limit. */
		onlimit?: () => void;
		/** Opens the Add sheet (managers with a torrent Downloader only). */
		onadd?: () => void;
		/** Opens the Remove dialog for the selected rows. */
		onremovemany?: (rows: DownloadRow[], withFiles?: boolean) => void;
		/** Try again for failed Usenet jobs. */
		onretry?: (rows: DownloadRow[]) => void;
		/** One action on the selected rows (batched per Downloader by the page). */
		onbulk?: (rows: DownloadRow[], action: BulkAction) => void;
		/** Opens the steering sheet (seeding rules and more) for these torrents. */
		onsteer?: (rows: DownloadRow[], mode: SteerMode) => void;
		/** The settings permission: changes to torrents HolyArrs did not send need it (the server refuses them otherwise). */
		changeOutside?: boolean;
		/** A selection action is on its way: the selection bars wait for it. */
		batchBusy?: boolean;
		/** Watch only: someone who may not manage the Library sees their own downloads with no actions. */
		watch?: boolean;
		/** Upload speed the server added up over every sending row (seeding too, and rows past the page); else the rows on hand. */
		upTotal?: number;
	} = $props();

	// The Type filter (All, Movies, TV shows, Music) narrows every tab and total.
	let kindOn = $state<KindKey>('');
	const typed = $derived(kindOn ? everyRow.filter((r) => kindOf(r) === kindOn) : everyRow);
	// The search narrows every tab too, so the tab counts follow it.
	// The search is the Downloads search field (DlFind), scoped to this page (search-field.svelte.ts).
	const q = $derived(dlScope.term);
	const searched = $derived(q.trim() ? typed.filter((r) => matches(r, q)) : typed);
	// The left rail: a status, a source (Usenet or Torrent); the Type filter is its Categories.
	let srcOn = $state<'' | 'usenet' | 'torrent'>('');
	const bySource = $derived(srcOn ? searched.filter((r) => protocolOf(r) === srcOn) : searched);
	// The tracker filter: one tracker host, or a tracker problem; counts follow the other filters.
	let trackOn = $state('');
	const trackOpts = $derived(trackerOptions(bySource));
	const trackHosts = $derived(trackOpts.filter((o) => o.key.startsWith('host:')));
	const trackProblems = $derived(bySource.filter((r) => r.tracker_problem).length);
	const trackKey = $derived(trackOn === 'p:any' ? (trackProblems ? trackOn : '') : trackOpts.some((o) => o.key === trackOn) ? trackOn : '');
	const rows = $derived(trackKey ? bySource.filter((r) => byTracker(r, trackKey)) : bySource);
	const kindCount = (k: string) => everyRow.filter((r) => kindOf(r) === k).length;
	const srcCount = (k: string) => everyRow.filter((r) => protocolOf(r) === k).length;

	// The picked tab (and sub-tab) lives in the address: &show=tab or tab.sub.
	const isPaused = (r: DownloadRow | undefined) => r?.state === 'paused';

	const dls = new Downloaders(() => watch);
	const tabNow = (r: DownloadRow): PaneTab => {
		const t = tabOf(r.id);
		return tabsOf(r).includes(t) ? t : 'Info';
	};
	let xtabOn = $state<Record<number, PaneTab>>({});
	const tabOf = (id: number) => xtabOn[id] ?? 'Info';
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
	const tabOrder = $derived(isDone ? [...shown].sort(newest) : shown);
	// A sorted column (kept with the column layout) overrides the tab's own order.
	const time = (v: string | undefined) => (v ? Date.parse(v) : undefined);
	const list = $derived.by(() => {
		const so = cols.layout.sort;
		return so && cols.vis.includes(so.id) ? sortRows(tabOrder, (r) => sortKey(r, so.id), so.desc) : tabOrder;
	});
	function sortKey(r: DownloadRow, id: ColId): number | string | undefined {
		const tor = r.protocol === 'torrent';
		switch (id) {
			case 'title': return rowName(r);
			case 'kind': return kindOf(r) || categoryWords(r.category) || '';
			case 'proto': return protocolOf(r);
			case 'progress': return r.progress;
			case 'status': return statusWord(r);
			case 'size': return r.size || undefined;
			case 'quality': return quality(r.name);
			case 'added': return time(r.started_at);
			// Swarm sorts by the facet picked in its header: seeds, peers or ratio.
			case 'swarm': {
				if (!tor) return undefined;
				const f = cols.layout.sort?.facet ?? 'seeds';
				return f === 'ratio' ? (r.ratio ?? undefined) : f === 'peers' ? (r.peers ?? 0) : (r.seeds ?? 0);
			}
			case 'tracker': return r.tracker;
			case 'requested': return r.requested_by;
			case 'source': return r.source_name;
			case 'category': return categoryWords(r.category);
		}
		return undefined;
	}
	const sum = $derived(totals(allQueue));
	// Upload comes from the Downloaders themselves once they report: seeding uploads without any downloading row.
	const upSum = $derived(dls.reporting.length ? dls.sumOf('up_speed') : (upTotal ?? sum.up));
	const openRow = $derived(everyRow.find((r) => r.id === openId));
	// The Downloaders' own caps: the tightest one is the Speed limit button's word.
	const capNow = $derived(Math.min(...dls.reporting.map((c) => c.status?.speed_limit ?? 0).filter((n) => n > 0)));
	const toReview = $derived(reviewCount(allQueue));
	// Time left: the longest of the rows' own estimates and a Usenet queue's.
	const etaAll = $derived(Math.max(sum.eta || 0, ...dls.clients.map((c) => (c.error ? 0 : (c.status?.time_left ?? 0)))));
	const etaWords = $derived(etaAll ? duration(etaAll) : '');
	// Usenet has no upload: with only Usenet Downloaders the Up pill goes.
	const noUp = $derived(dls.loaded && dls.clients.length > 0 && !dls.clients.some((c) => c.protocol === 'torrent'));
	/** The review call opens Problems › Review. */
	function openReview() {
		choose('problems');
		sub = 'decide';
		remember();
	}
	const done = $derived(historyTotals(typed.filter(inHistory)));

	function toggle(id: number) {
		openId = id;
		paneOpen = true;
	}
	// The selection: Ctrl or Cmd click toggles a row, Shift click picks a range
	// from the last picked one, Ctrl+A picks every row in the list.
	let picked = $state<Set<number>>(new Set());
	// Phones: "Select" in the All downloads menu starts picking before any row is picked.
	let selecting = $state(false);
	const choosing = $derived(picked.size > 0 || selecting);
	let anchor = 0;
	const pickedRows = $derived(list.filter((r) => picked.has(r.id)));
	// The toolbar's Start, Pause and Remove mean the ticked rows, else the open row.
	// Start and Pause follow pausable, as the pane's own button does (NZBGet takes those two).
	const able = (r: DownloadRow) => pausable(r) && !r.client_locked && busy !== r.id;
	const startT = $derived(toolbarTargets(pickedRows, openRow, 'paused', able));
	const pauseT = $derived(toolbarTargets(pickedRows, openRow, 'running', able));
	const removeT = $derived(toolbarTargets(pickedRows, openRow, 'any', (r) => busy !== r.id));
	$effect(() => {
		// Rows that left the list leave the selection.
		const ids = new Set(list.map((r) => r.id));
		if ([...picked].some((id) => !ids.has(id))) picked = new Set([...picked].filter((id) => ids.has(id)));
	});
	function pickOne(id: number, on?: boolean) {
		const n = new Set(picked);
		if (on ?? !n.has(id)) n.add(id);
		else n.delete(id);
		picked = n;
		anchor = id;
	}
	function pickTo(id: number) {
		picked = new Set([...picked, ...pickRange(list.map((r) => r.id), anchor || id, id)]);
	}
	const clearPicked = () => {
		picked = new Set();
		selecting = false;
	};
	// Torrents a torrent Downloader can steer by hash: seeding rules and more.
	// The row ⋯ menu: one place for a row's actions, the same items on desktop and phone.
	const ROW_WORD: Record<RowItem, string> = { pause: 'Pause', resume: 'Start', retry: 'Download again', up: 'Move up', down: 'Move down', top: 'Move to top', bottom: 'Move to bottom', priority: 'Priority', library: 'Open in Library', remove: 'Remove' };
	// Usenet jobs move and take a priority one job at a time (no bulk call).
	const usenetItem = (r: DownloadRow) => `/downloaders/${r.downloader_id}/items/${encodeURIComponent(r.hash ?? '')}`;
	const MOVED: Record<string, string> = { up: 'Moved up', down: 'Moved down', top: 'Moved to top', bottom: 'Moved to bottom' };
	let queueBusy = $state(0);
	async function usenetCall(r: DownloadRow, run: () => Promise<unknown>, done: string) {
		queueBusy = r.id;
		try {
			await run();
			toasts.push(done, 'ok');
		} catch (e) {
			toasts.push("Couldn't update download", 'fail', { body: message(e) });
		} finally {
			if (queueBusy === r.id) queueBusy = 0;
		}
	}
	const usenetMove = (r: DownloadRow, k: string) => usenetCall(r, () => api.post(`${usenetItem(r)}/${k}`), MOVED[k] ?? 'Moved');
	const usenetPriority = (r: DownloadRow, p: Priority, label: string) => usenetCall(r, () => api.post(`${usenetItem(r)}/priority`, { priority: p }), `Priority set to ${label}`);
	// menuItems turns row keys into menu entries; the row ⋯ and the pane ⋯ share it.
	function menuItems(r: DownloadRow, keys: RowItem[]): MenuItem[] {
		const off = busy === r.id || queueBusy === r.id;
		return keys.flatMap((k): MenuItem | MenuItem[] => {
			const label = ROW_WORD[k];
			if (k === 'priority') return [{ kind: 'heading', label }, ...PRIORITIES.map((p): MenuItem => ({ label: p.label, disabled: off, onselect: () => void usenetPriority(r, p.key, p.label) }))];
			if ((k === 'up' || k === 'down' || k === 'top' || k === 'bottom') && usenetQueued(r)) return { label, disabled: off, onselect: () => void usenetMove(r, k) };
			if (k === 'library') return { label, icon: 'film', href: to(`/library/${r.title_id}`) };
			if (k === 'remove') return { label, icon: 'trash', destructive: true, disabled: off, onselect: () => onremove(r) };
			if (k === 'pause' || k === 'resume') return { label, icon: k === 'pause' ? 'pause' : 'play', disabled: off, onselect: () => onact?.(r, k) };
			if (k === 'retry') return { label, icon: 'refresh', disabled: off, onselect: () => (onretry ? onretry([r]) : onact?.(r, 'retry')) };
			return { label, disabled: off, onselect: () => onbulk?.([r], k) };
		});
	}
	function rowMenu(r: DownloadRow): MenuItem[] {
		const items = menuItems(r, rowItems(r).filter((k) => k !== 'retry' || mayEdit(r)));
		if (!r.can_act) items.unshift({ kind: 'heading', label: 'HolyArrs cannot change this one' });
		return items;
	}
	// The pane's ⋯: the row's own list minus what the pane shows as buttons
	// (pause, try again, open, remove), plus a torrent's checks, force start and
	// sheets, and Mark failed last as the one destructive choice.
	const PANE_BUTTONS = new Set<RowItem>(['pause', 'resume', 'retry', 'library', 'remove']);
	function paneMenu(r: DownloadRow): MenuItem[] {
		// A read-only row has nothing to offer here; the Remove button's title already says so.
		if (!r.can_act) return [];
		const off = busy === r.id || queueBusy === r.id;
		const items = menuItems(r, rowItems(r).filter((k) => !PANE_BUTTONS.has(k)));
		const extra = new Set(paneExtras(r, { steer: !!onsteer && mayEdit(r), fail: !watch && !inHistory(r) && !r.client_item }));
		if (extra.has('recheck')) items.push({ label: 'Check again', disabled: off, onselect: () => onact?.(r, 'recheck') }, { label: 'Ask trackers again', disabled: off, onselect: () => onact?.(r, 'reannounce') });
		if (extra.has('force')) items.push({ label: 'Force start', disabled: off, onselect: () => onbulk?.([r], 'force') });
		if (extra.has('steer')) {
			items.push({ kind: 'separator' });
			for (const m of STEER) items.push({ label: m.label, disabled: off, onselect: () => onsteer?.([r], m.mode) });
		}
		if (extra.has('fail')) {
			if (items.length) items.push({ kind: 'separator' });
			items.push({ label: 'Mark failed', destructive: true, disabled: off, onselect: () => onfail(r) });
		}
		return items;
	}
	// Only qBittorrent takes location, speed and seeding changes; read-only rows never do.
	const steerable = (r: DownloadRow) => !!onsteer && mayEdit(r) && !!r.downloader_id && !!r.hash && protocolOf(r) === 'torrent' && controllable(r.downloader_kind);
	// Location, speed and seeding sheets, file priority, tracker edits and Download again on a torrent
	// HolyArrs did not send need the settings permission, as on the server.
	const mayEdit = (r: DownloadRow) => clientOk(r) && (!r.outside || changeOutside);
	// The selection bars send only rows HolyArrs may change, so one read-only row cannot fail the rest.
	const actable = (rows: DownloadRow[]) => rows.filter(clientOk);
	// The selection bar's checks and queue moves are torrent calls: only shown when a ticked torrent takes them.
	const bulkRows = $derived(actable(pickedRows).filter((r) => protocolOf(r) === 'torrent' && controllable(r.downloader_kind) && !!r.downloader_id && !!r.hash));
	const STEER: { mode: SteerMode; label: string; short: string }[] = [
		{ mode: 'location', label: 'Set location…', short: 'Set location' },
		{ mode: 'speed', label: 'Speed limits…', short: 'Speed limits' },
		{ mode: 'seed', label: 'Seeding rules…', short: 'Seeding rules' }
	];
	// The phone selection bar: the few actions that fit a thumb.
	const PHONE_BULK: { action: BulkAction; label: string; icon: 'play' | 'pause' | 'refresh' }[] = [{ action: 'start', label: 'Start', icon: 'play' }, { action: 'pause', label: 'Pause', icon: 'pause' }, { action: 'recheck', label: 'Check again', icon: 'refresh' }];
	// The phone bar's "All downloads" menu, through the same handlers as the toolbar's menus.
	const allItems = $derived(watch ? [] : [{ label: 'Select', onselect: () => (selecting = true) }, { label: 'Start all', onselect: () => onall?.(false) }, { label: 'Pause all', onselect: () => onall?.(true) }, { label: 'Speed limit…', onselect: () => onlimit?.() }, { label: 'Held files', onselect: () => void goto(to('/library/held')) }]);
	// The desktop selection bar: what the toolbar's Start, Pause and Remove do not cover.
	const BULK: { action: BulkAction; label: string }[] = [
		{ action: 'recheck', label: 'Check again' }, { action: 'reannounce', label: 'Ask trackers again' }, { action: 'top', label: 'Move to top' },
		{ action: 'up', label: 'Move up' }, { action: 'down', label: 'Move down' }, { action: 'bottom', label: 'Move to bottom' }, { action: 'force', label: 'Force start' }
	];
	// The detail pane under the list: its height is kept in this browser.
	const PANE_KEY = 'holyarrs.downloads.pane';
	let paneH = $state(260);
	// The pane takes the height its content needs (capped so the list keeps rows); a drag on the
	// divider sets a height by hand, a double-click goes back to fitting.
	let paneFit = $state(true);
	let paneOpen = $state(true);
	let grabY: number | null = null;
	const openTab = $derived(openRow ? tabNow(openRow) : '');
	const pane = new PaneDetail({ openId: () => openId, openTab: () => openTab, paneOpen: () => paneOpen, rows: () => everyRow, watch: () => watch });
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
	// Several torrents go through one bulk call; a single torrent and every Usenet job (which the
	// bulk call cannot address) go through their own call, so none is dropped.
	const runOn = (t: { rows: DownloadRow[] }, one: 'resume' | 'pause', many: BulkAction) => {
		const tor = t.rows.filter((r) => protocolOf(r) === 'torrent' && !!r.downloader_id && !!r.hash);
		if (tor.length > 1) onbulk?.(tor, many);
		for (const r of t.rows) if (tor.length < 2 || !tor.includes(r)) onact?.(r, one);
	};
	const barActs = $derived([
		...(onadd ? [{ key: 'add', icon: 'plus' as const, label: 'Add', hint: 'Add torrents by link or file', disabled: false, run: () => onadd?.() }] : []),
		{ key: 'start', icon: 'play' as const, label: countLabel('Start', startT.rows.length), hint: startT.why ? `Start (${startT.why})` : countLabel('Start', startT.rows.length), disabled: !startT.rows.length, run: () => runOn(startT, 'resume', 'start'), more: [{ label: 'Start all', onselect: () => onall?.(false) }] },
		{ key: 'pause', icon: 'pause' as const, label: countLabel('Pause', pauseT.rows.length), hint: pauseT.why ? `Pause (${pauseT.why})` : countLabel('Pause', pauseT.rows.length), disabled: !pauseT.rows.length, run: () => runOn(pauseT, 'pause', 'pause'), more: [{ label: 'Pause all', onselect: () => onall?.(true) }] },
		{ key: 'remove', icon: 'trash' as const, label: countLabel('Remove', removeT.rows.length), hint: removeT.why ? `Remove (${removeT.why})` : countLabel('Remove', removeT.rows.length), disabled: !removeT.rows.length, run: () => (removeT.rows.length === 1 ? onremove(removeT.rows[0]) : onremovemany?.(removeT.rows)) },
	]);
	const maxPane = () => Math.max(160, (colEl?.clientHeight ?? 600) - 140);
	function dragDown(e: PointerEvent) {
		if (e.button !== 0 || !paneOpen) return;
		e.preventDefault();
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		const y0 = e.clientY;
		const pane = el.parentElement?.querySelector<HTMLElement>('.dpane');
		const h0 = paneFit && pane ? pane.offsetHeight : paneH;
		paneFit = false;
		paneH = h0;
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
		const pane = (e.currentTarget as HTMLElement).parentElement?.querySelector<HTMLElement>('.dpane');
		const h0 = paneFit && pane ? pane.offsetHeight : paneH;
		paneFit = false;
		paneH = Math.max(120, Math.min(maxPane(), h0 + (e.key === 'ArrowUp' ? 24 : -24)));
		savePane();
	}
	// The underline under the picked panel tab: measured once per change, moved with transform only.
	function ink(node: HTMLElement, _key: string) {
		const set = () => {
			const a = node.querySelector<HTMLElement>('[aria-selected="true"]');
			if (!a) return;
			node.style.setProperty('--ix', `${a.offsetLeft}px`);
			node.style.setProperty('--iw', String(a.offsetWidth));
		};
		set();
		const ro = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(set);
		ro?.observe(node);
		return { update: () => requestAnimationFrame(set), destroy: () => ro?.disconnect() };
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
		// Only the pills themselves move with the arrows; the Review button and the status popover keep theirs.
		if (!(e.target as HTMLElement | null)?.closest('[data-nav]')) return;
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
		const t = e.target as HTMLElement | null;
		const inField = !t || !!t.closest('input, textarea, select, [contenteditable="true"]');
		// A dialog, sheet or open popover keeps its focus: no stealing it for the search.
		const box = '[role="dialog"], dialog, [popover]:popover-open';
		const inDialog = !!t?.closest?.(box) || !!(document.activeElement as HTMLElement | null)?.closest?.(box) || !!document.querySelector('dialog[open]');
		if (!searchShortcut(e, inField, inDialog)) return;
		e.preventDefault();
		e.stopImmediatePropagation();
		if (matchMedia('(max-width: 640px)').matches) {
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
	function listKey(e: KeyboardEvent) {
		// A menu or sheet already handled this key: the list behind it stays put.
		if (e.defaultPrevented) return;
		// Ctrl+F jumps to the files filter only from inside the pane; elsewhere the browser's Find stays.
		if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'F') && pane.fqEl && openRow && paneOpen && tabNow(openRow) === 'Files' && (e.target as HTMLElement | null)?.closest('.dpane')) {
			e.preventDefault();
			pane.fqEl.focus();
			pane.fqEl.select();
			return;
		}
		if (!watch && (e.ctrlKey || e.metaKey) && (e.key === 'a' || e.key === 'A')) {
			const t = e.target as HTMLElement | null;
			if (t?.closest('[data-dl-id], .dlrows, [data-testid="dl-listbox"]') && !t.closest('input, textarea, select, [contenteditable="true"]')) {
				e.preventDefault();
				picked = new Set(list.map((r) => r.id));
			}
			return;
		}
		if (e.key === 'Escape' && listEsc(choosing ? Math.max(1, picked.size) : 0, !!openId) === 'clear' && !(e.target as HTMLElement | null)?.closest('input, select, textarea, dialog, [role="dialog"], [role="menu"], [popover]')) {
			e.preventDefault();
			clearPicked();
			return;
		}
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp' && e.key !== 'Escape' && e.key !== 'Delete') return;
		const t = e.target as HTMLElement | null;
		if (!t || t.closest('input, select, textarea, [contenteditable="true"], [role="tab"], [role="separator"], dialog, [role="dialog"], [role="menu"], [popover]')) return;
		if (e.key === 'Escape') {
			if (listEsc(picked.size, !!openId) === 'close') {
				// Escape from inside the pane: the focus goes back to the row the pane was about, never to nowhere.
				const line = t.closest('.dpane') ? cols.listEl?.querySelector<HTMLElement>(`[data-dl-id="${openId}"] .dline`) : null;
				openId = 0;
				e.preventDefault();
				line?.focus();
			}
			return;
		}
		if (e.key === 'Delete') {
			if (t.closest('button, a')) return;
			if (pickedRows.length && onremovemany) {
				e.preventDefault();
				onremovemany(pickedRows, e.shiftKey);
				return;
			}
			const target = focusedRow();
			if (!target) return;
			e.preventDefault();
			if (target.can_act) onremove(target, e.shiftKey);
			else toasts.push("Can't remove this download", 'info', { body: 'Not from here' });
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
		cols.reveal(n);
		tick().then(() => {
			const row = cols.grid?.querySelector<HTMLElement>(`[data-dl-id="${r.id}"] > .dline`);
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
	const small = (id: number) => () => cols.grid?.querySelector(`[data-dl-id="${id}"] > .dline .post`);
	function xOut(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
	const cols = createColumns({ isDone: () => isDone, list: () => list });
	// Select mode ends when the screen leaves phone width or watch mode starts.
	let wasPhone = false;
	$effect(() => {
		const p = cols.phone;
		if ((wasPhone && !p) || watch) untrack(() => (selecting = false));
		wasPhone = p;
	});
	// The pane's tabs work like the status tabs: one Tab stop, Left and Right (Home, End) switch.
	function paneTabKey(e: KeyboardEvent, r: DownloadRow) {
		const tabs = [...(e.currentTarget as HTMLElement).parentElement!.querySelectorAll<HTMLElement>('[role="tab"]')];
		const i = tabs.indexOf(e.currentTarget as HTMLElement);
		const j = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : e.key === 'Home' ? 0 : e.key === 'End' ? tabs.length - 1 : -2;
		if (j === -2 || !tabs[j]) return;
		e.preventDefault();
		tabs[j].click();
		tick().then(() => tabs[j].focus());
	}
	// A click anywhere on a row opens or closes its details, as the ▾ does;
	// not on the bin, a link, a control, or while selecting text.
	function rowClick(e: MouseEvent, id: number) {
		const t = e.target as HTMLElement;
		if (t.closest('button, a, input, label, .grip, [data-col="bin"]')) return;
		if (!watch && (e.ctrlKey || e.metaKey)) {
			e.preventDefault();
			pickOne(id);
			return;
		}
		if (!watch && e.shiftKey) {
			e.preventDefault();
			getSelection()?.removeAllRanges();
			pickTo(id);
			return;
		}
		// Phones: once a long press started picking, a tap picks or unpicks a row.
		if (!watch && cols.phone && choosing) {
			pickOne(id);
			return;
		}
		if ((getSelection()?.toString() ?? '').length > 0) return;
		anchor = id;
		toggle(id);
	}
	function rowKey(e: KeyboardEvent, id: number) {
		if (e.target !== e.currentTarget) return;
		if (e.key === ' ') {
			// Space ticks the row for the toolbar and selection bar (Shift+Space ticks up to it); Enter opens it.
			e.preventDefault();
			if (watch) return;
			if (e.shiftKey) pickTo(id);
			else pickOne(id);
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
	<span class="tg">{#if !watch}<input type="checkbox" class="pk" class:any={choosing} tabindex="-1" checked={picked.has(r.id)} aria-label="Select {who(r)}" onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); pickOne(r.id); } else if (e.key === ' ') e.stopPropagation(); }} onclick={(e) => { e.stopPropagation(); if (e.shiftKey) { e.preventDefault(); pickTo(r.id); } else pickOne(r.id, e.currentTarget.checked); }} data-testid="dl-pick" />{/if}</span>
{/snippet}

{#snippet cell(r: DownloadRow, id: ColId, ph: boolean, hist: boolean)}
	{#if id === 'poster'}
		<span class="post" class:square={r.kind === 'artist'} class:ph data-col="poster"><Poster src={poster(r, titles)} name={titlesReady ? r.title || r.name.replace(/[._]+/g, ' ') : ''} /></span>
	{:else if id === 'kind'}
		{@const kw = kindOf(r) ? KIND_WORD[kindOf(r) as 'movie'] : categoryWords(r.category)}{@const ki = cols.kindIcon(kw)}<span class="kcell" class:ph data-col="kind"><span class="kt" class:none={!ki} class:wide={cols.kindWide} data-testid="dl-kind" role="img" aria-label={kw || 'No type yet'} use:tip={kw ? (kindOf(r) ? kw : `${kw} (from the client's category)`) : 'No type yet'}>{#if cols.kindWide && kw}<span class="kp" data-k={ki ?? 'other'}>{kw}</span>{:else if ki}<Icon name={ki} size={16} />{:else}{dash}{/if}</span></span>
	{:else if id === 'proto'}
		{@const p = protocolOf(r)}
		<span class="pcell" class:ph class:named={cols.srcName} data-col="proto">{#if p}{@const cm = clientMark(r.downloader_kind)}{@const pw = p === 'torrent' ? 'Torrent' : 'Usenet'}{@const logo = CLIENT_ICONS.has(r.downloader_kind ?? '')}<span class="pm" class:logo class:cmk={!logo && !!cm.mark} data-tone={logo ? undefined : cm.tone || undefined} data-testid="dl-protocol" data-protocol={p} data-kind={r.downloader_kind} use:tip={`${dlName(r)} · ${pw}`} role="img" aria-label="{dlName(r)} · {pw}">{#if logo}<img class="cico" src={to(`/clients/${r.downloader_kind}.svg`)} alt="" width="24" height="24" decoding="async" />{:else if cm.mark}{cm.mark}{:else}<Icon name={p === 'torrent' ? 'torrent' : 'usenet'} size={14} />{/if}</span>{#if cols.srcName}<span class="pnm" title={dlName(r)}>{dlKind(r) || dlName(r)}</span>{/if}{/if}</span>
	{:else if id === 'title'}
		<span class="nm" class:ph data-col="title">
			<span class="tt" title={rowName(r)}><span class="tn">{rowName(r)}</span><span class="tphone">{phoneTitle(r)}</span>{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
			<span class="rel" title={r.name}>{sub2(r)}</span>
			{#if subLine(r)}<span class="ep" data-testid="dl-epline">{subLine(r)}</span>{/if}
		</span>
	{:else if id === 'progress'}
			<!-- One bar on every row, 0 to 100: a finished row is full, never a tick in the bar's place, and nothing holds at 99. -->
			{@const hs = hist && stageOf(r) !== 'failed' ? historyStatus(r, importOf.get(r.id)) : null}
			{@const raw = percent(r.progress)}
			{@const pct = hs && !hs.bad ? 100 : raw}
			{@const tone = hs ? (hs.bad ? 'bad' : 'done') : stageOf(r) === 'paused' ? 'paused' : barTone(r)}
			{@const dn = (r.speed ?? 0) >= 1024}
			{@const upOn = (r.up_speed ?? 0) >= 1024}
			{@const eta = r.active && r.eta ? duration(r.eta) : ''}
			<!-- Speed and time left sit under the bar: one line of "how it is going" for the moving rows, nothing for the rest. -->
			<span class="pgw" class:ph data-col="progress"><span class="prg {tone}" role="progressbar" aria-label="Progress of {who(r)}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.floor(pct)} data-testid="dl-bar"><i style="transform: translateX({pct - 100}%)"></i><span class="bl">{Math.floor(pct)}%</span></span>{#if moving(r) && (dn || upOn || eta)}<span class="spd" data-testid="dl-row-speed">{[dn ? `↓ ${bytes(r.speed)}/s` : '', upOn ? `↑ ${bytes(r.up_speed)}/s` : '', eta ? `${eta} left` : ''].filter(Boolean).join(' · ')}</span>{/if}</span>
	{:else if id === 'status'}
		{#if hist}
			{@const s = historyStatus(r, importOf.get(r.id))}
			{@const cut = s.text.indexOf(': ')}
			{@const reason = cut > 0 ? shortReason(s.text.slice(cut + 2)) : ''}
			<span class="st" class:ph class:bad={s.bad} class:two={!!reason} data-col="status" title={s.text}><span class="dot" aria-hidden="true"></span><span class="tx"><span class="sw">{cut > 0 ? s.text.slice(0, cut) : s.text}</span>{#if reason}<span class="why" data-testid="dl-why">{reason}</span>{/if}</span></span>
		{:else}
			{@const t = barTone(r)}
			{@const detail = statusDetail(r, importOf.get(r.id))}
			{@const more = detail || statusHint(r)}
			{@const why = problem(r)}
			<!-- The word on its own line, a short line under it: the dot and the word carry the tone, the line stays quiet. -->
			<!-- The line is the problem, else a waiting row's place in the Downloader's line, else the step it is at (or why it failed, or a seeding ratio). -->
			{@const line = why ? '' : stageOf(r) === 'queued' && r.queue_pos ? (r.queue_pos === 1 ? 'Next in line' : `${cols.ordinal(r.queue_pos)} in line`) : stepLine(r, detail)}
			{@const under = why?.text || line}
			<span class="st {t}" class:ph class:two={!!under} data-col="status" title={[statusWord(r), under, more.toLowerCase() === under.toLowerCase() ? '' : more].filter(Boolean).join(' · ')}><span class="dot" aria-hidden="true"></span><span class="tx"><span class="sw">{statusWord(r)}</span>{#if why}<span class="why" data-testid="dl-why">{why.text.charAt(0).toUpperCase() + why.text.slice(1)}</span>{:else if line}<span class="why" data-testid={stageOf(r) === 'queued' ? 'dl-in-line' : 'dl-step'}>{line}</span>{/if}</span></span>
		{/if}
	{:else if id === 'size'}
		<span class="num" class:ph data-col="size">{r.size ? bytes(r.size) : dash}</span>
	{:else if id === 'quality'}
		<span class="txt" class:ph data-col="quality" title={r.name}>{quality(r.name) || dash}</span>
	{:else if id === 'added'}
		<span class="num" class:ph data-col="added" title={r.started_at ?? ''}>{r.started_at ? ago(r.started_at) : dash}</span>
	{:else if id === 'swarm'}
		<!-- Connected seeds and peers, named; the swarm's totals stay in the tooltip; the ratio on its own line. -->
		{#if r.protocol !== 'torrent'}<span class="num" class:ph data-col="swarm">{dash}</span>
		{:else}
			{@const st = swarmTotal(r.seeds, r.seeds_total)}
			{@const pt = swarmTotal(r.peers, r.peers_total)}
			{@const tip = [st ? `${st} seeds in the swarm` : '', pt ? `${pt} peers in the swarm` : ''].filter(Boolean).join(' · ') || 'Connected seeds and peers'}
			<!-- One compact line on the header's three tracks: a green mark with the seeds, a blue mark with the peers, the ratio as a pill (green from 1.0). -->
			<span class="sw1" class:ph data-col="swarm" title="{tip} · ratio {r.ratio != null ? r.ratio.toFixed(2) : '–'}"><span class="n"><i class="dot s" aria-hidden="true"></i>{swarmNum(r.seeds)}</span><span class="n"><i class="dot p" aria-hidden="true"></i>{swarmNum(r.peers)}</span>{#if r.ratio != null}<b class="rp" class:good={r.ratio >= 1}>{ratioText(r.ratio)}</b>{/if}</span>
		{/if}
	{:else if id === 'tracker'}
		<span class="txt" class:ph data-col="tracker" title={r.tracker ?? ''}>{r.tracker || dash}</span>
	{:else if id === 'requested'}
		<!-- Who asked: a person, HolyArrs itself (a wanted title it grabbed on its own), or Outside (added straight in the client). -->
		{@const person = !r.outside && r.requested_by ? r.requested_by : ''}
		{@const who = r.outside ? 'Outside' : person || (session.can('manage_library') ? 'HolyArrs' : '')}
		<span class="txt rq" class:ph class:soft={!person} data-col="requested" title={who}>{#if person}<i class="av" aria-hidden="true">{person.slice(0, 1).toUpperCase()}</i>{/if}{who || dash}</span>
	{:else if id === 'source'}
		<span class="txt" class:ph data-col="source" title={r.source_name ?? ''}>{#if r.source_name && r.source_id && session.can('settings')}<a href="/settings/sources/{r.source_id}" onclick={(e) => e.stopPropagation()}>{r.source_name}</a>{:else}{r.source_name || dash}{/if}</span>
	{:else if id === 'category'}
		{@const cw = categoryWords(r.category)}<span class="txt" class:ph data-col="category" title={r.category ?? ''}>{cw || dash}</span>
	{:else}
		<span class="num" class:ph data-col={id}>{dash}</span>
	{/if}
{/snippet}

{#snippet bin(r: DownloadRow)}
	<span class="del" data-col="bin">
		{#if watch}{:else if rowItems(r).length}<span class="rm" data-testid="dl-row-menu"><Menu label="Actions for {who(r)}" icon="more" width={210} items={rowMenu(r)} tabindex={tabRow === r.id ? 0 : -1} /></span>
		{:else}<button class="bin" type="button" disabled title="HolyArrs cannot change this one" aria-label="HolyArrs cannot change this one" tabindex={tabRow === r.id ? 0 : -1}><Icon name="more" size={19} /></button>{/if}
	</span>
{/snippet}

{#snippet row(r: DownloadRow, hist: boolean)}
	<li class="drow" class:sel={openId === r.id} class:picked={picked.has(r.id)} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions, a11y_no_static_element_interactions -->
		<div class="dline" class:hist tabindex={tabRow === r.id ? 0 : -1} onfocus={() => (rowFocus = r.id)} aria-current={openId === r.id} aria-label="{who(r)}: show details" data-testid="dl-line" use:longPress={() => { if (!watch) pickOne(r.id, true); }} onclick={(e) => rowClick(e, r.id)} onkeydown={(e) => rowKey(e, r.id)}>
			{@render lead(r)}
			{#each cols.cells as c (c.id)}{@render cell(r, c.id, c.ph, hist)}{/each}
			<span class="spare"></span>
			{@render bin(r)}
		</div>
	</li>
{/snippet}

{#snippet header()}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dline chead" data-testid="dl-colhead" oncontextmenu={cols.headMenu}>
		<!-- Select all: ticked when every row is picked, half-ticked when some are. -->
		<span class="hk">{#if !watch}<input type="checkbox" class="hpk" checked={list.length > 0 && picked.size === list.length} indeterminate={picked.size > 0 && picked.size < list.length} aria-label="Select all downloads" title="Select all" onchange={() => (picked.size === list.length ? clearPicked() : (picked = new Set(list.map((r) => r.id))))} data-testid="dl-head-pick" />{/if}</span>
		{#each cols.vis as id (id)}
			{@const c = COL[id]}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			{@const so = cols.layout.sort?.id === id ? cols.layout.sort : undefined}
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<span class="ch" class:end={c.num} class:fixed={!!c.fixed} class:moving={cols.ghost?.id === id} class:sorted={!!so} data-col={id} data-testid="dl-head" role={id === 'swarm' ? 'group' : 'button'} aria-label={cols.headName(id, so)} tabindex={id === cols.vis[0] ? 0 : -1} onpointerdown={(e) => cols.headDown(e, id)} onclick={(e) => { if (e.detail === 0) cols.sortBy(id); }} onkeydown={(e) => cols.headKey(e, id)} title="Click to sort · drag to move · right-click for columns">{#if id === 'proto' && !cols.srcName}<span class="lbl"><span class="sr">Downloader</span><Icon name="plug" size={15} /></span>{:else if id === 'poster'}<span class="lbl"><span class="sr">Poster</span><Icon name="image" size={15} /></span>{:else if id === 'kind' && !cols.kindWide}<span class="lbl"><span class="sr">Type</span><Icon name="grid" size={15} /></span>{:else if id !== 'swarm'}<span class="lbl">{c.label}</span>{/if}{#if id === 'swarm'}<span class="facets" role="group" aria-label="Swarm: sort by">{#each cols.FACETS as f (f.id)}<button type="button" class="fc" class:on={so?.facet === f.id} aria-pressed={so?.facet === f.id} tabindex="-1" onpointerdown={(e) => e.stopPropagation()} onclick={(e) => { e.stopPropagation(); cols.sortBy('swarm', f.id); }} data-testid="dl-facet-{f.id}">{f.label}{#if so?.facet === f.id}<span class="sbar" class:desc={so.desc} aria-hidden="true" data-testid="dl-sort"></span>{/if}</button>{/each}</span>{:else if so}<span class="sbar" class:desc={so.desc} aria-hidden="true" data-testid="dl-sort"></span>{/if}{#if !c.fixed}<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions --><span class="grip" role="separator" aria-orientation="vertical" aria-label="Resize {c.label}" aria-valuenow={cols.wOf(id)} aria-valuemin={cols.heads[id] ?? MIN_W} aria-valuemax={MAX_W} tabindex="0" title="Drag to cols.resize, double-click to fit" data-testid="dl-grip" data-grip={id} onpointerdown={(e) => cols.grab(e, id)} ondblclick={() => cols.fitOne(id)} onkeydown={(e) => cols.nudge(e, id)}></span>{/if}</span>
		{/each}
		<span></span>
		<span class="cm"><Menu label="Columns" icon="more" width={180} items={[{ label: 'Columns…', icon: 'more', onselect: cols.fromMenu }, { label: 'Reset columns', icon: 'refresh', onselect: cols.resetAll }]} /></span>
		{#if cols.lineX !== null}<span class="ins" style="left: {cols.lineX}px" data-testid="dl-insert" aria-hidden="true"></span>{/if}
	</div>
{/snippet}

{#snippet btns(r: DownloadRow)}
	{@const more = watch ? [] : paneMenu(r)}
	<div class="xbt" data-testid="dl-actions">
		{#if !watch}
			{#if retryable(r) && mayEdit(r)}<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} onclick={() => (onretry ? onretry([r]) : onact?.(r, 'retry'))} data-testid="dl-panel-retry-job">Download again</button>
			{:else if pausable(r) && !r.client_locked}<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} onclick={() => onact?.(r, isPaused(r) ? 'resume' : 'pause')} data-testid="dl-panel-pause">{isPaused(r) ? 'Start' : 'Pause'}</button>{/if}
			<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} title={r.can_act ? 'Remove' : 'HolyArrs cannot remove this one'} onclick={() => onremove(r)} data-testid="dl-panel-remove">Remove</button>
		{/if}
		{#if r.title_id && (r.linked || !r.outside)}<a class="xbn" href={to(`/library/${r.title_id}`)} data-testid="dl-open-title">Open in Library</a>{/if}
		{#if more.length}<span class="more" data-testid="dl-more"><Menu label="More actions" items={more} width={220} /></span>{/if}
	</div>
{/snippet}

{#snippet tabStrip(r: DownloadRow)}
	{@const tab = tabNow(r)}
	<div class="xtabs" role="tablist" aria-label="Details" use:ink={`${r.id}:${tab}:${paneOpen}`}>
		{#each tabsOf(r) as t (t)}
			<button type="button" role="tab" class="xtab" class:on={tab === t} aria-selected={tab === t} aria-controls="dlv-{r.id}" id="dlt-{r.id}-{t}" tabindex={tab === t ? 0 : -1} onclick={() => { xtabOn[r.id] = t; paneOpen = true; }} onkeydown={(e) => paneTabKey(e, r)} data-testid="dl-tab-{t.toLowerCase().replaceAll(' ', '-')}">{t}</button>
		{/each}
		<i class="ink" class:gone={!paneOpen} aria-hidden="true"></i>
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
					<div class="subin" data-testid={open ? 'dl-subseg' : undefined}>
						{#each so as x (x.key)}
							<button type="button" role="tab" class="stab" class:on={open && subOn === x.key} aria-selected={open && subOn === x.key} tabindex="-1" data-nav onclick={() => { if (on !== c.key) choose(c.key); chooseSub(x.key); }} data-testid={open ? 'dl-subseg-item' : undefined} data-key={x.key} data-count={x.count}>{x.label}<b>{x.count}</b></button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/each}
{/snippet}

{#snippet panel(r: DownloadRow)}
	{@const ds = decisionsOf(r)}
	{@const sizeWord = r.size ? bytes(r.size) : 'Not reported yet'}
	<section class="dpane" class:shut={!paneOpen} style={paneOpen ? (paneFit ? `max-height: ${maxPane()}px; --ph: ${maxPane()}px` : `height: ${paneH}px; --ph: ${paneH}px`) : ''} id="dlx-{r.id}" data-testid="dl-pane" aria-label="Details of {who(r)}">
		<div class="phd">
			<button type="button" class="grab" aria-label="Close details" onpointerdown={(e) => (grabY = e.clientY)} onpointerup={(e) => { if (grabY != null && e.clientY - grabY > 40) openId = 0; grabY = null; }} onclick={() => (openId = 0)} data-testid="dl-pane-grab"><i></i></button>
			<p class="xt pt"><b>{who(r)}</b><span class="hs"><i class="hdot {barTone(r)}" class:bad={problem(r)?.tone === 'fail' || r.phase === 'failed'} aria-hidden="true"></i><span data-testid="dl-health">{protocolOf(r) === 'torrent' ? healthLine({ ...r, done_at: pane.det.id === r.id ? pane.det.detail?.torrent?.done_at ?? r.finished_at : r.finished_at, seeding_time: pane.det.id === r.id ? pane.det.detail?.torrent?.seeding_time : undefined, ratio: pane.det.id === r.id ? pane.det.detail?.torrent?.ratio ?? r.ratio : r.ratio }) : `${statusWord(r)} · ${Math.floor(percent(r.progress))}% · ${sizeWord}`}</span></span></p>
			{@render tabStrip(r)}
			{#if paneOpen}
			<div class="pact" data-testid="dl-pane-actions">
				{#if r.replaced}<a class="tdl" href={to('/library/turned-down')} data-testid="dl-turned-down-link">Turned down copies</a>{/if}
				{@render btns(r)}
			</div>
			{/if}
			<button type="button" class="pcl" aria-expanded={paneOpen} aria-label={paneOpen ? 'Collapse details' : 'Expand details'} onclick={() => (paneOpen = !paneOpen)} data-testid="dl-pane-toggle"><Icon name="chevronDown" size={14} stroke={2.2} /></button>
		</div>
		{#if paneOpen}
		<div class="pb" data-testid="dl-panel" data-card="d">
			<span class="big" class:square={r.kind === 'artist'} data-testid="dl-big-poster"><Poster src={poster(r, titles)} name={titlesReady ? r.title || r.name.replace(/[._]+/g, ' ') : ''} /></span>
			<div class="xb">
				{#key tabNow(r)}<div class="tview" role="tabpanel" id="dlv-{r.id}" aria-labelledby="dlt-{r.id}-{tabNow(r)}"><DlTabsView {r} tab={tabNow(r)} {pane} {importOf} {watch} {mayEdit} /></div>{/key}
				{#if ds.length && !watch}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
			</div>
		</div>
		{/if}
	</section>
{/snippet}

<svelte:window onpointerdown={cols.outside} onkeydown={listKey} />

<div class="dlgrid" bind:this={cols.grid} style="--tpl: {cols.tpl}; --c-in: {cols.inner}px; --pi: {cols.posterW - 6}px; --lg: {cols.srcBig ? 24 : 18}px">
<div class="app" data-testid="dl-app" bind:this={cols.appEl} style="--app-h: {cols.appH}px">
<div class="strip hdr" data-testid="dl-strip">
	<div class="psearch" class:open={psOpen || !!dlScope.term} data-testid="dl-psearch">
		<button type="button" class="psb" aria-label="Search downloads" aria-expanded={psOpen || !!dlScope.term} onclick={() => { psOpen = true; tick().then(() => psEl?.focus()); }} data-testid="dl-psearch-btn"><Icon name="search" size={16} /></button>
		{#if psOpen || dlScope.term}
			<input type="search" bind:this={psEl} bind:value={dlScope.term} placeholder="Search downloads" aria-label="Search downloads" onblur={() => { if (!dlScope.term) psOpen = false; }} onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); dlScope.term = ''; psOpen = false; } }} data-testid="dl-psearch-input" />
			{#if dlScope.term}<button type="button" class="psx" aria-label="Clear search" onclick={() => { dlScope.term = ''; psEl?.focus(); }}><Icon name="x" size={14} /></button>{/if}
		{/if}
	</div>
	<DlBar bind:kindOn bind:srcOn bind:trackOn trackAll={bySource.length} trackHosts={trackHosts} {trackProblems} {kindCount} {srcCount} actions={barActs} {watch} bind:term={dlScope.term} shown={searched.length} total={typed.length} side={dlTop.collapsed} onside={() => dlTop.toggle()} searchKey={dlTop.searchKey} {allItems} />
</div>
<div class="body">
<div class="col" bind:this={colEl}>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="tabrow" data-testid="dl-tabrow" onkeydown={navKey}><div class="tabs" role="tablist" aria-label="Status">{@render pills()}</div>{#if problemWords(trackProblems)}<button type="button" class="trchip" class:on={trackKey === 'p:any'} aria-pressed={trackKey === 'p:any'} onclick={() => (trackOn = trackKey === 'p:any' ? '' : 'p:any')} data-testid="dl-tracker-issues">{problemWords(trackProblems)}</button>{/if}<DlStatus down="{bytes(sum.speed)}/s" up={noUp ? '' : `${bytes(upSum)}/s`} eta={etaWords} free={dls.freeAll.length ? bytes(Math.min(...dls.freeAll)) : ''} today={dls.sumOf('day_down') ? bytes(dls.sumOf('day_down')) : ''} session={dls.sumOf('session_down') ? bytes(dls.sumOf('session_down')) : ''} review={reviewShown(watch, toReview)} onreview={openReview} more={morePop} limit={Number.isFinite(capNow) ? `${bytes(capNow)}/s` : ''} onlimit={watch ? undefined : onlimit} /></div>
<p class="qline" data-testid="dl-qline">{#if onlimit && !watch}<button type="button" class="qd qbtn" onclick={() => onlimit?.()} title={Number.isFinite(capNow) ? `Speed limit: ${bytes(capNow)}/s. Change it` : 'Set a speed limit'} data-testid="dl-q-down"><span class="sr">Speed: </span>↓ {bytes(sum.speed)}/s{#if Number.isFinite(capNow)}<span class="qcap"><Icon name="gauge" size={12} /><span class="sr">Speed limit </span>{bytes(capNow)}/s</span>{:else}<span class="sr">. Set a speed limit</span>{/if}</button>{:else}<span class="qd" data-testid="dl-q-down">↓ {bytes(sum.speed)}/s</span>{/if}{#if dls.freeAll.length}<span class="qs" aria-hidden="true">·</span><span data-testid="dl-q-free">{bytes(Math.min(...dls.freeAll))} free</span>{/if}{#if etaWords}<span class="qs" aria-hidden="true">·</span><span>{etaWords} left</span>{/if}{#if reviewShown(watch, toReview)}<button type="button" class="qrv" onclick={openReview} data-testid="dl-q-review">{reviewWord(toReview)}</button>{/if}</p>
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="listbox" bind:this={cols.listEl} data-testid="dl-listbox" use:overlayScroll={{ header: ".chead" }}>
{#if (pickedRows.length || selecting) && !watch}
	<div class="selbar" role="toolbar" aria-label="Selected downloads" data-testid="dl-selbar">
		<span class="sn" aria-live="polite"><b>{pickedRows.length}</b> selected</span>
		{#if bulkRows.length}{#each BULK as b (b.action)}<button type="button" class="xbn" disabled={batchBusy} onclick={() => onbulk?.(bulkRows, b.action)} data-testid="dl-bulk-{b.action}">{b.label}</button>{/each}{/if}
		{#if pickedRows.some(steerable)}{#each STEER as m (m.mode)}<button type="button" class="xbn" onclick={() => onsteer?.(pickedRows.filter(steerable), m.mode)} data-testid="dl-bulk-steer-{m.mode}">{m.short}</button>{/each}{/if}
		{#if pickedRows.some((r) => retryable(r) && mayEdit(r))}<button type="button" class="xbn" disabled={batchBusy} onclick={() => onretry?.(pickedRows.filter((r) => retryable(r) && mayEdit(r)))} data-testid="dl-bulk-retry">Download again</button>{/if}
		<button type="button" class="xbn" onclick={() => (picked = new Set(list.map((r) => r.id)))} data-testid="dl-bulk-all">Select all</button>
		<button type="button" class="xbn" onclick={clearPicked} data-testid="dl-bulk-clear">Clear</button>
	</div>
		<div class="psel" role="toolbar" aria-label="Selected downloads" data-testid="dl-psel">
			<div class="pst"><span class="sn" aria-live="polite"><b>{pickedRows.length}</b> selected</span><button type="button" class="plk" onclick={() => (picked = new Set(list.map((r) => r.id)))} data-testid="dl-psel-all">Select all</button><button type="button" class="plk" onclick={clearPicked} data-testid="dl-psel-cancel">Cancel</button></div>
			<div class="psa">
				{#each PHONE_BULK.filter((b) => b.action !== 'recheck' || bulkRows.length) as b (b.action)}<button type="button" class="pa" disabled={batchBusy || (b.action === 'start' ? !startT.rows.length : b.action === 'pause' ? !pauseT.rows.length : false)} onclick={() => (b.action === 'start' ? runOn(startT, 'resume', 'start') : b.action === 'pause' ? runOn(pauseT, 'pause', 'pause') : onbulk?.(bulkRows, b.action))} data-testid="dl-psel-{b.action}"><Icon name={b.icon} size={20} /><span>{b.label}</span></button>{/each}
				<button type="button" class="pa bad" onclick={() => onremovemany?.(pickedRows)} data-testid="dl-psel-remove"><Icon name="trash" size={20} /><span>Remove</span></button>
			</div>
		</div>
{/if}
{#if list.length}
	<div class="tw" onscroll={cols.sync} data-testid="dl-tablebox"><div class="tin">
		{@render header()}
		<ul class="dlrows" class:vw={cols.windowed} bind:this={cols.ulEl} data-testid="dl-list" data-tab={on} data-rows={list.length} style={cols.windowed ? `padding-top: ${cols.span.a * cols.rh}px; padding-bottom: ${(list.length - cols.span.b) * cols.rh}px` : undefined}>
			{#each cols.shownRows as r (r.id)}{@render row(r, inHistory(r))}{/each}
		</ul>
	</div></div>
{:else}
	{@const em = emptyOf(on, subOn, q, kindOn, tabs, srcOn)}
	<div class="tw" data-testid="dl-tablebox"><div class="tin">{@render header()}</div></div>
	{#key em.title}
	<GhostEmpty title={em.title} text={em.text}>
		{#snippet action()}
			{#if em.action?.act === 'clear'}<button type="button" data-testid="dl-empty-act" onclick={() => (dlScope.term = '')}>{em.action.label}</button>
			{:else if em.action?.act === 'all'}<button type="button" data-testid="dl-empty-act" onclick={() => { kindOn = ''; srcOn = ''; }}>{em.action.label}</button>{/if}
		{/snippet}
	</GhostEmpty>
	{/key}
{/if}
</div>
{#if openRow}
	<div class="sheet" in:sheetIn out:sheetOut data-testid="dl-sheet">
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div class="dv" role="separator" aria-orientation="horizontal" aria-label="Resize details" aria-valuemin={0} aria-valuemax={900} aria-valuenow={paneOpen ? (paneFit ? 0 : paneH) : 0} tabindex="0" class:fixed={!paneOpen} title="Drag to cols.resize · double-click to fit the content" onpointerdown={dragDown} ondblclick={() => (paneFit = true)} onkeydown={dividerKey} data-testid="dl-divider"><i aria-hidden="true"></i></div>
	{@render panel(openRow)}
	</div>
{/if}
</div>
</div>
<!-- Connection health stays readable for screen readers and tests. -->
{#snippet morePop()}<DlMorePop {notes} {dls} {watch} />{/snippet}
<div class="vh"><span data-testid="sb-health">{dls.health}</span></div>
</div>

{#if cols.ghost}<div class="ghost" style="left: {cols.ghost.x}px; top: {cols.ghost.y}px" aria-hidden="true" data-testid="dl-ghost">{COL[cols.ghost.id].label}</div>{/if}

{#if cols.pop}
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<div class="picker" role="dialog" aria-label="Columns" tabindex="-1" bind:this={cols.pickEl} style="left: {cols.pop.x}px; top: {cols.pop.y}px" data-testid="dl-picker" onkeydown={cols.pickerKey}>
		<p class="ph-t">Columns</p>
		<ul class="pl">
			{#each cols.layout.order as id (id)}
				{@const c = COL[id]}
				<li data-id={id} class:held={cols.held === id} class:off={c.missing}>
					<button type="button" class="hd" data-handle={id} aria-label="Move {c.label}" aria-pressed={cols.held === id} aria-describedby="dl-pick-help" disabled={c.missing} onkeydown={(e) => cols.handleKey(e, id)} onpointerdown={(e) => cols.handleDown(e, id)}><svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true"><g fill="currentColor"><circle cx="5" cy="3" r="1.2"/><circle cx="9" cy="3" r="1.2"/><circle cx="5" cy="7" r="1.2"/><circle cx="9" cy="7" r="1.2"/><circle cx="5" cy="11" r="1.2"/><circle cx="9" cy="11" r="1.2"/></g></svg></button>
					<label class="pkl"><input type="checkbox" checked={!c.missing && !cols.layout.hidden.includes(id)} disabled={c.missing} onchange={() => cols.flip(id)} data-testid="dl-pick-{id}" /><span>{c.label}</span>{#if c.missing}<span class="nr">Not reported yet</span>{/if}</label>
				</li>
			{/each}
		</ul>
		<p id="dl-pick-help" class="help">Drag a handle, or press Space on it, then ↑/↓ and Enter.</p>
		<div class="pf"><button type="button" class="lk" onclick={cols.resetAll} data-testid="dl-pick-reset">Reset to default</button></div>
		<span class="sr" aria-live="polite">{cols.said}</span>
	</div>
{/if}

</div>

<style>
	/* Source hugs the logo, so its header icon sits centred over the logos. */
	.ch[data-col='proto']:has(:global(svg)) { justify-content: center; }
	.ch[data-col='kind']:has(:global(svg)) { justify-content: center; }
	.ch[data-col='proto'] .lbl, .ch[data-col='kind'] .lbl { display: flex; }
	/* The next divider sits a gap after the column, so the logo and icon shift half a gap to sit centred between the two lines. */
	.ch[data-col='poster'] { justify-content: center; }
	.ch[data-col='poster'] .lbl { display: flex; }
	@media (min-width: 641px) {
		.ch[data-col='proto']:has(:global(svg)) .lbl, .pcell:not(.named) > .pm, .ch[data-col='poster'] .lbl, .dline > .post, .ch[data-col='kind']:has(:global(svg)) .lbl, .kcell > .kt:not(.wide) { transform: translateX(calc(var(--gap) / 2 - 4px)); }
	}
	.ch[data-col='proto'] .lbl { overflow: visible; text-overflow: clip; }
	.pm.logo, .pm:has(.cico) { width: auto; height: auto; padding: 0; border: 0; border-radius: 0; background: none; box-shadow: none; }
	.cico { display: block; flex: none; width: var(--lg, 18px); height: var(--lg, 18px); object-fit: contain; }
	/* Large Source: the logo, then the client's own name, starting after the divider. */
	.pcell.named { justify-content: flex-start; gap: 6px; }
	.pcell.named > .pm { flex: none; }
	.pnm { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.strip { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 22px; padding: 8px 0; margin-bottom: 10px; border-bottom: 1px solid var(--line); font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.strip > span { display: inline-flex; align-items: center; gap: 6px; }
	.k { color: var(--text-muted); font: var(--t-footnote); }
	.strip b { font-weight: 600; }
	.sp { gap: 8px; }
	.up { color: var(--info-text); }
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
	/* One column grid for every row of both sections. */
	.dlgrid { --gap: 6px; min-width: 0; }
	.tw { overflow-x: auto; overflow-y: hidden; max-width: 100%; }
	/* Muted text in the list is mixed from the text colour, so it stays readable (4.5:1+) on the card's tinted rows. */
	.tin { min-width: var(--c-in); --text-muted: color-mix(in srgb, var(--text) 66%, var(--surface-2)); }
	.dlrows { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--line); }
	.drow { display: block; border-bottom: 1px solid var(--line); }
	.tg { position: relative; align-self: stretch; }
	/* The row checkbox has its own slim column at the left, so it never covers the poster. */
	.hk { position: relative; align-self: stretch; display: flex; align-items: center; }
	.hk::after { content: ''; position: absolute; top: 0; bottom: 0; right: calc(3px - var(--gap)); width: 1px; background: var(--line-strong); pointer-events: none; }
	.hpk { width: 16px; height: 16px; margin: 0 0 0 4px; accent-color: var(--accent); cursor: pointer; }
	.hpk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pk { position: absolute; left: 4px; top: 50%; width: 16px; height: 16px; margin: -8px 0 0; accent-color: var(--accent); opacity: 0; cursor: pointer; z-index: 2; transition: opacity 120ms var(--ease-out); }
	.drow:hover .pk, .pk.any, .pk:focus-visible { opacity: 1; }
	.drow.picked > .dline { background: color-mix(in srgb, var(--accent) 9%, transparent); }
	.selbar { position: sticky; top: 0; left: 0; z-index: 3; display: flex; flex-wrap: wrap; align-items: center; gap: 6px; padding: 8px var(--gutter, 12px); background: var(--surface-1); border-bottom: 1px solid var(--line); }
	.selbar .sn { font: var(--t-footnote); color: var(--text-muted); margin-right: 4px; }
	.selbar .sn b { color: var(--text); }
	.psel, .qline, .tt .tphone, .nm .ep { display: none; }
	@keyframes pselIn { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: none; } }
	@media (prefers-reduced-motion: reduce) {
		.psel { animation: none; }
	}
	@media (hover: none) {
		.pk { opacity: 1; }
	}
	.dline { display: grid; grid-template-columns: var(--tpl); align-items: center; column-gap: var(--gap); height: max(44px, calc(var(--pi, 26px) * 1.5 + 6px)); }
	.drow > .dline { cursor: pointer; }
	.drow > .dline:hover { background: var(--surface-2); }
	.drow > .dline:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.bin:focus-visible, .lk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* The poster sits at the right of its column so the row checkbox at the left edge never covers it. */
	.post { width: var(--pi, 26px); height: calc(var(--pi, 26px) * 1.5); justify-self: center; overflow: hidden; border-radius: 3px; --poster-ratio: 2 / 3; }
	.post.square { height: var(--pi, 26px); --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 3px; }
	.nm { display: flex; flex-direction: column; min-width: 0; line-height: 1.25; }
	.tt, .rel { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tt { font: var(--t-callout); font-weight: 500; display: flex; align-items: center; min-width: 0; }
	/* The name shrinks with an ellipsis; the badges never do. */
	.tt .tn, .tt .tphone { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.tt .ver { flex: none; }
	.rel { color: var(--text-muted); font: var(--t-caption); }
	.ver { margin-left: 6px; padding: 0 5px; border-radius: 4px; background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); }
	.prg { position: relative; display: flex; align-items: center; height: 14px; white-space: nowrap; padding: 0 6px; border-radius: 4px; overflow: hidden; background: var(--surface-3); color: var(--text); font-size: 10px; line-height: 14px; font-weight: 600; font-variant-numeric: tabular-nums; }
	.prg i { position: absolute; inset: 0; background: var(--ok); opacity: 0.5; transition: transform var(--dur-slow) var(--ease-out); }
	.prg.queued i { background: var(--info); }
	.prg.held i { background: var(--wait); }
	/* A failed row's bar stays neutral: the dot and the status words carry the red, not three things at once. */
	.prg.bad i { background: var(--text-muted); opacity: 0.3; }
	.bl, .br { position: relative; overflow: hidden; text-overflow: ellipsis; }
	.bw { font-weight: 500; }
	.hcell { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; height: 24px; padding: 0 8px; }
	.hsz { flex: none; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }
	.bl { font-weight: 600; text-shadow: 0 0 3px var(--surface-1); }
	.st { display: flex; align-items: center; gap: 6px; min-width: 0; font: var(--t-footnote); }
	.st .tx { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.st .sw, .st .why { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.st .why { color: var(--text-muted); font: var(--t-caption); }
	/* Two lines: the dot sits on the first. */
	.st.two { align-items: flex-start; }
	.st.two .dot { margin-top: 4px; }
	/* Requested by: a person with their initial; HolyArrs itself and Outside stay quiet. */
	.rq { display: flex; align-items: center; gap: 6px; }
	.rq.soft { color: var(--text-muted); }
	.rq .av { display: grid; place-items: center; flex: none; width: 18px; height: 18px; border-radius: 50%; background: var(--text); color: var(--bg); font: 600 10px / 1 var(--font-ui); font-style: normal; }
	.st .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.st.paused .dot { background: var(--text-muted); }
	.st.queued .dot { background: var(--info); }
	.st.held .dot { background: var(--wait); }
	.st.bad .dot { background: var(--fail); }
	.st.bad { color: var(--fail-text); }
	.dline > .ph { display: none; }
	/* A column narrowed past its content cuts it off cleanly instead of spilling into the next one. */
	@media (min-width: 641px) {
		.dline:not(.chead) > [data-col] { min-width: 0; overflow: hidden; }
	}
	:global(html.col-resizing), :global(html.col-resizing *) { cursor: col-resize !important; user-select: none !important; }
	.txt { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.num { text-align: right; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	/* The header row is exactly 28px (the ⋯ button may be taller), so every divider line stays inside it, clear of the border below. */
	.chead { grid-template-rows: minmax(0, 1fr); position: relative; height: 28px; border-bottom: 1px solid var(--line); color: color-mix(in srgb, var(--text) 78%, transparent); font: var(--t-caption); font-weight: 600; }
	.chead + .dlrows { border-top: 0; }
	.chead:hover { background: none; }
	.ch { position: relative; align-self: stretch; display: flex; align-items: center; min-width: 0; }
	/* Number columns: the heading sits over its right-aligned values. */
	.ch.end { justify-content: flex-end; }
	/* Every column name starts right after the divider line, as in qBittorrent; numbers below stay right-aligned. */
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
	.hd:focus-visible, .pkl input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pkl { display: flex; flex: 1; align-items: center; gap: 8px; min-width: 0; color: var(--text); font: var(--t-footnote); cursor: pointer; }
	.pkl > span:first-of-type { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.off .pkl { color: var(--text-muted); cursor: default; }
	.nr { margin-left: auto; padding-right: 4px; color: var(--text-muted); font: var(--t-caption); }
	.help { margin: 6px 6px 0; color: var(--text-muted); font: var(--t-caption); }
	.pf { display: flex; justify-content: flex-end; margin-top: 4px; border-top: 1px solid var(--line); padding-top: 4px; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	/* Sorting shows as a line under the column name (blue going up, red going down), so it takes no width. */
	.sbar { position: absolute; left: 0; right: calc(4px - var(--gap)); bottom: 0; height: 2px; border-radius: 1px; background: var(--info); }
	.sbar.desc { background: var(--fail); }
	.ch.sorted .lbl { color: var(--text); }
	.lbl { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	/* Divider lines run the full height of the header, from its top to the border below, like a table. */
	.grip { position: absolute; top: 0; bottom: 0; right: calc(-1 * var(--gap) - 3px); width: 12px; cursor: col-resize; touch-action: none; border-radius: 3px; z-index: 3; }
	.grip::after { content: ''; position: absolute; left: 5px; top: 0; bottom: 0; width: 1px; background: var(--line-strong); transition: background var(--dur-fast) var(--ease-out); }
	.grip:hover::after, .grip:focus-visible::after, .grip:global(.drag)::after { background: var(--accent); width: 2px; }
	/* A fixed-width column has no grip but keeps the divider, at the same spot a grip draws it. */
	.ch.fixed::after { content: ''; position: absolute; top: 0; bottom: 0; right: calc(3px - var(--gap)); width: 1px; background: var(--line-strong); pointer-events: none; }
	.grip:focus-visible { outline: none; }
	.cm { display: flex; justify-content: flex-end; }
	.del { display: flex; justify-content: flex-end; }
	.bin { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; transition: color var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.bin:hover:not(:disabled) { color: var(--fail-text); background: var(--surface-3); }
	.bin:disabled { opacity: 0.35; cursor: not-allowed; }
	/* The panel starts at the Title column: ▾ + poster + two gaps. */
	/* Details: the grown poster under the ▾ and poster columns, the facts to its right. */
	.xb { min-width: 0; }
	.big { width: 72px; height: 108px; overflow: hidden; border-radius: 6px; box-shadow: 0 4px 14px rgb(0 0 0 / 0.18); --poster-ratio: 2 / 3; }
	.big.square { height: 72px; --poster-ratio: 1; }
	.big :global(.poster) { border-radius: 6px; }
	.post { transition: opacity var(--dur-fast) var(--ease-out); }
	.kv { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 3px 16px; margin: 0; font: var(--t-footnote); }
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
	.fs { flex: none; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.app { display: flex; flex-direction: column; height: var(--app-h, auto); min-height: 420px; min-width: 0; overflow: clip; }
	.body { display: grid; grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); flex: 1 1 auto; min-height: 0; }
	.listbox { min-width: 0; min-height: 0; overflow: auto; overscroll-behavior: contain; }
	.listbox .tw { overflow: visible; }
	.listbox :global(.chead) { position: sticky; top: 0; z-index: 4; background: var(--bg); }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--wait); }
	.dot.ok { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 25%, transparent); }
	.hdr { flex: none; display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; margin: 0; padding: 3px var(--dl-gutter); border-bottom: 1px solid var(--line); background: none; }
	.col { position: relative; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.col > .listbox { flex: 1 1 auto; }
	.drow.sel > .dline { position: relative; z-index: 1; background: var(--surface-2); box-shadow: inset 3px 0 0 var(--accent), 0 5px 10px -6px rgb(0 0 0 / 0.4), 0 -5px 10px -6px rgb(0 0 0 / 0.22); }
	.drow.sel > .dline:hover { background: var(--surface-2); }
	.dv { flex: none; position: relative; height: 9px; border-top: 1px solid var(--line); cursor: row-resize; touch-action: none; }
	.dv.fixed { cursor: default; }
	.dv i { position: absolute; left: 50%; top: 3px; width: 38px; height: 3px; margin-left: -19px; border-radius: 2px; background: var(--line-strong, var(--text-muted)); opacity: 0.6; }
	.dv:hover i, .dv:focus-visible i { opacity: 1; }
	.dv:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* The divider and the pane rise and fold away together (sheetIn / sheetOut). */
	.sheet { flex: none; display: flex; flex-direction: column; min-height: 0; }
	.dpane { flex: none; display: flex; flex-direction: column; min-height: 0; background: none; }
	.dpane.shut { height: auto; }
	.phd { flex: none; }
	.trchip { flex: none; align-self: center; min-height: 24px; padding: 0 10px; border: 1px solid color-mix(in srgb, var(--fail) 45%, transparent); border-radius: 999px; background: color-mix(in srgb, var(--fail) 12%, transparent); color: var(--fail-text); font: var(--t-caption); font-weight: 600; cursor: pointer; white-space: nowrap; }
	.trchip.on, .trchip:hover { background: color-mix(in srgb, var(--fail) 22%, transparent); }
	.trchip:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.grab { display: none; }
	.phd { display: flex; align-items: center; gap: 2px 14px; flex-wrap: wrap; padding: 3px var(--dl-gutter); border-bottom: 1px solid var(--line); min-width: 0; }
	.dpane.shut .phd { border-bottom: 0; }
	.pt { flex: 0 1 380px; min-width: 0; margin: 0; }
	.pt b { font: 600 16px / 1.3 var(--font-ui); }
	.pt span { font: var(--t-callout); }
	.pt b, .pt span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.phd .xtabs { margin: 0; background: none; padding: 0; }
	.pcl { margin-left: auto; display: grid; place-items: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.pcl :global(svg) { transition: transform var(--dur-fast) var(--ease-out); }
	.dpane.shut .pcl :global(svg) { transform: rotate(180deg); }
	.pcl:hover { background: var(--surface-3); color: var(--text); }
	.pcl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pb { display: grid; grid-template-columns: 112px minmax(0, 1fr); align-items: start; gap: 12px; flex: 1 1 auto; min-height: 0; padding: 8px var(--dl-gutter); overflow: auto; }
	.pb .big { width: 112px; height: 168px; border-radius: var(--r-card, 10px); }
	.pb .big :global(.poster) { border-radius: var(--r-card, 10px); }
	.pb .big.square { height: 112px; }
	.pb .kv { gap: 3px 14px; font: var(--t-callout); }
	.pb .xtab { min-height: 28px; padding: 0 10px; font: var(--t-callout); font-weight: 500; }
	.pb .dep { margin-top: 8px; padding: 6px 10px; }
	.pb .xbt { margin-top: 6px; }
	.pb .xbn { min-height: 26px; padding: 0 10px; font: var(--t-caption); font-weight: 500; }
	.note { padding: 1px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--text-muted); font: var(--t-caption); }
	.kcell, .pcell { display: flex; align-items: center; min-width: 0; }
	.kcell .kt.none { color: var(--text-muted); }
	/* Wide, the pill starts right after the divider like every other column's text. */
	.kcell .kt.wide { justify-content: flex-start; }
	/* The wide Type: a small pill in the type's own colour (film orange, TV blue, music green, anything else grey). */
	.kp { padding: 1px 7px; border-radius: 999px; font: var(--t-caption); font-weight: 600; color: color-mix(in srgb, var(--kc) 70%, var(--text)); background: color-mix(in srgb, var(--kc) 16%, transparent); overflow: hidden; text-overflow: ellipsis; }
	.kp[data-k='film'] { --kc: var(--wait); }
	.kp[data-k='tv'] { --kc: var(--info); }
	.kp[data-k='music'] { --kc: var(--ok); }
	.kp[data-k='other'] { --kc: var(--text-muted); }
	.kcell .kt { display: flex; justify-content: center; align-items: center; height: auto; padding: 0; border: 0; background: none; width: 100%; margin: 0; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pcell { justify-content: center; }
	.pm { display: inline-grid; place-items: center; width: 22px; height: 22px; margin: 0; border-radius: 6px; background: var(--surface-3); color: var(--text-muted); }
	.prg.paused i { background: var(--text-muted); }
	.pgw { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.spd { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }
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
	.tabrow { container: tr / inline-size; }
	.vh { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
	.tabs { display: flex; flex: none; align-items: center; gap: 2px; width: max-content; padding: 2px; border-radius: 999px; background: var(--surface-2); }
	.pw { display: flex; align-items: center; flex: none; border-radius: 999px; background: transparent; transition: background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.pw.on { background: var(--hl-bg); box-shadow: var(--hl-shadow, none); }
	.tab { display: inline-flex; align-items: center; flex: none; gap: 6px; height: 28px; padding: 0 12px; border: 0; border-radius: 999px; background: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-footnote); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out); }
	.tab .n { color: inherit; font-weight: 600; font-variant-numeric: tabular-nums; }
	.tab:hover { color: var(--text); }
	.pw.on .tab { color: var(--hl-fg); }
	/* A tab with nothing in it reads its 0; it is not dimmed, so its label keeps readable contrast. */
	.tab:focus-visible, .stab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.subwrap { display: grid; grid-template-columns: 0fr; opacity: 0; transition: grid-template-columns 200ms var(--ease-out), opacity 200ms var(--ease-out); }
	.pw.open .subwrap { grid-template-columns: 1fr; opacity: 1; }
	.subin { display: flex; align-items: center; gap: 2px; min-width: 0; overflow: hidden; white-space: nowrap; }
	.pw.open .subin { padding: 0 2px 0 6px; margin-right: 2px; border-left: 1px solid var(--hl-line); }
	.stab { display: inline-flex; align-items: center; flex: none; gap: 6px; height: 24px; padding: 0 10px; border: 0; border-radius: 999px; background: none; color: var(--hl-sub); font: var(--t-footnote); white-space: nowrap; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.stab b { color: inherit; font-weight: 600; font-variant-numeric: tabular-nums; }
	.stab:hover { color: var(--hl-fg); }
	.stab.on { background: var(--hl-sub-on-bg); color: var(--hl-sub-on-fg); }
	/* No sub-option picked: the parent itself carries the red inner highlight. */
	.pw.leaf .tab { background: var(--hl-sub-on-bg); color: var(--hl-sub-on-fg); box-shadow: inset 0 0 0 2px var(--hl-bg); }
	/* The picked pill is raised on the track like a segmented control; its picked sub-option is tinted red. */
	.tabs { --hl-bg: var(--seg-thumb); --hl-fg: var(--text); --hl-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); --hl-line: var(--line-strong); --hl-sub: color-mix(in srgb, var(--text) 86%, var(--hl-bg)); --hl-sub-on-bg: var(--accent-soft); --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 85%, var(--text)); }
	/* Dark: the red words are lightened so they read on the grey pill (4.5:1 or more). */
	:global(:root[data-theme='dark']) .tabs { --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 25%, var(--text-on-red)); }
	@media (prefers-color-scheme: dark) {
		:global(:root:not([data-theme='light'])) .tabs { --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 25%, var(--text-on-red)); }
	}
	@media (prefers-reduced-motion: reduce) {
		.pw, .tab, .stab, .subwrap { transition: none; }
	}
	.dep { margin-top: 12px; padding: 10px 12px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); box-shadow: 0 1px 2px rgb(0 0 0 / 0.06), 0 6px 16px rgb(0 0 0 / 0.06); min-width: 0; }
	.story { border-color: color-mix(in srgb, var(--fail) 40%, var(--line)); }
	.src { margin-top: 8px; }
	.st-working { color: var(--ok-text); }
	.st-updating { color: var(--info-text); }
	.st-error { color: var(--fail-text); }
	.more { min-width: 36px; padding: 0 10px; }
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
	/* Polish pass: one radius scale (--r-sm 6, --r-btn 10, --r-card 12, --r-panel 16, pill),
	   one depth scale (--shadow-card, --shadow-lift, --shadow-modal), a 4/8 spacing grid. */
	.app { --r-poster: 4px; }
	.post, .post :global(.poster), .thumb, .thumb :global(.poster), .big, .big :global(.poster) { border-radius: var(--r-poster); }
	.big { box-shadow: var(--shadow-card); }
	.drow > .dline { transition: background-color var(--dur-fast) var(--ease-out); }
	.drow.sel > .dline { background: var(--surface-1); box-shadow: inset 3px 0 0 var(--accent), var(--shadow-card); }
	.drow.sel > .dline:hover { background: var(--surface-1); }
	.drow { animation: rowin var(--dur) var(--ease-out) backwards; }
	/* Rows drawn while scrolling a long list appear at once. */
	.dlrows.vw .drow { animation: none; }
	@keyframes rowin { from { opacity: 0; transform: translateY(-4px); } }
	.st { gap: 8px; }
	.prg { height: 16px; padding: 0 8px; border-radius: var(--r-pill); font-size: 10px; line-height: 16px; }
	.pcl, .bin { border-radius: var(--r-sm); }
	.ghost, .picker { box-shadow: var(--shadow-lift); }
	.dep { border-radius: var(--r-card); box-shadow: var(--shadow-card); }
	/* Resize handle: a wide hit area; the grip widens under the pointer and firms up while held. */
	.dv { height: 12px; border-top: 1px solid var(--line); }
	.dv i { top: 4px; width: 36px; margin-left: -18px; border-radius: var(--r-pill); transition: transform var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.dv:not(.fixed):hover i, .dv:focus-visible i { transform: scaleX(1.5); opacity: 1; }
	.dv:not(.fixed):active i { transform: scaleX(1.8) scaleY(1.3); background: var(--text-muted); }
	/* Panel header: title, underline tabs with a sliding ink, collapse. */
	.phd { position: relative; min-height: 48px; padding: 0 var(--dl-gutter); gap: 0 16px; }
	.phd .xtabs { position: relative; align-self: stretch; gap: 4px; flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; }
	.phd .xtabs::-webkit-scrollbar { display: none; }
	.phd .xtab { min-height: 48px; height: 100%; padding: 0 8px; border-radius: 0; background: none; box-shadow: none; font: var(--t-callout); font-weight: 500; white-space: nowrap; transition: color var(--dur-fast) var(--ease-out); }
	.phd .xtab:hover { color: var(--text); }
	.phd .xtab.on { background: none; box-shadow: none; color: var(--text); }
	.phd .xtab:focus-visible { box-shadow: inset var(--focus-ring); border-radius: var(--r-sm); }
	.ink { position: absolute; left: 0; bottom: 0; width: 1px; height: 2px; background: var(--accent); transform-origin: left; transform: translateX(var(--ix, 0)) scaleX(var(--iw, 0)); transition: transform var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out); pointer-events: none; }
	.ink.gone { opacity: 0; }
	/* Panel body: tab content crossfades in; the footer holds every action. */
	.pb { padding: 12px var(--dl-gutter); gap: 16px; animation: fadein var(--dur) var(--ease-out); }
	.tview { animation: tabin var(--dur) var(--ease-out); }
	@keyframes tabin { from { opacity: 0; transform: translateY(4px); } }
	@keyframes fadein { from { opacity: 0; } }
	/* Actions sit in the panel header, right side, next to the chevron. */
	.pact { position: relative; flex: none; margin-left: auto; display: flex; align-items: center; gap: 6px; min-width: 0; }
	.phd .pcl { margin-left: 0; }
	.pact .xbt { margin: 0; gap: 6px; flex-wrap: nowrap; }
	.pact .xbn { min-height: 32px; padding: 0 10px; border-radius: var(--r-btn); font: var(--t-subhead); font-weight: 500; transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), transform var(--dur-instant) var(--ease-out); }
	.pact .xbn:active:not(:disabled) { transform: scale(0.97); }
	.pact .more { display: inline-flex; }
	.tdl { color: var(--text-muted); font: var(--t-caption); text-decoration: underline; text-decoration-color: var(--line-strong); text-underline-offset: 3px; border-radius: var(--r-sm); }
	.tdl:hover { color: var(--text); }
	.tdl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	a.xbn { display: inline-flex; align-items: center; text-decoration: none; }
	@media (prefers-reduced-motion: reduce) {
		.drow, .pb, .tview { animation: none; }
		.ink, .dv i, .drow > .dline, .pact .xbn { transition: none; }
	}
	/* Phones: a fixed card per row (title, then bar, status and time); no picker. */
	@media (max-width: 640px) {
		.dlgrid { --gap: 8px; --pi: 26px !important; }
		.chead { display: none; }
		.tin { min-width: 0; }
		/* Three short lines in about 64px: title and badge, episode, then status with bar and time on the right. */
	.dline { grid-template-columns: 8px 36px minmax(0, 1fr) 40px auto 44px; grid-template-areas: 'tg post nm nm nm mn' 'tg post st bar time mn'; grid-template-rows: auto 18px; column-gap: 10px; row-gap: 2px; height: auto; min-height: 64px; padding: 4px 12px 4px 0; box-sizing: border-box; align-items: center; -webkit-touch-callout: none; -webkit-user-select: none; user-select: none; -webkit-tap-highlight-color: transparent; transition: background-color 160ms var(--ease-out), transform 160ms var(--ease-out); }
		.dline:global([data-pressing]) { transform: scale(0.985); background: color-mix(in srgb, var(--text) 7%, transparent); transition-duration: 450ms; }
		.drow.picked > .dline { background: color-mix(in srgb, var(--accent) 10%, transparent); }
		.dline > [data-col] { display: none; }
		.dline > .ph { display: none; }
		.dline > [data-col='poster'] { display: block; grid-area: post; align-self: start; }
		.dline > [data-col='title'] { display: flex; grid-area: nm; }
		.dline > [data-col='progress'] { display: flex; grid-area: bar; }
		.dline > [data-col='status'] { display: flex; grid-area: st; }
		.dline > [data-col='time'] { display: block; grid-area: time; }
		.dline > [data-col='title'] { flex-direction: column; justify-content: center; gap: 1px; min-width: 0; }
		.dline > [data-col='title'] .tn { display: none; }
		.dline > [data-col='title'] .tphone { display: inline; }
		.dline > [data-col='title'] .ep { display: block; overflow: hidden; color: var(--text-muted); font: 400 13px/16px var(--font-ui); text-overflow: ellipsis; white-space: nowrap; }
		.dline > [data-col='time'] { justify-self: end; color: var(--text-muted); font: var(--t-footnote); }
		/* The row's ⋯ menu sits at the card's right edge, beside both lines. */
	.dline > [data-col='bin'] { display: flex; grid-area: mn; align-self: center; justify-content: center; }
		/* Not picking: the box stays for screen readers, out of sight and out of the way. */
	.tg .pk:not(.any) { width: 1px; height: 1px; margin: 0; opacity: 0; clip-path: inset(50%); overflow: hidden; pointer-events: none; }
		.dline > [data-col='poster'] { width: 36px; height: 54px; }
		.dline > [data-col='title'] .rel { display: none; }
		.dline > [data-col='title'] .tt { font: 600 15px/19px var(--font-ui); }
		.dline > [data-col='status'] { font: 400 12px/16px var(--font-ui); }
		.dline > [data-col='time'] { font-variant-numeric: tabular-nums; white-space: nowrap; }
		.tg { align-self: center; }
		.tg .pk.any { width: 20px; height: 20px; margin-left: 4px; }
		.dline:has(.pk.any) { grid-template-columns: 32px 36px minmax(0, 1fr) 40px auto 44px; }
		/* The status line: full width for the tabs, with speed and free space as a quiet line under it. */
	.tabrow { -webkit-mask-image: linear-gradient(90deg, #000 calc(100% - 24px), transparent); mask-image: linear-gradient(90deg, #000 calc(100% - 24px), transparent); padding-right: 24px; }
		.tabrow :global([data-testid='dl-stat']) { display: none; }
		.qline { display: flex; align-items: center; gap: 6px; min-height: 24px; margin: 0; padding: 2px var(--dl-gutter) 4px; color: var(--text-muted); font: 400 12px/16px var(--font-ui); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; }
		.qline .qd { color: color-mix(in srgb, var(--ok) 45%, var(--text-muted)); font-weight: 600; }
		/* The speed on the phone's status line opens the speed limit, as the Speed pills do on wider screens; a taller hit area keeps the line itself low. */
	.qbtn { position: relative; display: inline-flex; align-items: center; gap: 6px; min-height: 44px; margin: 0; padding: 0; border: 0; background: none; font: inherit; cursor: pointer; }
		/* 44px tall on its own; the line grows to hold it, starting below the tabs' tap area (their ::after reaches 10px under them). */
	.qbtn::after { content: ''; position: absolute; inset: 0 -8px; }
		.qline:has(.qbtn) { min-height: 48px; padding-top: 4px; padding-bottom: 0; }
		.qbtn:focus-visible { outline: none; border-radius: 6px; box-shadow: var(--focus-ring); }
		.qcap { display: inline-flex; align-items: center; gap: 3px; color: var(--text-muted); font-weight: 400; }
		.qrv { margin-left: auto; min-height: 24px; padding: 0 10px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail-text); font: 600 12px/1 var(--font-ui); }
		/* The selection bar sits over the tab bar, thumb-sized. */
	.selbar { display: none; }
		.psel { position: fixed; left: 8px; right: 8px; bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom) + 8px); z-index: 45; display: grid; gap: 4px; padding: 6px 8px 8px; border: 1px solid var(--line-strong); border-radius: 18px; background: color-mix(in srgb, var(--surface-1) 97%, transparent); backdrop-filter: blur(20px) saturate(1.4); -webkit-backdrop-filter: blur(20px) saturate(1.4); box-shadow: var(--shadow-modal); animation: pselIn 220ms var(--ease-out); }
		.pst { display: flex; align-items: center; gap: 4px; }
		.pst .sn { flex: 1; padding-left: 8px; color: var(--text-muted); font: 400 13px/1 var(--font-ui); }
		.pst .sn b { color: var(--text); font-weight: 600; }
		.plk { min-height: 44px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: var(--accent-text, var(--text)); font: 600 14px/1 var(--font-ui); cursor: pointer; }
		.psa { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 4px; }
		.pa { display: grid; justify-items: center; align-content: center; gap: 4px; min-height: 52px; padding: 4px 2px; border: 0; border-radius: 12px; background: var(--surface-2); color: var(--text); font: 500 11px/1.1 var(--font-ui); cursor: pointer; -webkit-tap-highlight-color: transparent; }
		.pa span { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
		.pa:active, .plk:active { transform: scale(0.97); }
		.pa.bad { color: var(--fail-text); }
		.pa:disabled { opacity: 0.45; }
		.pa:focus-visible, .plk:focus-visible, .qrv:focus-visible { outline: none; box-shadow: var(--focus-ring); }
		.listbox:has(.psel) .dlrows { padding-bottom: 140px; }
		.dline > [data-col='progress'] { height: 4px; align-self: center; }
		.dline > [data-col='progress'] .bl, .spd { display: none; }
		.dline > .spare { display: none; }
		.tg { grid-area: tg; }
		.post { grid-area: post; align-self: start; justify-self: center; }
		.x { grid-template-columns: 56px minmax(0, 1fr); gap: 12px; padding: 4px 8px 12px calc(24px + var(--gap)); }
		.big { width: 56px; height: 84px; }
		.big.square { height: 56px; }
		.fs { white-space: normal; }
		/* Phones: a search icon that opens into the field. */
	.strip { gap: 4px 14px; }
		.picker { display: none; }
	}
	@media (max-width: 640px) {
		.psearch { display: flex; flex: none; align-items: center; gap: 2px; }
		.dv { display: none; }
		.dpane { background: var(--surface-1); }
		.dpane { position: fixed; left: 0; right: 0; bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom)); z-index: 40; height: min(64dvh, 520px) !important; border-radius: 16px 16px 0 0; box-shadow: 0 -12px 32px rgb(0 0 0 / 0.32); }
		.dpane.shut { height: auto !important; }
		.pb { grid-template-columns: 56px minmax(0, 1fr); gap: 12px; }
		.pb .big { width: 56px; height: 84px; border-radius: var(--r-poster); }
		.pb .big.square { height: 56px; }
		.pt { flex-basis: 100%; }
		.hdr { grid-template-columns: minmax(0, 1fr); }
		.gen { grid-template-columns: minmax(0, 1fr); }
	}
	/* Phones: title, tabs, then the actions on their own row, never over the sheet. */
	@media (max-width: 640px) {
		.phd .pt { order: 0; }
		.phd .xtabs { order: 1; }
		.phd .pcl { order: 2; margin-left: auto; }
		.pact { order: 3; flex: 1 0 100%; margin-left: 0; flex-wrap: wrap; padding-bottom: 4px; }
		.pact .xbt { flex-wrap: wrap; }
	}
	/* Phones: a bottom sheet at about 60% that leaves at least three rows of the list in view. */
	@media (max-width: 640px) {
		.dpane { height: min(60dvh, calc(100dvh - var(--tabbar, 56px) - env(safe-area-inset-bottom) - 340px), 520px) !important; }
		.grab { display: grid; place-items: center; order: -1; flex: 1 0 100%; height: 44px; margin: -13px 0; padding: 0; border: 0; background: none; cursor: grab; touch-action: none; }
		.grab i { width: 40px; height: 4px; border-radius: 2px; background: var(--line-strong, var(--text-muted)); }
		.dpane.shut .grab { display: none; }
	}
	@media (prefers-reduced-motion: reduce) {
		.prg i, .tg :global(svg), .bin, .cw, .post { transition: none; }
	}
	@media (max-width: 640px) {
		.dpane { border-radius: var(--r-panel) var(--r-panel) 0 0; box-shadow: var(--shadow-modal); }
		.pb { grid-template-columns: minmax(0, 1fr); }
		.pb .big { display: none; }
		.pact { flex-wrap: wrap; }
	}
	.pt span { font-variant-numeric: tabular-nums; }
	/* The main list in the same family: raised surface, tinted header band, zebra rows, a lift on hover. */
	@media (min-width: 641px) {
		.tin { border: 1px solid var(--line-strong); border-radius: var(--r-card); background: var(--surface-2); box-shadow: var(--shadow-card); overflow: clip; }
		.tin .chead { background: color-mix(in srgb, var(--surface-3) 70%, var(--surface-2)); border-bottom-color: var(--line-strong); }
		.listbox :global(.tin .chead) { background: color-mix(in srgb, var(--surface-3) 70%, var(--surface-2)); }
		.dlrows .drow:last-child { border-bottom: 0; }
		.dlrows .drow:nth-child(even) > .dline { background: color-mix(in srgb, var(--surface-3) 28%, var(--surface-2)); }
		.dlrows .drow > .dline:hover { background: var(--surface-3); box-shadow: 0 2px 8px -4px rgb(0 0 0 / 0.25); }
		.dlrows .drow.picked > .dline { background: color-mix(in srgb, var(--accent) 9%, var(--surface-2)); }
		.dlrows .drow.sel > .dline, .dlrows .drow.sel > .dline:hover { background: var(--surface-1); box-shadow: inset 3px 0 0 var(--accent), var(--shadow-card); }
	}
	/* Our own Downloader marks: short letters on a tinted rounded badge. */
	.pm.cmk { display: inline-grid; place-items: center; width: auto; min-width: 22px; height: 22px; padding: 0 3px; border-radius: 5px; font: 700 9px/1 var(--font-ui); letter-spacing: 0.02em; color: var(--cm); background: color-mix(in srgb, var(--cm) 16%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--cm) 40%, transparent); }
	.pm[data-tone='qb'] { --cm: #2f6fd6; }
	.pm[data-tone='tr'] { --cm: #d23c3c; }
	.pm[data-tone='de'] { --cm: #5b7388; }
	.pm[data-tone='sab'] { --cm: #a87a00; }
	.pm[data-tone='nzb'] { --cm: #2e8b4a; }
	:global([data-theme='dark']) .pm[data-tone='qb'] { --cm: #7aa8f5; }
	:global([data-theme='dark']) .pm[data-tone='tr'] { --cm: #f07a7a; }
	:global([data-theme='dark']) .pm[data-tone='de'] { --cm: #9db3c6; }
	:global([data-theme='dark']) .pm[data-tone='sab'] { --cm: #f0c84a; }
	:global([data-theme='dark']) .pm[data-tone='nzb'] { --cm: #6cd08c; }
	@media (prefers-color-scheme: dark) {
		:global(:root:not([data-theme='light'])) .pm[data-tone='qb'] { --cm: #7aa8f5; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='tr'] { --cm: #f07a7a; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='de'] { --cm: #9db3c6; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='sab'] { --cm: #f0c84a; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='nzb'] { --cm: #6cd08c; }
	}
	/* Info: one raised surface, a dense label/value grid with small group captions inline. */
	.pb { grid-template-columns: max-content minmax(0, 1fr); gap: 20px; }
	/* Swarm: one line, two marks and a ratio pill, on the same three tracks as the header's facet words so each number sits under its word; a four-digit count or a 99.0 ratio still fits the fixed width. */
	.sw1, .facets { display: grid; grid-template-columns: 42px 42px auto; column-gap: 4px; align-items: center; justify-content: start; }
	.sw1 { min-width: 0; font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.sw1 .n { display: inline-flex; align-items: center; gap: 4px; }
	.sw1 .dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--ok); }
	.sw1 .dot.p { background: var(--info); }
	.sw1 .rp { justify-self: start; padding: 1px 6px; border-radius: 999px; background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); font-weight: 500; font-variant-numeric: tabular-nums; }
	.sw1 .rp.good { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok-text); }
	/* The header's three facets: the picked one is bold and carries the sort line. */
	.fc { position: relative; justify-self: start; padding: 0; border: 0; background: none; color: var(--text-muted); font: var(--t-caption); cursor: pointer; }
	.fc.on { color: var(--text); font-weight: 600; }
	.fc:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 3px; }
	.fc .sbar { right: 0; bottom: -7px; }
	@media (min-width: 1800px) {
		.pb { grid-template-columns: max-content minmax(0, 1fr); gap: 24px; }
		.pb .big { width: 200px; height: 300px; }
		.pb .big.square { height: 200px; }
	}
	.pb .big { width: 160px; height: 240px; border-radius: var(--r-card); box-shadow: 0 2px 6px rgb(0 0 0 / 0.08), 0 12px 32px rgb(0 0 0 / 0.16); }
	.pb .big :global(.poster) { border-radius: var(--r-card); }
	.pb .big.square { height: 160px; }
	@media (max-width: 767px) {
		.pb { grid-template-columns: 56px minmax(0, 1fr); gap: 12px; }
		.pb .big { width: 56px; height: 84px; border-radius: var(--r-poster); }
		.pb .big.square { height: 56px; }
	}
	@media (max-width: 640px) {
		.pb { grid-template-columns: minmax(0, 1fr); }
		.pb .big { display: none; }
	}
	/* Themed scrollbars: thin, rounded thumb, clear track, no arrow buttons (light and dark via tokens). */
	/* Chrome drops the ::-webkit rules below (and draws its own bar with arrows) once scrollbar-width/-color are set, so those are Firefox only. */
	@supports not selector(::-webkit-scrollbar) {
		.tw, .pb, .picker { scrollbar-width: thin; scrollbar-color: color-mix(in srgb, var(--text) 24%, transparent) transparent; }
		.tw:hover, .pb:hover { scrollbar-color: color-mix(in srgb, var(--text) 38%, transparent) transparent; }
	}
	.tw::-webkit-scrollbar, .pb::-webkit-scrollbar, .picker::-webkit-scrollbar { width: 12px; height: 12px; background: transparent; }
	/* The track is a soft rounded lane in the rows' own tint, set in from the card's edges. */
	.tw::-webkit-scrollbar-track, .pb::-webkit-scrollbar-track, .picker::-webkit-scrollbar-track { margin: 4px; border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 6%, transparent) padding-box; }
	.tw::-webkit-scrollbar-corner, .pb::-webkit-scrollbar-corner { background: transparent; }
	.tw::-webkit-scrollbar-button, .pb::-webkit-scrollbar-button, .picker::-webkit-scrollbar-button { display: none; width: 0; height: 0; }
	.tw::-webkit-scrollbar-thumb, .pb::-webkit-scrollbar-thumb, .picker::-webkit-scrollbar-thumb { border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 26%, transparent) padding-box; }
	/* Desktop: the table is a card set in like the toolbar above it (14px from the side menu). */
	@media (min-width: 641px) {
		.body { padding: 0 12px 0 14px; }
		/* The toolbar and the pill row are cards on the page: no page-wide rules under them (they ran off the cards' edges). */
	.hdr, .tabrow { border-bottom: 0; }
		/* The list, the divider and the pane are one slab of the table's surface; only its four outer corners are rounded. */
	.col { overflow: clip; border-radius: var(--r-card); }
		.col > .listbox { background: var(--surface-2); border-radius: var(--r-card) var(--r-card) 0 0; }
		.dv { background: var(--surface-2); }
		/* The table's own frame drew a second line inside the card: the rows' lines are enough. */
	.col .tin { border-color: transparent; box-shadow: none; }
		/* The header and row checkboxes sit centred in the same 28px lane. */
	.hk, .tg:has(> .pk) { display: flex !important; align-items: center; justify-content: center; min-width: 28px; padding: 0 !important; }
		.hk .hpk, .tg > .pk { margin: 0; translate: none; transform: none; }
		/* With its centring margin gone, the box is centred by its top. */
	.tg > .pk { left: 6px; top: calc(50% - 8px); }
		/* The poster column starts under the checkbox lane's divider: centre its icon and posters between that line and the next. */
	.ch[data-col='poster'] .lbl, .dline > .post { transform: translateX(calc(var(--gap) / 2 + 0.5px)); }
	}
	.tw::-webkit-scrollbar-thumb:hover, .pb::-webkit-scrollbar-thumb:hover, .picker::-webkit-scrollbar-thumb:hover { background: color-mix(in srgb, var(--text) 40%, transparent) padding-box; }
	/* Detail panel in the toolbar's language: a glass card, pill tabs, a status dot, cards inside. */
	.hs { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text-muted); }
	.hdot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.hdot.paused { background: var(--text-muted); }
	.hdot.queued { background: var(--info); }
	.hdot.held { background: var(--wait); }
	.hdot.bad { background: var(--fail); }
	.pt { gap: 3px; padding: 8px 0; }
	.pt b { font: 600 17px / 1.25 var(--font-ui); letter-spacing: -0.01em; }
	.phd .xtabs { align-self: center; gap: 2px; padding: 2px; border: 1px solid var(--line); border-radius: 999px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); }
	.phd .xtab { min-height: 28px; height: auto; padding: 0 12px; border-radius: 999px; color: var(--text-muted); transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.phd .xtab:hover { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.phd .xtab.on, .phd .xtab.on:hover { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); }
	.phd .xtab:focus-visible { border-radius: 999px; box-shadow: var(--focus-ring); }
	.phd .ink { display: none; }
	.pb .big { width: 120px; height: 180px; border: 1px solid var(--line); box-shadow: none; }
	.pb .big.square { height: 120px; }
	@media (min-width: 641px) {
		/* The pane is part of the table's slab: same surface, same edges, no frame of its own; the divider's hairline and grip are the seam. */
	.dpane { margin: 0; border-radius: 0; background: var(--surface-2); overflow: hidden; }
		.dpane .phd { padding: 8px var(--dl-gutter); border: 0; border-bottom: 1px solid var(--line); border-radius: 0; background: transparent; box-shadow: none; }
		.pb { padding: 12px var(--dl-gutter) 16px; background: transparent; }
		/* The title takes the room it needs (up to 42% of the header) and the tabs centre in what is left. */
	.dpane .pt { flex: 0 1 auto; min-width: 0; max-width: 42%; }
		.dpane .phd .xtabs { margin-inline: auto; }
	}
	/* Narrower desktops: the title fills the first row beside the actions, so the tabs wrap to a second row of their own. Never three rows. */
	@media (min-width: 641px) and (max-width: 1240px) {
		.dpane .pt { max-width: none; flex: 1 1 200px; }
		.dpane .phd .xtabs { order: 2; flex: 0 1 auto; margin: 0 auto 6px 0; }
		.dpane .pact, .dpane .pcl { order: 1; flex: none; }
	}
	@media (min-width: 1800px) {
		.pb .big { width: 140px; height: 210px; }
		.pb .big.square { height: 140px; }
	}
	@media (max-width: 767px) {
		.pb .big { width: 56px; height: 84px; }
		.pb .big.square { height: 56px; }
	}
	@media (max-width: 640px) {
		.pb .big { display: none; }
		.phd .xtabs { max-width: 100%; }
		.phd .xtab { padding: 0 12px; }
	}
	/* Phones: every control a thumb can hit is at least 32px tall. */
	@media (max-width: 640px) {
		.tab, .stab { height: 32px; }
		.pcl { width: 32px; height: 32px; }
		.trchip, .qrv, .lk, .xbn { min-height: 32px; }
		.phd .xtab { min-height: 32px; }
	}
	/* Touch phones: every control a finger hits gets a 44px target; the
	   hit area grows with an invisible ::after, so nothing moves and desktop is unchanged. */
	@media (max-width: 767px) and (pointer: coarse) {
		.tab, .stab, .trchip { position: relative; }
		.tab::after, .stab::after, .trchip::after { content: ''; position: absolute; inset: -10px 0; }
		.subin { overflow: clip visible; }
	}
	@media (max-width: 640px) and (pointer: coarse) {
		.tabrow { height: 45px; }
		.psb, .psx { position: relative; }
		.psb::after { content: ''; position: absolute; inset: -6px 0 -6px -6px; }
		.psearch:not(.open) .psb::after { right: -6px; }
		.psx::after { content: ''; position: absolute; inset: -6px -6px -6px 0; }
	}
	/* A disabled bin is only a stand-in: a tap on it goes to the row underneath. */
	@media (pointer: coarse) {
		.bin:disabled { pointer-events: none; }
	}
</style>
