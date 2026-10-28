<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes, duration, percent } from '#lib/format.ts';
	import { decisionName, episodeStateWords, manyWords, phaseTone, removeBody, removeStart, rowName, type Decision, type DownloaderSettings, type DownloaderView, type DownloadRow, type RemoveChoice } from '#lib/downloaders.ts';
	import { decisions, refreshDecisions } from '#lib/decisions.svelte.ts';
	import Poster from '#lib/Poster.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { plainReason } from '#lib/plain.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { ImportRecord, TitleView } from '#lib/types.ts';
	import DlTable from '#lib/downloads/DlTable.svelte';
	import DecisionPanel from '#lib/downloads/DecisionPanel.svelte';
	import DlDetail, { type DetailLook } from '#lib/downloads/DlDetail.svelte';
	import VariantBar from '#lib/ui/VariantBar.svelte';
	import { popIn, popOut } from '#lib/ui/transitions.ts';
	import { tick } from 'svelte';
	import { statusWord } from '#lib/downloads/status-tabs.ts';
	import { byDownload, failReason, placed, statusDetail } from '#lib/downloads/imports.ts';

	// A torrent Downloader brings the Seeding sub-tab under Done.
	let torrent = $state(false);

	let rows = $state<DownloadRow[]>([]);
	let imports = $state<ImportRecord[]>([]);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let settings = $state<DownloaderSettings | null>(null);
	let selectedId = $state(0);
	const titleById = $derived(new Map(titles.map((title) => [title.id, title])));
	// Imports belong to their download's row; one with no row here stays in History.
	const importOf = $derived(byDownload(imports));

	// Needs a decision lives on the rows: a download whose Slots wait for a
	// person counts under Problems and its sheet holds the choices. A decision
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
	const selected = $derived(allRows.find((r) => r.id === selectedId) ?? null);
	function decisionsOf(r: DownloadRow): Decision[] {
		return r.id < 0 ? decisions.items.filter((d) => d.slot_id === -r.id) : (decisionsByDownload.get(r.id) ?? []);
	}

	// ?details=1…5 picks one of the Download details looks Luis is comparing;
	// none (or now) keeps today's side sheet. On phones every look opens as a
	// bottom sheet; on wide screens 2 opens the row in place and 3 is a card at
	// the row.
	let details = $state(0);
	const DETAILS = [
		{ value: 0, label: 'Now' },
		{ value: 1, label: '1 Side panel' },
		{ value: 2, label: '2 In the row' },
		{ value: 3, label: '3 Card' },
		{ value: 4, label: '4 Drawer' },
		{ value: 5, label: '5 Track' }
	];
	const LOOK: Record<number, DetailLook> = { 1: 'panel', 2: 'inline', 3: 'card', 4: 'drawer', 5: 'track' };
	const look = $derived(LOOK[details]);
	let phone = $state(false);
	onMount(() => {
		const mq = matchMedia('(max-width: 767px)');
		const set = () => (phone = mq.matches);
		set();
		mq.addEventListener('change', set);
		return () => mq.removeEventListener('change', set);
	});
	// Which container the details use right now.
	const holder = $derived(!look || phone ? 'sheet' : look === 'inline' ? 'row' : look === 'card' ? 'card' : 'sheet');

	// A row opens its details in a Sheet (from the right; a bottom sheet on phones).
	let detailOpen = $state(false);
	let expandId = $state(0);
	let cardId = $state(0);
	let opener: HTMLElement | null = null;
	function selectDownload(id: number) {
		if (holder === 'row') {
			expandId = expandId === id ? 0 : id;
			selectedId = id;
			return;
		}
		if (holder === 'card') {
			if (cardId === id) return closeCard();
			opener = document.querySelector<HTMLElement>(`[data-dl-id="${id}"]`);
			selectedId = id;
			cardId = id;
			void tick().then(() => {
				place();
				card?.focus();
			});
			return;
		}
		selectedId = id;
		detailOpen = true;
	}
	function closeDetails() {
		detailOpen = false;
		expandId = 0;
		closeCard(false);
	}
	// Switching looks closes whatever was open.
	$effect(() => {
		void holder;
		detailOpen = false;
		expandId = 0;
		cardId = 0;
	});

	// 3 · the card sits under its row (above it when there is no room below),
	// follows it while the page scrolls and closes on Escape or a click outside.
	let card = $state<HTMLElement>();
	let cardPos = $state({ top: 0, left: 0, up: false });
	function place() {
		if (!cardId || !card) return;
		const row = document.querySelector<HTMLElement>(`[data-dl-id="${cardId}"]`);
		if (!row) return;
		const r = row.getBoundingClientRect();
		const h = card.offsetHeight;
		const w = card.offsetWidth;
		const up = r.bottom + 8 + h > innerHeight - 12 && r.top - 8 - h > 12;
		const left = Math.max(16, Math.min(r.left + 44, innerWidth - w - 16));
		cardPos = { top: up ? r.top - 8 - h : r.bottom + 8, left, up };
	}
	function closeCard(focusBack = true) {
		if (!cardId) return;
		cardId = 0;
		if (focusBack) opener?.focus();
		opener = null;
	}
	function cardKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && cardId) {
			e.preventDefault();
			closeCard();
		}
	}
	function outside(e: PointerEvent) {
		if (!cardId || !card) return;
		const t = e.target as Element;
		if (card.contains(t)) return;
		// A click on its own row toggles it through the row's handler.
		if (t.closest?.(`[data-dl-id="${cardId}"]`)) return;
		// Menus and dialogs opened from the card live outside it.
		if (t.closest?.('dialog, [role="menu"]')) return;
		closeCard(false);
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

	// When the open row goes away (its decision was settled), the sheet closes.
	$effect(() => {
		if (loaded && !selected && (detailOpen || expandId || cardId)) closeDetails();
	});

	// &open=<download id> or &slot=<slot id> (Home's Needs attention) opens that row's sheet once.
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
		if (allRows.some((r) => r.id === id)) selectDownload(id);
	});

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
		void refreshDecisions();
		const offs = ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		const decOffs = ['slot', 'releases', 'resync'].map((t) => live.on(t, () => void refreshDecisions()));
		return () => {
			offs.forEach((off) => off());
			decOffs.forEach((off) => off());
			clearTimeout(timer);
		};
	});

	function who(r: DownloadRow) {
		const n = rowName(r);
		return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
	}

	let busy = $state(0);
	async function markFailed(r: DownloadRow) {
		closeDetails();
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
		closeDetails();
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
			<StatusPill kind={pill(s)} label={more ? `${statusWord(s)} · ${more}` : statusWord(s)} />
		</div>
	</div>
	{#if s.id > 0 && moving(s)}<ProgressBar value={percent(s.progress)} label="Progress of {who(s)}" showValue tone={s.progress >= 1 ? 'ok' : 'info'} />{/if}
	<dl class="facts">
		{#if s.id > 0}<dt>Progress</dt><dd>{percent(s.progress)}%</dd>{/if}
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
	{@const ds = decisionsOf(s)}
	{#if ds.length}<DecisionPanel items={ds} ondone={() => void load()} />{/if}
	{#if s.can_act}
		<div class="d-acts">
			<Button variant="secondary" full onclick={() => markFailed(s)} disabled={busy === s.id}>Mark failed</Button>
			<Button variant="destructive" full onclick={() => askRemove(s)} disabled={busy === s.id}>Remove</Button>
		</div>
	{/if}
{/snippet}

<PageLoader loading={!loaded} label="Loading Downloads">
{#if error && allRows.length === 0}
	<LoadError hasData={false} message="Downloads could not be loaded." onretry={load} />
{:else if allRows.length === 0}
	<EmptyState icon="download" title="No downloads yet" text="When HolyArrs chooses a Release for a movie or an episode, it sends it to a Downloader and you can watch it here, live." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
	<p class="aside-link"><a href={to('/settings/downloaders')}>Downloaders</a></p>
{:else}
	{#if error}<LoadError hasData message="Downloads could not be loaded." onretry={load} />{/if}
	<DlTable rows={allRows} {imports} titles={titleById} {busy} {torrent} ondetails={(r: DownloadRow) => selectDownload(r.id)} onfail={markFailed} onremove={askRemove} expandId={holder === 'row' ? expandId : 0} expand={holder === 'row' ? inPlace : undefined} />
{/if}
</PageLoader>

{#snippet looked(s: DownloadRow, l: DetailLook)}
	<DlDetail r={s} im={importOf.get(s.id)} decisions={decisionsOf(s)} art={s.artwork ?? titleById.get(s.title_id ?? 0)?.artwork[0]} {admin} {busy} look={l} onfail={markFailed} onremove={askRemove} ondone={() => void load()} />
{/snippet}
{#snippet inPlace(s: DownloadRow)}{@render looked(s, 'inline')}{/snippet}

{#if !look}
	<Sheet bind:open={detailOpen} title="Download details" size="md" placement="right">
		{#if selected}{@render detail(selected)}{/if}
	</Sheet>
{:else}
	<Sheet bind:open={detailOpen} title="Download details" size={look === 'drawer' ? 'lg' : 'sm'} placement={look === 'drawer' ? 'drawer' : look === 'panel' && !phone ? 'right' : 'auto'}>
		<!-- On phones the in-row look opens as the card's content in the bottom sheet. -->
		{#if selected}{@render looked(selected, look === 'inline' ? 'card' : look)}{/if}
	</Sheet>
{/if}

<svelte:window onkeydown={cardKey} onpointerdown={outside} onscroll={place} onresize={place} />
{#if holder === 'card' && cardId && selected}
	<div class="dcard" class:up={cardPos.up} bind:this={card} role="dialog" aria-label="Download details" tabindex="-1" style="top: {cardPos.top}px; left: {cardPos.left}px" data-testid="detail-card" in:popIn out:popOut>
		<div class="dcard-x"><Button variant="plain" size="sm" icon="x" aria-label="Close details" onclick={() => closeCard()}>Close</Button></div>
		{@render looked(selected, 'card')}
	</div>
{/if}

<VariantBar options={DETAILS} bind:value={details} param="details" storageKey="holyarrs.downloads.details" label="Download details layouts" />

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
	.dcard { position: fixed; z-index: 40; width: min(380px, calc(100vw - 32px)); max-height: min(70dvh, 560px); overflow: auto; padding: 14px 16px 16px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-modal), 0 0 0 0.5px var(--line-strong); transform-origin: top left; }
	.dcard.up { transform-origin: bottom left; }
	.dcard:focus { outline: none; }
	.dcard:focus-visible { box-shadow: var(--shadow-modal), var(--focus-ring); }
	.dcard-x { position: absolute; top: 8px; right: 8px; z-index: 1; }
	.dcard :global([data-testid='detail-title']) { padding-right: 72px; }
</style>
