<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { focusField, forgetReturn, playFieldEntry, restoreReturn, searchField, searchHooks } from '#lib/search-field.svelte.ts';
	import SearchField from '#lib/ui/SearchField.svelte';
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import { session } from '#lib/session.svelte.ts';
	import BrowseTile from '#lib/ui/BrowseTile.svelte';
	import MosaicReveal, { ENTER_DELAY, passAt } from '#lib/ui/MosaicReveal.svelte';
	import { browseCols, browseHref, browsePath, browseType, readMood, tileHue, type BrowseAnswer, type BrowseTile as Tile, type MoodView } from '#lib/search-browse.ts';
	import { reducedMotion } from '#lib/motion.ts';
	import MetadataNotice from '#lib/MetadataNotice.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import type { CardOwned } from '#lib/ui/CardBadges.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import { plainReason } from '#lib/plain.ts';
	import type { Ask, LibraryFolder, LookupArtist, LookupMovie, LookupShow, MusicSettings, PickRulesView, TitleView } from '#lib/types.ts';
	import type { PersonSummary } from '#lib/people.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import { albumWords } from '#lib/music.ts';
	import { createLatestOnly } from '#lib/history.ts';
	import { addRecent, readRecent, readScope, recentKey, scopeAddress, topMatches, type SearchScope } from '#lib/search.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { detailPath } from '#lib/title-details.ts';
	import { discoverCard, genrePath, ownedMark, type GenreAnswer, type ProviderItem } from '#lib/explore.ts';
	import { audienceOf } from '#lib/title-status.ts';

	// Search is a page; its one field is the top bar's (lib/search-field.svelte.ts;
	// on phones, where the top bar is hidden, a copy sits at the page top). The words and the scope
	// live in the address (?q=, ?kind=). Before typing it shows recent searches
	// (this device, per user) and the Browse grid (genres and their moods, per
	// tab); a mood tile opens a mood view here (?mood=&genre=&type=). Results
	// come in as you type. Kid limits are applied by the server on every lookup.

	type Card = { key: string; type: Exclude<SearchScope, 'all'>; name: string; year?: number; image?: string | null; href: string; owned: CardOwned | null; preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number }; artist?: LookupArtist; subtitle?: string };

	// The words live in the shared field (top bar); a direct link fills it from ?q=.
	if (!searchField.term.trim()) searchField.term = page.url.searchParams.get('q') ?? '';
	let scope = $state<SearchScope>(readScope(page.url.searchParams.get('kind')));
	let busy = $state(false);
	let error = $state('');
	let noTMDB = $state(false);
	let searched = $state('');
	let groups = $state<Record<Card['type'], Card[] | null>>({ show: null, movie: null, music: null, people: null });
	let tiles = $state<Tile[] | null>(null);
	let tilesFor = '';
	let mood = $state<MoodView | null>(readMood(page.url.searchParams));
	let moodItems = $state<ProviderItem[] | null>(null);
	let moodGenre = $state('');
	// The entry: a mosaic dissolve down the whole visible page, the tiles rising
	// row by row behind it; under reduced motion the band alone. Timing: a
	// 120 ms hold, the band builds to ~900 ms, the last visible row lands by
	// ~1.1 s, the field's glow settles by ~1.4 s. Only transform and opacity move.
	const calm = reducedMotion();
	let revealing = $state(true);
	// Search is a layer: Close, Escape on an empty field or Back returns to the
	// page you came from, at its scroll. Typing replaces the address, so only
	// links inside Search add history entries (counted in depth).
	let cameFrom = '';
	let depth = 0;
	let leaving = $state(false);
	let covering = $state(false);
	const LAND_MS = 520;
	function exit() {
		if (leaving) return;
		revealing = false;
		leaving = true;
		// The tiles drop out quickly, last visible row first; the band then sweeps back up (ease-in).
		const rows = calm ? [] : [...document.querySelectorAll<HTMLElement>('.search .tiles li, .search .grid li, .search .ui-shelf')].filter((n) => n.getBoundingClientRect().top < innerHeight);
		const tops = [...new Set(rows.map((n) => Math.round(n.getBoundingClientRect().top)))].sort((x, y) => y - x);
		for (const n of rows) {
			const order = tops.indexOf(Math.round(n.getBoundingClientRect().top));
			n.animate([{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'translateY(10px) scale(0.98)' }], { duration: 220, delay: order * 35, easing: 'cubic-bezier(.55,0,1,.45)', fill: 'forwards' });
		}
		setTimeout(() => (covering = true), calm ? 0 : 160);
	}
	function left() {
		if (cameFrom) {
			restoreReturn(location.pathname);
			history.go(-(depth + 1));
		} else void goto(to('/'));
	}
	const t0 = typeof performance === 'undefined' ? 0 : performance.now();
	// Each tile enters behind the band: rows ~70 ms apart, ~25 ms left to right
	// within a row, starting as the band passes it, bounded so the last lands by ~1.1 s.
	function landing(node: HTMLElement, at: { row: number; col: number }) {
		if (calm) return;
		const elapsed = performance.now() - t0;
		const top = document.querySelector('.ui-topbar')?.getBoundingClientRect().bottom ?? 0;
		const r = node.getBoundingClientRect();
		if (r.top > innerHeight) return;
		const f = (r.top - top) / Math.max(1, innerHeight - top);
		const start = Math.min(1100 - LAND_MS, Math.max(passAt(f), ENTER_DELAY + at.row * 70) + at.col * 25);
		const delay = Math.max(0, start - elapsed);
		node.style.willChange = 'transform, opacity';
		const a = node.animate([{ opacity: 0, transform: 'translateY(14px) scale(0.97)' }, { opacity: 1, transform: 'none' }], { duration: LAND_MS, delay, easing: 'cubic-bezier(.22,1,.36,1)', fill: 'backwards' });
		a.onfinish = a.oncancel = () => (node.style.willChange = '');
	}
	let cols = $state(6);
	let recent = $state<string[]>([]);
	const beginSearch = createLatestOnly();
	const who = $derived(audienceOf(session.can));
	const manage = $derived(session.can('manage_library'));

	const partOn = (name: string) => partsNow.answer?.parts.find((p) => p.part === name)?.on !== false;
	const scopeOptions = $derived(
		([
			{ value: 'all', label: 'All', icon: 'grid' },
			{ value: 'show', label: 'Shows', icon: 'tv' },
			{ value: 'movie', label: 'Movies', icon: 'film' },
			{ value: 'music', label: 'Music', icon: 'music' },
			{ value: 'people', label: 'People', icon: 'people' }
		] as { value: SearchScope; label: string; icon: IconName }[]).filter((o) => (o.value === 'show' ? partOn('shows') : o.value === 'movie' ? partOn('movies') : o.value === 'music' ? partOn('music') : true))
	);
	const labels: Record<Card['type'], string> = { show: 'Shows', movie: 'Movies', music: 'Music', people: 'People' };
	const order = $derived(scopeOptions.map((o) => o.value).filter((v): v is Card['type'] => v !== 'all'));

	// ── Library marking: one mark for every card, as on Explore ────────────
	const inLibrary: CardOwned = { state: 'available', label: 'In your Library' };
	function askMark(ask: { state: string; mine: boolean } | undefined | null): CardOwned | null {
		if (!ask || ask.state === 'turned_down') return null;
		return { state: 'requested', label: ask.state === 'approved' ? 'Approved, waiting for a copy' : 'Requested' };
	}
	const movieCard = (m: LookupMovie): Card | null =>
		m.ids.tmdb
			? { key: `m${m.ids.tmdb}`, type: 'movie', name: m.name, year: m.year || undefined, subtitle: m.year ? String(m.year) : undefined, image: m.poster, href: to(m.title_id ? `/library/${m.title_id}` : detailPath('movie', m.ids.tmdb)), owned: m.title_id ? inLibrary : askMark(m.ask), preview: { kind: 'movie', tmdb: m.ids.tmdb, year: m.year || undefined } }
			: null;
	const showCard = (s: LookupShow, i: number): Card => {
		const tmdb = s.ids.tmdb;
		return { key: `s${s.source}${s.ids[s.source] ?? s.name}${i}`, type: 'show', name: s.name, year: s.year, subtitle: s.year ? String(s.year) : undefined, image: s.poster, href: to(s.title_id ? `/library/${s.title_id}` : tmdb ? detailPath('show', tmdb) : `/search?q=${encodeURIComponent(s.name)}&kind=show`), owned: s.title_id ? inLibrary : askMark(s.ask), preview: tmdb ? { kind: 'show', tmdb, year: s.year } : undefined };
	};
	const artistCard = (a: LookupArtist): Card => ({ key: `a${a.mbid}`, type: 'music', name: a.name, subtitle: a.disambiguation || [a.type, a.country].filter(Boolean).join(' · ') || undefined, href: a.title_id ? to(`/library/${a.title_id}`) : '', owned: a.title_id ? inLibrary : askMark(a.ask), artist: a });
	const personCard = (p: PersonSummary): Card => ({ key: `p${p.id}`, type: 'people', name: p.name, subtitle: p.department, image: p.profile, href: to(`/people/${encodeURIComponent(p.id)}`), owned: null });

	const lists = $derived(order.map((t) => groups[t] ?? []));
	const top = $derived(topMatches(lists.filter((_, i) => order[i] !== 'people'), 6));
	const total = $derived(lists.reduce((n, g) => n + g.length, 0));
	const pending = $derived(order.some((t) => groups[t] === null));

	// ── Searching ──────────────────────────────────────────────────────────
	const ownAddresses = new Set<string>();
	function writeAddress() {
		const url = scopeAddress(page.url, scope, searchField.term);
		if (searchField.term.trim() || !mood) for (const k of ['mood', 'genre', 'type']) url.searchParams.delete(k);
		if (url.href === page.url.href) return;
		ownAddresses.add(url.href);
		void goto(url, { replace: true, shallow: true });
	}

	let timer: ReturnType<typeof setTimeout> | undefined;
	// Results follow the shared field as you type (any page's top bar).
	let lastTerm = untrack(() => searchField.term);
	$effect(() => {
		const t = searchField.term;
		if (t === lastTerm) return;
		lastTerm = t;
		if (!t.trim()) void run();
		else typed();
	});
	function typed() {
		clearTimeout(timer);
		timer = setTimeout(() => void run(), 250);
	}

	async function run(e?: SubmitEvent) {
		e?.preventDefault();
		clearTimeout(timer);
		const q = searchField.term.trim();
		writeAddress();
		const isLatest = beginSearch();
		error = '';
		noTMDB = false;
		if (q) mood = null;
		if (!q) {
			searched = '';
			busy = false;
			return;
		}
		if (e) remember(q);
		busy = true;
		const want = scope === 'all' ? order : order.filter((t) => t === scope);
		const next: Record<Card['type'], Card[] | null> = { show: null, movie: null, music: null, people: null };
		for (const t of order) next[t] = want.includes(t) ? null : [];
		groups = next;
		const enc = encodeURIComponent(q);
		const jobs: Record<Card['type'], () => Promise<Card[]>> = {
			movie: () => api.get<{ movies: LookupMovie[] }>(`/titles/lookup?term=${enc}`).then((r) => r.movies.map(movieCard).filter((c): c is Card => !!c)),
			show: () => api.get<{ shows: LookupShow[] }>(`/shows/lookup?term=${enc}`).then((r) => r.shows.map(showCard)),
			music: () => api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${enc}`).then((r) => r.artists.map(artistCard)),
			people: () => api.get<{ people: PersonSummary[] }>(`/people/search?q=${enc}`).then((r) => r.people.map(personCard))
		};
		let failures = 0;
		await Promise.all(
			want.map((t) =>
				jobs[t]()
					.then((cards) => {
						if (isLatest()) groups[t] = cards;
					})
					.catch((err) => {
						if (!isLatest()) return;
						groups[t] = [];
						failures++;
						if (err instanceof ApiError && err.status === 503) noTMDB = true;
						else if (!(err instanceof ApiError && err.status === 403)) error = message(err);
					})
			)
		);
		if (!isLatest()) return;
		if (failures < want.length) error = '';
		searched = q;
		busy = false;
	}

	function setScope(s: SearchScope) {
		if (s === scope) return;
		scope = s;
		void run();
	}

	// ── Recent searches (this device, per user) ────────────────────────────
	const key = () => recentKey(session.s.name);
	function loadRecent() {
		try {
			recent = readRecent(localStorage.getItem(key()));
		} catch {
			recent = [];
		}
	}
	function saveRecent() {
		try {
			localStorage.setItem(key(), JSON.stringify(recent));
		} catch {
			/* A private window: recents just are not kept. */
		}
	}
	function remember(q: string) {
		recent = addRecent(recent, q);
		saveRecent();
	}
	function clearRecent() {
		recent = [];
		saveRecent();
	}
	function pickRecent(q: string) {
		searchField.term = q;
		void run();
	}

	// ── Browse: the grid before typing, per tab (Movies and Shows only) ─────
	const browseFor = $derived(browseType(scope));
	async function loadTiles(t: ReturnType<typeof browseType>) {
		if (tilesFor === t) return;
		tilesFor = t;
		tiles = null;
		try {
			const a = await api.get<BrowseAnswer>(browsePath(t));
			if (tilesFor === t) tiles = (a.tiles ?? []).filter((x) => (x.kind === 'show' ? partOn('shows') : partOn('movies')));
		} catch {
			if (tilesFor === t) {
				tiles = [];
				tilesFor = '';
			}
		}
	}
	$effect(() => {
		if (!searchField.term.trim() && !mood) void loadTiles(browseFor);
	});

	// ── A mood view: that mood row of the genre page, as a grid ─────────────
	let moodFor = '';
	async function loadMood(m: MoodView) {
		const k = `${m.type}|${m.genre}|${m.mood}`;
		if (moodFor === k) return;
		moodFor = k;
		moodItems = null;
		try {
			const a = await api.get<GenreAnswer>(genrePath(m.type, m.genre));
			if (moodFor !== k) return;
			moodGenre = a.genre?.name ?? '';
			moodItems = a.rows.find((r) => r.type === 'mood' && r.title.toLowerCase() === m.mood.toLowerCase())?.items ?? [];
		} catch {
			if (moodFor === k) {
				moodItems = [];
				moodFor = '';
			}
		}
	}
	$effect(() => {
		if (mood) void loadMood(mood);
	});
	const moodCards = $derived((moodItems ?? []).map((item) => ({ item, c: discoverCard(item, 'search-mood') })));

	onMount(() => {
		loadRecent();
		const sync = () => (cols = browseCols(innerWidth));
		sync();
		addEventListener('resize', sync);
		if (manage) {
			api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
			api.get<MusicSettings>('/settings/music').then((r) => (musicDefaults = r)).catch(() => {});
		}
		if (searchField.term.trim()) void run();
		searchHooks.submit = () => void run(new SubmitEvent('submit'));
		searchHooks.blur = blurred;
		searchHooks.exit = exit;
		if (!(document.activeElement instanceof HTMLInputElement)) focusField();
		playFieldEntry(calm);
		return () => {
			searchHooks.submit = searchHooks.blur = searchHooks.exit = undefined;
			forgetReturn();
			searchField.term = '';
			clearTimeout(timer);
			removeEventListener('resize', sync);
		};
	});

	// A new ?q= from elsewhere (Ctrl+K, a link) runs it; our own address writes do nothing.
	afterNavigate(({ from, to: dest, type }) => {
		if (!from || !dest) return;
		if (!from.url.pathname.endsWith('/search')) {
			cameFrom = from.url.pathname;
			depth = 0;
			return;
		}
		if (type === 'popstate') depth = Math.max(0, depth - 1);
		else if (type === 'link') depth++;
		if (ownAddresses.delete(dest.url.href)) return;
		const q = dest.url.searchParams.get('q') ?? '';
		const s = readScope(dest.url.searchParams.get('kind'));
		mood = readMood(dest.url.searchParams);
		void tick().then(() => focusField());
		if (q === searchField.term && s === scope) return;
		searchField.term = lastTerm = q;
		scope = s;
		void run();
	});

	// Leaving the box after a search counts it as a recent one.
	function blurred() {
		if (searched && searched === searchField.term.trim()) remember(searched);
	}

	// ── Music: artists are added or asked for here ─────────────────────────
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let musicDefaults = $state<MusicSettings | null>(null);
	let addArtist: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let askArtist: ReturnType<typeof AskArtistDialog> | undefined = $state();
	function patchArtist(a: LookupArtist, change: Partial<LookupArtist>) {
		const list = groups.music;
		if (!list) return;
		const i = list.findIndex((c) => c.artist?.mbid === a.mbid);
		if (i >= 0) list[i] = artistCard({ ...a, ...change });
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
		void refreshInbox();
	}
	function artistRefused(a: LookupArtist, body: Record<string, unknown>) {
		if (typeof body.title_id === 'number') patchArtist(a, { title_id: body.title_id });
	}

</script>

<svelte:head><title>{searched ? `${searched} · Search` : 'Search'} · HolyArrs</title></svelte:head>

{#snippet card(c: Card)}
	{@const a = c.artist}
	{#if a && !a.title_id && !a.never_add && (manage || (session.can('ask') && !a.ask?.mine))}
		<WideCard cover title={c.name} image={c.image} kind="artist" subtitle={c.subtitle} owned={c.owned} typed={scope === 'all'}>
			{#snippet action()}
				{#if manage}<Button variant="primary" icon="plus" onclick={() => addArtist?.open(a)}>Add</Button>
				{:else}<Button variant="primary" icon="wish" onclick={() => askArtist?.open(a)}>{a.ask ? 'Ask too' : 'Ask'}</Button>{/if}
			{/snippet}
		</WideCard>
	{:else}
		<WideCard cover title={c.name} href={c.href || undefined} image={c.image} kind={c.type === 'music' ? 'artist' : c.type === 'people' ? undefined : c.type} subtitle={c.subtitle} owned={c.owned} typed={scope === 'all'} preview={c.preview} />
	{/if}
{/snippet}

<section class="search" class:calm data-testid="search-page">
	<h1 class="sr-only">Search</h1>
	<div class="close-row"><Button variant="plain" icon="x" onclick={exit} data-testid="search-page-close">Close</Button></div>
	<div class="phone-field"><SearchField searchKey="" testid="search-box-phone" id="q-phone" closable={false} /></div>
	<MetadataNotice />

	{#if scopeOptions.length > 2}
		<div class="types">
			<SegmentedControl options={scopeOptions} value={scope} label="Look in" testid="search-scope" onchange={setScope} />
		</div>
	{/if}

	<div class="results" aria-live="polite" aria-busy={busy}>
		{#if !searchField.term.trim() && mood}
			<section class="mood" aria-labelledby="mood-h" data-testid="search-mood">
				<div class="mood-head">
					<Button variant="plain" icon="chevronLeft" href={to(`/search${scope === 'all' ? '' : `?kind=${scope}`}`)}>Browse</Button>
					<h2 id="mood-h" class="h1">{mood.mood}</h2>
					<p class="muted">{moodGenre ? `${moodGenre} · ` : ''}{mood.type === 'shows' ? 'Shows' : mood.type === 'all' ? 'Movies and shows' : 'Movies'}</p>
				</div>
				{#if moodItems === null}
					<p class="muted searching">Finding {mood.mood.toLowerCase()} titles…</p>
				{:else if moodCards.length === 0}
					<div class="empty"><h2 class="h2">Nothing here yet</h2><p class="muted">No {mood.type === 'shows' ? 'shows' : mood.type === 'all' ? 'titles' : 'movies'} fit this mood right now. Try the genre page instead.</p><Button variant="secondary" href={to(mood.type === 'all' ? `/explore?genre=${mood.genre}` : `/explore?type=${mood.type}&genre=${mood.genre}`)}>Open {moodGenre || 'the genre'}</Button></div>
				{:else}
					<ul class="grid">
						{#each moodCards as { item, c } (c.key)}
							<li><WideCard cover title={c.name} href={to(item.title_id ? `/library/${item.title_id}` : detailPath(item.kind, item.tmdb_id))} image={c.wide ?? c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} owned={ownedMark(c, undefined, who)} fresh={c.fresh} preview={{ kind: item.kind, tmdb: item.tmdb_id, year: item.year }} /></li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if !searchField.term.trim()}
			{#if recent.length}
				<section class="recent" aria-labelledby="recent-h">
					<div class="recent-head">
						<h2 id="recent-h" class="h2">Recent searches</h2>
						<Button variant="plain" onclick={clearRecent}>Clear</Button>
					</div>
					<div class="chips">
						{#each recent as q (q)}<Chip label={q} icon="clock" onclick={() => pickRecent(q)} />{/each}
					</div>
				</section>
			{/if}
			<section class="browse" aria-labelledby="browse-h" data-testid="search-browse">
				<h2 id="browse-h" class="h1">Browse</h2>
				{#if tiles === null}
					<ul class="tiles" aria-hidden="true">
						{#each Array(cols * 2) as _, i (i)}<li class="ghost" use:landing={{ row: Math.floor(i / cols), col: i % cols }}></li>{/each}
					</ul>
				{:else if tiles.length}
					<ul class="tiles">
						{#each tiles as t, i (`${t.kind}:${t.genre}:${t.mood ?? ''}`)}
							<li use:landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={to(browseHref(t))} image={t.backdrop} hue={tileHue(t.name)} row={Math.min(6, Math.floor(i / cols))} /></li>
						{/each}
					</ul>
				{:else}
					<p class="muted">Genres show up here once Explore has titles to show.</p>
				{/if}
			</section>
		{:else if noTMDB && total === 0}
			<div class="empty"><h2 class="h2">The metadata server is down</h2><p class="muted">Searches and new artwork come back when it is. Try again in a minute.</p></div>
		{:else if error && total === 0}
			<p class="notice bad" role="alert">{plainReason(error).text}</p>
		{:else if total === 0 && (pending || busy)}
			<p class="muted searching">Searching for “{searchField.term.trim()}”…</p>
		{:else if total === 0 && searched}
			<div class="empty" data-testid="search-empty">
				<h2 class="h2">Nothing found for “{searched}”</h2>
				<ul class="tips">
					<li>Check the spelling, or use fewer words.</li>
					<li>Add the year, such as “Dune 2021”.</li>
					<li>Paste an id: tmdb:693134, tvmaze:82 or mbid:…</li>
				</ul>
				{#if scope !== 'all'}<Button variant="secondary" onclick={() => setScope('all')}>Look everywhere</Button>{/if}
				{#if manage}<Button variant="primary" icon="search" href={to('/settings/sources')}>Search indexers for “{searched}”</Button>{/if}
			</div>
		{:else if scope === 'all'}
			{#if top.length}
				<Shelf title="Top matches" item="var(--title-item)" placeholder="wide" lines={1}>
					{#each top as c (c.key)}{@render card(c)}{/each}
				</Shelf>
			{/if}
			{#each order as t (t)}
				{@const g = groups[t]}
				{#if g?.length}
					<Shelf title={labels[t]} subtitle={String(g.length)} item="var(--title-item)" placeholder="wide" lines={1} onseeall={() => setScope(t)}>
						{#each g as c (c.key)}{@render card(c)}{/each}
					</Shelf>
				{/if}
			{/each}
		{:else}
			{@const g = groups[scope] ?? []}
			<h2 class="h2">{labels[scope]} <span class="muted n">{g.length}</span></h2>
			<ul class="grid">
				{#each g as c (c.key)}<li>{@render card(c)}</li>{/each}
			</ul>
		{/if}
	</div>
</section>

{#if revealing}<MosaicReveal ondone={() => (revealing = false)} />{/if}
{#if covering}<MosaicReveal exit ondone={left} />{/if}

<AddArtistDialog bind:this={addArtist} {folders} {rules} defaults={musicDefaults} onadded={artistAdded} />
<AskArtistDialog bind:this={askArtist} onasked={artistAsked} onrefused={artistRefused} />

<style>
	.search { --title-item: max(200px, calc((100vw - 96px) / 5.16)); display: flex; flex-direction: column; gap: 24px; padding-bottom: 48px; min-width: 0; }
	@media (max-width: 1279px) { .search { --title-item: max(180px, calc((100vw - 64px) / 4.2)); } }
	@media (max-width: 767px) { .search { --title-item: calc((100vw - 40px) / 2.3); } }
	.close-row { display: flex; justify-content: flex-end; margin-bottom: -16px; }
	/* Phones only: the top bar (and its field) is hidden under 768 px. */
	.phone-field { display: none; }
	@media (max-width: 767px) { .phone-field { position: sticky; top: 8px; z-index: 22; display: flex; justify-content: center; } .phone-field :global(.ui-search) { width: 100%; } }
	/* The same floating glass pill as Explore's type switch. */
	.types { display: flex; justify-content: center; pointer-events: none; }
	.types :global(.ui-seg) { pointer-events: auto; border-radius: 999px; padding: 3px; background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.types :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 999px; }
	.types :global(.ui-seg button) { min-height: 44px; padding: 0 20px; border-radius: 999px; }
	@media (max-width: 767px) { .types :global(.ui-seg) { display: grid; width: 100%; } .types :global(.ui-seg button) { padding: 0 6px; } }
	@media (max-width: 419px) { .types :global(.ui-seg button span) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; } }
	.results { display: flex; flex-direction: column; gap: 32px; min-width: 0; }
	.h2 { margin: 0; font: var(--t-title3); }
	.h1 { margin: 0; font: var(--t-title1); letter-spacing: -0.02em; }
	/* Browse: tall tiles, 6 / 5 / 4 / 3 / 2 across (browseCols matches), 24 px gaps. */
	.browse { display: flex; flex-direction: column; gap: 16px; }
	.tiles { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 24px; margin: 0; padding: 0; list-style: none; }
	@media (max-width: 1439px) { .tiles { grid-template-columns: repeat(5, minmax(0, 1fr)); } }
	@media (max-width: 1279px) { .tiles { grid-template-columns: repeat(4, minmax(0, 1fr)); } }
	@media (max-width: 899px) { .tiles { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; } }
	@media (max-width: 599px) { .tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; } }
	.ghost { aspect-ratio: 3 / 4; border-radius: 16px; background: var(--surface-2); }
	.mood-head { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; margin-bottom: 8px; }
	.mood-head p { margin: 0; }
	.mood { display: flex; flex-direction: column; gap: 16px; }
	.n { font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.muted { color: var(--text-muted); }
	.recent { display: flex; flex-direction: column; gap: 12px; }
	.recent-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; max-width: 760px; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--title-item), 1fr)); gap: 16px 5px; margin: -16px 0 0; padding: 0; list-style: none; }
	.empty { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; max-width: 640px; }
	.tips { margin: 0; padding-left: 20px; color: var(--text-muted); font: var(--t-body); display: grid; gap: 4px; }
	.searching { margin: 0; }
	.notice { margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
</style>
