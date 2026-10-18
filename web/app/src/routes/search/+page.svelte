<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import Icon from '#lib/Icon.svelte';
	import Poster from '#lib/Poster.svelte';
	import { session } from '#lib/session.svelte.ts';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import { plainReason } from '#lib/plain.ts';
	import { loadChoices, pickId, saveChoices, secondVersion } from '#lib/library.ts';
	import type { Ask, AskLimit, GateDefaults, LibraryFolder, LookupArtist, LookupMovie, LookupShow, MusicSettings, PickRulesView, ShowSettings, TitleView } from '#lib/types.ts';
	import { limitLine } from '#lib/wishlist.ts';
	import type { LimitKind } from '#lib/types.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import AddShowDialog from '#lib/AddShowDialog.svelte';
	import AskShowDialog from '#lib/AskShowDialog.svelte';
	import { seasonWords } from '#lib/shows.ts';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import { albumWords } from '#lib/music.ts';
	import { createLatestOnly } from '#lib/history.ts';
	import { searchAddress, type SearchKind } from '#lib/search.ts';

	// Movies, shows or music; kept in the page address (?kind=show, ?kind=music).
	const startKind = page.url.searchParams.get('kind');
	let kind = $state<SearchKind>(startKind === 'show' || startKind === 'music' ? startKind : 'movie');
	let artists = $state<LookupArtist[] | null>(null);
	let musicDefaults = $state<MusicSettings | null>(null);
	let addArtist: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let askArtist: ReturnType<typeof AskArtistDialog> | undefined = $state();
	const beginSearch = createLatestOnly();

	function patchArtist(a: LookupArtist, change: Partial<LookupArtist>) {
		if (!artists) return;
		const i = artists.findIndex((x) => x.mbid === a.mbid);
		if (i >= 0) artists[i] = { ...artists[i], ...change };
	}

	function artistAdded(a: LookupArtist, t: TitleView) {
		patchArtist(a, { title_id: t.id });
		toasts.push(`${t.name} is in your Library. Albums, Pressings and tracks are being filled in.`, 'ok');
	}

	function artistAsked(a: LookupArtist, r: { ask: Ask; joined: boolean; already: boolean }) {
		patchArtist(a, { ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length, albums: r.ask.albums } });
		const what = `${r.ask.name} (${albumWords(r.ask.albums)})`;
		if (r.ask.state === 'approved') toasts.push(`${what} is approved; HolyArrs keeps it up to date.`, 'ok');
		else if (r.joined) toasts.push(`Someone asked for ${what} already; you are on the ask too and will hear when it is ready.`, 'ok');
		else toasts.push(`Asked for ${what}. You will hear when it is approved.`, 'ok');
		loadLimit();
		void refreshInbox();
	}

	function artistRefused(a: LookupArtist, body: Record<string, unknown>) {
		if (typeof body.title_id === 'number') patchArtist(a, { title_id: body.title_id });
	}
	let shows = $state<LookupShow[] | null>(null);
	let showDefaults = $state<ShowSettings | null>(null);
	let addShow: ReturnType<typeof AddShowDialog> | undefined = $state();
	let askShow: ReturnType<typeof AskShowDialog> | undefined = $state();
	const showKey = (s: LookupShow) => `${s.source}:${s.ids[s.source] ?? s.name}`;

	function setKind(k: SearchKind) {
		if (k === kind) return;
		beginSearch();
		busy = false;
		kind = k;
		results = null;
		shows = null;
		artists = null;
		error = '';
		noTMDB = false;
		askProblem = '';
		const url = searchAddress(page.url, k, term);
		void goto(url, { replace: true, shallow: true });
		if (term.trim()) void run();
	}

	function patchShow(s: LookupShow, change: Partial<LookupShow>) {
		if (!shows) return;
		const i = shows.findIndex((x) => showKey(x) === showKey(s));
		if (i >= 0) shows[i] = { ...shows[i], ...change };
	}

	function showAdded(s: LookupShow, t: TitleView) {
		patchShow(s, { title_id: t.id });
		toasts.push(`${t.name} is in your Library.`, 'ok');
	}

	function showAsked(s: LookupShow, r: { ask: Ask; joined: boolean; already: boolean }) {
		patchShow(s, { ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length, seasons: r.ask.seasons } });
		const what = `${r.ask.name} (${seasonWords(r.ask.seasons)})`;
		if (r.ask.state === 'approved') toasts.push(`${what} is approved; HolyArrs keeps it up to date.`, 'ok');
		else if (r.joined) toasts.push(`Someone asked for ${what} already; you are on the ask too and will hear when it is ready.`, 'ok');
		else toasts.push(`Asked for ${what}. You will hear when it is approved.`, 'ok');
		loadLimit();
		void refreshInbox();
	}

	function showRefused(s: LookupShow, body: Record<string, unknown>) {
		if (typeof body.title_id === 'number') patchShow(s, { title_id: body.title_id });
	}

	let term = $state(page.url.searchParams.get('q') ?? '');
	let results = $state<LookupMovie[] | null>(null);
	let attribution = $state('');
	let busy = $state(false);
	let error = $state('');
	let noTMDB = $state(false);
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let gateDefault = $state('released');

	// The add dialog
	let addOpen = $state(false);
	let picked = $state<LookupMovie | null>(null);
	let folderId = $state(0);
	let rulesId = $state(0);
	let gate = $state('released');
	let keep = $state(true);
	let searchOnAdd = $state(true);
	// A second Version, such as 4K, next to the main one.
	let more = $state(false);
	let moreLabel = $state('4K');
	let moreRules = $state(0);
	let moreFolder = $state(0);
	let adding = $state(false);
	let addError = $state('');

	// Members ask instead of adding.
	const asker = $derived(session.can('ask') && !session.can('manage_library'));
	let askLimit = $state<AskLimit | null>(null);
	// The line under the search box is for the kind being searched.
	const limitKindOf: Record<SearchKind, LimitKind> = { movie: 'movies', show: 'shows', music: 'music' };
	let asking = $state('');
	let askProblem = $state('');

	function loadLimit() {
		api
			.get<AskLimit>('/asks/limit')
			.then((r) => (askLimit = r))
			.catch(() => {});
	}

	async function ask(m: LookupMovie) {
		const tmdb = m.ids.tmdb;
		if (!tmdb) return;
		asking = tmdb;
		askProblem = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { id: `tmdb:${tmdb}` });
			const i = results?.findIndex((x) => x.ids.tmdb === tmdb) ?? -1;
			if (results && i >= 0) {
				results[i] = { ...results[i], ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length } };
			}
			if (r.ask.state === 'approved') toasts.push(`${r.ask.name} is approved; HolyArrs is looking for it.`, 'ok');
			else if (r.joined) toasts.push(`Someone asked for ${r.ask.name} already; you are on the ask too and will hear when it is ready.`, 'ok');
			else toasts.push(`Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
			loadLimit();
			void refreshInbox();
		} catch (err) {
			if (err instanceof ApiError && err.status === 409 && typeof err.body.title_id === 'number' && results) {
				const i = results.findIndex((x) => x.ids.tmdb === tmdb);
				if (i >= 0) results[i] = { ...results[i], title_id: err.body.title_id as number };
			}
			askProblem = message(err);
		} finally {
			asking = '';
		}
	}

	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	const movieRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('movie')));
	const gates: [string, string, string][] = [
		['announced', 'As soon as it is announced', 'Look even before it is in cinemas.'],
		['in_cinemas', 'Once it is in cinemas', 'Look from the cinema date.'],
		['released', 'Once it is out on digital or disc', 'Look from the first digital or disc date.']
	];

	onMount(() => {
		if (session.can('manage_library')) {
			api
				.get<{ library_folders: LibraryFolder[] }>('/library-folders')
				.then((r) => (folders = r.library_folders))
				.catch(() => {});
			api
				.get<{ pick_rules: PickRulesView[] }>('/pick-rules')
				.then((r) => (rules = r.pick_rules))
				.catch(() => {});
			api
				.get<GateDefaults>('/settings/release-gate')
				.then((r) => (gateDefault = r.release_gate))
				.catch(() => {});
			api
				.get<ShowSettings>('/settings/shows')
				.then((r) => (showDefaults = r))
				.catch(() => {});
			api
				.get<MusicSettings>('/settings/music')
				.then((r) => (musicDefaults = r))
				.catch(() => {});
		}
		if (session.can('ask') && !session.can('manage_library')) loadLimit();
		if (term) void run();
	});

	async function run(e?: SubmitEvent) {
		e?.preventDefault();
		const q = term.trim();
		if (!q) return;
		const searchKind = kind;
		const isLatest = beginSearch();
		busy = true;
		error = '';
		noTMDB = false;
		const url = searchAddress(page.url, searchKind, q);
		void goto(url, { replace: true, shallow: true });
		try {
			if (searchKind === 'music') {
				const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(q)}`);
				if (!isLatest()) return;
				artists = r.artists;
				attribution = 'Artist and album details from MusicBrainz.';
			} else if (searchKind === 'show') {
				const r = await api.get<{ shows: LookupShow[]; attribution?: string }>(`/shows/lookup?term=${encodeURIComponent(q)}`);
				if (!isLatest()) return;
				shows = r.shows;
				attribution = r.attribution ?? '';
			} else {
				const r = await api.get<{ movies: LookupMovie[]; attribution: string }>(`/titles/lookup?term=${encodeURIComponent(q)}`);
				if (!isLatest()) return;
				results = r.movies;
				attribution = r.attribution;
			}
		} catch (err) {
			if (!isLatest()) return;
			results = null;
			shows = null;
			artists = null;
			if (err instanceof ApiError && err.status === 503 && searchKind !== 'music') noTMDB = true;
			else error = message(err);
		} finally {
			if (isLatest()) busy = false;
		}
	}

	function openAdd(m: LookupMovie) {
		picked = m;
		addError = '';
		// The last choices this browser made, while they are still offered.
		const last = loadChoices();
		folderId = pickId(last.library_folder_id, movieFolders, movieFolders.find((f) => f.health === 'ok')?.id ?? movieFolders[0]?.id ?? 0);
		chooseRulesFor(folderId, last.pick_rules_id);
		gate = last.release_gate && gates.some(([g]) => g === last.release_gate) ? last.release_gate : gateDefault;
		keep = last.keep_up_to_date ?? true;
		searchOnAdd = last.search_on_add ?? true;
		more = false;
		const sv = secondVersion(movieRules, movieFolders, folderId);
		moreLabel = sv.label;
		moreRules = sv.pick_rules_id;
		moreFolder = sv.library_folder_id;
		addOpen = true;
	}

	// A folder's own Pick rules come first, then the remembered ones, then the first offered.
	function chooseRulesFor(fid: number, remembered?: number) {
		const own = movieFolders.find((f) => f.id === fid)?.default_pick_rules_id ?? 0;
		rulesId = pickId(remembered, movieRules, pickId(own, movieRules, movieRules[0]?.id ?? 0));
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		adding = true;
		addError = '';
		try {
			const r = await api.post<{ title: TitleView }>('/titles', {
				id: `tmdb:${picked.ids.tmdb}`,
				library_folder_id: folderId,
				pick_rules_id: rulesId,
				release_gate: gate,
				keep_up_to_date: keep,
				search_on_add: searchOnAdd,
				more_versions: more ? [{ label: moreLabel.trim(), pick_rules_id: moreRules, library_folder_id: moreFolder }] : []
			});
			saveChoices({ library_folder_id: folderId, pick_rules_id: rulesId, keep_up_to_date: keep, release_gate: gate, search_on_add: searchOnAdd });
			if (results) {
				const i = results.findIndex((x) => x.ids.tmdb === picked?.ids.tmdb);
				if (i >= 0) results[i] = { ...results[i], title_id: r.title.id };
			}
			addOpen = false;
			toasts.push(`${r.title.name} is in your Library.`, 'ok');
		} catch (err) {
			addError = message(err);
		} finally {
			adding = false;
		}
	}
