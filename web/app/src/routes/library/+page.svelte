<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { page } from '$app/state';
	import { beforeNavigate, goto } from '$app/navigation';
	import { date } from '#lib/format.ts';
	import { counts, filters, kinds, matches, matchesTitleQuery, ofKind, titleQuery, type Filter, type Kind } from '#lib/library.ts';
	import { showProgress, showState } from '#lib/shows.ts';
	import { artistProgress, artistState } from '#lib/music.ts';
	import Poster from '#lib/Poster.svelte';
	import Icon from '#lib/Icon.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import ListRow from '#lib/ui/ListRow.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { artName, slotKind } from '#lib/title-page.ts';
	import { peopleNames, plainReason } from '#lib/plain.ts';
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
			const titleId = target.url.pathname.startsWith(`${page.url.pathname}/`) && /^\d+$/.test(suffix) ? Number(suffix) : lastOpenedId;
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
	let sort = $state<'name' | 'year'>(page.url.searchParams.get('sort') === 'year' ? 'year' : 'name');
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
		// Names that look like internal ids read as "someone in your household".
		return peopleNames(linkedPeople(t).map((p) => p.name));
	}
	function outOn(t: TitleView, short = false): string {
		if (t.kind === 'show') return t.show?.next_air_date ? `${short ? '' : 'Next episode '}${date(t.show.next_air_date)}` : t.show?.status === 'ended' ? 'Ended' : short ? 'Not known' : 'No date yet';
		if (t.kind === 'artist') return t.artist?.next_release ? `${short ? '' : 'Next album '}${date(t.artist.next_release)}` : t.artist?.status === 'ended' ? 'No longer active' : short ? 'Not known' : 'No date yet';
		return date(t.digital_date ?? t.physical_date ?? t.cinema_date) || 'Not known';
	}
	function previewKind(t: TitleView): string {
		return `${kindLabel[t.kind] ?? t.kind}${t.year ? ` · ${t.year}` : ''}`;
	}
	function setSort(v: 'name' | 'year') {
		sort = v;
		syncFilterUrl();
	}
	function setRequester(v: string) {
		requesterFilter = v;
		syncFilterUrl();
	}
	const partOn = (name: string) => partsNow.answer?.parts.find((part) => part.part === name)?.on !== false;
	const filtersActive = $derived(kind !== 'all' || filter !== 'all' || requesterFilter !== 'anyone' || !!q);
	const requesterName = $derived(requesterFilter === 'me' ? 'Me' : (requesters.find((p) => String(p.id) === requesterFilter)?.name ?? 'Person'));
	const requesterText = $derived(requesterFilter === 'anyone' ? 'Requested by' : `Requested by: ${requesterName}`);
	const requesterItems = $derived<MenuItem[]>([
		{ label: `Anyone (${requesterScope.length})`, checked: requesterFilter === 'anyone', onselect: () => setRequester('anyone') },
		{ label: `Me (${myId ? (requesterCounts.get(myId) ?? 0) : 0})`, checked: requesterFilter === 'me', disabled: !myId, onselect: () => setRequester('me') },
		...requesters.map((person): MenuItem => ({ label: `${person.name} (${requesterCounts.get(person.id) ?? 0})`, checked: requesterFilter === String(person.id), onselect: () => setRequester(String(person.id)) }))
	]);
	const sortItems = $derived<MenuItem[]>([
		{ label: 'Name', checked: sort === 'name', onselect: () => setSort('name') },
		{ label: 'Newest year', checked: sort === 'year', onselect: () => setSort('year') }
	]);
	const browseItems = $derived<MenuItem[]>(
		moviesOn
			? [
					{ label: 'Movie series', icon: 'film', href: to('/library/collections') },
					{ label: 'People', icon: 'people', href: to('/people') },
					...(session.can('manage_library')
						? ([
								{ label: 'Subtitles', icon: 'subtitle', href: to('/library/missing-subtitles') },
								{ label: 'File codecs', icon: 'copy', href: to('/library/copies') }
							] as MenuItem[])
						: [])
				]
			: []
	);
	const addItems = $derived<MenuItem[]>([
		...(partOn('movies') ? ([{ label: 'Add a movie', icon: 'film', href: to('/search') }] as MenuItem[]) : []),
		...(partOn('shows') ? ([{ label: 'Add a show', icon: 'tv', href: to('/search?kind=show') }] as MenuItem[]) : []),
		...(partOn('music') ? ([{ label: 'Add an artist', icon: 'music', href: to('/search?kind=music') }] as MenuItem[]) : []),
		{ kind: 'separator' },
		{ kind: 'heading', label: 'Library tools' },
		{ label: 'Held files', href: to('/library/held') },
		{ label: 'Turned down releases', href: to('/library/turned-down') },
		...(partOn('subtitles') ? ([{ label: 'Missing subtitles', href: to('/library/missing-subtitles') }] as MenuItem[]) : []),
		{ label: 'Rename files', href: to('/library/rename') },
		{ label: 'Import a library', href: to('/library/import') }
	]);

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

