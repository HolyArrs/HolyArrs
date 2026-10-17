<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/EmptyState.svelte';
	import { session } from '#lib/session.svelte.ts';
	import {
		albumFolderLine,
		cutFilesLine,
		canAdopt,
		choices,
		episodeFileLine,
		musicFolderLine,
		trackFileLine,
		fileName,
		followJob,
		groupPlan,
		matchKey,
		score,
		showFolderLine,
		showIdOf,
		startPicks,
		type AdoptCandidate,
		type AdoptPlan,
		type AdoptResult,
		type JobView
	} from '#lib/organise.ts';
	import type { LibraryFolder, LookupArtist, LookupMovie, LookupShow } from '#lib/types.ts';

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
	let results = $state<AdoptResult[] | null>(null);

	let lookFor = $state<string | null>(null);
	let term = $state('');
	let found = $state<{ key: string; name: string; year?: number; title_id?: number }[]>([]);
	let looking = $state(false);
	let lookError = $state('');

	/** The kind of the picked Library folder: movies, shows or music. */
	const kind = $derived<'movie' | 'show' | 'music'>(plan?.kind ?? folderKind(folders.find((f) => f.id === folderId)?.kind));
	const word = $derived(kind === 'show' ? 'show' : kind === 'music' ? 'artist' : 'movie');
	const groups = $derived(plan ? groupPlan(plan.folders, word) : []);
	const picks = $derived(choices(Object.fromEntries(Object.entries(ticked).filter(([, on]) => on).map(([f]) => [f, sel[f] ?? ''])), kind));
	const running = $derived(!!job && !['done', 'failed', 'cancelled'].includes(job.State));
	const okCount = $derived(results ? results.filter((r) => r.title_id).length : 0);

	function folderKind(k: string | undefined): 'movie' | 'show' | 'music' {
		return k === 'show' || k === 'music' ? k : 'movie';
	}

	onMount(async () => {
		try {
			folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders.filter(
				(f) => f.kind === 'movie' || f.kind === 'show' || f.kind === 'music'
			);
			folderId = folders[0]?.id ?? 0;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	});

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
		try {
			const r = await api.post<{ job_id: number }>(`/library-folders/${folderId}/scan`, after ? { after } : undefined);
			const end = await follow(r.job_id);
			// A stopped music scan keeps the artist folders it finished.
			const kept = end.State === 'cancelled' ? (end.Result?.plan as AdoptPlan | undefined) : undefined;
			if (end.State !== 'done' && !kept?.partial) throw new Error(end.Error?.message || 'The scan did not finish.');
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
				const artists = (await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(term.trim())}`)).artists;
				found = artists.map((a) => ({ key: `mbid:${a.mbid}`, name: a.disambiguation ? `${a.name} (${a.disambiguation})` : a.name, title_id: a.title_id }));
				if (!found.length) lookError = 'Nothing found. Try fewer words, or mbid:<id>.';
			} else if (kind === 'show') {
				const shows = (await api.get<{ shows: LookupShow[] }>(`/shows/lookup?term=${encodeURIComponent(term.trim())}`)).shows;
				found = shows.map((s) => ({ key: showIdOf(s), name: s.name, year: s.year, title_id: s.title_id })).filter((s) => s.key);
				if (!found.length) lookError = 'Nothing found. Try fewer words, or tvdb:<id>.';
			} else {
				const m = term.trim().match(/^(.*?)\s+\(?((?:19|20)\d\d)\)?$/);
				const q = m ? `term=${encodeURIComponent(m[1])}&year=${m[2]}` : `term=${encodeURIComponent(term.trim())}`;
				const movies = (await api.get<{ movies: LookupMovie[] }>(`/titles/lookup?${q}`)).movies;
				found = movies.map((x) => ({ key: x.ids.tmdb ?? '', name: x.name, year: x.year, title_id: x.title_id })).filter((x) => x.key);
				if (!found.length) lookError = 'Nothing found. Try fewer words, or tmdb:<id>.';
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
		if (!id) return `No ${word} picked yet`;
		if (named[id]) return named[id];
		const m = [f.match, ...f.others].find((x) => x && matchKey(x) === id);
		return m ? `${m.name}${m.year ? ` (${m.year})` : ''}` : kind === 'movie' ? `TMDB ${id}` : id;
	}

	async function adopt() {
		if (!plan || !picks.length) return;
		adopting = true;
		error = '';
		try {
			const r = await api.post<{ job_id: number }>(`/library-folders/${plan.library_folder_id}/adopt`, { folders: picks });
			const end = await follow(r.job_id);
			if (end.State !== 'done') throw new Error(end.Error?.message || 'Adopting did not finish.');
			results = (end.Result?.results as AdoptResult[]) ?? [];
			const n = results.filter((x) => x.title_id).length;
			toasts.push(n === 1 ? `1 ${word} adopted. No file was moved.` : `${n} ${word}s adopted. No file was moved.`, n ? 'ok' : 'info');
			plan = null;
		} catch (e) {
			error = message(e);
		} finally {
			adopting = false;
			job = null;
		}
	}
</script>

<svelte:head><title>Import a library · HolyArrs</title></svelte:head>

<a class="back small muted" href={to(`/library`)}><Icon name="back" size={16} /> Library</a>

<header class="page-head">
	<div>
		<p class="eyebrow">Library management</p>
		<h1>Import a library</h1>
		<p>
			Bring movies, shows and music you already have into HolyArrs. It reads a Library folder, works out which movie, show or artist each folder holds (for
			a show, which episode each file is; for an artist, which album each folder is and which track each file is), and you approve them.
		</p>
	</div>
</header>

<p class="notice" data-testid="nothing-moved">
	<b>Nothing is moved or renamed.</b> Scanning only reads the folder. Adopting adds each movie, show or artist with its folder and files exactly as they
	are. You can rename them later from Rename files.
</p>

{#if !loaded}
	<div class="sk" style="height: 160px"></div>
{:else if !folders.length}
	<EmptyState icon="folder" title="No Library folder yet">
		<p>Add the folder that holds your movies, shows or music first.</p>
		{#snippet actions()}
			{#if session.can('settings')}<a class="btn btn-dark" href={to(`/settings/library-folders`)}>Library folders</a>{/if}
		{/snippet}
	</EmptyState>
{:else}
	<section class="card" aria-labelledby="h-pick">
		<h2 id="h-pick" class="h2">1. Pick a folder</h2>
		<div class="row pick">
			<div class="field grow">
				<label for="lf">Library folder</label>
				<select id="lf" class="input" bind:value={folderId} disabled={scanning || adopting}>
					{#each folders as f (f.id)}<option value={f.id}
							>{f.kind === 'show' ? 'Shows: ' : f.kind === 'music' ? 'Music: ' : ''}{f.label ? `${f.label} (${f.path})` : f.path}</option
						>{/each}
				</select>
			</div>
			<button class="btn btn-dark" type="button" onclick={() => scan()} disabled={scanning || adopting || !folderId} aria-busy={scanning}>
				<Icon name="search" size={18} /> Scan (nothing is moved)
			</button>
		</div>
		{#if running}
			<div aria-live="polite">
				<progress class="bar" max="1" value={job?.Progress ?? 0} aria-label="Progress"></progress>
				<p class="xs muted">{job?.ProgressNote || (scanning ? `Reading the folder and looking each ${word} up.` : 'Adopting.')}</p>
			</div>
		{/if}
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</section>

	{#if results}
		<section class="card" aria-labelledby="h-done" data-testid="adopt-results">
			<h2 id="h-done" class="h2">Done</h2>
			<p class="small">{okCount === 1 ? `1 ${word} is now in the Library.` : `${okCount} ${word}s are now in the Library.`} Their files were not moved.</p>
			<ul class="list">
				{#each results as r (r.folder)}
					<li>
						<span class="mono small path">{r.folder}</span>
						{#if r.title_id}
							<a class="small" href={to(`/library/${r.title_id}`)}>Open the {word}</a>
							{#if r.episodes !== undefined || r.unknown?.length}
								<span class="xs muted"
									>{r.episodes ?? 0} {kind === 'music' ? 'track' : 'episode'}{r.episodes === 1 ? '' : 's'}{r.unknown?.length
										? `, ${r.unknown.length} file${r.unknown.length === 1 ? '' : 's'} left alone`
										: ''}</span
								>
							{/if}
						{:else}
							<span class="small bad-text">Not adopted: {r.error}</span>
						{/if}
					</li>
				{/each}
			</ul>
			<div class="row end"><a class="btn btn-ghost" href={to(`/library`)}>Go to the Library</a></div>
		</section>
	{/if}

	{#if plan}
		<section class="card" aria-labelledby="h-check" data-testid="adopt-plan">
			<h2 id="h-check" class="h2">2. Check and approve</h2>
			<p class="small muted">
				{plan.folders.length} folder{plan.folders.length === 1 ? '' : 's'} in <span class="mono path">{plan.path}</span>. Tick the {word}s to adopt. Matched ones are
				ticked for you.
			</p>
			{#if plan.warning}<p class="notice bad" role="alert" data-testid="scan-warning">{plan.warning}</p>{/if}
			{#if !plan.folders.length}<p class="notice">This folder has no {word} folders in it.</p>{/if}
			{#if plan.more}
				<div class="notice" data-testid="scan-more">
					{#if plan.partial}<p class="small">The scan was stopped after <span class="mono">{plan.next}</span>; the folders it finished are below.</p>{:else}<p class="small">This scan read the first {plan.limit} folders, up to <span class="mono">{plan.next}</span>. Adopt these first, then read the next part.</p>{/if}
					<button class="btn" type="button" onclick={() => scan(plan?.next ?? '')} disabled={scanning || adopting}>Scan the rest</button>
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
											<input type="checkbox" bind:checked={ticked[f.folder]} disabled={!sel[f.folder]} aria-label="Adopt {f.folder}" />
											<span class="mono small path">{f.folder}</span>
										</label>
									{:else}
										<span class="mono small path">{f.folder}</span>
									{/if}
									{#if f.match && canAdopt(f)}<span class="chip {f.status === 'matched' ? 'ok' : 'wait'}">{score(f.match.score)}</span>{/if}
								</div>
								{#if canAdopt(f)}
									<p class="small"><b>{movieLabel(f)}</b></p>
									{#if f.others.length && f.status !== 'matched'}
										<div class="field other">
											<label class="xs" for="o-{f.folder}">Or one of these</label>
											<select id="o-{f.folder}" class="input" bind:value={sel[f.folder]}>
												{#each [f.match, ...f.others].filter((x) => !!x) as m (matchKey(m))}
													<option value={matchKey(m)}>{m.name}{m.year ? ` (${m.year})` : ''} · {score(m.score)}</option>
												{/each}
											</select>
										</div>
									{/if}
								{/if}
								<p class="xs muted">{f.why}{#if f.main}{' · '}File: <span class="mono path">{fileName(f.main)}</span>{#if f.videos[0]}{' '}({bytes(f.videos[0].size)}){/if}{/if}</p>
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
											{#if f.cut}<li class="xs muted">and more loose files, not listed</li>{/if}
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
								{#if f.status === 'in_library' && f.title_id}<a class="xs" href={to(`/library/${f.title_id}`)}>Open the {word}</a>{/if}
								{#if canAdopt(f)}
									{#if lookFor === f.folder}
										<form class="look" onsubmit={lookup}>
											<label class="sr-only" for="look-{f.folder}">Look up a {word} for {f.folder}</label>
											<input
												id="look-{f.folder}"
												class="input"
												bind:value={term}
												placeholder={kind === 'show' ? 'Name, or tvdb:123' : kind === 'music' ? 'Artist name, or mbid:<id>' : 'Name and year, or tmdb:123'}
												autocomplete="off"
											/>
											<button class="btn btn-ghost" type="submit" disabled={looking} aria-busy={looking}>Look up</button>
											<button class="btn btn-quiet" type="button" onclick={() => (lookFor = null)}>Close</button>
										</form>
										{#if lookError}<p class="form-error" role="alert">{lookError}</p>{/if}
										{#if found.length}
											<ul class="found">
												{#each found as m (m.key)}
													<li>
														<span class="small">{m.name}{m.year ? ` (${m.year})` : ''}</span>
														{#if m.title_id}
															<span class="xs muted">Already in the Library</span>
														{:else}
															<button class="btn btn-quiet" type="button" onclick={() => use(f.folder, m)}>Use this</button>
														{/if}
													</li>
												{/each}
											</ul>
										{/if}
									{:else}
										<button class="btn btn-quiet" type="button" onclick={() => openLookup(f)}>{f.status === 'matched' ? 'Not this one? Look up' : 'Look up'}</button>
									{/if}
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/each}
			<div class="row end">
				<span class="small muted">{picks.length} ticked</span>
				<button class="btn btn-dark" type="button" onclick={adopt} disabled={adopting || !picks.length} aria-busy={adopting}>
					{picks.length === 1 ? `Adopt 1 ${word}` : `Adopt ${picks.length} ${word}s`}
				</button>
			</div>
		</section>
	{/if}
{/if}

<style>
	.eyebrow { margin: 0 0 var(--s-2); font-size: 11px; font-weight: var(--weight-semibold); letter-spacing: .09em; text-transform: uppercase; color: var(--text-muted); }
	.back {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-height: 40px;
		margin-bottom: var(--s-2);
	}
	.pick {
		align-items: flex-end;
		gap: var(--s-3);
		flex-wrap: wrap;
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-radius: var(--r-btn);
		background: var(--surface-1);
	}
	.grow {
		flex: 1 1 240px;
		margin-bottom: 0;
	}
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
		.notice[data-testid='nothing-moved'] {
			display: block;
		}
		.notice[data-testid='nothing-moved'] b {
			display: block;
			margin-bottom: var(--s-1);
		}
		.pick > :global(.btn), .look > :global(.btn), .end > :global(.btn) { width: 100%; min-height: var(--tap); }
		.top, .found li { align-items: flex-start; flex-direction: column; }
		.back { min-height: 44px; }
	}
</style>
