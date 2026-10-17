<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { page } from '$app/state';
	import { beforeNavigate, goto } from '$app/navigation';
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
	let searchPending = $state(false);
	let restoreScroll = $state<{ x: number; y: number } | null>(null);
	let lastOpenedId = $state<number | null>(null);
	function scrollKey() {
		const url = page.shallow?.url ?? page.url;
		return `${url.pathname}${url.search}|${view}`;
	}
	beforeNavigate(({ to: target }) => {
		if (!target || target.url.pathname === page.url.pathname) return;
		try {
			const suffix = target.url.pathname.slice(page.url.pathname.length + 1);
			const titleId = target.url.pathname.startsWith(`${page.url.pathname}/`) && /^\d+$/.test(suffix) ? Number(suffix) : null;
			const saved = { key: scrollKey(), x: window.scrollX, y: window.scrollY, titleId };
			sessionStorage.setItem('holyarrs.preview.library-scroll', JSON.stringify(saved));
			const url = page.shallow?.url ?? page.url;
			const views = JSON.parse(sessionStorage.getItem('holyarrs.preview.library-views') ?? '{}');
			views[kind] = { ...saved, href: url.pathname + url.search };
			sessionStorage.setItem('holyarrs.preview.library-views', JSON.stringify(views));
		} catch { /* Scroll memory is optional when storage is unavailable. */ }
	});
	$effect(() => {
		if (!loaded || searchPending || !restoreScroll) return;
		const position = restoreScroll;
		restoreScroll = null;
		void tick().then(() => requestAnimationFrame(() => window.scrollTo({ left: position.x, top: position.y, behavior: 'instant' })));
	});
	const startFilter = page.url.searchParams.get('show') as Filter | null;
	let filter = $state<Filter>(startFilter && filters.some(([f]) => f === startFilter) ? startFilter : 'all');
	const startKind = page.url.searchParams.get('kind') as Kind | null;
	let kind = $state<Kind>(startKind && kinds.some(([k]) => k === startKind) ? startKind : 'all');
	let q = $state(page.url.searchParams.get('q')?.slice(0, 256) ?? '');
	let sort = $state(page.url.searchParams.get('sort') === 'year' ? 'year' : 'name');
	let requesterFilter = $state(readRequester(page.url.searchParams.get('requester')));
	function readRequester(value: string | null): string {
		return value === 'me' || (value && /^[1-9]\d*$/.test(value)) ? value : 'anyone';
	}
	$effect(() => {
		const params = (page.shallow?.url ?? page.url).searchParams;
		const selectedKind = params.get('kind') as Kind;
		const selectedStatus = params.get('show') as Filter;
		kind = kinds.some(([k]) => k === selectedKind) ? selectedKind : 'all';
		filter = filters.some(([f]) => f === selectedStatus) ? selectedStatus : 'all';
		q = params.get('q')?.slice(0, 256) ?? '';
		requesterFilter = readRequester(params.get('requester'));
		sort = params.get('sort') === 'year' ? 'year' : 'name';
	});
	let myId = $state<number | undefined>();
	const requesterScope = $derived(titles.filter((t) => ofKind(t, kind) && matches(t, filter) && matchesTitleQuery(t, query, indexedIds)));
	const requesterCounts = $derived.by(() => {
		const counts = new Map<number, number>();
		for (const title of requesterScope) {
			for (const person of linkedPeople(title)) counts.set(person.id, (counts.get(person.id) ?? 0) + 1);
		}
		return counts;
	});
	const requesters = $derived([...new Map(titles.flatMap((t) => linkedPeople(t)).map((p) => [p.id, p])).values()].sort((a, b) => a.name.localeCompare(b.name)));
	const query = $derived(titleQuery(q));
	const moviesOn = $derived(partsNow.answer?.parts.find((part) => part.part === 'movies')?.on !== false);
	let view = $state<'grid' | 'list'>(readView());
	let previewId = $state<number | null>(null);
	let previewReturn = $state<HTMLButtonElement | null>(null);
	let previewHeading = $state<HTMLHeadingElement | null>(null);
	let findInput = $state<HTMLInputElement | null>(null);
	let activeFilters = $state<HTMLDivElement | null>(null);
	const preview = $derived(previewId === null ? null : (titles.find((title) => title.id === previewId) ?? null));
	const kindLabel: Record<string, string> = { movie: 'Movie', show: 'Show', artist: 'Artist' };

	function readView(): 'grid' | 'list' {
		try {
			return localStorage.getItem('holyarrs.library-view') === 'list' ? 'list' : 'grid';
		} catch {
			return 'grid';
		}
	}
	function titleLink(id: number): string {
		const url = page.shallow?.url ?? page.url;
		return to(`/library/${id}?returnTo=${encodeURIComponent(url.pathname + url.search)}`);
	}
	function setView(v: 'grid' | 'list') {
		view = v;
		try {
			localStorage.setItem('holyarrs.library-view', v);
		} catch {
			// Not remembered in a private window; still works.
		}
	}
	async function openPreview(title: TitleView, event: MouseEvent) {
		previewId = title.id;
		previewReturn = event.currentTarget as HTMLButtonElement;
		await tick();
		previewHeading?.focus();
	}
	async function closePreview() {
		previewId = null;
		await tick();
		if (previewReturn?.isConnected) previewReturn.focus();
		else findInput?.focus();
	}
	function progressOf(title: TitleView): string {
		if (title.kind === 'show') return showProgress(title.show);
		if (title.kind === 'artist') return artistProgress(title.artist);
		return `${title.slots.length} Version${title.slots.length === 1 ? '' : 's'}`;
	}
	function setFilter(f: Filter) {
		filter = f;
		syncFilterUrl();
	}

	function setKind(k: Kind) {
		kind = k;
		syncFilterUrl();
	}
	function syncFilterUrl() {
		const url = new URL((page.shallow?.url ?? page.url).href);
		if (kind === 'all') url.searchParams.delete('kind');
		else url.searchParams.set('kind', kind);
		if (filter === 'all') url.searchParams.delete('show');
		else url.searchParams.set('show', filter);
		if (q) url.searchParams.set('q', q);
		else url.searchParams.delete('q');
		if (requesterFilter !== 'anyone') url.searchParams.set('requester', requesterFilter);
		else url.searchParams.delete('requester');
		if (sort !== 'name') url.searchParams.set('sort', sort);
		else url.searchParams.delete('sort');
		void goto(url, { replace: true, shallow: true, reset: false });
	}
	function clearFilters() {
		q = '';
		requesterFilter = 'anyone';
		filter = 'all';
		kind = 'all';
		syncFilterUrl();
	}
	async function removeFilter(which: 'type' | 'status' | 'requester' | 'search') {
		if (which === 'type') setKind('all');
		else if (which === 'status') setFilter('all');
		else if (which === 'requester') requesterFilter = 'anyone';
		else q = '';
		if (which === 'requester' || which === 'search') syncFilterUrl();
		await tick();
		const next = activeFilters?.querySelector('button');
		if (next) next.focus();
		else findInput?.focus();
	}

	const requestedTitles = $derived(titles.filter((t) => requesterFilter === 'anyone' || linkedPeople(t).some((p) => p.id === (requesterFilter === 'me' ? myId : Number(requesterFilter)))));
	const ofThisKind = $derived(requestedTitles.filter((t) => ofKind(t, kind)));
	const tally = $derived(counts(ofThisKind));
	const kindTally = $derived({
		all: requestedTitles.length,
		movie: requestedTitles.filter((t) => t.kind === 'movie').length,
		show: requestedTitles.filter((t) => t.kind === 'show').length,
		artist: requestedTitles.filter((t) => t.kind === 'artist').length
	});
	const kindWord: Record<string, string> = { show: ' · Show', artist: ' · Artist' };
	const shown = $derived(ofThisKind.filter((t) => matches(t, filter) && matchesTitleQuery(t, query, indexedIds)).toSorted((a, b) => sort === 'year' ? (b.year ?? 0) - (a.year ?? 0) || a.name.localeCompare(b.name) : a.name.localeCompare(b.name)));

	$effect(() => {
		loadRevision;
		if (!mounted) return;
		const run = ++searchRun;
		indexedIds = new Set();
		searchError = '';
		searchPending = !!query;
		if (!query) return;
		const controller = new AbortController();
		const timer = setTimeout(async () => {
			try {
				const found = await api.get<{ titles: TitleView[] }>(`/titles?q=${encodeURIComponent(query)}`, controller.signal);
				if (run === searchRun) indexedIds = new Set(found.titles.map((t) => t.id));
			} catch (e) {
				if (!controller.signal.aborted && run === searchRun) searchError = message(e);
			} finally {
				if (run === searchRun) searchPending = false;
			}
		}, 200);
		return () => {
			clearTimeout(timer);
			controller.abort();
		};
	});

	function linkedPeople(t: TitleView) {
		const people = new Map<number, { id: number; name: string }>();
		for (const ask of t.asks ?? []) {
			if (ask.title_id !== t.id) continue;
			for (const person of [ask.asked_by, ...ask.people]) {
				if (person.name.trim()) people.set(person.id, person);
			}
		}
		return [...people.values()];
	}
	function requestedBy(t: TitleView): string {
		return linkedPeople(t).map((p) => p.name).join(', ');
	}

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
			const [library, me] = await Promise.all([api.get<{ titles: TitleView[] }>('/titles'), api.get<{ id?: number }>('/auth/me')]);
			titles = library.titles;
			myId = me.id;
			error = '';
			loadRevision++;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	async function retryLoad() {
		loaded = false;
		await load();
	}

	onMount(() => {
		mounted = true;
		try {
			const views = JSON.parse(sessionStorage.getItem('holyarrs.preview.library-views') ?? '{}');
			const saved = views[kind] ?? JSON.parse(sessionStorage.getItem('holyarrs.preview.library-scroll') ?? 'null');
			if (saved?.key === scrollKey() && Number.isFinite(saved.x) && Number.isFinite(saved.y) && saved.x >= 0 && saved.y >= 0) {
				restoreScroll = { x: saved.x, y: saved.y };
				lastOpenedId = Number.isSafeInteger(saved.titleId) && saved.titleId > 0 ? saved.titleId : null;
			}
		} catch { /* Start normally if a saved position is unavailable. */ }
		void load();
		const offs = [live.on('title', load), live.on('slot', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
</script>

<svelte:head><title>Library · HolyArrs</title></svelte:head>

<div class="library-search-band"><div class="library-search-box"><label class="sr-only" for="find">Find in the Library</label><input id="find" class="input find" type="search" bind:this={findInput} bind:value={q} oninput={(event) => { q = event.currentTarget.value; syncFilterUrl(); }} placeholder="Search Library" autocomplete="off" maxlength="256" /><button class="clear-search" type="button" onclick={clearFilters}>Clear filters</button></div>	{#if session.can('manage_library')}
		<details class="library-tools">
			<summary class="btn btn-red">+ Add</summary>
			<nav class="tools-menu" aria-label="Library tools">
				<a href={to(`/search`)}>Add a movie</a>
				<a href={to(`/search?kind=show`)}>Add a show</a>
				<a href={to(`/search?kind=music`)}>Add an artist</a>
				<span class="menu-rule"></span>
				<a href={to(`/library/held`)}>Held files</a>
				<a href={to(`/library/turned-down`)}>Turned down releases</a>
				<a href={to(`/library/missing-subtitles`)}>Missing subtitles</a>
				<a href={to(`/library/rename`)}>Rename files</a>
				<a href={to(`/library/import`)}>Import a library</a>
			</nav>
		</details>
	{/if}</div>
<header class="library-heading">
	<div>
		<p class="section-kicker">Media collection</p>
		<h1>Library</h1>
		<p class="library-summary">{shown.length} {shown.length === 1 ? 'title' : 'titles'} in this view</p>
	</div>
	<div class="library-view-tools"><label class="sr-only" for="library-sort">Sort titles</label><select id="library-sort" class="sort-select" bind:value={sort} onchange={(event) => { sort = event.currentTarget.value; syncFilterUrl(); }}><option value="name">Name</option><option value="year">Newest year</option></select><div class="seg" role="group" aria-label="Layout"><button type="button" aria-pressed={view==='grid'} onclick={() => setView('grid')}>Posters</button><button type="button" aria-pressed={view==='list'} onclick={() => setView('list')}>List</button></div></div>
</header>
<div class="library-commandbar">
	<div class="filter-toolbar">
		<details class="filter-menu"><summary class="filter-trigger">Type</summary><div class="filter-popover" role="group" aria-label="Kind">{#each kinds as [key,label] (key)}<button type="button" aria-pressed={kind===key} onclick={() => setKind(key)}>{label}<span>{kindTally[key]}</span></button>{/each}</div></details>
		<details class="filter-menu status-filter"><summary class="filter-trigger" class:has-filter={filter!=='all'}>Status</summary><div class="filter-popover status-popover"><label for="status-value">Show titles matching</label><select id="status-value" class="input" value={filter} onchange={(event) => setFilter(event.currentTarget.value as Filter)}>{#each filters as [key,label] (key)}<option value={key}>{label} ({tally[key]})</option>{/each}</select></div></details>
		<details class="filter-menu status-filter requester-menu"><summary class="filter-trigger requester-trigger" class:has-filter={requesterFilter !== 'anyone'}>Requested by{requesterFilter === 'me' ? ': Me' : requesterFilter !== 'anyone' ? `: ${requesters.find((p) => String(p.id) === requesterFilter)?.name ?? 'Person'}` : ''}</summary><div class="filter-popover status-popover"><label for="requester-value">Requested by</label><select id="requester-value" class="input" bind:value={requesterFilter} onchange={(event) => { requesterFilter = event.currentTarget.value; syncFilterUrl(); }}><option value="anyone">Anyone ({requesterScope.length})</option><option value="me" disabled={!myId}>Me ({myId ? requesterCounts.get(myId) ?? 0 : 0})</option>{#each requesters as person (person.id)}<option value={String(person.id)}>{person.name} ({requesterCounts.get(person.id) ?? 0})</option>{/each}</select></div></details>
		{#if moviesOn}<a class="filter-trigger" href={to('/library/collections')}>Movie series</a><a class="filter-trigger" href={to('/people')}>People</a>{#if session.can('manage_library')}<a class="filter-trigger" href={to('/library/missing-subtitles')}>Subtitles</a><a class="filter-trigger" href={to('/library/copies')}>File codecs</a>{/if}{/if}
	</div>
	<p class="asset-count">{shown.length} {shown.length===1?'TITLE':'TITLES'}</p>
</div>
{#if kind !== 'all' || filter !== 'all' || requesterFilter !== 'anyone' || q}
	<div class="active-filters" role="group" aria-label="Active filters" bind:this={activeFilters}>
		{#if kind !== 'all'}<button type="button" class="active-filter" aria-label="Remove type filter" onclick={() => removeFilter('type')}>Type: {kinds.find(([key]) => key === kind)?.[1]} <span aria-hidden="true">×</span></button>{/if}
		{#if filter !== 'all'}<button type="button" class="active-filter" aria-label="Remove status filter" onclick={() => removeFilter('status')}>Status: {filters.find(([key]) => key === filter)?.[1]} <span aria-hidden="true">×</span></button>{/if}
		{#if requesterFilter !== 'anyone'}<button type="button" class="active-filter" aria-label="Remove requester filter" onclick={() => removeFilter('requester')}>Requested by: {requesterFilter === 'me' ? 'Me' : requesters.find((p) => String(p.id) === requesterFilter)?.name ?? 'Person'} <span aria-hidden="true">×</span></button>{/if}
		{#if q}<button type="button" class="active-filter" aria-label="Remove search filter" onclick={() => removeFilter('search')}>Search: {q} <span aria-hidden="true">×</span></button>{/if}
	</div>
{/if}
{#if searchError && !error}<p class="notice bad" role="alert">{searchError}</p>{/if}


<div class:with-preview={preview} class="library-workspace">
	<div class="library-content">
	{#if !loaded}
		<div class="grid">
			{#each Array(6) as _, i (i)}<div class="sk" style="aspect-ratio: 2 / 3"></div>{/each}
		</div>
	{:else if error}
		<div class="error-state">
			<p class="notice bad" role="alert">{error}</p>
			<button class="btn btn-ghost" type="button" onclick={retryLoad}>Try again</button>
		</div>
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
		<!-- Keyboard access lets this region scroll without a pointer. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<div class="list-scroll" role="region" aria-label="Library titles" tabindex="0">
		<table class="list-table">
		<caption class="sr-only">Titles in the Library</caption>
		<thead>
			<tr><th scope="col">Title</th><th scope="col">State</th><th scope="col" class="wide">Out on</th><th scope="col" class="wide">Keep up to date</th></tr>
		</thead>
		<tbody>
			{#each shown as t (t.id)}
				{@const requester = requestedBy(t)}
				<tr data-testid="library-row" class:selected={preview?.id === t.id} class:last-opened={lastOpenedId === t.id}>
					<td class="title-cell">{#if lastOpenedId === t.id}<span class="last-opened-label" data-testid="last-opened">Last opened</span>{/if}<a class="row-link" href={titleLink(t.id)}><span class="thumb"><Poster src={t.artwork[0]} name={t.name} /></span><span><b>{t.name}</b> <span class="muted xs">{t.year ?? ''}{kindWord[t.kind] ?? ''}</span></span></a><button class="preview-button list-preview" type="button" aria-label={`Preview ${t.name}`} aria-pressed={preview?.id === t.id} onclick={(event) => openPreview(t, event)}>Preview</button>{#if requester}<span class="requester xs muted" data-testid="requested-by">Requested by {requester}</span>{/if}</td>
					{#if t.kind === 'artist'}
						{@const c = chipOf(t)}
						<td>{#if t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}<span class="chip" data-testid="request-approved" title="Requested and approved">Approved</span>{/if}{#if c}<span class="chip {tone(c.state)}">{c.label}</span>{/if}<span class="xs muted block">{artistProgress(t.artist)}</span></td>
						<td class="wide small muted">{t.artist?.next_release ? `Next album ${date(t.artist.next_release)}` : t.artist?.status === 'ended' ? 'No longer active' : 'No date yet'}</td>
					{:else if t.kind === 'show'}
						{@const c = chipOf(t)}
						<td>{#if t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}<span class="chip" data-testid="request-approved" title="Requested and approved">Approved</span>{/if}{#if c}<span class="chip {tone(c.state)}">{c.label}</span>{/if}<span class="xs muted block">{showProgress(t.show)}</span></td>
						<td class="wide small muted">{t.show?.next_air_date ? `Next episode ${date(t.show.next_air_date)}` : t.show?.status === 'ended' ? 'Ended' : 'No date yet'}</td>
					{:else}
						<td>{#if t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}<span class="chip" data-testid="request-approved" title="Requested and approved">Approved</span>{/if}{#each t.slots as s (s.id)}<span class="chip {tone(s.state)}">{s.state_label}</span>{/each}</td>
						<td class="wide small muted">{date(t.digital_date ?? t.physical_date ?? t.cinema_date) || 'Not known'}</td>
					{/if}
					<td class="wide small">{t.keep_up_to_date ? 'Yes' : 'No'}</td>
				</tr>
			{/each}
		</tbody>
		</table>
		</div>
	{:else}
		<ul class="grid">
		{#each shown as t, i (t.id)}
			{@const c = chipOf(t)}
			{@const requester = requestedBy(t)}
			<li class:selected={preview?.id === t.id} class:last-opened={lastOpenedId === t.id} class="rise" style="--i: {i}">
				<a class="item" href={titleLink(t.id)} data-testid="library-card">
					<Poster src={t.artwork[0]} name={t.name} />
					<span class="name">{t.name}</span>
					{#if lastOpenedId === t.id}<span class="last-opened-label" data-testid="last-opened">Last opened</span>{/if}
					<span class="meta">
						<span class="muted xs">{t.year ?? ''}{kindWord[t.kind] ?? ''}</span>
						{#if t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}
							<span class="chip" data-testid="request-approved" title="Requested and approved">Approved</span>
						{/if}
						{#if c}<span class="chip {tone(c.state)}">{c.label}</span>{/if}
					</span>
					{#if requester}<span class="requester xs muted" data-testid="requested-by">Requested by {requester}</span>{/if}
				</a>
				<button class="preview-button" type="button" aria-label={`Preview ${t.name}`} aria-pressed={preview?.id === t.id} onclick={(event) => openPreview(t, event)}>Preview</button>
			</li>
		{/each}
		</ul>
	{/if}
	</div>
	{#if preview}
		{@const previewChip = chipOf(preview)}
		<aside class="preview" aria-labelledby="preview-title">
			<div class="preview-head">
				<span class="small muted">Title preview</span>
				<button class="preview-close" type="button" onclick={closePreview}>Close</button>
			</div>
			<div class="preview-art"><Poster src={preview.artwork[0]} name={preview.name} /></div>
			<p class="preview-kind">{kindLabel[preview.kind] ?? preview.kind}{preview.year ? ` · ${preview.year}` : ''}</p>
			<h2 id="preview-title" bind:this={previewHeading} tabindex="-1">{preview.name}</h2>
			{#if previewChip}<span class="chip {tone(previewChip.state)}">{previewChip.label}</span>{/if}
			<p class="small muted preview-progress">{progressOf(preview)}</p>
			<dl class="preview-facts">
				<dt>Keep up to date</dt><dd>{preview.keep_up_to_date ? 'Yes' : 'No'}</dd>
				<dt>Out on</dt>
				<dd>
					{preview.kind === 'show'
						? preview.show?.next_air_date ? date(preview.show.next_air_date) : preview.show?.status === 'ended' ? 'Ended' : 'Not known'
						: preview.kind === 'artist'
							? preview.artist?.next_release ? date(preview.artist.next_release) : preview.artist?.status === 'ended' ? 'No longer active' : 'Not known'
							: date(preview.digital_date ?? preview.physical_date ?? preview.cinema_date) || 'Not known'}
				</dd>
			</dl>
			{#if preview.slots.length}
				<div class="preview-section">
					<h3>Versions</h3>
					<ul>
						{#each preview.slots as slot (slot.id)}
							<li><span>{slot.version || 'Main'}</span><span class="chip {tone(slot.state)}">{slot.state_label}</span></li>
						{/each}
					</ul>
				</div>
			{/if}
			<a class="btn btn-red preview-open" href={titleLink(preview.id)}>Open title</a>
		</aside>
	{/if}
</div>

<style>
	.block {
		display: block;
		margin-top: 2px;
	}
	.library-tools {
		position: relative;
		flex: none;
	}
	.library-tools summary {
		list-style: none;
		cursor: pointer;
	}
	.library-tools summary::-webkit-details-marker { display: none; }
	.tools-menu {
		position: absolute;
		top: calc(100% + var(--s-2));
		right: 0;
		z-index: 10;
		display: grid;
		min-width: 230px;
		padding: var(--s-2);
		border: 1px solid var(--line-strong);
		border-radius: 12px;
		background: var(--surface-1);
		box-shadow: var(--shadow-modal);
	}
	.tools-menu a {
		display: flex;
		align-items: center;
		min-height: 38px;
		padding: 0 var(--s-3);
		border-radius: 4px;
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.tools-menu a:hover,
	.tools-menu a:focus-visible { color: var(--text); background: var(--surface-3); }
	.menu-rule { height: 1px; margin: var(--s-2); background: var(--line); }
	.library-workspace,
	.library-content { min-width: 0; }
	.error-state { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); flex-wrap: wrap; }
	.error-state .notice { flex: 1 1 320px; margin: 0; }
	.preview { display: none; }
	.preview-button { display: none; }
	.grid > li { position: relative; min-width: 0; }
	.grid > li.selected .item { border-color: color-mix(in srgb, var(--ha-red) 60%, var(--line)); background: var(--surface-1); }
	.list-table tr.selected { background: var(--surface-2); }
	.title-cell { min-width: 250px; }
	.title-cell .row-link { display: inline-flex; }
	.preview-head { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); }
	.preview-close {
		min-height: 34px;
		padding: 0 var(--s-2);
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.preview-close:hover { color: var(--text); }
	.preview-art { width: min(168px, 64%); margin: var(--s-4) auto var(--s-5); }
	.preview-kind { color: var(--text-muted); font-size: var(--text-xs); }
	.preview h2 { margin: 3px 0 var(--s-3); font: var(--weight-semibold) var(--text-lg) / var(--leading-tight) var(--font-display); overflow-wrap: anywhere; }
	.preview-progress { margin-top: var(--s-2); }
	.preview-facts { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: var(--s-2) var(--s-3); margin-top: var(--s-4); padding-top: var(--s-4); border-top: 1px solid var(--line); font-size: var(--text-sm); }
	.preview-facts dt { color: var(--text-muted); }
	.preview-facts dd { margin: 0; text-align: right; overflow-wrap: anywhere; }
	.preview-section { margin-top: var(--s-5); padding-top: var(--s-3); border-top: 1px solid var(--line); }
	.preview-section h3 { margin-bottom: var(--s-2); color: var(--text-muted); font-size: var(--text-xs); font-weight: var(--weight-medium); }
	.preview-section ul { display: grid; gap: var(--s-2); }
	.preview-section li { display: flex; align-items: center; justify-content: space-between; gap: var(--s-2); min-width: 0; font-size: var(--text-sm); }
	.preview-open { width: 100%; margin-top: var(--s-5); }
	@media (min-width: 1280px) {
		.library-workspace.with-preview { display: grid; grid-template-columns: minmax(0, 1fr) 360px; gap: 0; align-items: start; border: 1px solid var(--line); border-radius: 14px; overflow: clip; }
		.library-workspace.with-preview .library-content { border-right: 1px solid var(--line); }
		.library-workspace.with-preview .list-table .wide { display: none; }
		.preview { display: block; position: sticky; top: var(--s-3); padding: var(--s-5); background: var(--surface-1); }
		.preview-button { display: inline-flex; align-items: center; justify-content: center; min-height: 36px; padding: 0 var(--s-3); border: 1px solid var(--line-strong); border-radius: 10px; background: var(--surface-1); color: var(--text-muted); font-size: var(--text-xs); }
		.preview-button:hover, .preview-button[aria-pressed='true'] { border-color: color-mix(in srgb, var(--ha-red) 60%, var(--line-strong)); color: var(--text); background: var(--surface-2); }
		.grid > li > .preview-button { position: absolute; top: 12px; right: 12px; opacity: 0; transition: opacity var(--dur-fast); }
		.grid > li:hover > .preview-button, .grid > li:focus-within > .preview-button, .grid > li > .preview-button[aria-pressed='true'] { opacity: 1; }
		.title-cell { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); }
		.list-preview { flex: none; }
	}
	.n {
		margin-left: 4px;
		font-variant-numeric: tabular-nums;
	}
	.seg {
		display: inline-flex;
		padding: 3px;
		border: 1px solid var(--line);
		border-radius: 11px;
		background: var(--surface-2);
		overflow: hidden;
	}
	.seg button {
		min-height: 34px;
		padding: 0 var(--s-3);
		border-radius: 8px;
		font-size: var(--text-sm);
		color: var(--text-muted);
	}
	.seg button[aria-pressed='true'] {
		background: var(--surface-1);
		color: var(--text);
		box-shadow: 0 1px 2px rgb(20 24 18 / 10%);
	}
	.list-table {
		width: 100%;
		border-collapse: collapse;
		border: 1px solid var(--line);
		border-radius: 12px;
		background: var(--surface-1);
		overflow: hidden;
	}
	.list-table th {
		text-align: left;
		font-size: var(--text-xs);
		color: var(--text-muted);
		font-weight: var(--weight-medium);
		padding: 11px var(--s-4);
		border-bottom: 1px solid var(--line);
	}
	.list-table td {
		padding: 10px var(--s-4);
		border-bottom: 1px solid var(--line);
		vertical-align: middle;
	}
	.list-table tbody tr:hover { background: var(--surface-2); }
	.row-link {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		border-radius: var(--r-sm);
	}
	.thumb {
		width: 44px;
		flex: none;
	}
	@media (max-width: 600px) {
		.wide {
			display: none;
		}

	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(176px, 1fr));
		gap: var(--s-5) var(--s-4);
	}
	.item {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		height: 100%;
		padding: 0;
		border: 1px solid transparent;
		border-radius: 13px;
		transition: border-color var(--dur-fast), background var(--dur-fast), transform var(--dur-fast);
	}
	.item :global(.poster) {
		transition:
			transform var(--dur) var(--ease-out),
			border-color var(--dur);
	}
	.item:hover { transform: translateY(-1px); border-color: var(--line-strong); background: var(--surface-1); box-shadow: 0 3px 12px rgb(20 24 18 / 7%); }
	.item:hover :global(.poster) { border-color: color-mix(in srgb, var(--ha-red) 42%, var(--line-strong)); }
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
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-2);
	}
	.meta > .muted { flex-basis: 100%; }
	.requester { display: block; overflow-wrap: anywhere; white-space: normal; }
	.grid > .last-opened { outline: 2px solid var(--text-muted); outline-offset: 4px; border-radius: 10px; }
	.list-table tr.last-opened { background: var(--surface-2); outline: 2px solid var(--text-muted); outline-offset: -2px; }
	.last-opened-label { display: block; font-size: var(--text-xs); font-weight: 600; color: var(--text); }
	.active-filters { display: flex; flex-wrap: wrap; gap: var(--s-2); margin-bottom: var(--s-4); }
	.active-filter { display: inline-flex; align-items: center; gap: var(--s-2); min-height: 44px; max-width: 100%; padding: 6px 12px; border: 1px solid var(--line); border-radius: 999px; background: var(--surface-2); color: var(--text); font-size: var(--text-sm); text-align: left; overflow-wrap: anywhere; cursor: pointer; }
	.active-filter > span { flex: none; font-size: 18px; }
	@media (max-width: 767px) {
		.library-tools { width: 100%; }
		.library-tools summary { width: 100%; min-height: var(--tap); }
		.tools-menu { position: static; margin-top: var(--s-2); box-shadow: none; }
		.tools-menu a { min-height: var(--tap); }

		.grid {
			grid-template-columns: repeat(auto-fill, minmax(116px, 1fr));
			gap: var(--s-4) var(--s-3);
		}
	}
	@media (pointer: coarse) {
		.seg button,
		.preview-button,
		.preview-close,
		.library-tools summary,
		.tools-menu a { min-height: 44px; }
	}

.library-search-band{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:20px}
.library-search-box{display:flex;align-items:center;min-width:0;width:min(760px,100%);background:var(--surface-1);border:1px solid var(--line);border-radius:12px;padding:4px 8px;box-shadow:0 1px 2px rgb(20 24 18 / 4%)}
.find{flex:1;min-width:0;width:100%;background:transparent;border:0}
.clear-search{flex:none;border:1px solid var(--line);border-radius:9px;padding:4px 10px;font-size:var(--text-sm)}
.library-commandbar{display:flex;align-items:center;justify-content:space-between;gap:16px;min-height:64px;padding:10px 0 14px;border-bottom:1px solid var(--line);margin-bottom:20px}
.filter-toolbar{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.filter-menu{position:relative}
.filter-trigger{display:flex;align-items:center;gap:18px;min-height:38px;padding:4px 12px;border:1px solid var(--line);border-radius:10px;font-size:var(--text-sm);font-weight:500;cursor:pointer;background:var(--surface-1);list-style:none}
.filter-menu[open] > .filter-trigger.requester-trigger, .filter-trigger.requester-trigger.has-filter, .filter-trigger.requester-trigger { color: var(--text); max-width: 100%; overflow-wrap: anywhere; }
.filter-trigger::-webkit-details-marker{display:none}
.filter-menu[open]>.filter-trigger,.has-filter{color:var(--text);border-color:var(--accent)}
.filter-popover{position:absolute;z-index:12;top:calc(100% + 8px);left:0;width:260px;padding:10px;background:var(--surface-1);border:1px solid var(--line);border-radius:12px;box-shadow:var(--shadow-float)}
.filter-popover button{display:flex;justify-content:space-between;align-items:center;gap:20px;width:100%;min-height:38px;padding:6px 10px;text-align:left;border-radius:4px}
.filter-popover button:hover{background:var(--surface-2)}
.filter-popover button[aria-pressed='true']{background:var(--accent-soft);color:var(--accent)}
.library-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:20px;padding:6px 0 18px;border-bottom:1px solid var(--line)}
.library-heading h1{font-size:28px;font-weight:620;line-height:1.15;letter-spacing:-.025em}
.section-kicker{margin-bottom:5px;color:var(--text-muted);font-size:12px;font-weight:600;letter-spacing:.04em;text-transform:uppercase}
.library-summary{margin-top:7px;color:var(--text-muted);font-size:14px}
.asset-count{flex:none;font-size:12px;font-weight:600;color:var(--text-muted);font-variant-numeric:tabular-nums}
.grid .item :global(.poster){border-radius:12px}
@media(min-width:1024px){.library-search-band{position:absolute;top:-72px;left:0;right:10px;height:52px;margin:0}.library-search-box{position:absolute;left:max(0px,calc(50vw - var(--sidebar) - 380px));width:min(760px,calc(100% - 130px))}.library-search-band>.library-tools{margin-left:auto}}
@media(max-width:767px){.library-commandbar{position:relative}.requester-menu{position:static;max-width:100%}.requester-menu .filter-popover{top:100%;left:0}.library-search-band{flex-wrap:wrap}.clear-search{font-size:12px;padding-inline:6px}.filter-trigger{min-height:44px}.filter-popover{width:min(260px,calc(100vw - 48px))}}
@media(pointer:coarse){.filter-popover button,.clear-search{min-height:44px}}

.status-popover{width:400px;padding:18px 16px;display:grid;gap:12px}
.status-popover label{font-size:14px;font-weight:500;padding-inline:8px}
.status-popover .input{min-height:48px;border-color:var(--line);background:var(--surface-1)}
.library-view-tools{display:flex;align-items:center;gap:20px}
.sort-select{background:transparent;border:0;color:var(--text-muted);font:inherit;font-size:13px;min-height:36px;max-width:140px}
.list-scroll{max-width:100%;overflow-x:auto}
@media(min-width:1400px){.grid{grid-template-columns:repeat(5,minmax(0,1fr));gap:24px 20px}}
@media(max-width:767px){.status-popover{width:min(400px,calc(100vw - 100px))}.library-view-tools{gap:8px}.library-heading{align-items:flex-start;flex-direction:column}.library-commandbar{align-items:flex-start;flex-direction:column}.asset-count{order:-1}}
</style>
