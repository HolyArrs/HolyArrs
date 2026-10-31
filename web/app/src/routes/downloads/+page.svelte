<script lang="ts">
	import { libraryTitles, ownedLibrary } from '#lib/owned.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { dlScope } from '#lib/search-field.svelte.ts';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { decisionName, manyWords, removeBody, removeStart, rowName, rowPath, type Decision, type DownloaderSettings, type DownloaderView, type DownloadRow, type RemoveChoice } from '#lib/downloaders.ts';
	type ClientHealth = { id: number; name: string; health: 'ok' | 'slow' | 'offline' | 'waiting' | 'checking' | 'degraded' | (string & {}); stale?: boolean; seen_at?: string };
	type ViewAnswer = { downloads: DownloadRow[]; totals?: { speed: number; up_speed: number }; clients?: ClientHealth[]; outside_total?: number; next_cursor?: string; boot?: string; revs?: Record<string, number> };
	import { applyPatch, fromAnswer, isDl, type Sync } from '#lib/downloads/dlstore.ts';
	import { dlPush } from '#lib/downloads/push.svelte.ts';
	import { decisions, refreshDecisions } from '#lib/decisions.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { ImportRecord, TitleView } from '#lib/types.ts';
	import DlSections from '#lib/downloads/DlSections.svelte';
	import ConfirmMatch from '#lib/downloads/ConfirmMatch.svelte';
	import { byDownload } from '#lib/downloads/imports.ts';
	import AddTorrents from '#lib/downloads/AddTorrents.svelte';
	import SteerSheet from '#lib/downloads/SteerSheet.svelte';
	import type { SteerMode } from '#lib/downloads/manage.ts';
	import { BULK_DONE, byClient, type BulkAction } from '#lib/downloads/manage.ts';
	
	// Someone who may not manage the Library only watches the downloads for what they asked for.
	const watch = $derived(!session.can('manage_library'));
	let rows = $state<DownloadRow[]>([]);
	let imports = $state<ImportRecord[]>([]);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	// Counts the list reads, so the Confirm match banner reads again with them.
	let reads = $state(0);
	// The poster pictures come from the Library titles; until they arrive the rows show plain poster tiles (no initials).
	let titlesReady = $state(false);
	let error = $state('');
	// Calm notes for side reads that failed (posters, remove defaults, the
	// Downloader list) and for torrent clients that are slow or down.
	let sideNotes = $state<Record<string, string>>({});
	let clientHealth = $state<ClientHealth[]>([]);
	let upTotal = $state<number | undefined>(undefined);
	// Outside torrents come 200 at a time; Show more asks for the next 200 too.
	let outsideLimit = $state(200);
	let moreOutside = $state(false);
	const clientNotes = $derived(
		clientHealth
			.filter((c) => c.health !== 'ok')
			.map((c) =>
				c.health === 'waiting'
					? `${c.name} has not answered yet; its torrents show once it does.`
					: c.stale
						? `${c.name} is not answering right now; showing its last list.`
						: `${c.name} is not answering right now.`
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

	// Needs a decision lives on the rows: a download whose Slots wait for a
	// person sits in the Queue and its details hold the choices. A decision
	// with no download in the list (HolyArrs ran out of Releases before any
	// download, or the download is older than the list) gets a row of its own.
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
			label: 'Review',
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
		...rows.map((r) => (decisionsByDownload.has(r.id) && !r.active && r.phase !== 'needs_a_decision' ? { ...r, phase: 'needs_a_decision', label: 'Review' } : r)),
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
		if (slot && !d && decisions.items.length === 0 && !watch) return; // the decisions are still loading
		opened = true;
		const id = dl || (d?.download_id && rowIds.has(d.download_id) ? d.download_id : -slot);
		if (allRows.some((r) => r.id === id)) openId = id;
	});

	// A client item (a torrent HolyArrs did not send, or one older than the
	// list) has no download id: it gets a stable stand-in id for this visit,
	// below every decision row's.
	const clientIds = new Map<string, number>();
	let nextClientId = -1_000_000_000;
	function withId(r: DownloadRow): DownloadRow {
		if (!r.client_item || !r.hash) return r;
		const key = `${r.downloader_id}:${r.hash}`;
		let id = clientIds.get(key);
		if (id === undefined) clientIds.set(key, (id = nextClientId--));
		return { ...r, id };
	}

	// Push updates (setting downloads.sync = on): the view answer carries each
	// client's rev, and patches on /live keep the rows current between reads.
	// Without revs (legacy) the page polls exactly as before.
	let sync: Sync | null = null;
	const pushHealthy = () => sync !== null && live.status.state === 'live';
	function interest() {
		if (sync && document.visibilityState === 'visible') api.post('/live/interest', { view: 'downloads' }).catch(() => {});
	}

	function onDl(data: unknown) {
		if (!sync || !isDl(data)) return;
		if (data.kind === 'health') {
			const h: ClientHealth = { id: data.client, name: data.name ?? '', health: data.health, stale: data.stale, seen_at: data.seen_at };
			clientHealth = clientHealth.some((c) => c.id === h.id) ? clientHealth.map((c) => (c.id === h.id ? { ...h, name: h.name || c.name } : c)) : [...clientHealth, h];
			return;
		}
		const out = applyPatch(rows, sync, data, withId);
		if (out.kind === 'reread') soon();
		else if (out.kind === 'applied') {
			rows = out.rows;
			sync = out.sync;
			if (out.upTotal !== undefined) upTotal = out.upTotal;
		}
	}

	async function load() {
		try {
			const [d, i] = await Promise.all([
				api.get<ViewAnswer>(`/downloads/view?limit=100${outsideLimit > 200 ? `&outside_limit=${outsideLimit}` : ''}`),
				watch ? Promise.resolve({ imports: [] as ImportRecord[] }) : api.get<{ imports: ImportRecord[] }>('/imports?limit=100')
			]);
			rows = d.downloads.map(withId);
			clientHealth = d.clients ?? [];
			upTotal = d.totals?.up_speed;
			moreOutside = !!d.next_cursor;
			const was = sync;
			sync = fromAnswer(d);
			dlPush.on = sync !== null;
			if (sync && !was) interest();
			imports = i.imports;
			reads++;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	// Live news comes often while something downloads: read again at most
	// once every 400 ms. While something downloads, a slow poll keeps the
	// speeds moving between news.
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
		void libraryTitles()
			.then((r) => {
				titles = r;
				clearNote('titles');
			})
			.catch(() => note('titles', 'Posters could not be loaded; the downloads are all here.'))
			.finally(() => (titlesReady = true));
		// The remove defaults are a settings read; without that permission the server's own defaults stand (no 403 in the console).
		if (session.can('settings')) api
				.get<DownloaderSettings>('/settings/downloaders')
				.then((s) => (settings = s))
				.catch(() => note('settings', 'Your remove defaults could not be loaded; Remove asks with the usual choices.'));
		if (session.can('manage_library'))
			api
				.get<{ downloaders: DownloaderView[] }>('/downloaders')
				.then((r) => {
					torrentClients = r.downloaders.filter((d) => d.protocol === 'torrent').map((d) => ({ id: d.id, name: d.name, category: d.category }));
					torrent = torrentClients.length > 0;
				})
				.catch(() => note('downloaders', 'The Downloader list could not be loaded; adding torrents by hand is off for now.'));
		if (!watch) void refreshDecisions();
		// Library and download news on /live is for managers; a watcher receives only dl patches (push mode).
		const offs = watch ? [] : ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		const decOffs = watch ? [] : ['slot', 'releases', 'resync'].map((t) => live.on(t, () => void refreshDecisions()));
		const offDl = live.on('dl', onDl);
		// While push updates flow, the rows move with them: no 2 s reads.
		const poll = setInterval(() => {
			if (document.visibilityState === 'visible' && rows.some((r) => r.active) && !pushHealthy()) soon();
		}, 2000);
		// Without push the stream is for managers: a watcher reads again every 10 s while the tab is visible.
		const watchPoll = setInterval(() => {
			if (watch && document.visibilityState === 'visible' && !pushHealthy()) soon();
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
			clearInterval(poll);
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
		if (!confirm(`Mark ${who(r)} failed? Its Release is turned down, it is removed from the Downloader and HolyArrs looks for the next Release.`)) return;
		busy = r.id;
		try {
			await api.post(`/downloads/${r.id}/mark-failed`);
			toasts.push('Marked failed. HolyArrs looks for the next Release.', 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = 0;
		}
	}

	// The Remove dialog: one download, or a selection. Delete files is off
	// unless asked for (Shift+Delete), except a HolyArrs download follows the
	// remove defaults in Settings.
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
			toasts.push('These cannot be removed from here.', 'info');
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
		try {
			// Client items go in one call per Downloader; HolyArrs' own each through HolyArrs.
			const outsideRows = list.filter((r) => r.client_item);
			for (const [id, rows] of byClient(outsideRows)) {
				try {
					await api.post(`/downloaders/${id}/bulk`, { action: 'remove', hashes: rows.map((r) => r.hash), delete_files: choice.delete_files });
				} catch (err) {
					bad.push(message(err));
				}
			}
			for (const r of list.filter((x) => !x.client_item)) {
				try {
					await api.post(`/downloads/${r.id}/remove`, removeBody(list.length === 1 ? choice : { delete_files: choice.delete_files, turn_down: false, search_again: false }));
				} catch (err) {
					bad.push(`${who(r)}: ${message(err)}`);
				}
			}
			if (bad.length) {
				removeError = bad.join(' · ');
			} else {
				removeOpen = false;
				const one = list.length === 1 && !list[0].client_item;
				toasts.push(one && choice.turn_down && choice.search_again ? 'Removed. HolyArrs looks for the next Release.' : list.length > 1 ? `Removed ${list.length} downloads.` : 'Removed.', 'ok');
			}
			await load();
		} finally {
			removeBusy = false;
			busy = 0;
		}
	}
	// Client actions: the Downloader does the work; the row's new state is read back from it.
	const DONE: Record<string, string> = { pause: 'Paused.', resume: 'Started.', top: 'Moved to the top.', recheck: 'Checking again.', reannounce: 'Asked the trackers again.' };
	async function act(r: DownloadRow, action: 'pause' | 'resume' | 'top' | 'recheck' | 'reannounce') {
		busy = r.id;
		try {
			await api.post(`${rowPath(r)}/${action}`);
			toasts.push(DONE[action], 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = 0;
		}
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
			toasts.push('None of these can do that from here.', 'info');
			return;
		}
		const bad: string[] = [];
		for (const [id, rows] of groups) {
			try {
				await api.post(`/downloaders/${id}/bulk`, { action, hashes: rows.map((r) => r.hash) });
			} catch (e) {
				bad.push(`${rows[0].downloader || 'Downloader'}: ${message(e)}`);
			}
		}
		const n = list.length - left;
		if (bad.length) toasts.push(bad.join(' · '), 'fail');
		else toasts.push(`${BULK_DONE[action]}: ${n} ${n === 1 ? 'download' : 'downloads'}${left ? ` (${left} skipped)` : ''}.`, 'ok');
		await load();
	}
	type ClientStatus = { paused: boolean; speed_limit: number; up_limit: number };
	type ClientView = { id: number; name: string; protocol: string; controls: boolean; status?: ClientStatus; error?: string };
	async function all(pause: boolean) {
		try {
			const r = await api.post<{ clients: ClientView[] }>(pause ? '/downloads/pause-all' : '/downloads/resume-all');
			const bad = r.clients.filter((c) => c.controls && c.error);
			if (bad.length) toasts.push(`${bad.map((c) => c.name).join(', ')}: ${bad[0].error}`, 'fail');
			else toasts.push(pause ? 'Paused everything.' : 'Started everything.', 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}

	// The speed limit: one cap per Downloader, in MB/s (0 = none), read from the client.
	let limitOpen = $state(false);
	let clients = $state<ClientView[]>([]);
	let limits = $state<Record<number, { down: number; up: number }>>({});
	let limitError = $state('');
	let limitBusy = $state(false);
	const MB = 1_000_000;
	async function openLimit() {
		limitError = '';
		limitOpen = true;
		try {
			const r = await api.get<{ clients: ClientView[] }>('/downloads/clients');
			clients = r.clients.filter((c) => c.controls);
			limits = Object.fromEntries(clients.map((c) => [c.id, { down: +((c.status?.speed_limit ?? 0) / MB).toFixed(1), up: +((c.status?.up_limit ?? 0) / MB).toFixed(1) }]));
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
				failing = c.name;
				await api.put(`/downloaders/${c.id}/speed-limit`, { down: Math.round(Math.max(0, l.down || 0) * MB), up: Math.round(Math.max(0, l.up || 0) * MB) });
			}
			failing = '';
			limitOpen = false;
			toasts.push('Speed limit saved.', 'ok');
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

<svelte:head><title>Downloads · HolyArrs</title></svelte:head>

<PageLoader loading={!loaded} label="Loading Downloads">
{#if error && allRows.length === 0}
	<div class="pad"><LoadError hasData={false} message="Downloads could not be loaded." onretry={load} /></div>
{:else if allRows.length === 0}
	<div class="pad">
	{#if watch}
	<EmptyState icon="download" title="Nothing on its way" text="When something you asked for starts downloading, you can watch it here." />
	{:else}
	<EmptyState icon="download" title="No downloads yet" text="When HolyArrs chooses a Release for a movie or an episode, it sends it to a Downloader and you can watch it here, live." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
	{/if}
	{#if !watch}<p class="aside-link"><a href={to('/settings/downloaders')}>Downloaders</a>{#if torrentClients.length}<button type="button" class="addlk" onclick={() => (addOpen = true)} data-testid="dl-add-empty">Add a torrent</button>{/if}</p>{/if}
	</div>
{:else}
	{#if error}<div class="pad"><LoadError hasData message="Downloads could not be loaded." onretry={load} /></div>{/if}
	{#if notes.length}<ul class="notes" role="status" data-testid="dl-notes">{#each notes as n (n)}<li>{n}</li>{/each}</ul>{/if}
	{#if !watch}<div class="matchpad"><ConfirmMatch refresh={reads} onchange={() => void load()} /></div>{/if}
	<DlSections rows={allRows} {importOf} titles={titleById} {titlesReady} {busy} {torrent} {decisionsOf} bind:openId onfail={markFailed} onremove={askRemove} ondone={() => void load()} onact={act} onall={all} onlimit={openLimit} onadd={torrentClients.length ? () => (addOpen = true) : undefined} onbulk={bulk} onsteer={steer} onremovemany={askRemoveMany} {watch} {upTotal} />
	{#if moreOutside}<p class="more"><Button size="sm" variant="plain" onclick={() => { outsideLimit += 200; void load(); }} data-testid="dl-outside-more">Show more torrents</Button></p>{/if}
{/if}
</PageLoader>

<SteerSheet bind:open={steerOpen} mode={steerMode} rows={steerRows} ondone={() => void load()} />
<AddTorrents bind:open={addOpen} clients={torrentClients} ondone={() => void load()} />

<Sheet bind:open={removeOpen} title={targets.length > 1 ? `Remove ${targets.length} downloads?` : 'Remove this download?'} size="sm">
	{#if targets.length}
		<form id="remove-form" onsubmit={remove} data-testid="dl-remove-form">
			{#if target}
				<p class="who">{who(target)}</p>
				<details class="raw"><summary>Details</summary><p>{target.name}</p></details>
			{:else}
				<ul class="many" data-testid="dl-remove-list">{#each targets.slice(0, 6) as r (r.id)}<li>{who(r)}</li>{/each}{#if targets.length > 6}<li class="hint">and {targets.length - 6} more</li>{/if}</ul>
			{/if}
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.delete_files} data-testid="dl-remove-files" />
				<span><b>Also delete files</b><br /><span class="hint">Off: {target?.downloader || 'the Downloader'} keeps what it fetched.</span></span>
			</label>
			{#if target && !target.client_item}
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.turn_down} />
				<span><b>Add its Release to Turned down</b><br /><span class="hint">HolyArrs never picks it again for {manyWords(target.kind) ? `these ${manyWords(target.kind)?.many}` : 'this movie'}.</span></span>
			</label>
			<label class="checkline" class:off={!choice.turn_down}>
				<input type="checkbox" bind:checked={choice.search_again} disabled={!choice.turn_down} />
				<span><b>Search again</b><br /><span class="hint">Look for the next Release straight away. Without it the {manyWords(target.kind) ? `${manyWords(target.kind)?.many} wait` : 'movie waits'} for the next feed check.</span></span>
			</label>
			{/if}
			{#if blocked.length}<p class="hint" data-testid="dl-remove-blocked">{blocked.length === targets.length ? 'Removing' : `${blocked.length} of these are not from HolyArrs. Removing`} torrents HolyArrs did not send needs settings access{blocked.length === targets.length ? '.' : ', so they stay.'}</p>{/if}
			{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		{@const n = targets.length - blocked.length}
		<Button variant="plain" onclick={() => (removeOpen = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="remove-form" busy={removeBusy} disabled={removeBusy || n === 0} data-testid="dl-remove-confirm">{n > 1 ? `Remove ${n}` : 'Remove'}{choice.delete_files ? ' and delete files' : ''}</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={limitOpen} title="Speed limit" size="sm">
	<form id="limit-form" onsubmit={saveLimit}>
		<p class="hint">In MB/s for each Downloader. 0 means no limit.</p>
		{#each clients as c (c.id)}
			{#if limits[c.id]}
				<fieldset class="lim" data-testid="dl-limit-{c.id}">
					<legend>{c.name}</legend>
					<label>Download<input type="number" min="0" step="0.1" inputmode="decimal" bind:value={limits[c.id].down} /></label>
					{#if c.protocol === 'torrent'}<label>Upload<input type="number" min="0" step="0.1" inputmode="decimal" bind:value={limits[c.id].up} /></label>{/if}
				</fieldset>
			{/if}
		{:else}
			{#if !limitError}<p class="hint">Reading the Downloaders…</p>{/if}
		{/each}
		{#if limitError}<p class="form-error" role="alert">{limitError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (limitOpen = false)}>Cancel</Button>
		<Button type="submit" form="limit-form" busy={limitBusy} disabled={limitBusy || clients.length === 0}>Save</Button>
	{/snippet}
</Sheet>

<style>
	.lim { border: 0; padding: 0; margin: 0; display: flex; flex-wrap: wrap; gap: 12px; min-width: 0; }
	.lim legend { font: var(--t-headline); margin-bottom: 6px; padding: 0; }
	.lim label { display: flex; flex-direction: column; gap: 4px; font: var(--t-subhead); color: var(--text-muted); flex: 1 1 120px; min-width: 0; }
	.lim input { min-height: 44px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--border); background: var(--surface-2); color: var(--text); font: var(--t-callout); width: 100%; box-sizing: border-box; }
	.pad { padding: var(--s-6) var(--gutter); }
	.matchpad { padding: 0 var(--gutter); }
	.notes { list-style: none; margin: 0; padding: var(--s-2) var(--gutter) 0; display: grid; gap: 2px; color: var(--text-muted); font: var(--t-footnote); }
	.more { display: flex; justify-content: center; padding: var(--s-4) var(--gutter); }
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
