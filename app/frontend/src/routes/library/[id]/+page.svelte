<script lang="ts">
	import type { StatusKind } from '#lib/ui/StatusPill.svelte';
	import { downloadTone } from '#lib/downloads/index.ts';
	import { createTitleEdit } from '#lib/mediafiles/index.ts';
	import { createTitleVersions } from '#lib/mediafiles/index.ts';
	import { createTitleChoose } from '#lib/mediafiles/index.ts';
	import { createTitleRemove } from '#lib/mediafiles/index.ts';
	import { createTitleSearch } from '#lib/mediafiles/index.ts';
	import '#lib/ui/title-page.css';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { TitleEmpty as EmptyState } from '#lib/mediafiles/index.ts';
	import TitleHero, { richMetadata, runtimeWords, shortFolder, type RichMeta } from '#lib/ui/TitleHero.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import FollowButton from '#lib/ui/FollowButton.svelte';
	import AskedByLine from '#lib/ui/AskedByLine.svelte';
	import { loadAskedBy, type AskedByAnswer } from '#lib/ui/asked-by.ts';
	import ActionMenu from '#lib/ui/ActionMenu.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import TitleCast from '#lib/ui/TitleCast.svelte';
	import TitleRows from '#lib/ui/TitleRows.svelte';
	import LibrarySection from '#lib/ui/LibrarySection.svelte';
	import { audienceOf, factsOf, titleStatus } from '#lib/ui/title-status.ts';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import { artistSheets, askShowSheet, quickAskSheet } from '#lib/requests/index.ts';
	import { slotNotes, facts, keyReasons, pager, verdict, verdictTone, RenamePreview, CopyControls, deleteSummary, artName, crumbFor, qualityBadges } from '#lib/mediafiles/index.ts';
	import { ReportSheet } from '#lib/requests/index.ts';
	import { percent, timeLeft, groupLabel, kindTone } from '#lib/downloads/index.ts';
	import { SubtitlePanel } from '#lib/subtitles/index.ts';
	import { ShowPage } from '#lib/shows/index.ts';
	import { ArtistPage } from '#lib/music/index.ts';
	import { TitlePage } from '#lib/discover/index.ts';
	import { page } from '$app/state';
	import { peopleNames } from '#lib/core/plain.ts';
	import { to } from '#lib/core/paths.ts';
	import { api, message, ApiError } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ago, bytes, date, dateTime, tone, words } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { decided } from '#lib/core/plain.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { scrollToStart } from '#lib/core/motion.ts';
	import type { EventPage, EventRecord, ReleasePage, ReleaseView, TitleView, VersionView } from '#lib/core/types.ts';

	const id = $derived(Number(page.params.id));
	const libraryBack = $derived.by(() => {
		const target = page.url.searchParams.get('returnTo');
		const library = to('/library');
		// With no way back given: the Library of this title's kind; a title that never loaded has none, so the kind used last.
		const own = title ? to(crumbFor(title.kind).path) : library;
		return target && (target === library || target.startsWith(`${library}?`)) ? target : own;
	});
	let title = $state<TitleView | null>(null);
	// A movie or show with a TMDB id opens the one title page (lib/discover TitlePage), the same page New & Popular, Explore and search open; its Library tab and More menu hold the manager tools. The page below stays only for the cases that page does not cover.
	const shared = (t: TitleView | null): t is TitleView & { ids: { tmdb: string } } => !!t && (t.kind === 'movie' || t.kind === 'show') && !!t.ids?.tmdb;

	// Who asked.
	let askedBy = $state<AskedByAnswer>({ titles: {} });
	$effect(() => {
		const id = title?.id;
		if (!id) return;
		loadAskedBy(api.get, [id]).then((x) => (askedBy = x), () => {});
	});
	const askers = $derived(title ? (askedBy.titles[String(title.id)]?.askers ?? []) : []);
	let history = $state<EventRecord[]>([]);
	let historyTotal = $state(0);
	let releases = $state<ReleaseView[]>([]);
	let relTotal = $state(0);
	let relOffset = $state(0);
	const relLimit = 25;
	let running = $state<number[]>([]);
	/** The Slot whose Releases are shown; 0 shows every Version. */
	let slotPick = $state(0);
	const pages = $derived(pager(relTotal, relLimit, relOffset));
	let notFound = $state(false);
	let blocked = $state(false);
	let error = $state('');
	let searching = $state(false);
	let saving = $state(false);
	let refreshing = $state(false);

	const editFlowCtx = {
		get title() { return title; },
		set title(v) { title = v; },
		get saving() { return saving; },
		set saving(v) { saving = v; },
		get id() { return id; },
	};
	const editFlow = createTitleEdit(editFlowCtx);
	// Versions are one row; See all lays every Version out.
	let allVersions = $state(false);
	const leadVersion = (vs: VersionView[]) => (vs.find((v) => v.main) ?? vs[0])?.id;
	const gates: [string, string][] = [
		['announced', 'As soon as it is announced'],
		['in_cinemas', 'Once it is in cinemas'],
		['released', 'Once it is out on digital or disc']
	];
	const stageWord: Record<string, string> = { announced: 'Announced', in_cinemas: 'In cinemas', released: 'Out on digital or disc' };
	const files = $derived(title ? title.slots.filter((s) => s.file) : []);

	const versionsFlowCtx = {
		get title() { return title; },
		get rules() { return editFlow.rules; },
		set rules(v) { editFlow.rules = v; },
		get id() { return id; },
		get allVersions() { return allVersions; },
		set allVersions(v) { allVersions = v; },
		load,
	};
	const versionsFlow = createTitleVersions(versionsFlowCtx);
	const chooseFlowCtx = {
		get slotPick() { return slotPick; },
		set slotPick(v) { slotPick = v; },
		get title() { return title; },
		load,
		get relOffset() { return relOffset; },
		set relOffset(v) { relOffset = v; },
	};
	const chooseFlow = createTitleChoose(chooseFlowCtx);
	function goPage(off: number | null) {
		if (off === null) return;
		relOffset = off;
		void load('releases');
	}

	async function load(which: 'all' | 'title' | 'releases' = 'all') {
		try {
			if (which !== 'releases') title = await api.get<TitleView>(`/titles/${id}`);
			// The shared title page, a show's or an artist's page reads its own parts and history.
			if (shared(title) || title?.kind === 'show' || title?.kind === 'artist') {
				error = '';
				return;
			}
			if (which !== 'releases' && session.can('manage_library')) {
				const h = await api.get<EventPage>(`/events?title=${id}&limit=8`);
				history = h.events;
				historyTotal = h.total;
			}
			if (which !== 'title' && session.can('manage_library')) {
				const q = `limit=${relLimit}&offset=${relOffset}${slotPick ? `&slot_id=${slotPick}` : ''}`;
				const p = await api.get<ReleasePage>(`/titles/${id}/releases?${q}`);
				releases = p.releases;
				relTotal = p.total;
				running = p.running_job_ids ?? [];
			}
			error = '';
		} catch (e) {
			// A Kid gets 404 for a Library title above their age rating, the same as a missing one.
			if (e instanceof ApiError && e.status === 404) {
				if (session.kid) blocked = true;
				else notFound = true;
			} else error = message(e);
		}
	}

	$effect(() => {
		if (!Number.isFinite(id)) return;
		void load();
		const offs = [
			live.on('title', () => load('title')),
			live.on('slot', () => load('title')),
			live.on('download', (d) => {
				const dl = d as { ID?: number; id?: number };
				const dlID = dl?.ID ?? dl?.id;
				if (dlID && title?.slots.some((s) => s.active_download_id === dlID)) void load('title');
			}),
			live.on('releases', () => load()),
			live.on('event', (d) => {
				const e = d as { TitleID?: number | null };
				if (e?.TitleID === id) void load('title');
			}),
			live.on('job', (d) => {
				const j = d as { id?: number; state?: string };
				if (j?.id && running.includes(j.id) && j.state !== 'running' && j.state !== 'queued') void load('releases');
			}),
			live.on('resync', () => load())
		];
		return () => offs.forEach((off) => off());
	});

	// Rename and delete.
	const removeFlowCtx = {
		get id() { return id; },
		get title() { return title; },
	};
	const removeFlow = createTitleRemove(removeFlowCtx);
	/** Maps a download state to the pill that says it. */
	function dlKind(state: string, active: boolean): StatusKind {
		if (active) return 'downloading';
		const t = downloadTone(state);
		return t === 'ok' ? 'in_library' : t === 'fail' ? 'failed' : t === 'idle' ? 'requested' : 'missing';
	}
	const searchFlowCtx = {
		load,
		get searching() { return searching; },
		set searching(v) { searching = v; },
		get slotPick() { return slotPick; },
		get id() { return id; },
		get running() { return running; },
		set running(v) { running = v; },
		get relOffset() { return relOffset; },
		set relOffset(v) { relOffset = v; },
		get refreshing() { return refreshing; },
		set refreshing(v) { refreshing = v; },
		get title() { return title; },
	};
	const searchFlow = createTitleSearch(searchFlowCtx);


	// The hero: breadcrumb, meta line, badges from the kept files, status and menus.
	const crumbs = $derived(
		title ? [{ href: to(crumbFor('movie').path), label: crumbFor('movie').label }, { href: to(`/library/${title.id}`), label: title.year ? `${title.name} (${title.year})` : title.name }] : []
	);
	// Richer art and facts from the metadata endpoint, when the backend has it.
	let rich = $state<RichMeta | null>(null);
	$effect(() => {
		const tmdb = title?.ids?.tmdb;
		rich = null;
		if (!tmdb || (title?.backdrop && title?.genres?.length)) return;
		let live = true;
		void richMetadata(api.get, 'movie', tmdb).then((m) => live && (rich = m));
		return () => (live = false);
	});
	const heroMeta = $derived(title ? [title.year ? String(title.year) : '', runtimeWords(title.runtime_minutes ?? rich?.runtime), ...(title.genres?.length ? title.genres : (rich?.genres ?? [])).slice(0, 3)].filter(Boolean) : []);
	const notes = $derived(title ? slotNotes(title.slots).map((n) => plainReason(n)) : []);
	const heroBadges = $derived(qualityBadges(files.map((s) => s.file)));
	// One status vocabulary (title-status.ts): staff or member words.
	const status = $derived(title ? titleStatus({ ...factsOf(title), titleKind: 'movie' }, audienceOf(session.can)) : { kind: 'missing' as const, label: 'Missing' });
	const manage = $derived(session.can('manage_library'));
	// Manager actions live in the hero's ⋯ menu; the Library section holds the rest.
	const heroItems = $derived<MenuItem[]>([
		{ label: 'Search now', icon: 'search', onselect: () => searchFlow.searchNow(), disabled: searching },
		{ label: 'Choose a release', icon: 'grid', onselect: searchFlow.chooseRelease, disabled: searching },
		{ label: 'Keep up to date', checked: !!title?.keep_up_to_date, disabled: saving, onselect: () => title && setKeep(!title.keep_up_to_date) },
		{ kind: 'separator' },
		{ label: 'Edit', icon: 'tools', onselect: editFlow.openEdit },
		{ label: 'Rename files', icon: 'copy', onselect: removeFlow.openRename },
		{ label: 'Add a Version', icon: 'plus', onselect: versionsFlow.openVersion },
		{ label: 'Refresh details', icon: 'refresh', onselect: searchFlow.refresh, disabled: refreshing },
		{ label: 'All history', icon: 'clock', href: to(`/downloads/history?title=${id}`) },
		{ kind: 'separator' },
		{ label: 'Delete movie', icon: 'trash', destructive: true, onselect: removeFlow.openDelete }
	]);
	// Report a problem: anyone, once the movie has a file; members get it as their only More item.
	let reportOpen = $state(false);
	const mayReport = $derived(!!title && files.length > 0);
	const moreItems = $derived<MenuItem[]>([...(manage ? heroItems : []), ...(mayReport ? [...(manage ? [{ kind: 'separator' as const }] : []), { label: 'Report a problem', icon: 'alert' as const, onselect: () => (reportOpen = true) }] : [])]);
	function toLibrary() {
		const h = document.getElementById('h-library');
		scrollToStart(h?.closest('section'));
	}

	async function setKeep(v: boolean) {
		if (!title) return;
		saving = true;
		try {
			const r = await api.put<{ title: TitleView }>(`/titles/${id}`, { keep_up_to_date: v });
			title = r.title;
			toasts.push(v ? 'Kept up to date' : 'Not kept up to date', 'ok');
		} catch (e) {
			toasts.push(`Couldn't update`, 'fail', { body: message(e) });
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>{#if !shared(title)}<title>{title?.name ?? 'Title'} · HolyArrs</title>{/if}</svelte:head>

{#if blocked}
	<EmptyState icon="shield" title="This title is not available on this account.">
		{#snippet actions()}<Button href={libraryBack}>Back to the Library</Button>{/snippet}
	</EmptyState>
{:else if notFound}
	<EmptyState icon="library" title="This title is not in your Library">
		<p>It may have been removed.</p>
		{#snippet actions()}<Button href={libraryBack}>Back to the Library</Button>{/snippet}
	</EmptyState>
{:else if error && !title}
	<LoadError hasData={false} message={error} onretry={() => load()} />
{:else}
<PageLoader loading={!title} label="Loading the title" onretry={() => load()}>
{#if title}
{#if shared(title)}
	{#key title.id}<TitlePage kind={title.kind} tmdb={String(title.ids.tmdb)} libraryId={title.id} backHref={page.url.searchParams.has('returnTo') ? libraryBack : undefined} />{/key}
{:else if title.kind === 'show'}
	<ShowPage askSheet={askShowSheet} quickAsk={quickAskSheet} {title} onchange={(t) => (title = t)} {error} onretry={() => load('title')} />
{:else if title.kind === 'artist'}
	<ArtistPage sheets={artistSheets} {title} onchange={(t) => (title = t)} {error} onretry={() => load('title')} />
{:else}
	<div class="tp">
		<TitleHero
			title={title.name}
			kind="movie"
			tmdb={title.ids?.tmdb}
			poster={title.artwork[0] ?? rich?.poster}
			backdrop={title.backdrop ?? rich?.backdrop}
			backdrops={rich?.backdrops}
			logo={title.logo ?? rich?.logo}
			breadcrumb={crumbs}
			meta={heroMeta}
			certification={title.rating ?? rich?.certification}
			score={rich?.score}
			badges={heroBadges}
			status={status.kind}
			statusLabel={status.label}
			description={title.overview || rich?.overview}
			vtName={artName(title.id)}
		>
			{#snippet byline()}<AskedByLine {askers} />{/snippet}
			{#snippet actions()}
				{#if manage}
					<ActionButton icon="library" label="In Library: go to the Library section" caption="In Library" primary onclick={toLibrary} data-testid="hero-library" />
					<ActionMenu label="More actions" caption="More" icon="more" items={moreItems} />
				{:else if mayReport}
					<ActionMenu label="More actions" caption="More" icon="more" items={moreItems} />
				{/if}
				<FollowButton titleId={title!.id} kind="movie" />
			{/snippet}
		</TitleHero>
		{#if mayReport && title}<ReportSheet bind:open={reportOpen} {title} />{/if}

		{#if title.slots.some((s) => s.download)}
			<div class="tp-dls">
				{#each title.slots.filter((s) => s.download) as s (s.id)}
					{@const d = s.download!}
					<div class="tp-dl" data-testid="download">
						<StatusPill kind={dlKind(d.state, d.active)} testid="download-state" label="{s.version && title!.slots.length > 1 ? `${s.version}: ` : ''}{d.label}" />
						{#if d.active}<div class="tp-bar"><ProgressBar value={d.progress * 100} label="Download progress" /></div>
							<span class="small muted">{percent(d.progress)}{#if d.speed > 0} · {bytes(d.speed)}/s{/if}{#if timeLeft(d.eta)} · {timeLeft(d.eta)}{/if}</span>{/if}
						<p class="tp-dl-text" data-testid="download-text">{d.text}</p>
						{#if d.active && manage}
							<Button class="tp-btn" size="sm" variant="plain" onclick={() => searchFlow.markFailed(d.id)} busy={searchFlow.failing === d.id}>Mark failed</Button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if error}<LoadError hasData message={error} onretry={() => load()} />{/if}

		<TitleRows kind="movie" tmdb={title.ids?.tmdb} quickAsk={quickAskSheet}>
			{#snippet middle()}{#key title!.id}<TitleCast titleId={title!.id} />{/key}{/snippet}
		</TitleRows>

		<TitleSection id="h-about" title="About">
			<dl class="tp-about">
				<dt>Now</dt>
				<dd>{stageWord[title.stage] ?? words(title.stage)}</dd>
				{#if title.year}<dt>Released</dt><dd>{title.year}</dd>{/if}
				{#if title.cinema_date}<dt>In cinemas</dt><dd>{date(title.cinema_date)}</dd>{/if}
				{#if title.digital_date}<dt>Digital</dt><dd>{date(title.digital_date)}</dd>{/if}
				{#if title.physical_date}<dt>Disc</dt><dd>{date(title.physical_date)}</dd>{/if}
				{#if title.runtime_minutes ?? rich?.runtime}<dt>Runtime</dt><dd>{runtimeWords(title.runtime_minutes ?? rich?.runtime)}</dd>{/if}
				{#if title.rating ?? rich?.certification}<dt>Rated</dt><dd>{title.rating ?? rich?.certification}</dd>{/if}
				<dt>Original language</dt>
				<dd>{title.original_language ? title.original_language.toUpperCase() : 'Not known'}</dd>
				{#if title.genres?.length}<dt>Genres</dt><dd>{title.genres.join(', ')}</dd>{/if}
			</dl>
		</TitleSection>

		{#if title.asks?.length}
			<TitleSection id="h-asks" title="Requests" testid="title-asks">
				{#each title.asks as a (a.id)}
					<dl class="facts">
						<dt>Asked by</dt>
						<dd>{peopleNames(a.people.map((p) => p.name))} · {date(a.asked_at)}</dd>
						{#if decided(a)}<dt>Decided</dt>
							<dd>{decided(a)}{a.decided_at ? ` · ${date(a.decided_at)}` : ''}</dd>{/if}
						<dt>Now</dt>
						<dd>{a.state_label}</dd>
						{#if a.note}<dt>Note</dt>
							<dd>{a.note}</dd>{/if}
					</dl>
				{/each}
			</TitleSection>
		{/if}

		{#if manage}
			<LibrarySection>
				<TitleSection id="h-versions" title="Versions and files" primary>
					{#snippet head()}
						<div class="tp-head-end">
							{#if title!.versions.length > 1}
								<Button variant="plain" aria-expanded={allVersions} aria-controls="movie-versions" onclick={() => (allVersions = !allVersions)}>{allVersions ? 'Show less' : `See all ${title!.versions.length}`}</Button>
							{/if}
							<Button icon="search" onclick={() => searchFlow.searchNow()} disabled={searching} busy={searching} data-testid="search-now">Search now</Button>
							<Button icon="plus" onclick={versionsFlow.openVersion}>Add a Version</Button>
						</div>
					{/snippet}
					<ul class="tp-list tp-vrow" id="movie-versions" aria-label="Versions">
						{#each title.versions as v (v.id)}
							{@const s = versionsFlow.slotOf(v)}
							{@const f = s?.file}
							<li class="tp-row" data-testid="version" hidden={!allVersions && v.id !== leadVersion(title.versions)}>
								<div class="tp-row-main">
									<b>{v.label}{v.main ? ' (main)' : ''}</b>
									<span class="xs muted">Edition: {v.edition || 'Not specified'} · Audio required: {v.audio_languages?.join(', ') || 'Any'}</span>
									<span class="xs muted">Pick rules: {v.pick_rules_name}{v.keep_up_to_date ? '' : ' · not kept up to date'}</span>
									{#if f}
										<span class="xs muted">{bytes(f.Size)}{#if f.Quality?.resolution} · {f.Quality.resolution}p{f.Quality.source ? ` ${words(String(f.Quality.source))}` : ''}{/if} · Added {date(f.AddedAt)}</span>
									{:else}
										<span class="xs muted">No file yet.</span>
									{/if}
									{#if f || v.folder_path}
										<details class="tp-details">
											<summary>Details</summary>
											<ul>
												{#if f}<li>File: {shortFolder(f.RelativePath)}</li>{/if}
												{#if f?.ReleaseName}<li>From the release {f.ReleaseName}</li>{/if}
												{#if v.folder_path}<li>Folder: {shortFolder(v.folder_path)}</li>{/if}
											</ul>
										</details>
									{/if}
								</div>
								<div class="tp-row-end">
									{#if s}<span class="chip {tone(s.state)}">{s.state_label}</span>{/if}
									{#if !v.main}
										<Menu label="Version actions for {v.label}" items={[{ label: `Remove ${v.label}`, icon: 'trash', destructive: true, onselect: () => versionsFlow.removeVersion(v) }]} width={220} />
									{/if}
								</div>
							</li>
						{/each}
					</ul>
					{#if notes.length}
						<details class="tp-details" data-testid="slot-notes">
							<summary>Details</summary>
							<ul>{#each notes as n (n.text + (n.detail ?? ''))}<li>{n.text}</li>{/each}</ul>
						</details>
					{/if}
				</TitleSection>

				<CopyControls {title} onchange={() => load('title')} />

				<TitleSection id="h-rel" title="Releases" testid="releases" focusable>
					{#snippet head()}
						{#if relTotal}<span class="xs muted">{relTotal} found in the last search</span>{/if}
					{/snippet}
					{#if title.slots.length > 1}
						<div class="field slot-pick">
							<label for="slot-pick">Version</label>
							<select id="slot-pick" class="input" value={slotPick} onchange={(e) => chooseFlow.pickSlot(Number(e.currentTarget.value))}>
								<option value={0}>Every Version</option>
								{#each title.slots as s (s.id)}<option value={s.id}>{s.version || 'Main'}</option>{/each}
							</select>
							<span class="hint">Search now and Choose a release then look for this Version only.</span>
						</div>
					{/if}
					<p class="small muted" aria-live="polite" data-testid="search-running">
						{running.length ? 'Searching your Sources. The list fills in once they answer.' : ''}
					</p>
					{#if releases.length === 0}
						{#if !running.length}
							<p class="small muted">No search yet. Search to see every release and why it would be picked or not.</p>
							<Button class="tp-start tp-btn" icon="grid" onclick={() => searchFlow.searchNow(false)} disabled={searching}>Choose a release</Button>
						{/if}
					{:else}
						<ul class="rels">
							{#each releases as r (r.id)}
								{@const key = keyReasons(r.reasons)}
								<li class="rel" data-testid="release">
									<div class="rel-top">
										<span class="rel-name mono">{r.name}</span>
										<span class="chip {verdictTone(r)}" data-testid="verdict">{verdict(r)}</span>
									</div>
									<div class="xs muted">{facts(r, bytes)}{title.slots.length > 1 && r.version ? ` · ${r.version}` : ''}</div>
									{#if key.length}
										<ul class="reasons">
											{#each key as x, i (i)}<li class={x.effect}>{x.text}</li>{/each}
										</ul>
									{/if}
									{#if r.reasons.length}
										<details>
											<summary class="xs muted">All {r.reasons.length} reasons</summary>
											<ul class="reasons">
												{#each r.reasons as x, i (i)}<li class={x.effect}>{x.text}</li>{/each}
											</ul>
										</details>
									{/if}
									{#if r.state !== 'chosen'}
										<div class="row rel-actions">
											<Button onclick={() => chooseFlow.askChoose(r)} aria-label="Choose {r.name}">Choose</Button>
											{#if r.state !== 'turned_down'}
												<Button variant="plain" onclick={() => chooseFlow.askTurnDown(r)} aria-label="Turn down {r.name}">Turn down</Button>
											{/if}
										</div>
									{/if}
								</li>
							{/each}
						</ul>
						{#if pages.prev !== null || pages.next !== null}
							<nav class="row pager" aria-label="Release pages">
								<Button variant="plain" disabled={pages.prev === null} onclick={() => goPage(pages.prev)}>Back</Button>
								<span class="xs muted">{pages.from} to {pages.to} of {relTotal}</span>
								<Button variant="plain" disabled={pages.next === null} onclick={() => goPage(pages.next)}>More</Button>
							</nav>
						{/if}
					{/if}
				</TitleSection>

				<div id="title-subtitles">{#key title.id}<SubtitlePanel titleId={title.id} movie />{/key}</div>

				<TitleSection id="h-dates" title="When HolyArrs looks">
					<dl class="tp-about">
						<dt>Release gate</dt>
						<dd>{words(title.release_gate)}{title.gate_delay_days ? ` + ${title.gate_delay_days} days` : ''}</dd>
						{#if title.gate_opens_at}<dt>Gate opens</dt>
							<dd>{date(title.gate_opens_at)}</dd>{/if}
						{#if title.pick_rules_name}<dt>Pick rules</dt>
							<dd>{title.pick_rules_name}</dd>{/if}
						<dt>Keep up to date</dt>
						<dd>{title.keep_up_to_date ? 'On' : 'Off'}</dd>
						{#if title.refreshed_at}<dt>Refreshed</dt>
							<dd>{date(title.refreshed_at)}</dd>{/if}
					</dl>
					<p class="tp-why" data-testid="gate-reason">{title.gate_reason}</p>
				</TitleSection>

				<TitleSection id="h-history" title="History" testid="title-history">
					{#snippet head()}
						{#if historyTotal > 0}<Button class="tp-btn" variant="plain" size="sm" href={to(`/downloads/history?title=${id}`)}>All {historyTotal} rows</Button>{/if}
					{/snippet}
					{#if history.length === 0}
						<p class="small muted">Nothing yet.</p>
					{:else}
						<ol class="rows hist">
							{#each history as e (e.ID)}
								<li>
									<span class="chip {kindTone(e.Kind)}">{groupLabel(e.Kind)}</span>
									<span class="grow small">{e.Summary}</span>
									<time class="mono xs muted" datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
								</li>
							{/each}
						</ol>
					{/if}
				</TitleSection>
			</LibrarySection>
		{/if}
	</div>
{/if}
{/if}
</PageLoader>
{/if}

<dialog bind:this={chooseFlow.chooseDialog} aria-labelledby="choose-title" class="modal">
	{#if chooseFlow.target}
		<h2 id="choose-title" class="h2">{chooseFlow.needsForce ? 'Choose it anyway?' : 'Choose this Release?'}</h2>
		<p class="mono small path" style="margin-top: var(--s-3)">{chooseFlow.target.name}</p>
		<p class="small muted">{facts(chooseFlow.target, bytes)}</p>
		{#if chooseFlow.warnings.length || chooseFlow.forceReasons.length}
			<ul class="warns" data-testid="choose-warnings">
				{#each chooseFlow.warnings as w (w)}<li>{w}</li>{/each}
				{#each chooseFlow.forceReasons as w (w)}<li>It did not pass: {w}.</li>{/each}
			</ul>
		{/if}
		{#if chooseFlow.chooseError}<p class="form-error" role="alert">{chooseFlow.chooseError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => chooseFlow.chooseDialog?.close()}>Cancel</Button>
			<Button variant="primary" onclick={chooseFlow.choose} disabled={chooseFlow.choosing} busy={chooseFlow.choosing}>{chooseFlow.needsForce ? 'Choose anyway' : 'Choose'}</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={chooseFlow.turnDialog} aria-labelledby="turn-title" class="modal">
	{#if chooseFlow.target}
		<form onsubmit={chooseFlow.turnDown}>
			<h2 id="turn-title" class="h2">Turn down this Release?</h2>
			<p class="mono small path" style="margin-top: var(--s-3)">{chooseFlow.target.name}</p>
			<p class="small muted">It won’t be picked again for this movie. You can undo this later.</p>
			<div class="field" style="margin-top: var(--s-4)">
				<label for="turn-reason">Why (optional)</label>
				<input id="turn-reason" class="input" bind:value={chooseFlow.turnReason} maxlength="500" />
			</div>
			{#if chooseFlow.chooseError}<p class="form-error" role="alert">{chooseFlow.chooseError}</p>{/if}
			<div class="row modal-actions">
				<Button variant="plain" onclick={() => chooseFlow.turnDialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={chooseFlow.choosing} busy={chooseFlow.choosing}>Turn down</Button>
			</div>
		</form>
	{/if}
</dialog>

<dialog bind:this={editFlow.editDialog} aria-labelledby="edit-title" class="modal" onclose={() => (editFlow.editing = false)}>
	{#if title && editFlow.editing}
		<form onsubmit={editFlow.saveEdit}>
			<h2 id="edit-title" class="h2">Edit {title.name}</h2>
			<div class="field" style="margin-top: var(--s-5)">
				<label for="e-rules">Pick rules{title.versions.length > 1 ? ' (main Version)' : ''}</label>
				<select id="e-rules" class="input" bind:value={editFlow.eRules}>
					{#each editFlow.rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="e-gate">Release gate</label>
				<select id="e-gate" class="input" bind:value={editFlow.eGate}>
					{#each gates as [g, label] (g)}<option value={g}>{label}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="e-delay">Wait extra days</label>
				<input id="e-delay" class="input" type="number" min="0" max="365" bind:value={editFlow.eDelay} />
				<span class="hint">Days to wait after the gate date before looking.</span>
			</div>
			<label class="checkline">
				<input type="checkbox" bind:checked={editFlow.eKeep} />
				<span><b>Keep up to date</b><br /><span class="hint">HolyArrs looks for it and for a better copy later.</span></span>
			</label>
			{#if editFlow.editError}<p class="form-error" role="alert">{editFlow.editError}</p>{/if}
			<div class="row modal-actions">
				<Button variant="plain" onclick={() => editFlow.editDialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={saving} busy={saving}>Save</Button>
			</div>
		</form>
	{/if}
</dialog>

<dialog bind:this={removeFlow.renameDialog} aria-labelledby="rename-title" class="modal wide" onclose={() => (removeFlow.renameOpen = false)}>
	{#if title && removeFlow.renameOpen}
		<h2 id="rename-title" class="h2">Rename the files of {title.name}</h2>
		<p class="small muted">Nothing changes until you press Rename.</p>
		<RenamePreview titleId={id} onDone={() => load('title')} />
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => removeFlow.renameDialog?.close()}>Close</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={removeFlow.deleteDialog} aria-labelledby="delete-title" class="modal wide">
	{#if title}
		<h2 id="delete-title" class="h2">Delete {title.name}?</h2>
		<p class="small muted">Nothing has changed yet.</p>
		<label class="checkline">
			<input type="checkbox" bind:checked={removeFlow.delFiles} onchange={removeFlow.planDelete} />
			<span
				><b>Also delete its files</b><br /><span class="hint"
					>Files HolyArrs imported or adopted move to the holding folder in their Library folder, and are removed for good after the holding days (7 unless you
					changed it in Settings). Files it only found are never touched. Off: every file stays where it is.</span
				></span
			>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={removeFlow.delNever} onchange={removeFlow.planDelete} />
			<span
				><b>Add it to the Never-add list</b><br /><span class="hint">Outside lists can then never add it again. You can still add it yourself.</span></span
			>
		</label>
		<section class="plan" aria-labelledby="plan-title" aria-live="polite" data-testid="delete-plan">
			<h3 id="plan-title" class="h3">What will happen</h3>
			{#if !removeFlow.delPlan}
				<div class="sk" style="height: 60px"></div>
			{:else}
				<ul class="warns">
					{#each deleteSummary(removeFlow.delPlan) as line (line)}<li>{line}</li>{/each}
				</ul>
				{#if removeFlow.delPlan.held.length}
					<p class="xs muted">To the holding folder:</p>
					<ul class="files">
						{#each removeFlow.delPlan.held as f (f.file_id)}<li class="mono xs path">{f.path}</li>{/each}
					</ul>
				{/if}
				{#if removeFlow.delPlan.kept.length}
					<p class="xs muted">Staying where they are:</p>
					<ul class="files">
						{#each removeFlow.delPlan.kept as f (f.file_id)}<li class="xs"><span class="mono path">{f.path}</span> <span class="muted">({f.why})</span></li>{/each}
					</ul>
				{/if}
			{/if}
		</section>
		{#if removeFlow.delError}<p class="form-error" role="alert">{removeFlow.delError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => removeFlow.deleteDialog?.close()}>Cancel</Button>
			<Button variant="destructive" onclick={removeFlow.doDelete} disabled={removeFlow.delBusy || !removeFlow.delPlan} busy={removeFlow.delBusy}>Delete movie</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={versionsFlow.versionDialog} aria-labelledby="version-title" class="modal">
	{#if title}
		<form onsubmit={versionsFlow.addVersion}>
			<h2 id="version-title" class="h2">Add a Version of {title.name}</h2>
			<p class="small muted">Another copy, like 4K, with its own Pick rules and folder.</p>
			<div class="field" style="margin-top: var(--s-5)">
				<label for="v-label">Version name</label>
				<input id="v-label" class="input" bind:value={versionsFlow.vLabel} maxlength="40" required />
			</div>
			<div class="field">
                <label for="v-edition">Movie edition</label>
                <input id="v-edition" class="input" bind:value={versionsFlow.vEdition} maxlength="40" placeholder="For example, Theatrical or Directors Cut" />
                <span class="hint">Leave empty for the existing quality-only choice. An unknown edition needs review before it can fill an edition choice.</span>
            </div>
            <div class="field">
                <label for="v-audio">Required audio languages</label>
                <input id="v-audio" class="input" bind:value={versionsFlow.vAudio} placeholder="For example, en, ja" />
                <span class="hint">Audio must have every language. Subtitles don’t count.</span>
            </div>
            <div class="field">
                <label for="v-existing-folder">Existing movie folder (optional)</label>
                <input id="v-existing-folder" class="input" bind:value={versionsFlow.vFolderName} maxlength="200" />
                <span class="hint">To use a copy you have, name its folder. Leave empty for a new one.</span>
            </div>
            <div class="field">
                <label for="v-rules">Pick rules for this Version</label>
				<select id="v-rules" class="input" bind:value={versionsFlow.vRules}>
					{#each editFlow.rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="v-folder">Library folder for this Version</label>
				<select id="v-folder" class="input" bind:value={versionsFlow.vFolder}>
					{#each versionsFlow.movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
			</div>
			{#if versionsFlow.vError}<p class="form-error" role="alert">{versionsFlow.vError}</p>{/if}
			<div class="row modal-actions">
				<Button variant="plain" onclick={() => versionsFlow.versionDialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={versionsFlow.vBusy || !versionsFlow.vLabel.trim()} busy={versionsFlow.vBusy}>Add version</Button>
			</div>
		</form>
	{/if}
</dialog>

<style>
	.path { overflow-wrap: anywhere; }
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal.wide { width: min(640px, calc(100vw - 32px)); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	.plan { margin-top: var(--s-5); }
	.files { margin: var(--s-1) 0 var(--s-2); padding-left: var(--s-5); }
	.rel { padding: var(--s-4) 0; }
	.rel + .rel { border-top: 1px solid var(--line); }
	.rel-top { display: flex; justify-content: space-between; gap: var(--s-3); align-items: flex-start; margin-bottom: 4px; }
	.rel-name { font-size: var(--text-sm); overflow-wrap: anywhere; min-width: 0; }
	.reasons { margin-top: var(--s-2); font-size: var(--text-xs); color: var(--text-muted); }
	.reasons li { padding: 2px 0 2px var(--s-3); border-left: 2px solid var(--line-strong); }
	.reasons li.fail { border-left-color: var(--fail); }
	.reasons li.hold, .reasons li.warn { border-left-color: var(--wait); }
	.rel-actions { margin-top: var(--s-2); gap: var(--s-2); }
	.pager { justify-content: space-between; margin-top: var(--s-3); }
	.slot-pick { max-width: 280px; }
	.warns { margin-top: var(--s-4); padding: var(--s-3); border-left: 3px solid var(--wait); background: var(--surface-2, var(--surface-1)); border-radius: var(--r-sm); font-size: var(--text-sm); }
	.warns li + li { margin-top: var(--s-2); }
	details { margin-top: var(--s-2); }
	summary { cursor: pointer; min-height: 28px; display: inline-flex; align-items: center; }
	.hist li { flex-wrap: wrap; gap: var(--s-2) var(--s-3); }
	.hist .grow { min-width: 0; overflow-wrap: anywhere; }
</style>
