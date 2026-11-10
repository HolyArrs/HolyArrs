<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Downloads page body: a header strip with speed and time left, status tabs with sub-options, the list, the detail pane and the selection bars.
	import Poster from '#lib/ui/Poster.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { COL, MAX_W, MIN_W, move, quality, sortRows, visible, type ColId } from './columns.ts';
	import { PRIORITIES, clientOk, paneExtras, rowItems, usenetQueued, type Priority, type RowItem } from './rowmenu.ts';
	import DlBar from './DlBar.svelte';
	import DlStatus from './DlStatus.svelte';
	import DlMorePop from './DlMorePop.svelte';
	import { createColumns } from './columns-grid.svelte.ts';
	import DlColumnPicker from './DlColumnPicker.svelte';
	import DlSelBars from './DlSelBars.svelte';
	import DlRow from './DlRow.svelte';
	import DlPane from './DlPane.svelte';
	import { Downloaders } from './clients.svelte.ts';
	import { PaneDetail } from './pane-detail.svelte.ts';
	import { tabsOf, type PaneTab } from './pane-facts.ts';
	import GhostEmpty from './GhostEmpty.svelte';
	import { reviewCount, reviewWord } from './status-views.ts';
	import { listEsc, reviewShown, searchShortcut } from './keys.ts';
	import { overlayScroll } from './overlay-scroll.ts';
	import { dlTop } from './dl-top.svelte.ts';
	import { size as bytes, categoryWords, protocolOf, type KindKey } from './size.ts';
	import { kindOf, rowName } from './downloaders.ts';
	import type { ImportRecord } from './types.ts';
	import { ALL, counts, emptyOf, pick, showOf, statusWord, subCounts, tabsFor } from './status-tabs.ts';
	import { controllable, pausable } from './usenet.ts';
	import { poster, who } from './row-art.ts';
	import { byTracker, countLabel, pickRange, problemWords, toolbarTargets, trackerOptions, type BulkAction, type SteerMode } from './manage.ts';
	import { problem } from './problem.ts';
	import { historyTotals, inHistory, matches, moving, totals } from './sections.ts';
	import { tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto, replaceState } from '$app/navigation';
	import { slide, fade, type TransitionConfig } from 'svelte/transition';
	import { to } from '../core/paths.ts';
	import { dlScope } from '../core/search-field.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { api, message } from '../core/api.ts';
	import { duration } from '../core/format.ts';
	import { session } from '../core/session.svelte.ts';
	import { reducedMotion } from '../core/motion.ts';
	import { DUR, easeIn, easeOut } from '../core/transitions.ts';
	import { type Decision, type DownloadRow } from '../core/types.ts';
	import type { TitleView } from '../core/types.ts';
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
		/** Notes from the page, such as a Downloader not answering, shown in the status popover. */
		notes?: string[];
		importOf: Map<number, ImportRecord>;
		titles: Map<number, TitleView>;
		/** False while Library titles are still loading, so posters show a plain tile. */
		titlesReady?: boolean;
		busy: number;
		/** A torrent Downloader adds the Seeding sub-tab under Done. */
		torrent?: boolean;
		decisionsOf: (r: DownloadRow) => Decision[];
		/** Row whose details are open (0 for none). */
		openId?: number;
		onfail: (r: DownloadRow) => void;
		onremove: (r: DownloadRow, withFiles?: boolean) => void;
		ondone: () => void;
		onact?: (r: DownloadRow, action: ClientAction) => void;
		onall?: (pause: boolean) => void;
		onlimit?: () => void;
		onadd?: () => void;
		onremovemany?: (rows: DownloadRow[], withFiles?: boolean) => void;
		onretry?: (rows: DownloadRow[]) => void;
		onbulk?: (rows: DownloadRow[], action: BulkAction) => void;
		onsteer?: (rows: DownloadRow[], mode: SteerMode) => void;
		/** Changes to torrents HolyArrs did not send need the settings permission; the server refuses them otherwise. */
		changeOutside?: boolean;
		/** A selection action is in flight; the selection bars wait for it. */
		batchBusy?: boolean;
		/** Someone who may not manage the Library sees only their own downloads, with no actions. */
		watch?: boolean;
		/** Upload speed summed by the server over every sending row; falls back to the rows on hand. */
		upTotal?: number;
	} = $props();
	let kindOn = $state<KindKey>('');
	const typed = $derived(kindOn ? everyRow.filter((r) => kindOf(r) === kindOn) : everyRow);
	// The Downloads search narrows every tab, so the tab counts follow it.
	const q = $derived(dlScope.term);
	const searched = $derived(q.trim() ? typed.filter((r) => matches(r, q)) : typed);
	let srcOn = $state<'' | 'usenet' | 'torrent'>('');
	const bySource = $derived(srcOn ? searched.filter((r) => protocolOf(r) === srcOn) : searched);
	let trackOn = $state('');
	const trackOpts = $derived(trackerOptions(bySource));
	const trackHosts = $derived(trackOpts.filter((o) => o.key.startsWith('host:')));
	const trackProblems = $derived(bySource.filter((r) => r.tracker_problem).length);
	const trackKey = $derived(trackOn === 'p:any' ? (trackProblems ? trackOn : '') : trackOpts.some((o) => o.key === trackOn) ? trackOn : '');
	const rows = $derived(trackKey ? bySource.filter((r) => byTracker(r, trackKey)) : bySource);
	const kindCount = (k: string) => everyRow.filter((r) => kindOf(r) === k).length;
	const srcCount = (k: string) => everyRow.filter((r) => protocolOf(r) === k).length;
	// The picked tab and sub-tab live in the address as show=tab or tab.sub.
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
			/* The router is not ready yet; the next pick writes the address. */
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
		setTimeout(() => document.querySelector('.pw.on')?.scrollIntoView({ block: 'nearest', inline: 'nearest' }), 220);
	}
	function chooseSub(k: string) {
		sub = sub === k ? '' : k;
		remember();
	}
	const allQueue = $derived(typed.filter((r) => !inHistory(r)));
	const isDone = $derived(on === 'done');
	const newest = (a: DownloadRow, b: DownloadRow) => (b.finished_at || b.updated_at).localeCompare(a.finished_at || a.updated_at);
	const tabOrder = $derived(isDone ? [...shown].sort(newest) : shown);
	// A sorted column overrides the tab's own order.
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
			// Swarm sorts by the facet picked in its header.
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
	// Falls back to the Downloaders' own reports, which cover seeding with no downloading row.
	const upSum = $derived(dls.reporting.length ? dls.sumOf('up_speed') : (upTotal ?? sum.up));
	const openRow = $derived(everyRow.find((r) => r.id === openId));
	// The tightest cap names the Speed limit button.
	const capNow = $derived(Math.min(...dls.reporting.map((c) => c.status?.speed_limit ?? 0).filter((n) => n > 0)));
	const toReview = $derived(reviewCount(allQueue));
	const etaAll = $derived(Math.max(sum.eta || 0, ...dls.clients.map((c) => (c.error ? 0 : (c.status?.time_left ?? 0)))));
	const etaWords = $derived(etaAll ? duration(etaAll) : '');
	// Usenet has no upload, so the Up pill goes when only Usenet Downloaders exist.
	const noUp = $derived(dls.loaded && dls.clients.length > 0 && !dls.clients.some((c) => c.protocol === 'torrent'));
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
	// Ctrl or Cmd click toggles a row, Shift click picks a range, Ctrl+A picks every row.
	let picked = $state<Set<number>>(new Set());
	let selecting = $state(false);
	const choosing = $derived(picked.size > 0 || selecting);
	let anchor = 0;
	const pickedRows = $derived(list.filter((r) => picked.has(r.id)));
	// Toolbar actions apply to the ticked rows, else the open row. Start and Pause follow pausable, as the pane's own button does (NZBGet takes only those two).
	const able = (r: DownloadRow) => pausable(r) && !r.client_locked && busy !== r.id;
	const startT = $derived(toolbarTargets(pickedRows, openRow, 'paused', able));
	const pauseT = $derived(toolbarTargets(pickedRows, openRow, 'running', able));
	const removeT = $derived(toolbarTargets(pickedRows, openRow, 'any', (r) => busy !== r.id));
	$effect(() => {
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
	// Torrents a torrent Downloader can steer by hash. Getters, so each word follows the language picked when it is read.
	const ROW_WORD: Record<RowItem, string> = {
		get pause() { return t('downloads.act.pause'); },
		get resume() { return t('downloads.act.start'); },
		get retry() { return t('downloads.act.again'); },
		get up() { return t('downloads.act.up'); },
		get down() { return t('downloads.act.down'); },
		get top() { return t('downloads.act.top'); },
		get bottom() { return t('downloads.act.bottom'); },
		get priority() { return t('downloads.act.priority'); },
		get library() { return t('downloads.act.library'); },
		get remove() { return t('downloads.act.remove'); }
	};
	// Usenet jobs move and take a priority one at a time; there is no bulk call.
	const usenetItem = (r: DownloadRow) => `/downloaders/${r.downloader_id}/items/${encodeURIComponent(r.hash ?? '')}`;
	const MOVED: Record<string, string> = {
		get up() { return t('downloads.act.movedUp'); },
		get down() { return t('downloads.act.movedDown'); },
		get top() { return t('downloads.act.movedTop'); },
		get bottom() { return t('downloads.act.movedBottom'); }
	};
	let queueBusy = $state(0);
	async function usenetCall(r: DownloadRow, run: () => Promise<unknown>, done: string) {
		queueBusy = r.id;
		try {
			await run();
			toasts.push(done, 'ok');
		} catch (e) {
			toasts.push(t('downloads.act.updateFailed'), 'fail', { body: message(e) });
		} finally {
			if (queueBusy === r.id) queueBusy = 0;
		}
	}
	const usenetMove = (r: DownloadRow, k: string) => usenetCall(r, () => api.post(`${usenetItem(r)}/${k}`), MOVED[k] ?? t('downloads.act.moved'));
	const usenetPriority = (r: DownloadRow, p: Priority, label: string) => usenetCall(r, () => api.post(`${usenetItem(r)}/priority`, { priority: p }), t('downloads.act.prioritySet', { label }));
	// Shared by the row menu and the pane menu.
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
		if (!r.can_act) items.unshift({ kind: 'heading', label: t('downloads.act.cannotChange') });
		return items;
	}
	// The pane menu is the row's list minus what the pane shows as buttons, plus a torrent's checks, force start, sheets and Mark failed.
	const PANE_BUTTONS = new Set<RowItem>(['pause', 'resume', 'retry', 'library', 'remove']);
	function paneMenu(r: DownloadRow): MenuItem[] {
		if (!r.can_act) return [];
		const off = busy === r.id || queueBusy === r.id;
		const items = menuItems(r, rowItems(r).filter((k) => !PANE_BUTTONS.has(k)));
		const extra = new Set(paneExtras(r, { steer: !!onsteer && mayEdit(r), fail: !watch && !inHistory(r) && !r.client_item }));
		if (extra.has('recheck')) items.push({ label: t('downloads.act.checkAgain'), disabled: off, onselect: () => onact?.(r, 'recheck') }, { label: t('downloads.act.askTrackers'), disabled: off, onselect: () => onact?.(r, 'reannounce') });
		if (extra.has('force')) items.push({ label: t('downloads.act.forceStart'), disabled: off, onselect: () => onbulk?.([r], 'force') });
		if (extra.has('steer')) {
			items.push({ kind: 'separator' });
			for (const m of STEER) items.push({ label: m.label, disabled: off, onselect: () => onsteer?.([r], m.mode) });
		}
		if (extra.has('fail')) {
			if (items.length) items.push({ kind: 'separator' });
			items.push({ label: t('downloads.act.markFailed'), destructive: true, disabled: off, onselect: () => onfail(r) });
		}
		return items;
	}
	// Only qBittorrent takes location, speed and seeding changes.
	const steerable = (r: DownloadRow) => !!onsteer && mayEdit(r) && !!r.downloader_id && !!r.hash && protocolOf(r) === 'torrent' && controllable(r.downloader_kind);
	// Location, speed and seeding sheets, file priority, tracker edits and Download again on a torrent HolyArrs did not send need the settings permission, as on the server.
	const mayEdit = (r: DownloadRow) => clientOk(r) && (!r.outside || changeOutside);
	// Bulk calls skip read-only rows so one of them cannot fail the rest.
	const actable = (rows: DownloadRow[]) => rows.filter(clientOk);
	const bulkRows = $derived(actable(pickedRows).filter((r) => protocolOf(r) === 'torrent' && controllable(r.downloader_kind) && !!r.downloader_id && !!r.hash));
	const STEER: { mode: SteerMode; label: string; short: string }[] = [
		{ mode: 'location', get label() { return t('downloads.steer.locationMenu'); }, get short() { return t('downloads.steer.location'); } },
		{ mode: 'speed', get label() { return t('downloads.steer.speedMenu'); }, get short() { return t('downloads.steer.speed'); } },
		{ mode: 'seed', get label() { return t('downloads.steer.seedMenu'); }, get short() { return t('downloads.steer.seed'); } }
	];
	const allItems = $derived(watch ? [] : [{ label: t('downloads.all.select'), onselect: () => (selecting = true) }, { label: t('downloads.all.startAll'), onselect: () => onall?.(false) }, { label: t('downloads.all.pauseAll'), onselect: () => onall?.(true) }, { label: t('downloads.all.speedLimit'), onselect: () => onlimit?.() }, { label: t('downloads.all.held'), onselect: () => void goto(to('/library/held')) }]);
	let paneOpen = $state(true);
	const openTab = $derived(openRow ? tabNow(openRow) : '');
	const pane = new PaneDetail({ openId: () => openId, openTab: () => openTab, paneOpen: () => paneOpen, rows: () => everyRow, watch: () => watch });
	let colEl = $state<HTMLElement>();
	const maxPane = () => Math.max(160, (colEl?.clientHeight ?? 600) - 140);
	let psOpen = $state(false);
	let psEl = $state<HTMLInputElement>();
	// Several torrents use one bulk call; a single torrent and every Usenet job (which the bulk call cannot address) use their own call, so none is dropped.
	const runOn = (t: { rows: DownloadRow[] }, one: 'resume' | 'pause', many: BulkAction) => {
		const tor = t.rows.filter((r) => protocolOf(r) === 'torrent' && !!r.downloader_id && !!r.hash);
		if (tor.length > 1) onbulk?.(tor, many);
		for (const r of t.rows) if (tor.length < 2 || !tor.includes(r)) onact?.(r, one);
	};
	const barActs = $derived([
		...(onadd ? [{ key: 'add', icon: 'plus' as const, label: t('downloads.bar.add'), hint: t('downloads.act.addHint'), disabled: false, run: () => onadd?.() }] : []),
		{ key: 'start', icon: 'play' as const, label: countLabel(t('downloads.act.start'), startT.rows.length), hint: startT.why ? t('downloads.act.withWhy', { label: t('downloads.act.start'), why: startT.why }) : countLabel(t('downloads.act.start'), startT.rows.length), disabled: !startT.rows.length, run: () => runOn(startT, 'resume', 'start'), more: [{ label: t('downloads.all.startAll'), onselect: () => onall?.(false) }] },
		{ key: 'pause', icon: 'pause' as const, label: countLabel(t('downloads.act.pause'), pauseT.rows.length), hint: pauseT.why ? t('downloads.act.withWhy', { label: t('downloads.act.pause'), why: pauseT.why }) : countLabel(t('downloads.act.pause'), pauseT.rows.length), disabled: !pauseT.rows.length, run: () => runOn(pauseT, 'pause', 'pause'), more: [{ label: t('downloads.all.pauseAll'), onselect: () => onall?.(true) }] },
		{ key: 'remove', icon: 'trash' as const, label: countLabel(t('downloads.act.remove'), removeT.rows.length), hint: removeT.why ? t('downloads.act.withWhy', { label: t('downloads.act.remove'), why: removeT.why }) : countLabel(t('downloads.act.remove'), removeT.rows.length), disabled: !removeT.rows.length, run: () => (removeT.rows.length === 1 ? onremove(removeT.rows[0]) : onremovemany?.(removeT.rows)) },
	]);
	const focusedRow = () => {
		const el = (document.activeElement as HTMLElement | null)?.closest<HTMLElement>('[data-dl-id]');
		return el ? everyRow.find((r) => r.id === Number(el.dataset.dlId)) : undefined;
	};
	let rowFocus = $state(0);
	/** One Tab stop for the list: the last focused row, else the open one, else the first. */
	const tabRow = $derived(list.some((r) => r.id === rowFocus) ? rowFocus : list.some((r) => r.id === openId) ? openId : (list[0]?.id ?? 0));
	function navKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && subOn) {
			e.preventDefault();
			chooseSub(subOn);
			(e.currentTarget as HTMLElement).querySelector<HTMLElement>('.pw.on .tab')?.focus();
			return;
		}
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
		// Only the pills take the arrows; the Review button and status popover keep theirs.
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
		// Do not steal focus from a dialog, sheet or open popover.
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
		// A menu or sheet already handled this key.
		if (e.defaultPrevented) return;
		// Ctrl+F jumps to the files filter only inside the pane; elsewhere the browser's Find stays.
		if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'F') && pane.fqEl && openRow && paneOpen && tabNow(openRow) === 'Files' && (e.target as HTMLElement | null)?.closest('.dpane')) {
			e.preventDefault();
			pane.fqEl.focus();
			pane.fqEl.select();
			return;
		}
		if (!watch && (e.ctrlKey || e.metaKey) && (e.key === 'a' || e.key === 'A')) {
			const el = e.target as HTMLElement | null;
			if (el?.closest('[data-dl-id], .dlrows, [data-testid="dl-listbox"]') && !el.closest('input, textarea, select, [contenteditable="true"]')) {
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
		const el = e.target as HTMLElement | null;
		if (!el || el.closest('input, select, textarea, [contenteditable="true"], [role="tab"], [role="separator"], dialog, [role="dialog"], [role="menu"], [popover]')) return;
		if (e.key === 'Escape') {
			if (listEsc(picked.size, !!openId) === 'close') {
				// Focus goes back to the pane's row, never to nowhere.
				const line = el.closest('.dpane') ? cols.listEl?.querySelector<HTMLElement>(`[data-dl-id="${openId}"] .dline`) : null;
				openId = 0;
				e.preventDefault();
				line?.focus();
			}
			return;
		}
		if (e.key === 'Delete') {
			if (el.closest('button, a')) return;
			if (pickedRows.length && onremovemany) {
				e.preventDefault();
				onremovemany(pickedRows, e.shiftKey);
				return;
			}
			const target = focusedRow();
			if (!target) return;
			e.preventDefault();
			if (target.can_act) onremove(target, e.shiftKey);
			else toasts.push(t('downloads.list.cantRemove'), 'info', { body: t('downloads.list.notFromHere') });
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
	// The open row's poster grows from the row's small one into the details (FLIP).
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
	// A click on a row toggles its details, except on the bin, a link, a control, or while selecting text.
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
	}
</script>
{#snippet header()}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dline chead" data-testid="dl-colhead" oncontextmenu={cols.headMenu}>
		<span class="hk">{#if !watch}<input type="checkbox" class="hpk" checked={list.length > 0 && picked.size === list.length} indeterminate={picked.size > 0 && picked.size < list.length} aria-label={t('downloads.list.selectAll')} title={t('downloads.list.selectAllTitle')} onchange={() => (picked.size === list.length ? clearPicked() : (picked = new Set(list.map((r) => r.id))))} data-testid="dl-head-pick" />{/if}</span>
		{#each cols.vis as id (id)}
			{@const c = COL[id]}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			{@const so = cols.layout.sort?.id === id ? cols.layout.sort : undefined}
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<span class="ch" class:end={c.num} class:fixed={!!c.fixed} class:moving={cols.ghost?.id === id} class:sorted={!!so} data-col={id} data-testid="dl-head" role={id === 'swarm' ? 'group' : 'button'} aria-label={cols.headName(id, so)} tabindex={id === cols.vis[0] ? 0 : -1} onpointerdown={(e) => cols.headDown(e, id)} onclick={(e) => { if (e.detail === 0) cols.sortBy(id); }} onkeydown={(e) => cols.headKey(e, id)} title={t('downloads.list.headTitle')}>{#if id === 'proto' && !cols.srcName}<span class="lbl"><span class="sr">{t('downloads.col.proto')}</span><Icon name="plug" size={15} /></span>{:else if id === 'poster'}<span class="lbl"><span class="sr">{t('downloads.col.poster')}</span><Icon name="image" size={15} /></span>{:else if id === 'kind' && !cols.kindWide}<span class="lbl"><span class="sr">{t('downloads.col.kind')}</span><Icon name="grid" size={15} /></span>{:else if id !== 'swarm'}<span class="lbl">{c.label}</span>{/if}{#if id === 'swarm'}<span class="facets" role="group" aria-label={t('downloads.list.swarmSort')}>{#each cols.FACETS as f (f.id)}<button type="button" class="fc" class:on={so?.facet === f.id} aria-pressed={so?.facet === f.id} tabindex="-1" onpointerdown={(e) => e.stopPropagation()} onclick={(e) => { e.stopPropagation(); cols.sortBy('swarm', f.id); }} data-testid="dl-facet-{f.id}">{f.label}{#if so?.facet === f.id}<span class="sbar" class:desc={so.desc} aria-hidden="true" data-testid="dl-sort"></span>{/if}</button>{/each}</span>{:else if so}<span class="sbar" class:desc={so.desc} aria-hidden="true" data-testid="dl-sort"></span>{/if}{#if !c.fixed}<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions --><span class="grip" role="separator" aria-orientation="vertical" aria-label={t('downloads.list.resize', { label: c.label })} aria-valuenow={cols.wOf(id)} aria-valuemin={cols.heads[id] ?? MIN_W} aria-valuemax={MAX_W} tabindex="0" title={t('downloads.list.gripTitle')} data-testid="dl-grip" data-grip={id} onpointerdown={(e) => cols.grab(e, id)} ondblclick={() => cols.fitOne(id)} onkeydown={(e) => cols.nudge(e, id)}></span>{/if}</span>
		{/each}
		<span></span>
		<span class="cm"><Menu label={t('downloads.col.columns')} icon="more" width={180} items={[{ label: t('downloads.list.columnsMenu'), icon: 'more', onselect: cols.fromMenu }, { label: t('downloads.list.resetColumns'), icon: 'refresh', onselect: cols.resetAll }]} /></span>
		{#if cols.lineX !== null}<span class="ins" style="left: {cols.lineX}px" data-testid="dl-insert" aria-hidden="true"></span>{/if}
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
<svelte:window onpointerdown={cols.outside} onkeydown={listKey} />
<div class="dlgrid" bind:this={cols.grid} style="--tpl: {cols.tpl}; --c-in: {cols.inner}px; --pi: {cols.posterW - 6}px; --lg: {cols.srcBig ? 24 : 18}px">
<div class="app" data-testid="dl-app" bind:this={cols.appEl} style="--app-h: {cols.appH}px">
<div class="strip hdr" data-testid="dl-strip">
	<div class="psearch" class:open={psOpen || !!dlScope.term} data-testid="dl-psearch">
		<button type="button" class="psb" aria-label={t('downloads.list.search')} aria-expanded={psOpen || !!dlScope.term} onclick={() => { psOpen = true; tick().then(() => psEl?.focus()); }} data-testid="dl-psearch-btn"><Icon name="search" size={16} /></button>
		{#if psOpen || dlScope.term}
			<input type="search" bind:this={psEl} bind:value={dlScope.term} placeholder={t('downloads.list.search')} aria-label={t('downloads.list.search')} onblur={() => { if (!dlScope.term) psOpen = false; }} onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); dlScope.term = ''; psOpen = false; } }} data-testid="dl-psearch-input" />
			{#if dlScope.term}<button type="button" class="psx" aria-label={t('downloads.list.clearSearch')} onclick={() => { dlScope.term = ''; psEl?.focus(); }}><Icon name="x" size={14} /></button>{/if}
		{/if}
	</div>
	<DlBar bind:kindOn bind:srcOn bind:trackOn trackAll={bySource.length} trackHosts={trackHosts} {trackProblems} {kindCount} {srcCount} actions={barActs} {watch} bind:term={dlScope.term} shown={searched.length} total={typed.length} side={dlTop.collapsed} onside={() => dlTop.toggle()} searchKey={dlTop.searchKey} {allItems} />
</div>
<div class="body">
<div class="col" bind:this={colEl}>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="tabrow" data-testid="dl-tabrow" onkeydown={navKey}><div class="tabs" role="tablist" aria-label={t('downloads.list.status')}>{@render pills()}</div>{#if problemWords(trackProblems)}<button type="button" class="trchip" class:on={trackKey === 'p:any'} aria-pressed={trackKey === 'p:any'} onclick={() => (trackOn = trackKey === 'p:any' ? '' : 'p:any')} data-testid="dl-tracker-issues">{problemWords(trackProblems)}</button>{/if}<DlStatus down="{bytes(sum.speed)}/s" up={noUp ? '' : `${bytes(upSum)}/s`} eta={etaWords} free={dls.freeAll.length ? bytes(Math.min(...dls.freeAll)) : ''} today={dls.sumOf('day_down') ? bytes(dls.sumOf('day_down')) : ''} session={dls.sumOf('session_down') ? bytes(dls.sumOf('session_down')) : ''} review={reviewShown(watch, toReview)} onreview={openReview} more={morePop} limit={Number.isFinite(capNow) ? `${bytes(capNow)}/s` : ''} onlimit={watch ? undefined : onlimit} /></div>
<p class="qline" data-testid="dl-qline">{#if onlimit && !watch}<button type="button" class="qd qbtn" onclick={() => onlimit?.()} title={Number.isFinite(capNow) ? t('downloads.list.limitChange', { speed: `${bytes(capNow)}/s` }) : t('downloads.list.setLimit')} data-testid="dl-q-down"><span class="sr">{t('downloads.list.speedSr')} </span>↓ {bytes(sum.speed)}/s{#if Number.isFinite(capNow)}<span class="qcap"><Icon name="gauge" size={12} /><span class="sr">{t('downloads.list.limitSr')} </span>{bytes(capNow)}/s</span>{:else}<span class="sr">. {t('downloads.list.setLimit')}</span>{/if}</button>{:else}<span class="qd" data-testid="dl-q-down">↓ {bytes(sum.speed)}/s</span>{/if}{#if dls.freeAll.length}<span class="qs" aria-hidden="true">·</span><span data-testid="dl-q-free">{t('downloads.list.free', { size: bytes(Math.min(...dls.freeAll)) })}</span>{/if}{#if etaWords}<span class="qs" aria-hidden="true">·</span><span>{t('downloads.list.left', { time: etaWords })}</span>{/if}{#if reviewShown(watch, toReview)}<button type="button" class="qrv" onclick={openReview} data-testid="dl-q-review">{reviewWord(toReview)}</button>{/if}</p>
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="listbox" bind:this={cols.listEl} data-testid="dl-listbox" use:overlayScroll={{ header: ".chead" }}>
{#if (pickedRows.length || selecting) && !watch}
	<DlSelBars {pickedRows} {bulkRows} steer={STEER} {startT} {pauseT} {batchBusy} {steerable} {mayEdit} {runOn} {onbulk} {onsteer} {onretry} {onremovemany} onall={() => (picked = new Set(list.map((r) => r.id)))} onclear={clearPicked} />
{/if}
{#if list.length}
	<div class="tw" onscroll={cols.sync} data-testid="dl-tablebox"><div class="tin">
		{@render header()}
		<ul class="dlrows" class:vw={cols.windowed} bind:this={cols.ulEl} data-testid="dl-list" data-tab={on} data-rows={list.length} style={cols.windowed ? `padding-top: ${cols.span.a * cols.rh}px; padding-bottom: ${(list.length - cols.span.b) * cols.rh}px` : undefined}>
			{#each cols.shownRows as r (r.id)}<DlRow {r} hist={inHistory(r)} {cols} {watch} {picked} {choosing} {openId} {tabRow} {importOf} {titles} {titlesReady} {rowMenu} onpickone={pickOne} onpickto={pickTo} onrowclick={rowClick} onrowkey={rowKey} onrowfocus={(id) => (rowFocus = id)} />{/each}
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
	<DlPane r={openRow} bind:paneOpen bind:openId tab={tabNow(openRow)} ontab={(id, t) => (xtabOn[id] = t)} {pane} {importOf} {titles} {titlesReady} {watch} {busy} {decisionsOf} {ondone} {onact} {onretry} {onremove} {mayEdit} {paneMenu} {maxPane} />
{/if}
</div>
</div>
{#snippet morePop()}<DlMorePop {notes} {dls} {watch} />{/snippet}
<div class="vh"><span data-testid="sb-health">{dls.health}</span></div>
</div>
<DlColumnPicker {cols} />
</div>
<style>
	.ch[data-col='proto']:has(:global(svg)) { justify-content: center; }
	.ch[data-col='kind']:has(:global(svg)) { justify-content: center; }
	.ch[data-col='proto'] .lbl, .ch[data-col='kind'] .lbl { display: flex; }
	.ch[data-col='poster'] { justify-content: center; }
	.ch[data-col='poster'] .lbl { display: flex; }
	@media (min-width: 641px) {
		.ch[data-col='proto']:has(:global(svg)) .lbl, .ch[data-col='poster'] .lbl, .ch[data-col='kind']:has(:global(svg)) .lbl { transform: translateX(calc(var(--gap) / 2 - 4px)); }
	}
	.ch[data-col='proto'] .lbl { overflow: visible; text-overflow: clip; }
	.strip { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 22px; padding: 8px 0; margin-bottom: 10px; border-bottom: 1px solid var(--line); font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.strip b { font-weight: 600; }
	.dlgrid { --gap: 6px; min-width: 0; }
	.tw { overflow-x: auto; overflow-y: hidden; max-width: 100%; }
	/* Muted text is mixed from the text colour to stay readable (4.5:1) on tinted rows. */
	.tin { min-width: var(--c-in); --text-muted: color-mix(in srgb, var(--text) 66%, var(--surface-2)); }
	.dlrows { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--line); }
	.hk { position: relative; align-self: stretch; display: flex; align-items: center; }
	.hk::after { content: ''; position: absolute; top: 0; bottom: 0; right: calc(3px - var(--gap)); width: 1px; background: var(--line-strong); pointer-events: none; }
	.hpk { width: 16px; height: 16px; margin: 0 0 0 4px; accent-color: var(--accent); cursor: pointer; }
	.hpk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.qline { display: none; }
	.dline { display: grid; grid-template-columns: var(--tpl); align-items: center; column-gap: var(--gap); height: max(44px, calc(var(--pi, 26px) * 1.5 + 6px)); }
	@media (min-width: 641px) {
		.dline:not(.chead) > [data-col] { min-width: 0; overflow: hidden; }
	}
	:global(html.col-resizing), :global(html.col-resizing *) { cursor: col-resize !important; user-select: none !important; }
	/* The header row is exactly 28px so every divider stays inside it. */
	.chead { grid-template-rows: minmax(0, 1fr); position: relative; height: 28px; border-bottom: 1px solid var(--line); color: color-mix(in srgb, var(--text) 78%, transparent); font: var(--t-caption); font-weight: 600; }
	.chead + .dlrows { border-top: 0; }
	.chead:hover { background: none; }
	.ch { position: relative; align-self: stretch; display: flex; align-items: center; min-width: 0; }
	.ch.end { justify-content: flex-end; }
	.ch { cursor: grab; user-select: none; touch-action: none; }
	.ch.moving { opacity: 0.4; }
	.ins { position: absolute; top: 2px; bottom: 2px; width: 2px; margin-left: -1px; border-radius: 1px; background: var(--accent); pointer-events: none; z-index: 4; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.sbar { position: absolute; left: 0; right: calc(4px - var(--gap)); bottom: 0; height: 2px; border-radius: 1px; background: var(--info); }
	.sbar.desc { background: var(--fail); }
	.ch.sorted .lbl { color: var(--text); }
	.lbl { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.grip { position: absolute; top: 0; bottom: 0; right: calc(-1 * var(--gap) - 3px); width: 12px; cursor: col-resize; touch-action: none; border-radius: 3px; z-index: 3; }
	.grip::after { content: ''; position: absolute; left: 5px; top: 0; bottom: 0; width: 1px; background: var(--line-strong); transition: background var(--dur-fast) var(--ease-out); }
	.grip:hover::after, .grip:focus-visible::after, .grip:global(.drag)::after { background: var(--accent); width: 2px; }
	.ch.fixed::after { content: ''; position: absolute; top: 0; bottom: 0; right: calc(3px - var(--gap)); width: 1px; background: var(--line-strong); pointer-events: none; }
	.grip:focus-visible { outline: none; }
	.cm { display: flex; justify-content: flex-end; }
	.app { display: flex; flex-direction: column; height: var(--app-h, auto); min-height: 420px; min-width: 0; overflow: clip; }
	.body { display: grid; grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); flex: 1 1 auto; min-height: 0; }
	.listbox { min-width: 0; min-height: 0; overflow: auto; overscroll-behavior: contain; }
	.listbox .tw { overflow: visible; }
	.listbox :global(.chead) { position: sticky; top: 0; z-index: 4; background: var(--bg); }
	.hdr { flex: none; display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; margin: 0; padding: 3px var(--dl-gutter); border-bottom: 1px solid var(--line); background: none; }
	.col { position: relative; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.col > .listbox { flex: 1 1 auto; }
	.trchip { flex: none; align-self: center; min-height: 24px; padding: 0 10px; border: 1px solid color-mix(in srgb, var(--fail) 45%, transparent); border-radius: 999px; background: color-mix(in srgb, var(--fail) 12%, transparent); color: var(--fail-text); font: var(--t-caption); font-weight: 600; cursor: pointer; white-space: nowrap; }
	.trchip.on, .trchip:hover { background: color-mix(in srgb, var(--fail) 22%, transparent); }
	.trchip:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dline :global(.num) { text-align: right; font-variant-numeric: tabular-nums; }
	.app { --dl-gutter: 10px; --dl-icon: 16px; --dl-icon-sm: 14px; }
	.app :global(svg.icon), .app :global(.ui-ibtn svg) { width: var(--dl-icon); height: var(--dl-icon); }
	.psearch { display: none; }
	.grip:active::after { background: var(--accent); width: 2px; }
	.psb, .psx { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--text); cursor: pointer; }
	.psb:hover, .psx:hover { background: var(--surface-3); }
	.psb:focus-visible, .psx:focus-visible, .psearch input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.psearch input { width: 150px; height: 30px; padding: 0 8px; border: 1px solid var(--line); border-radius: 8px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); }
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
	.tab:focus-visible, .stab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.subwrap { display: grid; grid-template-columns: 0fr; opacity: 0; transition: grid-template-columns 200ms var(--ease-out), opacity 200ms var(--ease-out); }
	.pw.open .subwrap { grid-template-columns: 1fr; opacity: 1; }
	.subin { display: flex; align-items: center; gap: 2px; min-width: 0; overflow: hidden; white-space: nowrap; }
	.pw.open .subin { padding: 0 2px 0 6px; margin-right: 2px; border-left: 1px solid var(--hl-line); }
	.stab { display: inline-flex; align-items: center; flex: none; gap: 6px; height: 24px; padding: 0 10px; border: 0; border-radius: 999px; background: none; color: var(--hl-sub); font: var(--t-footnote); white-space: nowrap; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.stab b { color: inherit; font-weight: 600; font-variant-numeric: tabular-nums; }
	.stab:hover { color: var(--hl-fg); }
	.stab.on { background: var(--hl-sub-on-bg); color: var(--hl-sub-on-fg); }
	.pw.leaf .tab { background: var(--hl-sub-on-bg); color: var(--hl-sub-on-fg); box-shadow: inset 0 0 0 2px var(--hl-bg); }
	.tabs { --hl-bg: var(--seg-thumb); --hl-fg: var(--text); --hl-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); --hl-line: var(--line-strong); --hl-sub: color-mix(in srgb, var(--text) 86%, var(--hl-bg)); --hl-sub-on-bg: var(--accent-soft); --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 85%, var(--text)); }
	/* Dark theme lightens the red words so they read on the grey pill. */
	:global(:root[data-theme='dark']) .tabs { --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 25%, var(--text-on-red)); }
	@media (prefers-color-scheme: dark) {
		:global(:root:not([data-theme='light'])) .tabs { --hl-sub-on-fg: color-mix(in srgb, var(--accent-text) 25%, var(--text-on-red)); }
	}
	@media (prefers-reduced-motion: reduce) {
		.pw, .tab, .stab, .subwrap { transition: none; }
	}
	.app { --r-poster: 4px; }
	@keyframes rowin { from { opacity: 0; transform: translateY(-4px); } }
	@keyframes tabin { from { opacity: 0; transform: translateY(4px); } }
	@keyframes fadein { from { opacity: 0; } }
	@media (max-width: 640px) {
		.dlgrid { --gap: 8px; --pi: 26px !important; }
		.chead { display: none; }
		.tin { min-width: 0; }
	.dline { grid-template-columns: 8px 36px minmax(0, 1fr) 40px auto 44px; grid-template-areas: 'tg post nm nm nm mn' 'tg post st bar time mn'; grid-template-rows: auto 18px; column-gap: 10px; row-gap: 2px; height: auto; min-height: 64px; padding: 4px 12px 4px 0; box-sizing: border-box; align-items: center; -webkit-touch-callout: none; -webkit-user-select: none; user-select: none; -webkit-tap-highlight-color: transparent; transition: background-color 160ms var(--ease-out), transform 160ms var(--ease-out); }
		.dline:global([data-pressing]) { transform: scale(0.985); background: color-mix(in srgb, var(--text) 7%, transparent); transition-duration: 450ms; }
		.dline > [data-col] { display: none; }
		.dline > [data-col='poster'] { display: block; grid-area: post; align-self: start; }
		.dline > [data-col='title'] { display: flex; grid-area: nm; }
		.dline > [data-col='progress'] { display: flex; grid-area: bar; }
		.dline > [data-col='status'] { display: flex; grid-area: st; }
		.dline > [data-col='time'] { display: block; grid-area: time; }
		.dline > [data-col='title'] { flex-direction: column; justify-content: center; gap: 1px; min-width: 0; }
		.dline > [data-col='time'] { justify-self: end; color: var(--text-muted); font: var(--t-footnote); }
	.dline > [data-col='bin'] { display: flex; grid-area: mn; align-self: center; justify-content: center; }
		.dline > [data-col='poster'] { width: 36px; height: 54px; }
		.dline > [data-col='status'] { font: 400 12px/16px var(--font-ui); }
		.dline > [data-col='time'] { font-variant-numeric: tabular-nums; white-space: nowrap; }
	.tabrow { -webkit-mask-image: linear-gradient(90deg, #000 calc(100% - 24px), transparent); mask-image: linear-gradient(90deg, #000 calc(100% - 24px), transparent); padding-right: 24px; }
		.tabrow :global([data-testid='dl-stat']) { display: none; }
		.qline { display: flex; align-items: center; gap: 6px; min-height: 24px; margin: 0; padding: 2px var(--dl-gutter) 4px; color: var(--text-muted); font: 400 12px/16px var(--font-ui); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; }
		.qline .qd { color: color-mix(in srgb, var(--ok) 45%, var(--text-muted)); font-weight: 600; }
		/* The speed on the phone status line opens the speed limit; the taller hit area keeps the line low. */
	.qbtn { position: relative; display: inline-flex; align-items: center; gap: 6px; min-height: 44px; margin: 0; padding: 0; border: 0; background: none; font: inherit; cursor: pointer; }
	.qbtn::after { content: ''; position: absolute; inset: 0 -8px; }
		.qline:has(.qbtn) { min-height: 48px; padding-top: 4px; padding-bottom: 0; }
		.qbtn:focus-visible { outline: none; border-radius: 6px; box-shadow: var(--focus-ring); }
		.qcap { display: inline-flex; align-items: center; gap: 3px; color: var(--text-muted); font-weight: 400; }
		.qrv { margin-left: auto; min-height: 24px; padding: 0 10px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail-text); font: 600 12px/1 var(--font-ui); }
		.qrv:focus-visible { outline: none; box-shadow: var(--focus-ring); }
		.listbox:has(:global(.psel)) .dlrows { padding-bottom: 140px; }
		.dline > [data-col='progress'] { height: 4px; align-self: center; }
	.strip { gap: 4px 14px; }
	}
	@media (max-width: 640px) {
		.psearch { display: flex; flex: none; align-items: center; gap: 2px; }
		.hdr { grid-template-columns: minmax(0, 1fr); }
	}
	@media (min-width: 641px) {
		.tin { border: 1px solid var(--line-strong); border-radius: var(--r-card); background: var(--surface-2); box-shadow: var(--shadow-card); overflow: clip; }
		.tin .chead { background: color-mix(in srgb, var(--surface-3) 70%, var(--surface-2)); border-bottom-color: var(--line-strong); }
		.listbox :global(.tin .chead) { background: color-mix(in srgb, var(--surface-3) 70%, var(--surface-2)); }
	}
	.facets { display: grid; grid-template-columns: 42px 42px auto; column-gap: 4px; align-items: center; justify-content: start; }
	.fc { position: relative; justify-self: start; padding: 0; border: 0; background: none; color: var(--text-muted); font: var(--t-caption); cursor: pointer; }
	.fc.on { color: var(--text); font-weight: 600; }
	.fc:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 3px; }
	.fc .sbar { right: 0; bottom: -7px; }
	/* Chrome drops its ::-webkit scrollbar rules once scrollbar-width or scrollbar-color is set, so those two are Firefox only. */
	@supports not selector(::-webkit-scrollbar) {
		.tw { scrollbar-width: thin; scrollbar-color: color-mix(in srgb, var(--text) 24%, transparent) transparent; }
		.tw:hover { scrollbar-color: color-mix(in srgb, var(--text) 38%, transparent) transparent; }
	}
	.tw::-webkit-scrollbar { width: 12px; height: 12px; background: transparent; }
	.tw::-webkit-scrollbar-track { margin: 4px; border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 6%, transparent) padding-box; }
	.tw::-webkit-scrollbar-corner { background: transparent; }
	.tw::-webkit-scrollbar-button { display: none; width: 0; height: 0; }
	.tw::-webkit-scrollbar-thumb { border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 26%, transparent) padding-box; }
	@media (min-width: 641px) {
		.body { padding: 0 12px 0 14px; }
	.hdr, .tabrow { border-bottom: 0; }
	.col { overflow: clip; border-radius: var(--r-card); }
		.col > .listbox { background: var(--surface-2); border-radius: var(--r-card) var(--r-card) 0 0; }
	.col .tin { border-color: transparent; box-shadow: none; }
	.hk { display: flex !important; align-items: center; justify-content: center; min-width: 28px; padding: 0 !important; }
		.hk .hpk { margin: 0; translate: none; transform: none; }
	.ch[data-col='poster'] .lbl { transform: translateX(calc(var(--gap) / 2 + 0.5px)); }
	}
	.tw::-webkit-scrollbar-thumb:hover { background: color-mix(in srgb, var(--text) 40%, transparent) padding-box; }
	@media (max-width: 640px) {
		.tab, .stab { height: 32px; }
		.trchip, .qrv { min-height: 32px; }
	}
	/* Touch phones get 44px targets; the hit area grows with an invisible ::after so nothing moves. */
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
</style>