</script>

<svelte:head><title>Search · HolyArrs</title></svelte:head>

<section class="search-workspace">
<PageHeader
	title="Search"
	subtitle={kind === 'music'
		? 'Find an artist by name, or paste mbid: with its MusicBrainz id.'
		: kind === 'show'
			? 'Find a show by name, or paste tmdb:, tvdb:, tvmaze: or imdb: with its id.'
			: 'Find a movie by name, or paste tmdb: or imdb: with its id.'}
/>

<div class="kind" role="group" aria-label="Look for">
	<Chip icon="film" label="Movies" selected={kind === 'movie'} onclick={() => setKind('movie')} />
	<Chip icon="tv" label="Shows" selected={kind === 'show'} onclick={() => setKind('show')} />
	<Chip icon="music" label="Music" selected={kind === 'music'} onclick={() => setKind('music')} />
</div>

<form class="searchbar" role="search" onsubmit={run}>
	<span class="search-ic" aria-hidden="true"><Icon name="search" size={18} /></span>
	<label class="sr-only" for="q">{kind === 'music' ? 'Artist name or id' : kind === 'show' ? 'Show name or id' : 'Movie name or id'}</label>
	<input id="q" type="search" bind:value={term} placeholder={kind === 'music' ? 'Tin Lanterns, mbid:…' : kind === 'show' ? 'Harbour Lights, tvmaze:82 …' : 'Dune Part Two, tmdb:693134 …'} autocomplete="off" spellcheck="false" />
	<Button variant="primary" type="submit" disabled={!term.trim()} {busy}>Search</Button>
