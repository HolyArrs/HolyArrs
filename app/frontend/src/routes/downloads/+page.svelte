<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { libraryBrief } from '#lib/mediafiles/index.ts';
	import { decisionName, manyWords, removeBody, removeFilesWords, removeStart, rowName, rowPath, type DownloaderSettings, type DownloaderView, type RemoveChoice, applyPatch, fromAnswer, isDl, mergeClient, wentStale, type Sync, dlPush, decisions, refreshDecisions, DlSections, ConfirmMatch, byDownload, AddTorrents, SteerSheet, BULK_DONE, byClient, type BulkAction } from '#lib/downloads/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { to } from '#lib/core/paths.ts';
	import { dlScope } from '#lib/core/search-field.svelte.ts';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { type Decision, type DownloadRow } from '#lib/core/types.ts';
	type ClientHealth = { id: number; name: string; health: 'ok' | 'slow' | 'offline' | 'waiting' | 'checking' | 'degraded' | (string & {}); stale?: boolean; seen_at?: string };
	type ViewAnswer = { downloads: DownloadRow[]; totals?: { speed: number; up_speed: number }; clients?: ClientHealth[]; outside_total?: number; outside_rest?: Rest; next_cursor?: string; boot?: string; revs?: Record<string, number> };
	import { session } from '#lib/core/session.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { TitleView } from '#lib/core/types.ts';
	import type { ImportRecord, Rest, SteerMode } from '#lib/downloads/index.ts';
	
	// Someone who may not manage the Library only watches the downloads for what they asked for.
	const watch = $derived(!session.can('manage_library'));
	// Raw: rows are only ever replaced (patches and reads build new arrays), so 2,000 rows are not wrapped in proxies on every patch.
	let rows = $state.raw<DownloadRow[]>([]);
	let imports = $state<ImportRecord[]>([]);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	// Counts the list reads, so the Confirm match banner reads again with them.
	let reads = $state(0);
	// The poster pictures come from the Library titles; until they arrive the rows show plain poster tiles (no initials).
	let titlesReady = $state(false);
	let error = $state('');
	// Calm notes for side reads that failed (posters, remove defaults, the Downloader list) and for torrent clients that are slow or down.
	let sideNotes = $state<Record<string, string>>({});
	let clientHealth = $state<ClientHealth[]>([]);
	let upTotal = $state<number | undefined>(undefined);
	// Outside torrents come 200 at a time; Show more asks for the next 200 too.
	let outsideLimit = $state(200);
	let moreOutside = $state(false);
	// Torrents HolyArrs did not add that this page has not loaded, per Downloader and stage: the tabs count them too.
	let outsideRest = $state<Rest>({});
	// The most torrents the server sends that it did not add.
	const outsideMax = 2000;
	const clientNotes = $derived(
		clientHealth
			.filter((c) => c.health !== 'ok')
			.map((c) =>
				c.health === 'waiting'
					? t('downloads.page.waiting', { name: c.name })
					: c.stale
						? t('downloads.page.stale', { name: c.name })
						: t('downloads.page.notAnswering', { name: c.name })
			)
	);
	const notes = $derived([...Object.values(sideNotes), ...clientNotes]);
	function note(key: string, text: string) {
		sideNotes = { ...sideNotes, [key]: text };
	}
	function clearNote(key: string) {
		if (!(key in sideNotes)) return;
		const { [key]: _, ...rest } = sideNotes;
		sideNotes = rest;
	}
	let settings = $state<DownloaderSettings | null>(null);
	let openId = $state(0);
	// A torrent Downloader brings the Seeding sub-tab under Done.
	let torrent = $state(false);
	// The torrent Downloaders a manager can add torrents to by hand.
	let torrentClients = $state<{ id: number; name: string; category: string }[]>([]);
	let addOpen = $state(false);
	const titleById = $derived(new Map(titles.map((title) => [title.id, title])));
	// Imports belong to their download's row.
	const importOf = $derived(byDownload(imports));

	// Needs a decision lives on the rows: a download whose Slots wait for a person sits in the Queue and its details hold the choices. A decision with no download in the list (HolyArrs ran out of Releases before any download) gets a row of its own.
	const rowIds = $derived(new Set(rows.map((r) => r.id)));
	const decisionsByDownload = $derived.by(() => {
		const m = new Map<number, Decision[]>();
		for (const d of decisions.items) if (d.download_id && rowIds.has(d.download_id)) m.set(d.download_id, [...(m.get(d.download_id) ?? []), d]);
		return m;
	});
	function decisionRow(d: Decision): DownloadRow {
		return {
			id: -d.slot_id,
			phase: 'needs_a_decision',
			state: '',
			label: t('downloads.status.review'),
			text: '',
			progress: 0,
			speed: 0,
			active: false,
			name: d.release_name ?? decisionName(d),
			title: d.title,
			title_id: d.title_id,
			year: d.year,
			version: d.version,
			downloader: '',
			size: 0,
			updated_at: d.since,
			can_act: false,
			kind: d.kind,
			album: d.album,
			covers: d.episode ? `${d.episode.label}${d.episode.name ? ` ${d.episode.name}` : ''}` : undefined,
			artwork: d.artwork
		};
	}
	const allRows = $derived([
		...rows.map((r) => (decisionsByDownload.has(r.id) && !r.active && r.phase !== 'needs_a_decision' ? { ...r, phase: 'needs_a_decision', label: t('downloads.status.review') } : r)),
		...decisions.items.filter((d) => !(d.download_id && rowIds.has(d.download_id))).map(decisionRow)
	]);
	function decisionsOf(r: DownloadRow): Decision[] {
		if (r.client_item) return [];
		return r.id < 0 ? decisions.items.filter((d) => d.slot_id === -r.id) : (decisionsByDownload.get(r.id) ?? []);
	}

	// When the open row goes away (its decision was settled), its details close.
	$effect(() => {
		if (loaded && openId && !allRows.some((r) => r.id === openId)) openId = 0;
	});

	// &open=<download id> or &slot=<slot id> (Home's Needs attention) opens that row's details once.
	let opened = false;
	$effect(() => {
		if (opened || !loaded) return;
		// A watcher has no decisions to wait for: the link opens only a row already in the list.
		if (watch) opened = true;
		const q = page.url.searchParams;
		const dl = Number(q.get('open'));
		const slot = Number(q.get('slot'));
		if (!dl && !slot) {
			opened = true;
			return;
		}
		const d = slot ? decisions.items.find((x) => x.slot_id === slot) : undefined;
		if (slot && !d && decisions.items.length === 0 && !watch) return; // The decisions are still loading.
		opened = true;
		const id = dl || (d?.download_id && rowIds.has(d.download_id) ? d.download_id : -slot);
		if (allRows.some((r) => r.id === id)) openId = id;
	});

	// A client item (a torrent HolyArrs did not send, or one older than the list) has no download id: it gets a stable stand-in id for this visit, below every decision row's.
	const clientIds = new Map<string, number>();
	let nextClientId = -1_000_000_000;
	function withId(r: DownloadRow): DownloadRow {
		if (!r.client_item || !r.hash) return r;
		const key = `${r.downloader_id}:${r.hash}`;
		let id = clientIds.get(key);
		if (id === undefined) clientIds.set(key, (id = nextClientId--));
		return { ...r, id };
	}

	// Push updates: the view answer carries each client's rev, and patches on /live keep the rows current between reads. No revs means the background reader is not running (safe mode), so the page reads every 10 s instead.
	let sync: Sync | null = null;
	const pushHealthy = () => sync !== null && live.status.state === 'live';
	function interest() {
		if (sync && document.visibilityState === 'visible') live.interest('downloads');
	}

	function onDl(data: unknown) {
		if (!sync || !isDl(data)) return;
		if (data.kind === 'health') {
			const h: ClientHealth = { id: data.client, name: data.name ?? '', health: data.health, stale: data.stale, seen_at: data.seen_at };
			if (wentStale(clientHealth.find((c) => c.id === h.id), data)) soon();
			clientHealth = clientHealth.some((c) => c.id === h.id) ? clientHealth.map((c) => (c.id === h.id ? { ...h, name: h.name || c.name } : c)) : [...clientHealth, h];
			return;
		}
		const out = applyPatch(rows, sync, data, withId);
		if (out.kind === 'reread') {
			if (out.client === undefined) soon();
			else rereadClient(out.client);
		}
		else if (out.kind === 'applied') {
			rows = out.rows;
			sync = out.sync;
			if (out.upTotal !== undefined) upTotal = out.upTotal;
		}
	}

	function showMore() {
		outsideLimit = Math.min(outsideLimit + 200, outsideMax);
		void load();
	}

	// Reads can overlap (a timed read, then a Remove's own read): only the newest one is applied, so an older answer never brings a removed row back.
	let loadSeq = 0;
	async function load() {
		const seq = ++loadSeq;
		try {
			const [d, i] = await Promise.all([
				api.get<ViewAnswer>(`/downloads/view?limit=100${outsideLimit > 200 ? `&outside_limit=${outsideLimit}` : ''}`),
				watch ? Promise.resolve({ imports: [] as ImportRecord[] }) : api.get<{ imports: ImportRecord[] }>('/imports?limit=100')
			]);
			if (seq !== loadSeq) return;
			rows = d.downloads.map(withId);
			clientHealth = d.clients ?? [];
			upTotal = d.totals?.up_speed;
			moreOutside = !!d.next_cursor;
			outsideRest = d.outside_rest ?? {};
			const was = sync;
			sync = fromAnswer(d);
			dlPush.on = sync !== null;
			if (sync && !was) interest();
			imports = i.imports;
			reads++;
			error = '';
		} catch (e) {
			if (seq === loadSeq) error = message(e);
		} finally {
			loaded = true;
		}
	}

	// A gap on one client rereads that client's page only (its rows and rev); the other clients keep their rows and keep applying patches.
	const rereading = new Set<number>();
	function rereadClient(id: number) {
		if (rereading.has(id)) return;
		rereading.add(id);
		api
			.get<ViewAnswer>(`/downloads/view?limit=100&client=${id}${outsideLimit > 200 ? `&outside_limit=${outsideLimit}` : ''}`)
			.then((d) => {
				if (!sync) return;
				if (d.boot && sync.boot && d.boot !== sync.boot) return soon();
				const out = mergeClient(rows, sync, id, d.downloads.map(withId), d.revs);
				rows = out.rows;
				sync = out.sync;
				// That client has more torrents than this page holds: offer them.
				if (d.next_cursor) moreOutside = true;
				// Only that client's left-out torrents changed.
				if (d.outside_rest) outsideRest = { ...outsideRest, [String(id)]: d.outside_rest[String(id)] ?? {} };
			})
			.catch(() => soon())
			.finally(() => rereading.delete(id));
	}

	// Live news comes often while something downloads: read again at most once every 400 ms.
	let timer: ReturnType<typeof setTimeout> | undefined;
	function soon() {
		if (timer) return;
		timer = setTimeout(() => {
			timer = undefined;
			void load();
		}, 400);
	}

	onMount(() => {
		void load();
		void libraryBrief()
			.then((r) => {
				titles = r;
				clearNote('titles');
			})
			.catch(() => note('titles', t('downloads.page.noPosters')))
			.finally(() => (titlesReady = true));
		// The remove defaults are a settings read; without that permission the server's own defaults stand (no 403 in the console).
		if (session.can('settings')) api
				.get<DownloaderSettings>('/settings/downloaders')
				.then((s) => (settings = s))
				.catch(() => note('settings', t('downloads.page.noDefaults')));
		if (session.can('manage_library'))
			api
				.get<{ downloaders: DownloaderView[] }>('/downloaders')
				.then((r) => {
					// Add offers only Downloaders that take writes; a read-only one would refuse the add.
					torrentClients = r.downloaders.filter((d) => d.protocol === 'torrent' && d.kind === 'qbittorrent' && d.writable).map((d) => ({ id: d.id, name: d.name, category: d.category }));
					torrent = torrentClients.length > 0;
				})
				.catch(() => note('downloaders', t('downloads.page.noDownloaders')));
		if (!watch) void refreshDecisions();
		// Library and download news on /live is for managers; a watcher receives only dl patches (push mode).
		const offs = watch ? [] : ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		const decOffs = watch ? [] : ['slot', 'releases', 'resync'].map((t) => live.on(t, () => void refreshDecisions()));
		const offDl = live.on('dl', onDl);
		// Without a healthy stream (no reader, or the stream failed) the page reads again every 10 s while visible.
		const watchPoll = setInterval(() => {
			if (document.visibilityState === 'visible' && !pushHealthy()) soon();
		}, 10_000);
		// With push: a slow safety read, and the server is told someone watches (faster client reads).
		const safety = setInterval(() => {
			if (pushHealthy() && document.visibilityState === 'visible') soon();
		}, 30_000);
		const interestPoll = setInterval(interest, 30_000);
		// Back on the tab: read at once instead of waiting for the next tick.
		const back = () => {
			if (document.visibilityState === 'visible') void load();
		};
		document.addEventListener('visibilitychange', back);
		return () => {
			document.removeEventListener('visibilitychange', back);
			offs.forEach((off) => off());
			decOffs.forEach((off) => off());
			offDl();
			dlPush.on = false;
			clearTimeout(timer);
			clearInterval(watchPoll);
			clearInterval(safety);
			clearInterval(interestPoll);
		};
	});

	function who(r: DownloadRow) {
		const n = rowName(r);
		return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
	}

	let busy = $state(0);
	async function markFailed(r: DownloadRow) {
		if (!confirm(t('downloads.page.markFailedAsk', { name: who(r) }))) return;
		busy = r.id;
		try {
			await api.post(`/downloads/${r.id}/mark-failed`);
			toasts.push(t('downloads.page.markedFailed'), 'ok', { body: t('downloads.page.nextRelease') });
			await load();
		} catch (e) {
			toasts.push(t('downloads.page.markFailedFailed'), 'fail', { body: message(e) });
		} finally {
			busy = 0;
		}
	}

	// The Remove dialog: one download, or a selection. Delete files is off unless asked for (Shift+Delete), except a HolyArrs download follows the remove defaults in Settings.
	let removeOpen = $state(false);
	let targets = $state<DownloadRow[]>([]);
	const target = $derived(targets.length === 1 ? targets[0] : null);
	let choice = $state<RemoveChoice>(removeStart(null));
	let removeError = $state('');
	let removeBusy = $state(false);
	// Torrents HolyArrs did not send are removed only by someone with the settings permission.
	const blocked = $derived(session.can('settings') ? [] : targets.filter((r) => r.outside));
	function askRemove(r: DownloadRow, withFiles = false) {
		askRemoveMany([r], withFiles);
	}
	function askRemoveMany(rows: DownloadRow[], withFiles = false) {
		const ok = rows.filter((r) => r.can_act);
		if (!ok.length) {
			toasts.push(t('downloads.page.cantRemove'), 'info', { body: t('downloads.list.notFromHere') });
			return;
		}
		targets = ok;
		const own = ok.length === 1 && !ok[0].client_item;
		choice = own ? removeStart(settings) : { delete_files: false, turn_down: false, search_again: false };
		if (withFiles) choice.delete_files = true;
		removeError = '';
		removeOpen = true;
	}
	async function remove(e: SubmitEvent) {
		e.preventDefault();
		const list = targets.filter((r) => !blocked.includes(r));
		if (!list.length) return;
		removeBusy = true;
		busy = list.length === 1 ? list[0].id : busy;
		const bad: string[] = [];
		// Rows whose remove failed: a second press of Remove sends only these, never the ones already gone.
		const failed = new Set<DownloadRow>();
		try {
			// Client items go in one call per Downloader; HolyArrs' own each through HolyArrs.
			const outsideRows = list.filter((r) => r.client_item);
			for (const [id, rows] of byClient(outsideRows)) {
				try {
					await api.post(`/downloaders/${id}/bulk`, { action: 'remove', hashes: rows.map((r) => r.hash), delete_files: choice.delete_files });
				} catch (err) {
					bad.push(message(err));
					rows.forEach((r) => failed.add(r));
				}
			}
			// Usenet client items have no torrent bulk call: one call each.
			for (const r of outsideRows.filter((x) => x.protocol && x.protocol !== 'torrent' && x.downloader_id && x.hash)) {
				try {
					await api.post(`/downloaders/${r.downloader_id}/items/${encodeURIComponent(r.hash!)}/remove`, { delete_files: choice.delete_files });
				} catch (err) {
					bad.push(`${who(r)}: ${message(err)}`);
					failed.add(r);
				}
			}
			for (const r of list.filter((x) => !x.client_item)) {
				try {
					await api.post(`/downloads/${r.id}/remove`, removeBody(list.length === 1 ? choice : { delete_files: choice.delete_files, turn_down: false, search_again: false }));
				} catch (err) {
					bad.push(`${who(r)}: ${message(err)}`);
					failed.add(r);
				}
			}
			if (bad.length) {
				removeError = bad.join(' · ');
				targets = targets.filter((r) => failed.has(r) || blocked.includes(r));
			} else {
				removeOpen = false;
				const one = list.length === 1 && !list[0].client_item;
				const searching = one && choice.turn_down && choice.search_again;
				toasts.push(list.length > 1 ? t('downloads.page.nRemoved', { count: list.length }) : t('downloads.page.removed'), 'ok', searching ? { body: t('downloads.page.nextRelease') } : {});
			}
			await load();
		} finally {
			removeBusy = false;
			busy = 0;
		}
	}
	// Client actions: the Downloader does the work; the row's new state is read back from it.
	const DONE: Record<string, string> = {
		get pause() { return t('downloads.page.done.pause'); },
		get resume() { return t('downloads.page.done.resume'); },
		get top() { return t('downloads.act.movedTop'); },
		get recheck() { return t('downloads.page.done.recheck'); },
		get reannounce() { return t('downloads.page.done.reannounce'); },
		get retry() { return t('downloads.page.done.retry'); }
	};
	async function act(r: DownloadRow, action: 'pause' | 'resume' | 'top' | 'recheck' | 'reannounce' | 'retry') {
		busy = r.id;
		try {
			await api.post(`${rowPath(r)}/${action}`);
			toasts.push(DONE[action], 'ok');
			await load();
		} catch (e) {
			toasts.push(t('downloads.act.updateFailed'), 'fail', { body: message(e) });
		} finally {
			busy = 0;
		}
	}
	// Try again for many failed Usenet jobs: one call each. One batch at a time: a second click while a selection action, Download again or Pause all is still on its way is dropped, so nothing is sent twice.
	let batchBusy = $state(false);
	async function batch(run: () => Promise<void>) {
		if (batchBusy) return;
		batchBusy = true;
		try {
			await run();
		} finally {
			batchBusy = false;
		}
	}
	async function retryMany(list: DownloadRow[]) {
		const bad: string[] = [];
		for (const r of list) {
			try {
				await api.post(`${rowPath(r)}/retry`);
			} catch (e) {
				bad.push(`${r.name || t('downloads.page.download')}: ${message(e)}`);
			}
		}
		if (bad.length) toasts.push(t('downloads.page.cantSendBack'), 'fail', { body: bad.join(' · ') });
		else toasts.push(list.length === 1 ? t('downloads.page.done.retry') : t('downloads.page.nSentBack', { count: list.length }), 'ok');
		await load();
	}
	// The steering sheet: seeding rules and more for one or many torrents.
	let steerOpen = $state(false);
	let steerMode = $state<SteerMode>('seed');
	let steerRows = $state<DownloadRow[]>([]);
	function steer(rows: DownloadRow[], mode: SteerMode) {
		steerRows = rows;
		steerMode = mode;
		steerOpen = true;
	}
	// A selection action: one call per torrent Downloader for every row it holds.
	async function bulk(list: DownloadRow[], action: BulkAction) {
		const groups = byClient(list);
		const left = list.length - [...groups.values()].reduce((n, g) => n + g.length, 0);
		if (!groups.size) {
			toasts.push(t('downloads.page.cantHere'), 'info', { body: t('downloads.page.notForThese') });
			return;
		}
		const bad: string[] = [];
		for (const [id, rows] of groups) {
			try {
				await api.post(`/downloaders/${id}/bulk`, { action, hashes: rows.map((r) => r.hash) });
			} catch (e) {
				bad.push(`${rows[0].downloader || t('downloads.page.downloader')}: ${message(e)}`);
			}
		}
		const n = list.length - left;
		if (bad.length) toasts.push(t('downloads.page.cantUpdateMany'), 'fail', { body: bad.join(' · ') });
		else toasts.push(t(n === 1 ? 'downloads.page.bulkDone.one' : 'downloads.page.bulkDone.other', { n, what: BULK_DONE[action] }), 'ok', left ? { body: t('downloads.page.skipped', { left }) } : {});
		await load();
	}
	type ClientStatus = { paused: boolean; speed_limit: number; up_limit: number };
	type ClientView = { id: number; name: string; protocol: string; controls: boolean; status?: ClientStatus; error?: string };
	async function all(pause: boolean) {
		try {
			const r = await api.post<{ clients: ClientView[] }>(pause ? '/downloads/pause-all' : '/downloads/resume-all');
			const bad = r.clients.filter((c) => c.controls && c.error);
			if (bad.length) toasts.push(bad.length === 1 ? t(pause ? 'downloads.page.cantPauseOne' : 'downloads.page.cantStartOne', { name: bad[0].name }) : t(pause ? 'downloads.page.cantPauseMany' : 'downloads.page.cantStartMany', { count: bad.length }), 'fail', { body: bad[0].error });
			else toasts.push(pause ? t('downloads.page.allPaused') : t('downloads.page.allStarted'), 'ok');
			await load();
		} catch (e) {
			toasts.push(t(pause ? 'downloads.page.cantPauseAll' : 'downloads.page.cantStartAll'), 'fail', { body: message(e) });
		}
	}

	// The speed limit: one cap per Downloader, in MB/s (0 is none), read from the client.
	let limitOpen = $state(false);
	let clients = $state<ClientView[]>([]);
	let limits = $state<Record<number, { down: number; up: number }>>({});
	let limitsWere: Record<number, { down: number; up: number }> = {};
	let limitError = $state('');
	let limitBusy = $state(false);
	const MB = 1_000_000;
	async function openLimit() {
		limitError = '';
		limitOpen = true;
		try {
			const r = await api.get<{ clients: ClientView[] }>('/downloads/clients');
			// Only Downloaders whose limits were read: one that did not answer would otherwise start at 0 and lose its cap on save.
			clients = r.clients.filter((c) => c.controls && !!c.status);
			limits = Object.fromEntries(clients.map((c) => [c.id, { down: +((c.status?.speed_limit ?? 0) / MB).toFixed(1), up: +((c.status?.up_limit ?? 0) / MB).toFixed(1) }]));
			limitsWere = $state.snapshot(limits);
		} catch (e) {
			limitError = message(e);
		}
	}
	async function saveLimit(e: SubmitEvent) {
		e.preventDefault();
		limitBusy = true;
		let failing = '';
		try {
			for (const c of clients) {
				const l = limits[c.id];
				// Only what was changed is sent, so an untouched Downloader (a read-only one, say) is left alone.
				const was = limitsWere[c.id];
				if (was && was.down === l.down && was.up === l.up) continue;
				failing = c.name;
				await api.put(`/downloaders/${c.id}/speed-limit`, { down: Math.round(Math.max(0, l.down || 0) * MB), up: Math.round(Math.max(0, l.up || 0) * MB) });
			}
			failing = '';
			limitOpen = false;
			toasts.push(t('downloads.page.limitSaved'), 'ok');
		} catch (err) {
			limitError = failing ? `${failing}: ${message(err)}` : message(err);
		} finally {
			limitBusy = false;
		}
	}

	// The Downloads search field (DlFind) filters this page while it is open.
	$effect(() => {
		dlScope.on = true;
		dlScope.off = false;
		dlScope.term = '';
		return () => {
			dlScope.on = false;
			dlScope.off = false;
			dlScope.term = '';
		};
	});
