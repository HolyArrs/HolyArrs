<script lang="ts">
	import { libraryTitles, ownedLibrary } from '#lib/owned.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { dlScope } from '#lib/search-field.svelte.ts';
	import { replaceState } from '$app/navigation';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import { subLook, subOf } from '#lib/downloads/subLook.svelte.ts';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { decisionName, manyWords, removeBody, removeStart, rowName, type Decision, type DownloaderSettings, type DownloaderView, type DownloadRow, type RemoveChoice } from '#lib/downloaders.ts';
	import { decisions, refreshDecisions } from '#lib/decisions.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { ImportRecord, TitleView } from '#lib/types.ts';
	import DlSections from '#lib/downloads/DlSections.svelte';
	import { byDownload } from '#lib/downloads/imports.ts';
	import { addSample, inHistory, totals } from '#lib/downloads/sections.ts';

	let rows = $state<DownloadRow[]>([]);
	let imports = $state<ImportRecord[]>([]);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	// The poster pictures come from the Library titles; the rows wait for them, so no row first shows initials.
	let titlesReady = $state(false);
	let error = $state('');
	let settings = $state<DownloaderSettings | null>(null);
	let openId = $state(0);
	// A torrent Downloader brings the Seeding sub-tab under Done.
	let torrent = $state(false);
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
			label: 'Needs a decision',
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
		...rows.map((r) => (decisionsByDownload.has(r.id) && !r.active && r.phase !== 'needs_a_decision' ? { ...r, phase: 'needs_a_decision', label: 'Needs a decision' } : r)),
		...decisions.items.filter((d) => !(d.download_id && rowIds.has(d.download_id))).map(decisionRow)
	]);
	function decisionsOf(r: DownloadRow): Decision[] {
		return r.id < 0 ? decisions.items.filter((d) => d.slot_id === -r.id) : (decisionsByDownload.get(r.id) ?? []);
	}

	// The speed graph: one reading of the Queue's total speed per load, kept here.
	let samples = $state<number[]>([]);
	// The server's speed history since the app started, read once so the graph is full on first paint.
	let history = $state<{ down: number; up: number }[] | null>(null);

	// When the open row goes away (its decision was settled), its details close.
	$effect(() => {
		if (loaded && openId && !allRows.some((r) => r.id === openId)) openId = 0;
	});

	// &open=<download id> or &slot=<slot id> (Home's Needs attention) opens that row's details once.
	let opened = false;
	$effect(() => {
		if (opened || !loaded) return;
		const q = page.url.searchParams;
		const dl = Number(q.get('open'));
		const slot = Number(q.get('slot'));
		if (!dl && !slot) {
			opened = true;
			return;
		}
		const d = slot ? decisions.items.find((x) => x.slot_id === slot) : undefined;
		if (slot && !d && decisions.items.length === 0) return; // the decisions are still loading
		opened = true;
		const id = dl || (d?.download_id && rowIds.has(d.download_id) ? d.download_id : -slot);
		if (allRows.some((r) => r.id === id)) openId = id;
	});

	async function load() {
		try {
			const [d, i, h] = await Promise.all([
				api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=100'),
				api.get<{ imports: ImportRecord[] }>('/imports?limit=100'),
				history ? Promise.resolve(null) : api.get<{ samples: { down: number; up: number }[] }>('/downloads/speeds').catch(() => ({ samples: [] }))
			]);
			if (h) history = h.samples;
			rows = d.downloads;
			imports = i.imports;
			samples = addSample(samples, totals(d.downloads.filter((r) => !inHistory(r))).speed);
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	// Live news comes often while something downloads: read again at most
	// once every 400 ms. While something downloads, a slow poll keeps the
	// speed graph moving between news.
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
		void libraryTitles().then((r) => (titles = r)).catch(() => {}).finally(() => (titlesReady = true));
		// The remove defaults are a settings read; without that permission the server's own defaults stand (no 403 in the console).
		if (session.can('settings')) api.get<DownloaderSettings>('/settings/downloaders').then((s) => (settings = s)).catch(() => {});
		if (session.can('manage_library'))
			api
				.get<{ downloaders: DownloaderView[] }>('/downloaders')
				.then((r) => (torrent = r.downloaders.some((d) => d.protocol === 'torrent')))
				.catch(() => {});
		void refreshDecisions();
		const offs = ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		const decOffs = ['slot', 'releases', 'resync'].map((t) => live.on(t, () => void refreshDecisions()));
		const poll = setInterval(() => {
			if (document.visibilityState === 'visible' && rows.some((r) => r.active)) soon();
		}, 2000);
		return () => {
			offs.forEach((off) => off());
			decOffs.forEach((off) => off());
			clearTimeout(timer);
			clearInterval(poll);
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

	// The Remove dialog.
	let removeOpen = $state(false);
	let target = $state<DownloadRow | null>(null);
	let choice = $state<RemoveChoice>(removeStart(null));
	let removeError = $state('');
	function askRemove(r: DownloadRow) {
		target = r;
		choice = removeStart(settings);
		removeError = '';
		removeOpen = true;
	}
	async function remove(e: SubmitEvent) {
		e.preventDefault();
		if (!target) return;
		busy = target.id;
		try {
			await api.post(`/downloads/${target.id}/remove`, removeBody(choice));
			removeOpen = false;
			toasts.push(choice.turn_down && choice.search_again ? 'Removed. HolyArrs looks for the next Release.' : 'Removed.', 'ok');
			await load();
		} catch (err) {
			removeError = message(err);
		} finally {
			busy = 0;
		}
	}
	// A switch for how the sub-statuses look (?sub=a..g); the URL is the saved copy, the store is what both sides read.
	const LOOKS = [{ value: 'a', label: 'a \u00b7 Expand' }, { value: 'b', label: 'b \u00b7 Attach' }, { value: 'c', label: 'c \u00b7 Filter' }, { value: 'd', label: 'd \u00b7 Tray' }, { value: 'e', label: 'e \u00b7 Split' }, { value: 'f', label: 'f \u00b7 Menu' }, { value: 'g', label: 'g \u00b7 Underline' }];
	subLook.value = subOf(page.url.searchParams.get('sub'));
	$effect(() => {
		const pick = subLook.value;
		const u = new URL(location.href);
		u.searchParams.delete('tabs');
		u.searchParams.delete('rail');
		if ((u.searchParams.get('sub') ?? 'a') === pick && u.href === location.href) return;
		u.searchParams.set('sub', pick);
		try { replaceState(u, page.state); } catch { /* router not ready */ }
	});
	// The top bar's search field filters this page while it is open.
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

<PageLoader loading={!loaded || !titlesReady} label="Loading Downloads">
{#if error && allRows.length === 0}
	<div class="pad"><LoadError hasData={false} message="Downloads could not be loaded." onretry={load} /></div>
{:else if allRows.length === 0}
	<div class="pad">
	<EmptyState icon="download" title="No downloads yet" text="When HolyArrs chooses a Release for a movie or an episode, it sends it to a Downloader and you can watch it here, live." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
	<p class="aside-link"><a href={to('/settings/downloaders')}>Downloaders</a></p>
	</div>
{:else}
	{#if error}<div class="pad"><LoadError hasData message="Downloads could not be loaded." onretry={load} /></div>{/if}
	<DlSections rows={allRows} {importOf} titles={titleById} {busy} {samples} history={history ?? []} {torrent} {decisionsOf} bind:openId onfail={markFailed} onremove={askRemove} ondone={() => void load()} />
{/if}
</PageLoader>

<div class="railbar" data-testid="sub-look"><SegmentedControl options={LOOKS} bind:value={subLook.value} label="Sub-status look" size="sm" announce={(l) => `Showing ${l}`} /></div>

<Sheet bind:open={removeOpen} title="Remove this download?" size="sm">
	{#if target}
		<form id="remove-form" onsubmit={remove}>
			<p class="who">{who(target)}</p>
			<details class="raw"><summary>Details</summary><p>{target.name}</p></details>
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.delete_files} />
				<span><b>Delete its files</b><br /><span class="hint">Off: {target.downloader || 'the Downloader'} keeps what it fetched.</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.turn_down} />
				<span><b>Add its Release to Turned down</b><br /><span class="hint">HolyArrs never picks it again for {manyWords(target.kind) ? `these ${manyWords(target.kind)?.many}` : 'this movie'}.</span></span>
			</label>
			<label class="checkline" class:off={!choice.turn_down}>
				<input type="checkbox" bind:checked={choice.search_again} disabled={!choice.turn_down} />
				<span><b>Search again</b><br /><span class="hint">Look for the next Release straight away. Without it the {manyWords(target.kind) ? `${manyWords(target.kind)?.many} wait` : 'movie waits'} for the next feed check.</span></span>
			</label>
			{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="remove-form" busy={busy !== 0} disabled={busy !== 0}>Remove</Button>
	{/snippet}
</Sheet>

<style>
	.railbar { position: fixed; z-index: 30; left: 0; right: 0; bottom: 44px; width: max-content; max-width: calc(100vw - 24px); overflow-x: auto; margin-inline: auto; padding: 4px; border-radius: 20px; background: color-mix(in srgb, var(--surface-1) 78%, transparent); border: 1px solid var(--line); box-shadow: var(--shadow-lift); backdrop-filter: blur(20px) saturate(160%); -webkit-backdrop-filter: blur(20px) saturate(160%); }
	@media (max-width: 767px) { .railbar { bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom) + 40px); } }
	.pad { padding: var(--s-6) var(--gutter); }
	.aside-link { text-align: center; margin-top: -24px; }
	.aside-link a { display: inline-flex; align-items: center; min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	.raw { color: var(--text-muted); font: var(--t-footnote); }
	.raw summary { width: max-content; min-height: 24px; cursor: pointer; border-radius: 6px; }
	.raw summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw p { margin-top: 4px; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.who { font: var(--t-headline); margin-bottom: 4px; }
	form { display: flex; flex-direction: column; gap: 14px; }
	.checkline { display: flex; gap: 12px; align-items: flex-start; font: var(--t-callout); cursor: pointer; }
	.checkline input { width: 20px; height: 20px; margin-top: 1px; flex: none; accent-color: var(--accent); }
	.hint { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.off { opacity: 0.6; }
	.form-error { color: var(--fail); font: var(--t-callout); }
</style>