</form>

{#if asker && limitLine(askLimit, limitKindOf[kind])}
	<p class="small muted limit" data-testid="ask-limit">{limitLine(askLimit, limitKindOf[kind])}</p>
{/if}
{#if askProblem}
	<p class="notice warn ask-problem" role="alert">{plainReason(askProblem).text}</p>
{/if}

<div class="results" aria-live="polite" aria-busy={busy}>
	{#if kind === 'music'}
		{#if error}
			<p class="notice bad" role="alert">{plainReason(error).text}</p>
		{:else if busy && !artists}
			<ul class="list" aria-hidden="true">{#each Array(3) as _, i (i)}<li class="hit"><Skeleton shape="poster" /><Skeleton lines={2} /></li>{/each}</ul>
		{:else if artists === null}
			<EmptyState icon="music" title="Look for an artist" text="Type a name above. Details come from MusicBrainz; {asker ? 'ask for every album or a few, and it lands on your Wishlist' : 'pick one to add it to your Library'}." />
		{:else if artists.length === 0}
			<EmptyState icon="search" title="Nothing found" text="No artist matches “{term}”. Check the spelling, or try a MusicBrainz id such as mbid:5b11f4ce-a62d-471e-81fc-a69a8278c7da." />
		{:else}
			<ul class="list">
				{#each artists as a, i (a.mbid)}
					<li class="hit rise" style="--i: {i}" data-testid="artist-hit">
						<div class="art"><Poster name={a.name} /></div>
						<div class="grow">
							<h2 class="h3">{a.name}{#if a.disambiguation}{' '}<span class="muted small">({a.disambiguation})</span>{/if}</h2>
							<p class="xs muted">{[a.type ? a.type.charAt(0).toUpperCase() + a.type.slice(1) : '', a.country, a.status === 'ended' ? 'No longer active' : 'Still active'].filter(Boolean).join(' · ')}</p>
						</div>
						<div class="act">
							{#if a.title_id && !asker}
								<Button href={to(`/library/${a.title_id}`)} icon="check">In your Library</Button>
							{:else if a.never_add}
								<StatusPill kind="turned_down" label="On the Never-add list" />
							{:else if session.can('manage_library')}
								<Button variant="primary" icon="plus" onclick={() => addArtist?.open(a)}>Add</Button>
							{:else if a.ask?.mine}
								<Button href={to(`/wishlist`)} icon="wish">You asked · {a.ask.state_label}</Button>
							{:else if session.can('ask')}
								<Button variant="primary" icon="wish" onclick={() => askArtist?.open(a)}>{a.ask ? 'Ask too' : a.title_id ? 'Ask for more' : 'Ask'}</Button>
								{#if a.ask}<span class="xs muted asked-already">Someone asked for {albumWords(a.ask.albums)} · {a.ask.state_label}</span>
								{:else if a.title_id}<span class="xs muted asked-already">In the Library; ask for albums it does not have yet.</span>{/if}
							{:else}
								<span class="xs muted">Your account may not ask for music; ask the owner.</span>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
			{#if attribution}<p class="xs muted attribution">{attribution}</p>{/if}
		{/if}
	{:else if noTMDB}
		<EmptyState icon="key" title="Add your TMDB key first" text="HolyArrs looks {kind === 'show' ? 'shows' : 'movies'} up on TMDB. Add your TMDB read access token in Settings, then search again." action={{ label: 'Add the TMDB key', href: to('/settings/tmdb') }} />
	{:else if kind === 'show'}
		{#if error}
			<p class="notice bad" role="alert">{plainReason(error).text}</p>
		{:else if busy && !shows}
			<ul class="list" aria-hidden="true">{#each Array(3) as _, i (i)}<li class="hit"><Skeleton shape="poster" /><Skeleton lines={3} /></li>{/each}</ul>
		{:else if shows === null}
			<EmptyState icon="tv" title="Look for a show" text="Type a name above. {asker ? 'Ask for the whole show or a few seasons, and it lands on your Wishlist' : 'Pick one to add it to your Library'}." />
		{:else if shows.length === 0}
			<EmptyState icon="search" title="Nothing found" text="No show matches “{term}”. Check the spelling, or try an id such as tvmaze:82." />
		{:else}
			<ul class="list">
				{#each shows as s, i (showKey(s) + i)}
					<li class="hit rise" style="--i: {i}" data-testid="show-hit">
						<div class="art"><Poster src={s.poster} name={s.name} /></div>
						<div class="grow">
							<h2 class="h3">{s.name}{#if s.year}{" "}<span class="muted">({s.year})</span>{/if}</h2>
							<p class="xs muted">{[s.network, s.status === 'ended' ? 'Ended' : s.status === 'announced' ? 'Announced' : 'Still running'].filter(Boolean).join(' · ')}</p>
							{#if s.overview}<p class="small muted overview">{s.overview}</p>{/if}
						</div>
						<div class="act">
							{#if s.title_id && !asker}
								<Button href={to(`/library/${s.title_id}`)} icon="check">In your Library</Button>
							{:else if s.never_add}
								<StatusPill kind="turned_down" label="On the Never-add list" />
							{:else if session.can('manage_library')}
								<Button variant="primary" icon="plus" onclick={() => addShow?.open(s)}>Add</Button>
							{:else if s.ask?.mine}
								<Button href={to(`/wishlist`)} icon="wish">You asked · {s.ask.state_label}</Button>
							{:else if session.can('ask')}
								<Button variant="primary" icon="wish" onclick={() => askShow?.open(s)}>{s.ask ? 'Ask too' : s.title_id ? 'Ask for more' : 'Ask'}</Button>
								{#if s.ask}<span class="xs muted asked-already">Someone asked for {seasonWords(s.ask.seasons)} · {s.ask.state_label}</span>
								{:else if s.title_id}<span class="xs muted asked-already">In the Library; ask for seasons it does not have yet.</span>{/if}
							{:else}
								<span class="xs muted">Your account may not ask for shows; ask the owner.</span>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
			{#if attribution}<p class="xs muted attribution">{attribution}</p>{/if}
		{/if}
	{:else if error}
		<p class="notice bad" role="alert">{plainReason(error).text}</p>
	{:else if busy && !results}
		<ul class="list" aria-hidden="true">{#each Array(3) as _, i (i)}<li class="hit"><Skeleton shape="poster" /><Skeleton lines={3} /></li>{/each}</ul>
	{:else if results === null}
		<EmptyState icon="film" title="Look for a movie" text="Type a name above. Results come from TMDB; {asker ? 'ask for one and it lands on your Wishlist' : 'pick one to add it to your Library'}." />
	{:else if results.length === 0}
		<EmptyState icon="search" title="Nothing found" text="No movie matches “{term}”. Check the spelling, or try the year or a TMDB id." />
	{:else}
		<ul class="list">
			{#each results as m, i (m.ids.tmdb ?? i)}
				<li class="hit rise" style="--i: {i}">
					<div class="art"><Poster src={m.poster} name={m.name} /></div>
					<div class="grow">
						<h2 class="h3">{m.name}{#if m.year}{" "}<span class="muted">({m.year})</span>{/if}</h2>
						{#if m.overview}<p class="small muted overview">{m.overview}</p>{/if}
					</div>
					<div class="act">
						{#if m.title_id}
							<Button href={to(`/library/${m.title_id}`)} icon="check">In your Library</Button>
						{:else if m.never_add}
							<StatusPill kind="turned_down" label="On the Never-add list" />
						{:else if session.can('manage_library')}
							<Button variant="primary" icon="plus" onclick={() => openAdd(m)}>Add</Button>
						{:else if m.ask?.mine}
							<Button href={to(`/wishlist`)} icon="wish">You asked · {m.ask.state_label}</Button>
						{:else if session.can('ask')}
							<Button variant="primary" icon="wish" busy={asking === m.ids.tmdb} onclick={() => ask(m)}>{m.ask ? 'Ask too' : 'Ask'}</Button>
							{#if m.ask}<span class="xs muted asked-already">Someone asked already · {m.ask.state_label}</span>{/if}
						{:else}
							<span class="xs muted">Your account may not ask for movies; ask the owner.</span>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
		{#if attribution}<p class="xs muted attribution">{attribution}</p>{/if}
	{/if}
</div>
</section>

<Sheet bind:open={addOpen} title={picked ? `Add ${picked.name}` : 'Add a movie'}>
	{#if picked}
		<form id="add-movie" class="sheet-form" onsubmit={add}>
			{#if movieFolders.length === 0}
				<p class="notice warn">
					There is no Library folder for movies yet. <a href={to(`/settings/library-folders`)} class="ul">Add one</a> first.
				</p>
			{:else}
				<div class="field">
					<label for="folder">Library folder</label>
					<select id="folder" class="input" bind:value={folderId} onchange={() => chooseRulesFor(folderId)}>
						{#each movieFolders as f (f.id)}
							<option value={f.id}>{f.label || f.path} ({f.path})</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="rules">Pick rules</label>
					<select id="rules" class="input" bind:value={rulesId}>
						{#each movieRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
					<span class="hint">Which Releases count, and which one is best.</span>
				</div>
				<div class="field">
					<label for="gate">Release gate</label>
					<select id="gate" class="input" bind:value={gate}>
						{#each gates as [g, label] (g)}
							<option value={g}>{label}</option>
						{/each}
					</select>
					<span class="hint">{gates.find(([g]) => g === gate)?.[2]}</span>
				</div>
				<label class="checkline">
					<input type="checkbox" bind:checked={keep} />
					<span><b>Keep up to date</b><br /><span class="hint">HolyArrs looks for it and for a better copy later.</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={searchOnAdd} />
					<span><b>Search now</b><br /><span class="hint">Start looking for a Release straight away.</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={more} />
					<span><b>Also keep a second Version</b><br /><span class="hint">For example a 4K copy next to the HD one, with its own Pick rules and folder.</span></span>
				</label>
				{#if more}
					<fieldset class="more">
						<legend class="sr-only">Second Version</legend>
						<div class="field">
							<label for="more-label">Version name</label>
							<input id="more-label" class="input" bind:value={moreLabel} maxlength="40" required />
						</div>
						<div class="field">
							<label for="more-rules">Pick rules for this Version</label>
							<select id="more-rules" class="input" bind:value={moreRules}>
								{#each movieRules as r (r.id)}
									<option value={r.id}>{r.name}</option>
								{/each}
							</select>
						</div>
						<div class="field">
							<label for="more-folder">Library folder for this Version</label>
							<select id="more-folder" class="input" bind:value={moreFolder}>
								{#each movieFolders as f (f.id)}
									<option value={f.id}>{f.label || f.path} ({f.path})</option>
								{/each}
							</select>
							{#if moreFolder === folderId}<span class="hint">Same folder as the main copy: it gets its own folder named with “[{moreLabel.trim() || '4K'}]”.</span>{/if}
						</div>
					</fieldset>
				{/if}
			{/if}
			{#if addError}<p class="form-error" role="alert">{plainReason(addError).text}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (addOpen = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="add-movie" disabled={movieFolders.length === 0} busy={adding}>Add to Library</Button>
	{/snippet}
</Sheet>


<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} onadded={showAdded} />
<AskShowDialog bind:this={askShow} onasked={showAsked} onrefused={showRefused} />
<AddArtistDialog bind:this={addArtist} {folders} {rules} defaults={musicDefaults} onadded={artistAdded} />
<AskArtistDialog bind:this={askArtist} onasked={artistAsked} onrefused={artistRefused} />

<style>
	.search-workspace { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.kind { display: flex; flex-wrap: wrap; gap: 8px; }
	.searchbar { position: relative; display: flex; align-items: center; gap: 8px; max-width: 720px; }
	.search-ic { position: absolute; left: 14px; top: 50%; translate: 0 -50%; display: grid; color: var(--text-muted); pointer-events: none; }
	.searchbar input { flex: 1; min-width: 0; height: 44px; padding: 0 12px 0 40px; border: 1px solid transparent; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-body); }
	.searchbar input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.notice { display: flex; gap: 8px; margin: 0; padding: 10px 12px; border: 0; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
	.limit { margin: 0; color: var(--text-muted); font: var(--t-callout); }
	.ask-problem { max-width: 720px; }
	.results { min-width: 0; }
	.list { display: flex; flex-direction: column; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.hit { display: grid; grid-template-columns: 72px minmax(0, 1fr) auto; gap: 14px; align-items: start; min-width: 0; padding: 12px; border-radius: var(--r-btn); }
	.hit + .hit { border-top: 1px solid var(--line); border-radius: 0; }
	.art { width: 72px; flex: none; border-radius: 8px; overflow: hidden; box-shadow: var(--shadow-card); }
	.grow { min-width: 0; }
	.grow :global(.h3) { margin: 0; font: var(--t-headline); }
	.grow .muted { color: var(--text-muted); font-weight: 400; }
	.grow p { margin: 2px 0 0; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.overview { margin-top: 4px; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.act { display: flex; flex-direction: column; align-items: flex-end; gap: 6px; flex: none; max-width: 220px; text-align: right; }
	.asked-already { display: block; color: var(--text-muted); font: var(--t-footnote); }
	.act > .xs { color: var(--text-muted); font: var(--t-footnote); }
	.attribution { margin: 8px 0 0; color: var(--text-muted); font: var(--t-footnote); }
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.sheet-form .notice.warn { color: var(--text); }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
	.ul { text-decoration: underline; color: var(--text); }
	.more { margin: 12px 0 0; padding: 12px 0 0 16px; border: 0; border-left: 2px solid var(--line-strong); }
	@media (max-width: 600px) {
		.hit { grid-template-columns: 60px minmax(0, 1fr); }
		.art { width: 60px; }
		.act { grid-column: 1 / -1; align-items: stretch; max-width: none; text-align: left; }
		.act :global(.ui-btn) { width: 100%; }
		.asked-already { text-align: left; }
		.act > :global(.ui-pill) { align-self: flex-start; }
	}
</style>
