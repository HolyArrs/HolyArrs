<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes, duration, percent } from '#lib/format.ts';
	import { episodeStateWords, manyWords, phaseTone, removeBody, removeStart, rowName, type DownloaderSettings, type DownloaderView, type DownloadRow, type RemoveChoice } from '#lib/downloaders.ts';
	import Poster from '#lib/Poster.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { plainReason } from '#lib/plain.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import OpsDownloadsTabs from '#lib/ui/OpsDownloadsTabs.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { ImportRecord, TitleView } from '#lib/types.ts';
	import VariantBar from '#lib/ui/VariantBar.svelte';
	import DlTable from '#lib/downloads/DlTable.svelte';
	import { SETS, SET_IDS, setOf, statusWord } from '#lib/downloads/status-sets.ts';
	import { byDownload, failReason, placed, statusDetail } from '#lib/downloads/imports.ts';

	// The status-chip sets Luis is comparing on the table (?set=1…7, kept for the session; 6 when none is picked).
	const SET_OPTIONS = SET_IDS.map((id) => ({ value: id, label: SETS[id].label }));
	let setPick = $state<number>(setOf(page.url.searchParams.get('set')));
	const set = $derived(setOf(setPick));
	// A torrent Downloader brings the Seeding chip in set 5.
	let torrent = $state(false);

	let rows = $state<DownloadRow[]>([]);
	let imports = $state<ImportRecord[]>([]);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let settings = $state<DownloaderSettings | null>(null);
	let selectedId = $state(0);
	const selected = $derived(rows.find((r) => r.id === selectedId) ?? null);
	const titleById = $derived(new Map(titles.map((title) => [title.id, title])));
	// Imports belong to their download's row; one with no row here stays in History.
	const importOf = $derived(byDownload(imports));

	// A row opens its details in a Sheet (from the right; a bottom sheet on phones).
	let detailOpen = $state(false);
	function selectDownload(id: number) {
		selectedId = id;
		detailOpen = true;
	}

	/** toneKind picks the kit status colour for a tone; the word stays the row's own. */
	function toneKind(t: string): StatusKind {
		return t === 'ok' ? 'in_library' : t === 'fail' ? 'failed' : t === 'idle' ? 'turned_down' : 'missing';
	}
	function pill(r: DownloadRow): StatusKind {
		return r.phase === 'downloading' ? 'downloading' : toneKind(phaseTone(r.phase));
	}
	const moving = (r: DownloadRow) => r.phase !== 'removed' && r.phase !== 'failed' && r.phase !== 'vanished';
	const admin = $derived(session.can('settings'));

	async function load() {
		try {
			const [d, i] = await Promise.all([
				api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=100'),
				api.get<{ imports: ImportRecord[] }>('/imports?limit=100')
			]);
			rows = d.downloads;
			imports = i.imports;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	// Live news comes often while something downloads: read again at most
	// once a second.
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
		void api.get<{ titles: TitleView[] }>('/titles').then((r) => (titles = r.titles)).catch(() => {});
		// The remove defaults are a settings read; without that permission the server's own defaults stand (no 403 in the console).
		if (session.can('settings')) api.get<DownloaderSettings>('/settings/downloaders').then((s) => (settings = s)).catch(() => {});
		if (session.can('manage_library'))
			api
				.get<{ downloaders: DownloaderView[] }>('/downloaders')
				.then((r) => (torrent = r.downloaders.some((d) => d.protocol === 'torrent')))
				.catch(() => {});
		const offs = ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		return () => {
			offs.forEach((off) => off());
			clearTimeout(timer);
		};
	});

	function who(r: DownloadRow) {
		const n = rowName(r);
		return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
	}

	let busy = $state(0);
	async function markFailed(r: DownloadRow) {
		detailOpen = false;
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
		detailOpen = false;
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
</script>

<svelte:head><title>Downloads · HolyArrs</title></svelte:head>

<PageHeader title="Downloads" subtitle="What your Downloaders are fetching right now, and what HolyArrs moved into the Library." />
<OpsDownloadsTabs />

{#snippet detail(s: DownloadRow)}
	{@const p = s.text ? plainReason(s.text) : null}
	{@const im = importOf.get(s.id)}
	{@const put = placed(im)}
	{@const why = failReason(im)}
	{@const more = statusDetail(s, im)}
	<div class="d-head">
		<span class="d-art" class:square={s.kind === 'artist'} aria-hidden="true"><Poster src={s.artwork ?? titleById.get(s.title_id ?? 0)?.artwork[0]} name={s.title || s.name} /></span>
		<div class="d-names">
			<p class="d-title">{rowName(s)}</p>
			{#if manyWords(s.kind) && s.covers}<p class="d-sub">{s.covers}</p>{/if}
			<StatusPill kind={pill(s)} label={more ? `${statusWord(s, set)} · ${more}` : statusWord(s, set)} />
		</div>
	</div>
	{#if moving(s)}<ProgressBar value={percent(s.progress)} label="Progress of {who(s)}" showValue tone={s.progress >= 1 ? 'ok' : 'info'} />{/if}
	<dl class="facts">
		<dt>Progress</dt><dd>{percent(s.progress)}%</dd>
		{#if s.active && s.eta}<dt>Time left</dt><dd>{duration(s.eta)}</dd>{/if}
		{#if s.active && s.speed}<dt>Speed</dt><dd>{bytes(s.speed)}/s</dd>{/if}
		{#if s.size}<dt>Size</dt><dd>{bytes(s.size)}</dd>{/if}
		{#if s.downloader}<dt>Downloader</dt><dd>{s.downloader}</dd>{/if}
		{#if s.version}<dt>Version</dt><dd>{s.version}</dd>{/if}
		{#if put}<dt>Import</dt><dd data-testid="import-how">{put.how}</dd>{#if put.where}<dt>Folder</dt><dd data-testid="import-where">{put.where}</dd>{/if}{/if}
		{#if why}<dt>Import</dt><dd data-testid="import-failed">Failed: {why}</dd>{/if}
	</dl>
	{#if p}<div class="note" class:bad={phaseTone(s.phase) === 'fail'}><p data-testid="row-text">{p.text}</p></div>{/if}
	{#if manyWords(s.kind) && s.episodes && s.episodes.length > 1}
		<details class="eps" data-testid="row-episodes">
			<summary>The {s.episodes.length} {manyWords(s.kind)?.many} it covers</summary>
			<ul class="eplist">
				{#each s.episodes as e (e.slot_id)}
					{@const st = episodeStateWords(e.state)}
					<li data-testid="row-episode">
						<span class="eplabel">{e.label}</span>
						<span class="epname">{e.name ?? ''}</span>
						<StatusPill kind={toneKind(st.tone)} label={st.text} />
					</li>
				{/each}
			</ul>
		</details>
	{/if}
	<details class="raw">
		<summary>Details</summary>
		<p>{s.name}</p>
		{#if admin && p?.detail}<p>{p.detail}</p>{/if}
	</details>
	{#if s.phase === 'needs_a_decision'}
		<Button variant="primary" full href={to('/downloads/decide')}>Open Needs a decision</Button>
	{/if}
	{#if s.can_act}
		<div class="d-acts">
			<Button variant="secondary" full onclick={() => markFailed(s)} disabled={busy === s.id}>Mark failed</Button>
			<Button variant="destructive" full onclick={() => askRemove(s)} disabled={busy === s.id}>Remove</Button>
		</div>
	{/if}
{/snippet}

<PageLoader loading={!loaded} label="Loading Downloads">
{#if error && rows.length === 0}
	<LoadError hasData={false} message="Downloads could not be loaded." onretry={load} />
{:else if rows.length === 0}
	<EmptyState icon="download" title="No downloads yet" text="When HolyArrs chooses a Release for a movie or an episode, it sends it to a Downloader and you can watch it here, live." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
	<p class="aside-link"><a href={to('/settings/downloaders')}>Downloaders</a></p>
{:else}
	{#if error}<LoadError hasData message="Downloads could not be loaded." onretry={load} />{/if}
	<DlTable {rows} {imports} titles={titleById} {busy} {set} {torrent} ondetails={(r: DownloadRow) => selectDownload(r.id)} onfail={markFailed} onremove={askRemove} />
{/if}
</PageLoader>

<Sheet bind:open={detailOpen} title="Download details" size="md" placement="right">
	{#if selected}{@render detail(selected)}{/if}
</Sheet>

<VariantBar options={SET_OPTIONS} bind:value={setPick} param="set" storageKey="holyarrs.downloads-set" label="Downloads status chips" />

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
	.aside-link { text-align: center; margin-top: -24px; }
	.aside-link a { display: inline-flex; align-items: center; min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	.note { padding: 8px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); overflow-wrap: anywhere; }
	.note.bad { box-shadow: inset 3px 0 0 var(--fail); }
	.raw { color: var(--text-muted); font: var(--t-footnote); }
	.raw summary { width: max-content; min-height: 24px; cursor: pointer; border-radius: 6px; }
	.raw summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw p { margin-top: 4px; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.d-head { display: flex; gap: 14px; align-items: flex-start; }
	.d-art { flex: none; width: 64px; --poster-ratio: 2 / 3; }
	.d-art.square { --poster-ratio: 1; }
	.d-names { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; min-width: 0; }
	.d-title { font: var(--t-title3); overflow-wrap: anywhere; }
	.d-sub { font: var(--t-callout); color: var(--text-muted); }
	.facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 8px 12px; margin: 0; padding-top: 12px; border-top: 1px solid var(--line); font: var(--t-callout); }
	.facts dt { color: var(--text-muted); }
	.facts dd { margin: 0; font-family: inherit; text-align: right; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
	.d-acts { display: grid; gap: 8px; }
	.eps summary { width: max-content; max-width: 100%; min-height: 32px; display: flex; align-items: center; cursor: pointer; color: var(--accent-text); font: var(--t-subhead); border-radius: 6px; }
	.eps summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.eplist { list-style: none; padding: 0; margin: 6px 0 0; display: grid; gap: 6px; }
	.eplist li { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; font: var(--t-callout); }
	.eplabel { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.epname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.who { font: var(--t-headline); margin-bottom: 4px; }
	form { display: flex; flex-direction: column; gap: 14px; }
	.checkline { display: flex; gap: 12px; align-items: flex-start; font: var(--t-callout); cursor: pointer; }
	.checkline input { width: 20px; height: 20px; margin-top: 1px; flex: none; accent-color: var(--accent); }
	.hint { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.off { opacity: 0.6; }
	.form-error { color: var(--fail); font: var(--t-callout); }
</style>