<div class="library">
	<PageHeader title="Library" subtitle={loaded && !error ? `${shown.length} ${shown.length === 1 ? 'title' : 'titles'}${filtersActive ? ' match' : ''}` : undefined}>
		{#snippet actions()}
			{#if browseItems.length}<Menu label="Browse the Library" variant="button" text="Browse" icon="library" items={browseItems} width={240} />{/if}
			{#if session.can('manage_library')}<Menu label="Add and Library tools" variant="button" text="Add" icon="plus" items={addItems} width={250} />{/if}
		{/snippet}
	</PageHeader>

	<div class="toolbar">
		<div class="find-box">
			<span class="find-ic" aria-hidden="true"><Icon name="search" size={17} /></span>
			<label class="sr-only" for="find">Find in the Library</label>
			<input id="find" class="input find" type="search" bind:this={findInput} bind:value={q} oninput={(event) => { q = event.currentTarget.value; syncFilterUrl(); }} placeholder="Search Library" autocomplete="off" maxlength="256" />
		</div>
		<div class="view-tools">
			<Menu label="Sort titles" variant="button" text={sort === 'year' ? 'Newest year' : 'Name'} icon="chevronUpDown" items={sortItems} width={200} />
			<SegmentedControl label="Layout" value={view} options={[{ value: 'grid', label: 'Posters', icon: 'grid' }, { value: 'list', label: 'List', icon: 'menu' }]} onchange={(v) => setView(v)} />
		</div>
	</div>

	<div class="filters">
		<div class="chips" role="group" aria-label="Kind">
			{#each kinds as [key, label] (key)}<Chip {label} count={kindTally[key]} selected={kind === key} onclick={() => setKind(key)} />{/each}
		</div>
		<div class="chips" role="group" aria-label="Status">
			{#each filters as [key, label] (key)}<Chip {label} count={tally[key]} selected={filter === key} onclick={() => setFilter(key)} />{/each}
			<Menu label="Requested by" variant="button" text={requesterText} icon="user" items={requesterItems} width={260} align="start" />
		</div>
		{#if requesterFilter !== 'anyone' || q || filtersActive}
			<div class="chips active" role="group" aria-label="Active filters" bind:this={activeFilters}>
				{#if requesterFilter !== 'anyone'}<Chip label="Requested by: {requesterName}" selected onclick={() => removeFilter('requester')} onremove={() => removeFilter('requester')} />{/if}
				{#if q}<Chip label="Search: {q}" selected onclick={() => removeFilter('search')} onremove={() => removeFilter('search')} />{/if}
				{#if filtersActive}<Button variant="plain" size="md" onclick={clearFilters}>Clear</Button>{/if}
			</div>
		{/if}
	</div>
	{#if searchError && !error}<p class="search-error" role="alert"><Icon name="alert" size={16} />{plainReason(searchError).text}</p>{/if}

	<div class:with-preview={preview} class="workspace">
		<div class="content">
		{#if !loaded}
			<ul class="grid" aria-hidden="true">
				{#each Array(10) as _, i (i)}<li><Skeleton shape="poster" /><Skeleton width="70%" /></li>{/each}
			</ul>
		{:else if error}
			<LoadError hasData={false} message={plainReason(error).text} onretry={retryLoad} />
		{:else if titles.length === 0}
			<EmptyState icon="library" title="Your Library is empty" text={session.can('manage_library') ? 'Find a movie, a show or an artist on the Search page and add it. HolyArrs then keeps it up to date.' : 'Nothing has been added yet. Look for a movie, a show or an artist on the Search page.'} action={{ label: 'Search for a movie', href: to('/search'), icon: 'search' }} />
			<div class="empty-more">
				<Button href={to('/search?kind=show')}>Search for a show</Button>
				<Button href={to('/search?kind=music')}>Search for an artist</Button>
				{#if session.can('manage_library')}<Button href={to('/library/import')}>Import a library</Button>{/if}
				{#if session.can('settings')}<Button href={to('/settings/library-folders')}>Library folders</Button>{/if}
			</div>
		{:else if shown.length === 0}
			<EmptyState compact icon="search" title="Nothing matches this filter." action={filtersActive ? { label: 'Clear', onclick: clearFilters } : undefined} />
		{:else if view === 'list'}
			<ul class="rows" aria-label="Library titles">
				{#each shown as t (t.id)}
					{@const c = chipOf(t)}
					{@const requester = requestedBy(t)}
					<li data-testid="library-row" class:selected={preview?.id === t.id} class:last-opened={lastOpenedId === t.id}>
						<ListRow title={t.name} subtitle={[t.year, kindLabel[t.kind], t.kind === 'movie' ? '' : progressOf(t)].filter(Boolean).join(' · ')} meta="{outOn(t)} · Keep up to date: {t.keep_up_to_date ? 'Yes' : 'No'}" href={titleLink(t.id)} image={t.artwork[0]} icon={t.kind === 'show' ? 'tv' : t.kind === 'artist' ? 'music' : 'film'}>
							{#snippet trailing()}
								{#if t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}<StatusPill kind="approved" testid="request-approved" />{/if}
								{#if t.kind === 'movie'}
									{#each t.slots as s (s.id)}<StatusPill kind={slotKind(s.state)} label={s.state_label} />{/each}
								{:else if c}<StatusPill kind={slotKind(c.state)} label={c.label} />{/if}
								<span class="preview-only"><IconButton icon="info" tooltip={false} label="Preview {t.name}" aria-pressed={preview?.id === t.id} onclick={(event: MouseEvent) => openPreview(t, event)} /></span>
							{/snippet}
						</ListRow>
						{#if lastOpenedId === t.id || requester}
							<p class="row-notes">
								{#if lastOpenedId === t.id}<span class="last-opened-label" data-testid="last-opened">Last opened</span>{/if}
								{#if requester}<span class="requester" data-testid="requested-by">Requested by {requester}</span>{/if}
							</p>
						{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<ul class="grid" aria-label="Library titles">
			{#each shown as t, i (t.id)}
				{@const c = chipOf(t)}
				{@const requester = requestedBy(t)}
				<li class="item rise" data-testid="library-card" class:selected={preview?.id === t.id} class:last-opened={lastOpenedId === t.id} style="--i: {Math.min(i, 8)}">
					<PosterCard title={t.name} href={titleLink(t.id)} image={t.artwork[0]} subtitle={[t.year, t.kind === 'movie' ? '' : kindLabel[t.kind]].filter(Boolean).join(' · ')} kind={t.kind === 'artist' ? 'artist' : t.kind === 'show' ? 'show' : 'movie'} status={c ? slotKind(c.state) : undefined} statusLabel={c?.label} vtName={artName(t.id)}>
						{#snippet action()}<span class="preview-only"><IconButton icon="info" tooltip={false} variant="glass" size="sm" label="Preview {t.name}" aria-pressed={preview?.id === t.id} onclick={(event: MouseEvent) => openPreview(t, event)} /></span>{/snippet}
					</PosterCard>
					{#if lastOpenedId === t.id || requester || t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}
						<div class="card-notes">
							{#if lastOpenedId === t.id}<span class="last-opened-label" data-testid="last-opened">Last opened</span>{/if}
							{#if t.asks?.some((ask) => ask.state === 'approved' && ask.title_id === t.id)}<StatusPill kind="approved" testid="request-approved" />{/if}
							{#if requester}<span class="requester" data-testid="requested-by">Requested by {requester}</span>{/if}
						</div>
					{/if}
				</li>
			{/each}
			</ul>
		{/if}
		</div>
		{#if preview}
			{@const previewChip = chipOf(preview)}
			<aside class="preview" aria-labelledby="preview-title">
				<div class="preview-head">
					<span class="preview-kind">{previewKind(preview)}</span>
					<IconButton icon="x" label="Close preview" onclick={closePreview} />
				</div>
				<div class="preview-art"><Poster src={preview.artwork[0]} name={preview.name} /></div>
				<h2 id="preview-title" bind:this={previewHeading} tabindex="-1">{preview.name}</h2>
				{#if previewChip}<StatusPill kind={slotKind(previewChip.state)} label={previewChip.label} />{/if}
				<p class="preview-progress">{progressOf(preview)}</p>
				<dl class="preview-facts">
					<dt>Keep up to date</dt><dd>{preview.keep_up_to_date ? 'Yes' : 'No'}</dd>
					<dt>Out on</dt><dd>{outOn(preview, true)}</dd>
				</dl>
				{#if preview.slots.length}
					<div class="preview-section">
						<h3>Versions</h3>
						<ul>
							{#each preview.slots as slot (slot.id)}
								<li><span>{slot.version || 'Main'}</span><StatusPill kind={slotKind(slot.state)} label={slot.state_label} /></li>
							{/each}
						</ul>
					</div>
				{/if}
				<Button variant="primary" full href={titleLink(preview.id)}>Open title</Button>
			</aside>
		{/if}
	</div>
</div>

<style>
	.library { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
	.find-box { position: relative; flex: 1 1 320px; max-width: 560px; min-width: 0; }
	.find-ic { position: absolute; left: 12px; top: 50%; translate: 0 -50%; color: var(--text-muted); display: grid; pointer-events: none; }
	.find { width: 100%; min-height: 44px; padding-left: 38px; border-radius: var(--r-btn); background: var(--surface-2); border-color: transparent; }
	.view-tools { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
	.filters { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.search-error { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--fail); font: var(--t-callout); }
	.workspace, .content { min-width: 0; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(164px, 1fr)); gap: 24px 18px; margin: 0; padding: 0; list-style: none; }
	.grid > li { position: relative; min-width: 0; display: flex; flex-direction: column; gap: 6px; }
	.card-notes { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; min-width: 0; }
	.requester { color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.last-opened-label { font: var(--t-footnote); font-weight: 600; color: var(--text); }
	.grid > li.last-opened { outline: 2px solid var(--text-muted); outline-offset: 4px; border-radius: var(--r-card); }
	.grid > li.selected { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: var(--r-card); }
	.rows { display: flex; flex-direction: column; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.rows > li { display: block; min-height: 0; padding: 0; min-width: 0; border-radius: var(--r-btn); }
	.rows > li + li { border-top: 1px solid var(--line); }
	.rows > li.selected, .rows > li.last-opened { background: var(--surface-2); }
	.rows > li.last-opened { box-shadow: inset 0 0 0 2px var(--text-muted); }
	.row-notes { display: flex; flex-wrap: wrap; gap: 4px 12px; margin: -4px 0 0; padding: 0 12px 10px 58px; }
	.empty-more { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; }
	.preview-only { display: none; }
	.preview { display: none; }
	.preview-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.preview-kind { color: var(--text-muted); font: var(--t-subhead); }
	.preview-art { width: min(168px, 64%); margin: 8px auto 16px; border-radius: var(--r-card); overflow: hidden; box-shadow: var(--shadow-card); }
	.preview h2 { margin: 0 0 8px; font: var(--t-title3); overflow-wrap: anywhere; }
	.preview h2:focus { outline: none; }
	.preview-progress { margin: 8px 0 0; color: var(--text-muted); font: var(--t-callout); }
	.preview-facts { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 8px 12px; margin: 16px 0 0; padding-top: 16px; border-top: 1px solid var(--line); font: var(--t-callout); }
	.preview-facts dt { color: var(--text-muted); }
	.preview-facts dd { margin: 0; text-align: right; overflow-wrap: anywhere; }
	.preview-section { margin: 16px 0; padding-top: 12px; border-top: 1px solid var(--line); }
	.preview-section h3 { margin: 0 0 8px; font: var(--t-subhead); color: var(--text-muted); }
	.preview-section ul { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.preview-section li { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-width: 0; font: var(--t-callout); }
	@media (min-width: 1280px) {
		.workspace.with-preview { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 24px; align-items: start; }
		.preview { display: flex; flex-direction: column; position: sticky; top: 72px; padding: 16px 20px 20px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
		.preview-only { display: inline-flex; }
		.grid > li .preview-only { opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
		.grid > li:hover .preview-only, .grid > li:focus-within .preview-only, .grid > li.selected .preview-only { opacity: 1; }
	}
	@media (min-width: 1400px) { .grid { grid-template-columns: repeat(auto-fill, minmax(176px, 1fr)); } }
	@media (max-width: 767px) {
		.grid { grid-template-columns: repeat(auto-fill, minmax(112px, 1fr)); gap: 18px 12px; }
		.find-box { flex-basis: 100%; max-width: none; }
		.row-notes { padding-left: 58px; }
		.rows :global(.ui-row) { flex-wrap: wrap; }
		.rows :global(.ui-row > .main) { flex: 1 1 100%; }
		.rows :global(.ui-row > .trail) { flex: 1 1 100%; flex-wrap: wrap; justify-content: flex-start; margin-left: 52px; min-width: 0; }
	}
</style>
