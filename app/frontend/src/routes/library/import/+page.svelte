<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import { densityLabels, loadDensity, loadPosterSize, posterLabels, saveDensity, savePosterSize, type Density, type PosterSize } from '#lib/ui/library-art.ts';
	import { reaskWhileFilling } from '#lib/music/index.ts';
	import { DlFind } from '#lib/downloads/index.ts';
	import { ImportList, ImportToolbar, LibraryToolPage, PosterLightbox, choices, fileName, findFolders, followJob, scanCount, matchKey, canTick, pickedOf, readImportView, showIdOf, sortFolders, startPicks, statusCounts, statusLabel, tickCounts, type AdoptCandidate, type AdoptPlan, type AdoptResult, type ImportFilter, type ImportView, type JobView, type Named } from '#lib/mediafiles/index.ts';
	import type { LookupArtist } from '#lib/music/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { views as savedViews } from '#lib/core/views.svelte.ts';
	import type { LibraryFolder, LookupMovie } from '#lib/core/types.ts';
	import { unit } from '#lib/mediafiles/index.ts';
	import type { LookupShow } from '#lib/core/types.ts';
	import { appLocale, t } from '#lib/core/i18n/i18n.svelte.ts';

	let folders = $state<LibraryFolder[]>([]);
	let loaded = $state(false);
	let folderId = $state(0);
	let error = $state('');

	let scanning = $state(false);
	let job = $state<JobView | null>(null);
	let plan = $state<AdoptPlan | null>(null);
	/** The movie (TMDB id) or show ("<source>:<id>") chosen for each folder. */
	let sel = $state<Record<string, string>>({});
	/** Movies, shows or artists picked by a lookup, by id, to show what was chosen. */
	let named = $state<Record<string, Named>>({});
	let ticked = $state<Record<string, boolean>>({});
	let adopting = $state(false);
	// Follow what is brought in: it goes on my calendar (ticked by default).
	let followAll = $state(true);
	let results = $state<AdoptResult[] | null>(null);
	// A large folder scanned in parts: where the next scan starts, kept after adopting.
	let restFrom = $state('');

	let lookFor = $state<string | null>(null);
	let term = $state('');
	let found = $state<{ key: string; name: string; year?: number; poster?: string; title_id?: number }[]>([]);
	let looking = $state(false);
	let lookError = $state('');

	/** The kind of the picked Library folder: movies, shows or music. */
	const kind = $derived<'movie' | 'show' | 'music'>(plan?.kind ?? folderKind(folders.find((f) => f.id === folderId)?.kind));
	const word = $derived(kind === 'show' ? 'show' : kind === 'music' ? 'artist' : 'movie');
	const w = $derived(t(`library.import.word.${word}`));
	const ws = $derived(t(`library.import.words.${word}`));
	const lookFolder = $derived(lookFor === null ? undefined : plan?.folders.find((f) => f.folder === lookFor));
	const picks = $derived(choices(Object.fromEntries(Object.entries(ticked).filter(([, on]) => on).map(([f]) => [f, sel[f] ?? ''])), kind));
	// A scan's note says how many folders are read: shown in the person's words.
	const scanned = $derived(scanning ? scanCount(job?.ProgressNote) : null);
	const running = $derived(!!job && !['done', 'failed', 'cancelled'].includes(job.State));
	const okCount = $derived(results ? results.filter((r) => r.title_id).length : 0);

	// The scanned folders: unsure ones first, narrowed by the search words and the status filter.
	let q = $state('');
	let filter = $state<ImportFilter>('all');
	let findOpen = $state(false);
	let zoom = $state<AdoptCandidate | null>(null);
	const sorted = $derived(plan ? sortFolders(plan.folders, appLocale()) : []);
	const shown = $derived(findFolders(sorted, q, filter));
	const tickNow = $derived(tickCounts(shown, sel, ticked));
	const allTicked = $derived(tickNow.can > 0 && tickNow.on === tickNow.can);
	function tickShown() {
		const on = !allTicked;
		for (const f of shown) if (canTick(f, sel)) ticked[f.folder] = on;
	}
	const filters = $derived<{ value: ImportFilter; label: string; n: number }[]>([
		{ value: 'all', label: t('mediafiles.filter.all'), n: plan?.folders.length ?? 0 },
		...statusCounts(plan?.folders ?? []).map((c) => ({ value: c.status, label: statusLabel(c.status, kind), n: c.n }))
	]);
	// A filter whose folders are all gone falls back to All.
	$effect(() => {
		if (filter !== 'all' && !filters.some((c) => c.value === filter)) filter = 'all';
	});

	// The view and art size are this page's own, kept per person the way the Library keeps its own.
	const myId = $derived(session.s.id);
	let view = $state<ImportView>(readImportView(savedViews.get('library.import.view')));
	let posterSize = $state<PosterSize>('m');
	let density = $state<Density>('large');
	let planWidth = $state(1000);
	const phone = $derived(planWidth < 640);
	$effect(() => {
		posterSize = loadPosterSize(myId, undefined, 'import');
		density = loadDensity(myId, undefined, 'import');
	});
	function setView(v: ImportView) {
		view = v;
		savedViews.set('library.import.view', v);
	}
	const sizeItems = $derived<MenuItem[] | null>(
		phone || (view !== 'posters' && view !== 'banners')
			? null
			: view === 'posters'
				? [{ kind: 'heading', label: t('library.page.posterSize') }, ...(['s', 'm', 'l'] as const).map((z) => ({ label: posterLabels[z], checked: posterSize === z, onselect: () => { posterSize = z; savePosterSize(myId, z, undefined, 'import'); } }))]
				: [{ kind: 'heading', label: t('library.page.bannerSize') }, ...(['large', 'comfortable', 'compact'] as const).map((d) => ({ label: densityLabels[d], checked: density === d, onselect: () => { density = d; saveDensity(myId, d, undefined, 'import'); } }))]
	);

	function folderKind(k: string | undefined): 'movie' | 'show' | 'music' {
		return k === 'show' || k === 'music' ? k : 'movie';
	}

	let foldersFailed = $state(false);
	async function loadFolders() {
		foldersFailed = false;
		try {
			folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders.filter(
				(f) => f.kind === 'movie' || f.kind === 'show' || f.kind === 'music'
			);
			folderId = folders[0]?.id ?? 0;
		} catch {
			foldersFailed = true;
		} finally {
			loaded = true;
		}
	}
	onMount(loadFolders);

	function follow(id: number) {
		return followJob(
			(x) => api.get<JobView>(`/jobs/${x}`),
			id,
			(j) => (job = j),
			{
				poke: (wake) =>
					live.on('job', (d) => {
						if ((d as { id?: number })?.id === id) wake();
					})
			}
		);
	}

	async function scan(after = '') {
		if (!folderId) return;
		scanning = true;
		error = '';
		plan = null;
		results = null;
		restFrom = '';
		q = '';
		filter = 'all';
		try {
			const r = await api.post<{ job_id: number }>(`/library-folders/${folderId}/scan`, after ? { after } : undefined);
			const end = await follow(r.job_id);
			// A stopped music scan keeps the artist folders it finished.
			const kept = end.State === 'cancelled' ? (end.Result?.plan as AdoptPlan | undefined) : undefined;
			if (end.State !== 'done' && !kept?.partial) throw new Error(end.Error?.message || t('library.import.scanUnfinished'));
			const p = (kept ?? end.Result?.plan) as AdoptPlan;
			plan = p;
			const start = startPicks(p.folders);
			const s: Record<string, string> = {};
			for (const f of p.folders) if (f.match) s[f.folder] = matchKey(f.match);
			sel = s;
			named = {};
			ticked = Object.fromEntries(Object.keys(start).map((f) => [f, true]));
		} catch (e) {
			error = message(e);
		} finally {
			scanning = false;
			job = null;
		}
	}

	function openLookup(f: AdoptCandidate) {
		lookFor = f.folder;
		term = f.parsed_name + (f.parsed_year ? ` ${f.parsed_year}` : '');
		found = [];
		lookError = '';
	}

	async function lookup(e: SubmitEvent) {
		e.preventDefault();
		if (!term.trim()) return;
		looking = true;
		lookError = '';
		try {
			if (kind === 'music') {
				const ask = () => api.get<{ artists: LookupArtist[]; pending?: boolean }>(`/artists/lookup?term=${encodeURIComponent(term.trim())}`);
				// A name never searched before answers "pending" while the search runs: ask again quietly.
				const artists = (await reaskWhileFilling(await ask(), ask, () => {})).artists;
				found = artists.map((a) => ({ key: `mbid:${a.mbid}`, name: a.disambiguation ? `${a.name} (${a.disambiguation})` : a.name, poster: a.image, title_id: a.title_id }));
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'mbid:<id>' });
			} else if (kind === 'show') {
				const shows = (await api.get<{ shows: LookupShow[] }>(`/shows/lookup?term=${encodeURIComponent(term.trim())}`)).shows;
				found = shows.map((s) => ({ key: showIdOf(s), name: s.name, year: s.year, poster: s.poster, title_id: s.title_id })).filter((s) => s.key);
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'tvdb:<id>' });
			} else {
				const m = term.trim().match(/^(.*?)\s+\(?((?:19|20)\d\d)\)?$/);
				const ask = m ? `term=${encodeURIComponent(m[1])}&year=${m[2]}` : `term=${encodeURIComponent(term.trim())}`;
				const movies = (await api.get<{ movies: LookupMovie[] }>(`/titles/lookup?${ask}`)).movies;
				found = movies.map((x) => ({ key: x.ids.tmdb ?? '', name: x.name, year: x.year, poster: x.poster, title_id: x.title_id })).filter((x) => x.key);
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'tmdb:<id>' });
			}
		} catch (err) {
			lookError = message(err);
		} finally {
			looking = false;
		}
	}

	function use(folder: string, m: { key: string; name: string; year?: number; poster?: string }) {
		const id = m.key;
		if (!id) return;
		sel[folder] = id;
		named[id] = { name: m.name, year: m.year, poster: m.poster };
		ticked[folder] = true;
		lookFor = null;
	}

	async function adopt() {
		if (!plan || !picks.length) return;
		adopting = true;
		error = '';
		try {
			const r = await api.post<{ job_id: number }>(`/library-folders/${plan.library_folder_id}/adopt`, { folders: picks, follow: followAll });
			const end = await follow(r.job_id);
			if (end.State !== 'done') throw new Error(end.Error?.message || t('library.import.adoptUnfinished'));
			results = (end.Result?.results as AdoptResult[]) ?? [];
			const n = results.filter((x) => x.title_id).length;
			toasts.push(t(n === 1 ? 'library.import.adoptedOne' : 'library.import.adoptedMany', { n, word: w, words: ws }), n ? 'ok' : 'info', { body: t('library.import.noFileMoved') });
			restFrom = plan?.more ? (plan.next ?? '') : '';
			plan = null;
		} catch (e) {
			error = message(e);
		} finally {
			adopting = false;
			job = null;
		}
	}
