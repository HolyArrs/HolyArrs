<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { reaskWhileFilling } from '#lib/music/index.ts';
	import { LibraryToolPage, albumFolderLine, cutFilesLine, canAdopt, choices, episodeFileLine, musicFolderLine, trackFileLine, fileName, followJob, groupPlan, matchKey, score, showFolderLine, showIdOf, startPicks, type AdoptCandidate, type AdoptPlan, type AdoptResult, type JobView } from '#lib/mediafiles/index.ts';
	import type { LookupArtist } from '#lib/music/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { LibraryFolder, LookupMovie } from '#lib/core/types.ts';
	import { unit } from '#lib/mediafiles/index.ts';
	import type { LookupShow } from '#lib/core/types.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let folders = $state<LibraryFolder[]>([]);
	let loaded = $state(false);
	let folderId = $state(0);
	let error = $state('');

	let scanning = $state(false);
	let job = $state<JobView | null>(null);
	let plan = $state<AdoptPlan | null>(null);
	/** The movie (TMDB id) or show ("<source>:<id>") chosen for each folder. */
	let sel = $state<Record<string, string>>({});
	/** Names of movies or shows picked by a lookup, by id, to show what was chosen. */
	let named = $state<Record<string, string>>({});
	let ticked = $state<Record<string, boolean>>({});
	let adopting = $state(false);
	// Follow what is brought in: it goes on my calendar (ticked by default).
	let followAll = $state(true);
	let results = $state<AdoptResult[] | null>(null);
	// A large folder scanned in parts: where the next scan starts, kept after adopting.
	let restFrom = $state('');

	let lookFor = $state<string | null>(null);
	let term = $state('');
	let found = $state<{ key: string; name: string; year?: number; title_id?: number }[]>([]);
	let looking = $state(false);
	let lookError = $state('');

	/** The kind of the picked Library folder: movies, shows or music. */
	const kind = $derived<'movie' | 'show' | 'music'>(plan?.kind ?? folderKind(folders.find((f) => f.id === folderId)?.kind));
	const word = $derived(kind === 'show' ? 'show' : kind === 'music' ? 'artist' : 'movie');
	const w = $derived(t(`library.import.word.${word}`));
	const ws = $derived(t(`library.import.words.${word}`));
	const groups = $derived(plan ? groupPlan(plan.folders, word) : []);
	const picks = $derived(choices(Object.fromEntries(Object.entries(ticked).filter(([, on]) => on).map(([f]) => [f, sel[f] ?? ''])), kind));
	const running = $derived(!!job && !['done', 'failed', 'cancelled'].includes(job.State));
	const okCount = $derived(results ? results.filter((r) => r.title_id).length : 0);

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
				found = artists.map((a) => ({ key: `mbid:${a.mbid}`, name: a.disambiguation ? `${a.name} (${a.disambiguation})` : a.name, title_id: a.title_id }));
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'mbid:<id>' });
			} else if (kind === 'show') {
				const shows = (await api.get<{ shows: LookupShow[] }>(`/shows/lookup?term=${encodeURIComponent(term.trim())}`)).shows;
				found = shows.map((s) => ({ key: showIdOf(s), name: s.name, year: s.year, title_id: s.title_id })).filter((s) => s.key);
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'tvdb:<id>' });
			} else {
				const m = term.trim().match(/^(.*?)\s+\(?((?:19|20)\d\d)\)?$/);
				const q = m ? `term=${encodeURIComponent(m[1])}&year=${m[2]}` : `term=${encodeURIComponent(term.trim())}`;
				const movies = (await api.get<{ movies: LookupMovie[] }>(`/titles/lookup?${q}`)).movies;
				found = movies.map((x) => ({ key: x.ids.tmdb ?? '', name: x.name, year: x.year, title_id: x.title_id })).filter((x) => x.key);
				if (!found.length) lookError = t('library.import.nothingFound', { id: 'tmdb:<id>' });
			}
		} catch (err) {
			lookError = message(err);
		} finally {
			looking = false;
		}
	}

	function use(folder: string, m: { key: string; name: string; year?: number }) {
		const id = m.key;
		if (!id) return;
		sel[folder] = id;
		named[id] = `${m.name}${m.year ? ` (${m.year})` : ''}`;
		ticked[folder] = true;
		lookFor = null;
	}

	function movieLabel(f: AdoptCandidate): string {
		const id = sel[f.folder];
		if (!id) return t('library.import.nonePicked', { word: w });
		if (named[id]) return named[id];
		const m = [f.match, ...f.others].find((x) => x && matchKey(x) === id);
		return m ? `${m.name}${m.year ? ` (${m.year})` : ''}` : kind === 'movie' ? t('library.import.movieId', { id }) : id;
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
				<p class="xs muted">{job?.ProgressNote || (scanning ? t('library.import.reading', { word: w }) : t('library.import.adopting'))}</p>
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
			{#each groups as g (g.status)}
				<div class="group" data-testid="group-{g.status}">
					<h3 class="h3">{g.label} <span class="muted">({g.folders.length})</span></h3>
					<p class="xs muted">{g.about}</p>
					<ul class="list">
						{#each g.folders as f (f.folder)}
							<li class="folder">
								<div class="top">
									{#if canAdopt(f)}
										<label class="tick">
											<input type="checkbox" bind:checked={ticked[f.folder]} disabled={!sel[f.folder]} aria-label={t('library.import.adoptFolder', { folder: f.folder })} />
											<span class="mono small path">{f.folder}</span>
										</label>
									{:else}
										<span class="mono small path">{f.folder}</span>
									{/if}
									{#if f.match && canAdopt(f)}<Badge label={score(f.match.score)} />{/if}
								</div>
								{#if canAdopt(f)}
									<p class="small"><b>{movieLabel(f)}</b></p>
									{#if f.others.length && f.status !== 'matched'}
										<div class="field other">
											<label class="xs" for="o-{f.folder}">{t('library.import.orOne')}</label>
											<select id="o-{f.folder}" class="input" bind:value={sel[f.folder]}>
												{#each [f.match, ...f.others].filter((x) => !!x) as m (matchKey(m))}
													<option value={matchKey(m)}>{m.name}{m.year ? ` (${m.year})` : ''} · {score(m.score)}</option>
												{/each}
											</select>
										</div>
									{/if}
								{/if}
								<p class="xs muted">{f.why}{#if f.main}{' · '}{t('mediafiles.sheet.file')}: <span class="mono path">{fileName(f.main)}</span>{#if f.videos[0]}{' '}({bytes(f.videos[0].size)}){/if}{/if}</p>
								{#if f.albums?.length}
									<details class="eps" data-testid="music-albums">
										<summary class="xs">{musicFolderLine(f)}</summary>
										<ul class="files albums">
											{#each f.albums as al (al.folder)}
												<li class="album" data-testid="music-album">
													<span class="small"><span class="mono path">{al.folder}</span></span>
													<span class="xs muted">{albumFolderLine(al)}</span>
													<ul class="files">
														{#each al.files as tf (tf.path)}
															<li class="xs" class:left={!tf.track}>
																<span class="mono path">{fileName(tf.path)}</span>
																<span class="muted">{trackFileLine(tf)}</span>
															</li>
														{/each}
														{#if cutFilesLine(al)}<li class="xs muted">{cutFilesLine(al)}</li>{/if}
													</ul>
												</li>
											{/each}
											{#each f.files ?? [] as ef (ef.path)}
												<li class="xs left">
													<span class="mono path">{fileName(ef.path)}</span>
													<span class="muted">{episodeFileLine(ef)}</span>
												</li>
											{/each}
											{#if f.cut}<li class="xs muted">{t('library.import.moreLoose')}</li>{/if}
										</ul>
									</details>
								{:else if f.files?.length}
									<details class="eps" data-testid="show-files">
										<summary class="xs">{showFolderLine(f)}</summary>
										<ul class="files">
											{#each f.files as ef (ef.path)}
												<li class="xs" class:left={!ef.episodes.length}>
													<span class="mono path">{fileName(ef.path)}</span>
													<span class="muted">{episodeFileLine(ef)}</span>
												</li>
											{/each}
										</ul>
									</details>
								{/if}
								{#if f.status === 'in_library' && f.title_id}<a class="xs" href={to(`/library/${f.title_id}`)}>{t('library.import.openThe', { word: w })}</a>{/if}
								{#if canAdopt(f)}
									{#if lookFor === f.folder}
										<form class="look" onsubmit={lookup}>
											<label class="sr-only" for="look-{f.folder}">{t('library.import.lookUpFor', { word: w, folder: f.folder })}</label>
											<input
												id="look-{f.folder}"
												class="input"
												bind:value={term}
												placeholder={kind === 'show' ? t('library.import.phShow') : kind === 'music' ? t('library.import.phMusic') : t('library.import.phMovie')}
												autocomplete="off"
											/>
											<Button type="submit" busy={looking}>{t('library.byHand.lookUp')}</Button>
											<Button variant="plain" onclick={() => (lookFor = null)}>{t('shows.page.close')}</Button>
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
															<Button variant="plain" onclick={() => use(f.folder, m)}>{t('library.byHand.useThis')}</Button>
														{/if}
													</li>
												{/each}
											</ul>
										{/if}
									{:else}
										<div><Button variant="plain" icon="search" onclick={() => openLookup(f)}>{f.status === 'matched' ? t('library.import.notThis') : t('library.byHand.lookUp')}</Button></div>
									{/if}
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/each}
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

<style>
	:global(.tp) .h2 { margin: 0 0 12px; font: var(--t-title3); }
	:global(.tp) .h3 { margin: 0 0 2px; font: var(--t-headline); }
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
	.follow-all { margin-right: auto; }
	.bar {
		width: 100%;
		margin-top: var(--s-4);
	}
	.group {
		margin-top: var(--s-5);
	}
	.list,
	.found {
		list-style: none;
		padding: 0;
		margin: var(--s-2) 0 0;
	}
	.list { border: 1px solid var(--line); border-radius: var(--r-btn); background: var(--surface-1); overflow: hidden; }
	.list > li {
		padding: var(--s-3) var(--s-4);
		display: flex;
		flex-direction: column;
		gap: var(--s-1);
		border: 0;
		border-radius: 0;
		background: transparent;
	}
	.list > li + li {
		margin-top: 0;
		border-top: 1px solid var(--line);
	}
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-2);
	}
	.tick {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-height: 40px;
		min-width: 0;
	}
	.tick input {
		width: 20px;
		height: 20px;
		flex: none;
	}
	.path {
		overflow-wrap: anywhere;
	}
	.other {
		max-width: 480px;
		margin-bottom: 0;
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
		padding: var(--s-1) 0;
	}
	.end {
		justify-content: flex-end;
		align-items: center;
		gap: var(--s-3);
		margin-top: var(--s-5);
	}
	.bad-text {
		color: var(--fail);
	}
	.eps summary {
		min-height: 40px;
		display: flex;
		align-items: center;
		cursor: pointer;
	}
	.files {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.files li {
		display: flex;
		flex-wrap: wrap;
		gap: 2px var(--s-2);
		padding: 2px 0;
	}
	.files li.left .path {
		color: var(--text-muted);
	}
	.files li.album {
		flex-direction: column;
		padding: var(--s-2) 0;
	}
	.albums > li + li {
		border-top: 1px solid var(--line);
	}
	@media (max-width: 600px) {
		.pick > :global(.ui-btn), .look > :global(.ui-btn), .end > :global(.ui-btn) { width: 100%; }
		.top, .found li { align-items: flex-start; flex-direction: column; }

	}
</style>