</script>

<svelte:head><title>{t('downloads.page.title')}</title></svelte:head>

<PageLoader loading={!loaded} label={t('downloads.page.loading')}>
<!-- One heading in every state (loading, error, empty, list), for screen readers and the menu check. -->
<h1 class="sr">{t('downloads.page.heading')}</h1>
{#if error && allRows.length === 0}
	<div class="pad"><LoadError hasData={false} message={t('downloads.page.couldNotLoad')} onretry={load} /></div>
{:else if allRows.length === 0}
	<div class="pad">
	{#if watch}
	<EmptyState icon="download" title={t('downloads.page.nothingOnWay')} text={t('downloads.page.watchText')} />
	{:else}
	<EmptyState icon="download" title={t('downloads.page.noDownloads')} text={t('downloads.page.liveText')} action={{ label: t('downloads.page.addMedia'), href: to('/search'), icon: 'search' }} />
	{/if}
	{#if !watch}<p class="aside-link"><a href={to('/settings/downloaders')}>{t('downloads.page.downloaders')}</a>{#if torrentClients.length}<button type="button" class="addlk" onclick={() => (addOpen = true)} data-testid="dl-add-empty">{t('downloads.page.addTorrent')}</button>{/if}</p>{/if}
	</div>
{:else}
	{#if error}<div class="pad"><LoadError hasData message={t('downloads.page.couldNotLoad')} onretry={load} /></div>{/if}
	{#if notes.length}<ul class="notes" role="status" data-testid="dl-notes">{#each notes as n (n)}<li>{n}</li>{/each}</ul>{/if}
	{#if !watch}<div class="matchpad"><ConfirmMatch refresh={reads} onchange={() => void load()} /></div>{/if}
	<DlSections changeOutside={session.can('settings')} rows={allRows} {importOf} titles={titleById} {titlesReady} {busy} {torrent} {decisionsOf} bind:openId onfail={markFailed} onremove={askRemove} ondone={() => void load()} onact={act} onall={(p) => batch(() => all(p))} onlimit={openLimit} onadd={torrentClients.length ? () => (addOpen = true) : undefined} onbulk={(l, a) => batch(() => bulk(l, a))} onsteer={steer} onremovemany={askRemoveMany} onretry={(l) => batch(() => retryMany(l))} {batchBusy} {watch} {upTotal} {notes} outside={{ rest: outsideRest, canMore: moreOutside && outsideLimit < outsideMax, capMax: outsideMax, onmore: showMore }} />
{/if}
</PageLoader>

<SteerSheet bind:open={steerOpen} mode={steerMode} rows={steerRows} ondone={() => void load()} />
<AddTorrents bind:open={addOpen} clients={torrentClients} ondone={() => void load()} />

<Sheet bind:open={removeOpen} title={targets.length > 1 ? t('downloads.page.removeN', { count: targets.length }) : t('downloads.page.removeOne')} size="sm">
	{#if targets.length}
		<form id="remove-form" onsubmit={remove} data-testid="dl-remove-form">
			{#if target}
				<p class="who">{who(target)}</p>
				<details class="raw"><summary>{t('downloads.page.details')}</summary><p>{target.name}</p></details>
			{:else}
				<ul class="many" data-testid="dl-remove-list">{#each targets.slice(0, 6) as r (r.id)}<li>{who(r)}</li>{/each}{#if targets.length > 6}<li class="hint">{t('downloads.page.andMore', { count: targets.length - 6 })}</li>{/if}</ul>
			{/if}
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.delete_files} data-testid="dl-remove-files" />
				<span><b>{t('downloads.page.alsoDelete')}</b><br /><span class="hint" data-testid="dl-remove-files-hint">{choice.delete_files ? removeFilesWords(targets)   : t('downloads.page.offKeeps', { name: target?.downloader || t('downloads.rm.theDownloader') })}</span></span>
			</label>
			{#if target && !target.client_item}
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.turn_down} />
				<span><b>{t('downloads.page.addTurnedDown')}</b><br /><span class="hint">{t('downloads.page.neverPicks', { what: manyWords(target.kind) ? t('downloads.dp.these', { many: manyWords(target.kind)?.many ?? '' }) : t('downloads.page.thisMovie') })}</span></span>
			</label>
			<label class="checkline" class:off={!choice.turn_down}>
				<input type="checkbox" bind:checked={choice.search_again} disabled={!choice.turn_down} />
				<span><b>{t('downloads.page.searchAgain')}</b><br /><span class="hint">{t('downloads.page.lookNext', { what: manyWords(target.kind) ? t('downloads.page.moviesWait', { many: manyWords(target.kind)?.many ?? '' }) : t('downloads.page.movieWaits') })}</span></span>
			</label>
			{/if}
			{#if blocked.length}<p class="hint" data-testid="dl-remove-blocked">{blocked.length === targets.length ? t('downloads.page.blockedAll') : t('downloads.page.blockedSome', { count: blocked.length })}</p>{/if}
			{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		{@const n = targets.length - blocked.length}
		<Button variant="plain" onclick={() => (removeOpen = false)}>{t('downloads.sel.cancel')}</Button>
		<Button variant="destructive" type="submit" form="remove-form" busy={removeBusy} disabled={removeBusy || n === 0} data-testid="dl-remove-confirm">{n > 1 ? t('downloads.page.removeCount', { count: n }) : t('downloads.act.remove')}{choice.delete_files ? t('downloads.page.andDeleteFiles') : ''}</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={limitOpen} title={t('downloads.page.speedLimit')} size="sm">
	<form id="limit-form" onsubmit={saveLimit}>
		<p class="hint">{t('downloads.page.mbHint')}</p>
		{#each clients as c (c.id)}
			{#if limits[c.id]}
				<fieldset class="lim" data-testid="dl-limit-{c.id}">
					<legend>{c.name}</legend>
					<label>{t('downloads.ss.download')}<input type="number" min="0" step="0.1" inputmode="decimal" bind:value={limits[c.id].down} /></label>
					{#if c.protocol === 'torrent'}<label>{t('downloads.ss.upload')}<input type="number" min="0" step="0.1" inputmode="decimal" bind:value={limits[c.id].up} /></label>{/if}
				</fieldset>
			{/if}
		{:else}
			{#if !limitError}<p class="hint">{t('downloads.page.reading')}</p>{/if}
		{/each}
		{#if limitError}<p class="form-error" role="alert">{limitError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (limitOpen = false)}>{t('downloads.sel.cancel')}</Button>
		<Button type="submit" form="limit-form" busy={limitBusy} disabled={limitBusy || clients.length === 0}>{t('downloads.tv.save')}</Button>
	{/snippet}
</Sheet>

<style>
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.lim { border: 0; padding: 0; margin: 0; display: flex; flex-wrap: wrap; gap: 12px; min-width: 0; }
	.lim legend { font: var(--t-headline); margin-bottom: 6px; padding: 0; }
	.lim label { display: flex; flex-direction: column; gap: 4px; font: var(--t-subhead); color: var(--text-muted); flex: 1 1 120px; min-width: 0; }
	.lim input { min-height: 44px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--line-strong); background: var(--surface-2); color: var(--text); font: var(--t-callout); width: 100%; box-sizing: border-box; }
	.pad { padding: var(--s-6) var(--gutter); }
	.matchpad { padding: 0 var(--gutter); }
	.notes { list-style: none; margin: 0; padding: var(--s-2) var(--gutter) 0; display: grid; gap: 2px; color: var(--text-muted); font: var(--t-footnote); }
	/* On desktop the notes live in the status popover (DlSections), behind a dot; the bare list is for phones. */
	@media (min-width: 641px) { .notes { display: none; } }
	.matchpad:empty { display: none; }
	.aside-link { text-align: center; margin-top: -24px; }
	.aside-link a { display: inline-flex; align-items: center; min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	.addlk { display: inline-flex; align-items: center; min-height: 44px; margin-left: 20px; background: none; border: 0; padding: 0; color: var(--accent-text); font: var(--t-callout); font-weight: 500; cursor: pointer; }
	.raw { color: var(--text-muted); font: var(--t-footnote); }
	.raw summary { width: max-content; min-height: 24px; cursor: pointer; border-radius: 6px; }
	.raw summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw p { margin-top: 4px; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.many { margin: 0; padding-left: 18px; font: var(--t-callout); display: grid; gap: 2px; }
	.who { font: var(--t-headline); margin-bottom: 4px; }
	form { display: flex; flex-direction: column; gap: 14px; }
	.checkline { display: flex; gap: 12px; align-items: flex-start; font: var(--t-callout); cursor: pointer; }
	.checkline input { width: 20px; height: 20px; margin-top: 1px; flex: none; accent-color: var(--accent); }
	.hint { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.off { opacity: 0.6; }
	.form-error { color: var(--fail); font: var(--t-callout); }
</style>