</script>

<svelte:head><title>{t('library.import.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={t('library.import.title')}>

<p class="tp-msg good" data-testid="nothing-moved">
	<Icon name="shield" size={18} /><span>{t('library.import.stay')}</span>
</p>

<PageLoader loading={!loaded} label={t('library.import.loadingFolders')}>
{#if foldersFailed}
	<LoadError hasData={false} message={t('library.import.foldersFailed')} onretry={loadFolders} />
{:else if !folders.length}
	<EmptyState icon="folder" title={t('library.import.noFolder')} text={t('library.import.noFolderHint')} action={session.can('settings') ? { label: t('library.import.libraryFolders'), href: to('/settings/library-folders') } : undefined} />
{:else}
	<section class="tp-sheet tp-pad" aria-labelledby="h-pick">
		<h2 id="h-pick" class="h2">{t('library.byHand.step1')}</h2>
		<div class="row pick">
			<div class="field grow">
				<label for="lf">{t('library.import.libraryFolder')}</label>
				<select id="lf" class="input" bind:value={folderId} disabled={scanning || adopting}>
					{#each folders as f (f.id)}<option value={f.id}
							>{f.kind === 'show' ? t('library.import.showsPrefix') : f.kind === 'music' ? t('library.import.musicPrefix') : ''}{f.label ? `${f.label} (${f.path})` : f.path}</option
						>{/each}
				</select>
			</div>
			<Button variant="primary" icon="search" onclick={() => scan()} disabled={adopting || !folderId} busy={scanning}>{t('library.byHand.scan')}</Button>
		</div>
		{#if running}
			<div aria-live="polite">
				<div class="bar"><ProgressBar value={(job?.Progress ?? 0) * 100} label={t('library.import.progress')} /></div>
				<p class="xs muted">{scanned ? t('library.import.readCount', { done: scanned.done.toLocaleString(appLocale()), total: scanned.total.toLocaleString(appLocale()) }) : job?.ProgressNote || (scanning ? t('library.import.reading', { word: w }) : t('library.import.adopting'))}</p>
			</div>
		{/if}
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<p class="xs muted"><Words key="library.import.noMatchFiles" links={{ a: to('/library/import-files') }} /></p>
	</section>

	{#if results}
		<section class="tp-sheet tp-pad" aria-labelledby="h-done" data-testid="adopt-results">
			<h2 id="h-done" class="h2">{t('library.import.done')}</h2>
			<p class="small">{t(okCount === 1 ? 'library.import.nowOne' : 'library.import.nowMany', { n: okCount, word: w, words: ws })}</p>
			<ul class="list">
				{#each results as r (r.folder)}
					<li>
						<span class="mono small path">{r.folder}</span>
						{#if r.title_id}
							<a class="small" href={to(`/library/${r.title_id}`)}>{t('library.import.openThe', { word: w })}</a>
							{#if r.episodes !== undefined || r.unknown?.length}
								<span class="xs muted"
									>{unit(kind === 'music' ? 'track' : 'episode', r.episodes ?? 0)}{r.unknown?.length
										? `, ${t('mediafiles.organise.filesLeftAlone', { files: unit('file', r.unknown.length) })}`
										: ''}</span
								>
							{/if}
						{:else}
							<span class="small bad-text">{t('library.import.notAdopted', { why: r.error ?? '' })}</span>
						{/if}
					</li>
				{/each}
			</ul>
			<div class="row end">{#if restFrom}<Button onclick={() => scan(restFrom)} disabled={scanning || adopting}>{t('library.import.scanRest')}</Button>{/if}<Button href={to('/library')}>{t('library.import.goLibrary')}</Button></div>
		</section>
	{/if}

	{#if plan}
		<section class="tp-sheet tp-pad" aria-labelledby="h-check" data-testid="adopt-plan">
			<h2 id="h-check" class="h2">{t('library.import.step2')}</h2>
			<p class="small muted">
				{t(plan.folders.length === 1 ? 'library.import.foldersInOne' : 'library.import.foldersInMany', { n: plan.folders.length, path: plan.path, words: ws })}
			</p>
			{#if plan.warning}<p class="notice bad" role="alert" data-testid="scan-warning">{plan.warning}</p>{/if}
			{#if !plan.folders.length}<p class="notice">{t('library.import.noFolders', { word: w })}</p>{/if}
			{#if plan.more}
				<div class="notice" data-testid="scan-more">
					{#if plan.partial}<p class="small">{t('library.import.stopped', { next: plan.next ?? '' })}</p>{:else}<p class="small">{t('library.import.first', { limit: plan.limit, next: plan.next ?? '' })}</p>{/if}
					<Button onclick={() => scan(plan?.next ?? '')} disabled={scanning || adopting}>{t('library.import.scanRest')}</Button>
				</div>
			{/if}
			{#if plan.folders.length}
				<div class="checkbar" bind:clientWidth={planWidth}>
					<ImportToolbar {phone} compact={phone || planWidth < 900} {view} {setView} {sizeItems} sizeLabel={view === 'posters' ? posterLabels[posterSize] : densityLabels[density]} {filters} bind:filter findLabel={t('library.import.searchFolders')} bind:findOpen hasWords={!!q.trim()}>
						{#snippet find()}<DlFind bind:term={q} shown={shown.length} total={plan?.folders.length ?? 0} searchKey="" placeholder={t('library.import.searchFolders')} note={t(shown.length === 1 ? 'library.import.folderCountOne' : 'library.import.folderCountMany', { n: shown.length.toLocaleString(appLocale()) })} testid="import-find" />{/snippet}
					</ImportToolbar>
					{#if shown.length}
						<div class="tickbar" data-testid="import-tickbar">
							<span class="count" aria-live="polite">{t('mediafiles.select.count', { n: tickNow.on.toLocaleString(appLocale()) })}</span>
							<Button variant="plain" size="md" disabled={!tickNow.can} onclick={tickShown} data-testid="import-select-all">{allTicked ? t('mediafiles.select.none') : t('mediafiles.select.all', { n: tickNow.can.toLocaleString(appLocale()) })}</Button>
						</div>
						<ImportList folders={shown} {view} size={posterSize} {density} {kind} {phone} bind:sel bind:ticked {named} onlook={openLookup} onzoom={(f) => (zoom = f)} />
					{:else}
						<p class="notice" data-testid="import-none">{t('library.import.noneMatch')}</p>
					{/if}
				</div>
			{/if}
			<div class="row end">
				<label class="checkline follow-all" data-testid="follow-box">
					<input type="checkbox" bind:checked={followAll} />
					<span><b>{t('library.import.follow')}</b><br /><span class="hint">{t('library.import.followHint')}</span></span>
				</label>
				<span class="small muted">{t('library.byHand.ticked', { n: picks.length })}</span>
				<Button variant="primary" onclick={adopt} disabled={!picks.length} busy={adopting}>
					{t(picks.length === 1 ? 'library.import.adoptOne' : 'library.import.adoptMany', { n: picks.length, word: w, words: ws })}
				</Button>
			</div>
		</section>
	{/if}
{/if}
</PageLoader>
</LibraryToolPage>

{#if lookFolder}
	<Sheet open title={t('library.import.lookUpFor', { word: w, folder: lookFolder.folder })} size="md" placement="top" onclose={() => (lookFor = null)}>
		<form class="look" onsubmit={lookup} data-testid="import-lookup">
			<label class="sr-only" for="look-term">{t('library.import.lookUpFor', { word: w, folder: lookFolder.folder })}</label>
			<!-- svelte-ignore a11y_autofocus -->
			<input id="look-term" class="input" bind:value={term} placeholder={kind === 'show' ? t('library.import.phShow') : kind === 'music' ? t('library.import.phMusic') : t('library.import.phMovie')} autocomplete="off" autofocus />
			<Button type="submit" busy={looking}>{t('library.byHand.lookUp')}</Button>
		</form>
		{#if lookError}<p class="form-error" role="alert">{lookError}</p>{/if}
		{#if found.length}
			<ul class="found">
				{#each found as m (m.key)}
					<li>
						<span class="small">{m.name}{m.year ? ` (${m.year})` : ''}</span>
						{#if m.title_id}
							<span class="xs muted">{t('mediafiles.adopt.inLibrary')}</span>
						{:else}
							<Button variant="plain" onclick={() => lookFolder && use(lookFolder.folder, m)}>{t('library.byHand.useThis')}</Button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</Sheet>
{/if}

{#if zoom}
	{@const p = pickedOf(zoom, sel[zoom.folder], named)}
	<PosterLightbox art={p.poster} title={`${p.name}${p.year ? ` (${p.year})` : ''}`} sub={[zoom.folder, zoom.main ? fileName(zoom.main) : ''].filter(Boolean).join(' · ')} square={kind === 'music'} onclose={() => (zoom = null)} />
{/if}

<style>
	:global(.tp) .h2 { margin: 0 0 12px; font: var(--t-title3); }
	.notice { display: flex; gap: 10px; align-items: flex-start; margin: 0; padding: 12px 14px; border: 0; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
	.notice[data-testid='scan-more'] { flex-direction: column; }
	.pick {
		align-items: flex-end;
		gap: var(--s-3);
		flex-wrap: wrap;
		padding: var(--s-3);
		border-radius: var(--r-btn);
		background: var(--surface-2);
	}
	.grow {
		flex: 1 1 240px;
		margin-bottom: 0;
	}
	.follow-all { margin-inline-end: auto; }
	.bar {
		width: 100%;
		margin-top: var(--s-4);
	}
	/* The toolbar and the folders sit a step under the intro, as on the Library page. */
	.checkbar { display: flex; flex-direction: column; gap: 12px; min-width: 0; margin-top: 14px; }
	.tickbar { display: flex; align-items: center; gap: 6px; min-width: 0; }
	.tickbar .count { padding: 0 8px; font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.tickbar :global(.ui-btn.plain) { padding-inline: 6px; }
	.list,
	.found {
		list-style: none;
		padding: 0;
		margin: var(--s-2) 0 0;
	}
	.list { border: 1px solid var(--line); border-radius: var(--r-btn); background: var(--surface-1); overflow: hidden; }
	.list > li {
		display: flex;
		flex-direction: column;
		gap: var(--s-1);
		padding: var(--s-3) var(--s-4);
	}
	.list > li + li { border-top: 1px solid var(--line); }
	.path {
		overflow-wrap: anywhere;
	}
	.look {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
	}
	.look .input {
		flex: 1 1 200px;
	}
	.found li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--s-2);
		min-height: 44px;
		padding: var(--s-1) 0;
	}
	.found li + li { border-top: 1px solid var(--line); }
	.end {
		justify-content: flex-end;
		align-items: center;
		gap: var(--s-3);
		margin-top: var(--s-5);
	}
	.bad-text {
		color: var(--fail);
	}
	@media (max-width: 600px) {
		.pick > :global(.ui-btn), .look > :global(.ui-btn), .end > :global(.ui-btn) { width: 100%; }
	}
</style>
