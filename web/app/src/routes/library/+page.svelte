<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { date, tone } from '#lib/format.ts';
	import { counts, filters, kinds, matches, matchesTitleQuery, ofKind, titleQuery, type Filter, type Kind } from '#lib/library.ts';
	import { showProgress, showState } from '#lib/shows.ts';
	import { artistProgress, artistState } from '#lib/music.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Poster from '#lib/Poster.svelte';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import type { TitleView } from '#lib/types.ts';

	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let searchError = $state('');
	let indexedIds = $state<Set<number>>(new Set());
	let mounted = $state(false);
	let loadRevision = $state(0);
	let searchRun = 0;
	const startFilter = page.url.searchParams.get('show') as Filter | null;
	let filter = $state<Filter>(startFilter && filters.some(([f]) => f === startFilter) ? startFilter : 'all');
	const startKind = page.url.searchParams.get('kind') as Kind | null;
	let kind = $state<Kind>(startKind && kinds.some(([k]) => k === startKind) ? startKind : 'all');
	let q = $state('');
	const query = $derived(titleQuery(q));
	const moviesOn = $derived(partsNow.answer?.parts.find((part) => part.part === 'movies')?.on !== false);
	let view = $state<'grid' | 'list'>(readView());

	function readView(): 'grid' | 'list' {
		try {
			return localStorage.getItem('holyarrs.library-view') === 'list' ? 'list' : 'grid';
		} catch {
			return 'grid';
		}
	}
	function setView(v: 'grid' | 'list') {
		view = v;
		try {
			localStorage.setItem('holyarrs.library-view', v);
		} catch {
			// Not remembered in a private window; still works.
		}
	}
	function setFilter(f: Filter) {
		filter = f;
		const url = new URL(page.url.href);
		if (f === 'all') url.searchParams.delete('show');
		else url.searchParams.set('show', f);
		void goto(url, { replace: true, shallow: true });
	}

	function setKind(k: Kind) {
		kind = k;
		const url = new URL(page.url.href);
		if (k === 'all') url.searchParams.delete('kind');
		else url.searchParams.set('kind', k);
		void goto(url, { replace: true, shallow: true });
	}

	const ofThisKind = $derived(titles.filter((t) => ofKind(t, kind)));
	const tally = $derived(counts(ofThisKind));
	const kindTally = $derived({
		all: titles.length,
		movie: titles.filter((t) => t.kind === 'movie').length,
		show: titles.filter((t) => t.kind === 'show').length,
		artist: titles.filter((t) => t.kind === 'artist').length
	});
	const kindWord: Record<string, string> = { show: ' · Show', artist: ' · Artist' };
	const shown = $derived(ofThisKind.filter((t) => matches(t, filter) && matchesTitleQuery(t, query, indexedIds)));

	$effect(() => {
		loadRevision;
		if (!mounted) return;
		const run = ++searchRun;
		indexedIds = new Set();
		searchError = '';
		if (!query) return;
		const controller = new AbortController();
		const timer = setTimeout(async () => {
			try {
				const found = await api.get<{ titles: TitleView[] }>(`/titles?q=${encodeURIComponent(query)}`, controller.signal);
				if (run === searchRun) indexedIds = new Set(found.titles.map((t) => t.id));
			} catch (e) {
				if (!controller.signal.aborted && run === searchRun) searchError = message(e);
			}
		}, 200);
		return () => {
			clearTimeout(timer);
			controller.abort();
		};
	});

	/** chipOf is one state chip for a title: its first Slot, or a show's sum. */
	function chipOf(t: TitleView): { state: string; label: string } | null {
		if (t.kind === 'show') {
			const s = showState(t.show);
			return s.label ? s : null;
		}
		if (t.kind === 'artist') {
			const s = artistState(t.artist);
			return s.label ? s : null;
		}
		return t.slots[0] ? { state: t.slots[0].state, label: t.slots[0].state_label } : null;
	}

	async function load() {
		try {
			titles = (await api.get<{ titles: TitleView[] }>('/titles')).titles;
			error = '';
			loadRevision++;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	onMount(() => {
		mounted = true;
		void load();
		const offs = [live.on('title', load), live.on('slot', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
</script>

<svelte:head><title>Library · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Library</h1>
		{#if moviesOn}<p class="small"><a href={to('/people')}>People and their movies</a> · <a href={to('/library/collections')}>Movie series</a>{#if session.can('manage_library')} · <a href={to('/library/copies')}>Movie file codecs</a>{/if}</p>{/if}
		<p>Every movie, show and artist HolyArrs looks after.</p>
	</div>
	{#if session.can('manage_library')}
		<div class="row head-actions">
			<a class="btn btn-quiet" href={to(`/library/held`)}>Held</a>
			<a class="btn btn-quiet" href={to(`/library/turned-down`)}>Turned down</a>
			<a class="btn btn-quiet" href={to(`/library/missing-subtitles`)}>Missing subtitles</a>
			<a class="btn btn-quiet" href={to(`/library/rename`)}>Rename files</a>
			<a class="btn btn-quiet" href={to(`/library/import`)}>Import a library</a>
			<a class="btn btn-ghost" href={to(`/search?kind=show`)}>Add a show</a>
			<a class="btn btn-ghost" href={to(`/search?kind=music`)}>Add an artist</a>
			<a class="btn btn-red" href={to(`/search`)}>Add a movie</a>
		</div>
	{/if}
</header>

{#if loaded && titles.length > 0}
	<div class="seg kinds" role="group" aria-label="Kind">
		{#each kinds as [key, label] (key)}
			<button type="button" aria-pressed={kind === key} onclick={() => setKind(key)}>{label} <span class="n">{kindTally[key]}</span></button>
		{/each}
	</div>
	<div class="tools">
		<div class="pills" role="group" aria-label="Show">
			{#each filters as [key, label] (key)}
				<button class="pill" type="button" aria-pressed={filter === key} onclick={() => setFilter(key)}>{label} <span class="n">{tally[key]}</span></button>
			{/each}
		</div>
		<div class="row tools-end">
			<label class="sr-only" for="find">Find in the Library</label>
			<input id="find" class="input find" type="search" bind:value={q} placeholder="Find by name" autocomplete="off" maxlength="256" />
			<div class="seg" role="group" aria-label="Layout">
				<button type="button" aria-pressed={view === 'grid'} onclick={() => setView('grid')}>Posters</button>
				<button type="button" aria-pressed={view === 'list'} onclick={() => setView('list')}>List</button>
			</div>
		</div>
	</div>
{/if}

{#if searchError && !error}<p class="notice bad" role="alert">{searchError}</p>{/if}

{#if !loaded}
	<div class="grid">
		{#each Array(6) as _, i (i)}<div class="sk" style="aspect-ratio: 2 / 3"></div>{/each}
	</div>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if titles.length === 0}
	<EmptyState icon="library" title="Your Library is empty">
		{#if session.can('manage_library')}
			<p>Find a movie, a show or an artist on the Search page and add it. HolyArrs then keeps it up to date.</p>
		{:else}
			<p>Nothing has been added yet. Look for a movie, a show or an artist on the Search page.</p>
		{/if}
		{#snippet actions()}
			<a class="btn btn-red" href={to(`/search`)}>Search for a movie</a>
			<a class="btn btn-ghost" href={to(`/search?kind=show`)}>Search for a show</a>
			<a class="btn btn-ghost" href={to(`/search?kind=music`)}>Search for an artist</a>
			{#if session.can('manage_library')}<a class="btn btn-ghost" href={to(`/library/import`)}>Import a library</a>{/if}
			{#if session.can('settings')}<a class="btn btn-ghost" href={to(`/settings/library-folders`)}>Library folders</a>{/if}
		{/snippet}
	</EmptyState>
{:else if shown.length === 0}
	<p class="muted">Nothing matches this filter.</p>
{:else if view === 'list'}
	<table class="list-table">
		<caption class="sr-only">Titles in the Library</caption>
		<thead>
			<tr><th scope="col">Title</th><th scope="col">State</th><th scope="col" class="wide">Out on</th><th scope="col" class="wide">Keep up to date</th></tr>
		</thead>
		<tbody>
			{#each shown as t (t.id)}
				<tr data-testid="library-row">
					<td><a class="row-link" href={to(`/library/${t.id}`)}><span class="thumb"><Poster src={t.artwork[0]} name={t.name} /></span><span><b>{t.name}</b> <span class="muted xs">{t.year ?? ''}{kindWord[t.kind] ?? ''}</span></span></a></td>
					{#if t.kind === 'artist'}
						{@const c = chipOf(t)}
						<td>{#if c}<span class="chip {tone(c.state)}">{c.label}</span>{/if}<span class="xs muted block">{artistProgress(t.artist)}</span></td>
						<td class="wide small muted">{t.artist?.next_release ? `Next album ${date(t.artist.next_release)}` : t.artist?.status === 'ended' ? 'No longer active' : 'No date yet'}</td>
					{:else if t.kind === 'show'}
						{@const c = chipOf(t)}
						<td>{#if c}<span class="chip {tone(c.state)}">{c.label}</span>{/if}<span class="xs muted block">{showProgress(t.show)}</span></td>
						<td class="wide small muted">{t.show?.next_air_date ? `Next episode ${date(t.show.next_air_date)}` : t.show?.status === 'ended' ? 'Ended' : 'No date yet'}</td>
					{:else}
						<td>{#each t.slots as s (s.id)}<span class="chip {tone(s.state)}">{s.state_label}</span>{/each}</td>
						<td class="wide small muted">{date(t.digital_date ?? t.physical_date ?? t.cinema_date) || 'Not known'}</td>
					{/if}
					<td class="wide small">{t.keep_up_to_date ? 'Yes' : 'No'}</td>
				</tr>
			{/each}
		</tbody>
	</table>
{:else}
	<ul class="grid">
		{#each shown as t, i (t.id)}
			{@const c = chipOf(t)}
			<li class="rise" style="--i: {i}">
				<a class="item" href={to(`/library/${t.id}`)} data-testid="library-card">
					<Poster src={t.artwork[0]} name={t.name} />
					<span class="name">{t.name}</span>
					<span class="meta">
						<span class="muted xs">{t.year ?? ''}{kindWord[t.kind] ?? ''}</span>
						{#if c}<span class="chip {tone(c.state)}">{c.label}</span>{/if}
					</span>
				</a>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.kinds {
		margin-bottom: var(--s-4);
	}
	.kinds button {
		min-height: 40px;
		padding: 0 var(--s-4);
	}
	.kinds .n {
		opacity: 1;
	}
	.kinds button[aria-pressed='true'] {
		background: var(--ha-red-soft);
	}
	.block {
		display: block;
		margin-top: 2px;
	}
	.head-actions {
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.tools {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-3);
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--s-5);
	}
	.tools-end {
		gap: var(--s-3);
		flex-wrap: wrap;
	}
	.find {
		width: 200px;
		max-width: 100%;
	}
	.pills {
		display: flex;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.n {
		margin-left: 4px;
		font-variant-numeric: tabular-nums;
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--line-strong);
		border-radius: var(--r-pill);
		overflow: hidden;
	}
	.seg button {
		min-height: 36px;
		padding: 0 var(--s-3);
		font-size: var(--text-sm);
		color: var(--text-muted);
	}
	.seg button[aria-pressed='true'] {
		background: var(--surface-3);
		color: var(--text);
	}
	.list-table {
		width: 100%;
		border-collapse: collapse;
	}
	.list-table th {
		text-align: left;
		font-size: var(--text-xs);
		color: var(--text-muted);
		font-weight: var(--weight-medium);
		padding: var(--s-2);
		border-bottom: 1px solid var(--line);
	}
	.list-table td {
		padding: var(--s-2);
		border-bottom: 1px solid var(--line);
		vertical-align: middle;
	}
	.row-link {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		border-radius: var(--r-sm);
	}
	.thumb {
		width: 36px;
		flex: none;
	}
	@media (max-width: 600px) {
		.wide {
			display: none;
		}
		.tools-end,
		.find {
			width: 100%;
		}
		.find {
			flex: 1;
		}
	}
	.pill {
		min-height: 36px;
		padding: 0 var(--s-4);
		border-radius: var(--r-pill);
		border: 1px solid var(--line-strong);
		color: var(--text-muted);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		transition:
			background var(--dur-fast),
			color var(--dur-fast),
			border-color var(--dur);
	}
	.pill:hover {
		color: var(--text);
		background: var(--surface-3);
	}
	.pill[aria-pressed='true'] {
		background: var(--ha-red-soft);
		border-color: var(--ha-red);
		color: var(--text);
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: var(--s-5) var(--s-4);
	}
	.item {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		border-radius: var(--r-btn);
	}
	.item :global(.poster) {
		transition:
			transform var(--dur) var(--ease-out),
			border-color var(--dur);
	}
	.item:hover :global(.poster) {
		transform: translateY(var(--lift));
		border-color: var(--line-strong);
	}
	.name {
		font-weight: var(--weight-semibold);
		font-size: var(--text-sm);
		line-height: 1.3;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.meta {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-2);
	}
	@media (max-width: 767px) {
		.grid {
			grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
			gap: var(--s-4) var(--s-3);
		}
	}
</style>
