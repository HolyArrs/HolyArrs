<script module lang="ts">
	// Back to Search reuses what was on screen: the answers kept when Search was
	// left, for that history entry and address only (a fresh visit loads anew).
	let kept: { key: string; data: unknown } | null = null;
	function entryKey(): string {
		if (typeof history === 'undefined') return '';
		const i = (history.state as Record<string, unknown> | null)?.['sveltekit:history'];
		return typeof i === 'number' ? `${i}|${location.pathname}${location.search}` : '';
	}
</script>

<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { focusField, forgetReturn, playFieldEntry, restoreReturn, searchField, searchHooks } from '#lib/search-field.svelte.ts';
	import SearchField from '#lib/ui/SearchField.svelte';
	import { afterNavigate, beforeNavigate, goto, snapshot } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import { session } from '#lib/session.svelte.ts';
	import BrowseTile from '#lib/ui/BrowseTile.svelte';
	import GenreTile from '#lib/ui/GenreTile.svelte';
	import MosaicReveal, { ENTER_DELAY, ENTER_SWEEP, passAt } from '#lib/ui/MosaicReveal.svelte';
	import { browseCategoryPath, browseCols, browseHref, browsePath, browseSectionPath, browseType, BROWSE_ROWS, musicTiles, readCategory, readMood, shownCount, strictTiles, tileHue, type PlainTile, type BrowseAnswer, type BrowseSection, type BrowseSectionAnswer, type BrowseTile as Tile, type CategoryView, type MoodView } from '#lib/search-browse.ts';
	import { reducedMotion } from '#lib/motion.ts';
	import MetadataNotice from '#lib/MetadataNotice.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import type { CardOwned } from '#lib/ui/CardBadges.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import { Orbit, SkRail, SkGrid } from '#lib/ui/sk/index.ts';
	import { plainReason } from '#lib/plain.ts';
	import type { Ask, LibraryFolder, LookupArtist, LookupMovie, LookupShow, MusicSettings, PickRulesView, TitleView } from '#lib/types.ts';
	import type { PersonSummary } from '#lib/people.ts';
	import { byDept, chipDept, PEOPLE_ROWS, SHELF_MIN, visibleChips, type PeopleFilter, type ShelfMeta, type ShelfPerson } from '#lib/people-shelves.ts';
	import { appPicture, artistHref } from '#lib/music-pages.ts';
	import { fetchMusicExplore, genreHref, musicCardHref, musicCardNote, musicImage, type MusicCardData } from '#lib/music-trending.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import QuickAsk from '#lib/QuickAsk.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { albumWords } from '#lib/music.ts';
	import { createLatestOnly } from '#lib/history.ts';
	import { live } from '#lib/live-search.ts';
	import { addRecent, libraryNote, readRecent, rankTop, readScope, recentKey, scopeAddress, type SearchScope } from '#lib/search.ts';
	import { partOn, partsNow, refreshParts } from '#lib/parts.svelte.ts';
	import { detailPath } from '#lib/title-details.ts';
	import { discoverCard, followsByTmdb, genrePath, ownedMark, type ExploreCard, type GenreAnswer, type ProviderItem } from '#lib/explore.ts';
	import { audienceOf } from '#lib/title-status.ts';
	import { posterStatus } from '#lib/library.ts';

	// Search is a page; its one field is the top bar's (lib/search-field.svelte.ts;
	// on phones, where the top bar is hidden, a copy sits at the page top). The words and the scope
	// live in the address (?q=, ?kind=). Before typing it shows recent searches
	// (this device, per user) and the Browse grid (genres and their moods, per
	// tab); a mood tile opens a mood view here (?mood=&genre=&type=). Results
	// come in as you type. Kid limits are applied by the server on every lookup.

	type Card = { key: string; type: Exclude<SearchScope, 'all'>; titleId?: number; name: string; year?: number; image?: string | null; href: string; owned: CardOwned | null; preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number; titleId?: number }; artist?: LookupArtist; subtitle?: string; /** A movie or show as an Explore card, for its Add or Ask. */ ec?: ExploreCard };

	// The words live in the shared field (top bar); a direct link fills it from ?q=.
	if (!searchField.term.trim()) searchField.term = page.url.searchParams.get('q') ?? '';
	let scope = $state<SearchScope>(readScope(page.url.searchParams.get('kind')));
	let busy = $state(false);
	let error = $state('');
	let noTMDB = $state(false);
	let searched = $state('');
	let groups = $state<Record<Card['type'], Card[] | null>>({ show: null, movie: null, music: null, people: null });
	/** What was on screen when the current search started: kept up until the new answers arrive. */
	let stale = $state.raw<{ groups: Record<Card['type'], Card[] | null>; lib: LibraryAnswer | null } | null>(null);
	let plainTiles = $state<PlainTile[] | null>(null);
	/** Music scope with the v2 rows: the genre tiles (hue and grey art), each opening its Music genre page. */
	let musicGenres = $state<MusicCardData[]>([]);
	let plainFor = '';
	let mgenre = $state<string | null>(page.url.searchParams.get('mgenre')?.trim() || null);
	let mgenreCards = $state<(MusicCardData & { note?: string })[] | null>(null);
	let tiles = $state<Tile[] | null>(null);
	let tilesFor = '';
	let mood = $state<MoodView | null>(readMood(page.url.searchParams));
	let moodItems = $state<ProviderItem[] | null>(null);
	let moodGenre = $state('');
	// Browse sections after the page's genre tiles (Moods, Stories…), each loaded as the grid nears it;
	// a category tile opens a category view here (?cat=&type=).
	let sections = $state<BrowseSection[]>([]);
	let sectionTiles = $state<Record<string, Tile[] | null>>({});
	let cat = $state<CategoryView | null>(readCategory(page.url.searchParams));
	let catItems = $state<ProviderItem[] | null>(null);
	let catName = $state('');
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
	const LAND_MS = 700;
	function exit() {
		if (leaving) return;
		revealing = false;
		leaving = true;
		// The tiles drop out quickly, last visible row first; the band then sweeps back up (ease-in).
		// The page stays put and the pixels cover it, bottom up.
		covering = true;
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
		if (calm || restored) return;
		const elapsed = performance.now() - t0;
		// The first screen only: tiles that arrive after the entry just appear.
		if (elapsed > ENTER_DELAY + ENTER_SWEEP) return;
		const top = document.querySelector('.ui-topbar')?.getBoundingClientRect().bottom ?? 0;
		const r = node.getBoundingClientRect();
		if (r.top > innerHeight) return;
		const f = (r.top - top) / Math.max(1, innerHeight - top);
		// The pixels themselves reveal the tile (it is already there under the
		// mosaic); as the band passes its row the tile only settles a hair, so the
		// page comes through the pixelation instead of after it.
		const start = Math.min(ENTER_DELAY + ENTER_SWEEP - LAND_MS, passAt(Math.max(0, f)) + at.col * 18);
		const delay = Math.max(0, start - elapsed);
		node.style.willChange = 'transform';
		const a = node.animate([{ transform: 'scale(1.025)' }, { transform: 'none' }], { duration: LAND_MS, delay, easing: 'cubic-bezier(.22,1,.36,1)', fill: 'backwards' });
		a.onfinish = a.oncancel = () => (node.style.willChange = '');
	}
	let cols = $state(6);
	let recent = $state<string[]>([]);
	const beginSearch = createLatestOnly();
	const who = $derived(audienceOf(session.can));
	const manage = $derived(session.can('manage_library'));

	const scopeOptions = $derived(
		([
			{ value: 'all', label: 'All', icon: 'grid' },
			{ value: 'people', label: 'People', icon: 'people' },
			{ value: 'movie', label: 'Movies', icon: 'film' },
			{ value: 'show', label: 'TV shows', icon: 'tv' },
			{ value: 'music', label: 'Music', icon: 'music' }
		] as { value: SearchScope; label: string; icon: IconName }[]).filter((o) => (o.value === 'show' ? partOn('shows') : o.value === 'movie' ? partOn('movies') : o.value === 'music' ? partOn('music') : true))
	);
	const labels: Record<Card['type'], string> = { show: 'TV shows', movie: 'Movies', music: 'Music', people: 'People' };
	const order = $derived(scopeOptions.map((o) => o.value).filter((v): v is Card['type'] => v !== 'all'));

	// ── Library marking: one mark for every card, as on Explore ────────────
	const inLibrary: CardOwned = { state: 'available', label: 'In your Library' };
	function askMark(ask: { state: string; mine: boolean } | undefined | null): CardOwned | null {
		if (!ask || ask.state === 'turned_down') return null;
		return { state: 'requested', label: ask.state === 'approved' ? 'Approved, waiting for a copy' : 'Requested' };
	}
	/** asCard: a search result as an Explore card, so its + adds or asks the same way. */
	const asCard = (key: string, kind: 'movie' | 'show', name: string, year: number | undefined, tmdb: string, titleId: number | undefined, ask: LookupMovie['ask']): ExploreCard =>
		({ key, kind, name, year, titleId, ref: `tmdb:${tmdb}`, status: titleId ? 'in_library' : undefined, badges: [], mine: !!ask?.mine, ask: ask && ask.state !== 'turned_down' ? ask : undefined });
	const movieCard = (m: LookupMovie): Card | null =>
		m.ids.tmdb
			? { key: `m${m.ids.tmdb}`, type: 'movie', titleId: m.title_id || undefined, name: m.name, year: m.year || undefined, subtitle: m.year ? String(m.year) : undefined, image: m.poster, href: to(m.title_id ? `/library/${m.title_id}` : detailPath('movie', m.ids.tmdb)), owned: m.title_id ? inLibrary : askMark(m.ask), preview: { kind: 'movie', tmdb: m.ids.tmdb, year: m.year || undefined, titleId: m.title_id || undefined }, ec: asCard(`m${m.ids.tmdb}`, 'movie', m.name, m.year || undefined, m.ids.tmdb, m.title_id || undefined, m.ask) }
			: null;
	const showCard = (s: LookupShow, i: number): Card => {
		const tmdb = s.ids.tmdb;
		return { key: `s${s.source}${s.ids[s.source] ?? s.name}${i}`, type: 'show', titleId: s.title_id || undefined, name: s.name, year: s.year, subtitle: s.year ? String(s.year) : undefined, image: s.poster, href: to(s.title_id ? `/library/${s.title_id}` : tmdb ? detailPath('show', tmdb) : `/search?q=${encodeURIComponent(s.name)}&kind=show`), owned: s.title_id ? inLibrary : askMark(s.ask), preview: tmdb ? { kind: 'show', tmdb, year: s.year, titleId: s.title_id || undefined } : undefined, ec: tmdb ? asCard(`s${s.source}${s.ids[s.source] ?? s.name}${i}`, 'show', s.name, s.year, tmdb, s.title_id || undefined, s.ask) : undefined };
	};
	const artistCard = (a: LookupArtist): Card => ({ key: `a${a.mbid}`, type: 'music', titleId: a.title_id || undefined, name: a.name, subtitle: a.disambiguation || [a.type, a.country].filter(Boolean).join(' · ') || undefined, image: appPicture(a.image), href: artistHref(a.mbid), owned: a.title_id ? inLibrary : askMark(a.ask), artist: a });
	const personCard = (p: PersonSummary): Card => ({ key: `p${p.id}`, type: 'people', name: p.name, subtitle: p.department, image: p.profile, href: to(`/people/${encodeURIComponent(p.id)}`), owned: null });

	// ── In your Library: local matches first, left out of the wider results ──
	type LibraryAnswer = { titles: TitleView[]; albums: { id: number; title_id: number; name: string; artist: string; year?: number; image?: string }[]; people: { name: string; titles: { id: number; name: string; kind: string }[] }[] };
	let lib = $state<LibraryAnswer | null>(null);
	const ONLY_KEY = 'holyarrs.search.only-library';
	let onlyLib = $state(false);
	function setOnlyLib(v: boolean) {
		onlyLib = v;
		try {
			localStorage.setItem(ONLY_KEY, v ? '1' : '0');
		} catch {
			/* a private window: the choice just is not remembered */
		}
	}
	const whereOptions: { value: 'everywhere' | 'library'; label: string }[] = [{ value: 'everywhere', label: 'Everywhere' }, { value: 'library', label: 'In your Library' }];
	const markOf = (t: TitleView) => ownedMark({ key: String(t.id), kind: t.kind === 'show' ? 'show' : t.kind === 'artist' ? 'artist' : 'movie', name: t.name, titleId: t.id } as ExploreCard, t, who);
	// While a search runs and nothing has come back yet, the page keeps what it showed before.
	const freshTotal = $derived(order.reduce((n, t) => n + (groups[t]?.length ?? 0), 0) + (lib ? lib.titles.length + lib.albums.length + lib.people.length : 0));
	// Results land together: the skeleton stays until every group answered, so rows never push
	// each other down (loading audit #1). A slow group stops holding the rest after 2.5 s.
	let settleCap = $state(false);
	$effect(() => {
		void searchField.term;
		settleCap = false;
		const t = setTimeout(() => (settleCap = true), 2500);
		return () => clearTimeout(t);
	});
	const waiting = $derived(!!searchField.term.trim() && (order.some((t) => groups[t] === null) || busy || lib === null) && (freshTotal === 0 || !settleCap));
	const holding = $derived(waiting && !!stale);
	// A fresh search keeps the page you were on until its answer is slow: the skeleton shows once the
	// request has been out 200 ms (the 250 ms typing pause doesn't count), so a quick answer swaps straight in.
	let slow = $state(false);
	$effect(() => {
		if (!(waiting && !stale && busy)) return void (slow = false);
		const t = setTimeout(() => (slow = true), 200);
		return () => clearTimeout(t);
	});
	const shownGroups = $derived(holding && stale ? stale.groups : groups);
	const shownLib = $derived(holding && stale ? stale.lib : lib);
	const libCards = $derived<Card[]>([
		...(shownLib?.titles ?? []).map((t): Card => ({ key: `lt${t.id}`, type: t.kind === 'show' ? 'show' : t.kind === 'artist' ? 'music' : 'movie', titleId: t.id, name: t.name, year: t.year, image: t.backdrop ?? t.artwork[0], href: to(`/library/${t.id}`), owned: markOf(t), preview: t.ids?.tmdb && t.kind !== 'artist' ? { kind: t.kind === 'show' ? 'show' : 'movie', tmdb: t.ids.tmdb, year: t.year, titleId: t.id } : undefined, subtitle: libraryNote(t, [t.year, posterStatus(t, who)?.label].filter(Boolean).join(' · ')) })),
		...(shownLib?.albums ?? []).map((a): Card => ({ key: `la${a.id}`, type: 'music', name: a.name, image: a.image ? to(a.image) : undefined, href: to(`/library/${a.title_id}`), owned: inLibrary, subtitle: [a.artist, a.year].filter(Boolean).join(' · ') })),
		...(shownLib?.people ?? []).map((p): Card => ({ key: `lp${p.name}`, type: 'people', name: p.name, href: to(`/library/${p.titles[0]?.id}`), owned: null, subtitle: p.titles.map((x) => x.name).slice(0, 2).join(', ') }))
	]);
	const libIds = $derived(new Set((shownLib?.titles ?? []).map((t) => t.id)));
	const wider = $derived(Object.fromEntries(order.map((t) => [t, onlyLib ? [] : (shownGroups[t] ?? []).filter((c) => !c.titleId || !libIds.has(c.titleId))])) as Record<Card['type'], Card[]>);
	const lists = $derived(order.map((t) => wider[t] ?? []));
	const top = $derived(rankTop(lists.filter((_, i) => order[i] !== 'people'), searched, 6));
	const total = $derived(lists.reduce((n, g) => n + g.length, 0) + libCards.length);
	const pending = $derived(order.some((t) => groups[t] === null));

	// ── Searching ──────────────────────────────────────────────────────────
	const ownAddresses = new Set<string>();
	function writeAddress() {
		const url = scopeAddress(page.url, scope, searchField.term);
		if (searchField.term.trim() || !(mood || mgenre || cat)) for (const k of ['mood', 'genre', 'type', 'mgenre', 'cat']) url.searchParams.delete(k);
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
		timer = setTimeout(() => void run(undefined, false), 250);
	}

	// Typing never asks MusicBrainz while live search is on (or not known
	// yet): the Music group waits for Enter (or a page opened with the
	// words) and the dropdown shows the music matches meanwhile. Movies,
	// shows and people follow typing as before.
	async function run(e?: SubmitEvent, full = true) {
		e?.preventDefault();
		clearTimeout(timer);
		const q = searchField.term.trim();
		writeAddress();
		const isLatest = beginSearch();
		// Which parts are on decides which groups to ask; wait for the answer once.
		if (!partsNow.answer) {
			await refreshParts();
			if (!isLatest()) return;
		}
		error = '';
		noTMDB = false;
		if (q) mood = mgenre = cat = null;
		if (!q) {
			lib = null;
			stale = null;
			searched = '';
			busy = false;
			return;
		}
		if (e) remember(q);
		stale = total > 0 ? { groups: shownGroups, lib: shownLib } : null;
		busy = true;
		const holdMusic = !full && live.on !== false;
		const want = (scope === 'all' ? order : order.filter((t) => t === scope)).filter((t) => !(holdMusic && t === 'music'));
		const next: Record<Card['type'], Card[] | null> = { show: null, movie: null, music: null, people: null };
		for (const t of order) next[t] = want.includes(t) ? null : [];
		groups = next;
		const enc = encodeURIComponent(q);
		lib = null;
		api.get<LibraryAnswer>(`/library/search?q=${enc}&kind=${scope}`)
			.then((r) => {
				if (isLatest()) lib = r;
			})
			.catch(() => {
				if (isLatest()) lib = { titles: [], albums: [], people: [] };
			});
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
		focusField();
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
		sections = [];
		sectionTiles = {};
		sectionRows = {};
		try {
			const a = await api.get<BrowseAnswer>(browsePath(t));
			if (tilesFor !== t) return;
			tiles = strictTiles(a.tiles ?? [], t).filter((x) => (x.kind === 'show' ? partOn('shows') : partOn('movies')));
			sections = a.sections ?? [];
		} catch {
			if (tilesFor === t) {
				tiles = [];
				tilesFor = '';
			}
		}
	}
	// A section loads once, in order (the server keeps backdrops unique across them).
	// While the server is still building it, ghost tiles show and the page asks again.
	const sectionBusy = new Set<string>();
	// Rows shown per section: BROWSE_ROWS first, more as its end nears the viewport.
	let sectionRows = $state<Record<string, number>>({});
	const rowsOf = (id: string) => sectionRows[id] ?? BROWSE_ROWS;
	function moreRows(node: HTMLElement, id: string) {
		if (typeof IntersectionObserver === 'undefined') {
			sectionRows = { ...sectionRows, [id]: 1000 };
			return;
		}
		const io = new IntersectionObserver((e) => { if (e.some((x) => x.isIntersecting)) sectionRows = { ...sectionRows, [id]: rowsOf(id) + BROWSE_ROWS }; }, { rootMargin: '0px 0px 600px 0px' });
		io.observe(node);
		return { destroy: () => io.disconnect() };
	}
	async function loadSection(id: string) {
		const t = tilesFor as ReturnType<typeof browseType> | '';
		if (!t || sectionTiles[id] !== undefined || sectionBusy.has(`${t}|${id}`)) return;
		sectionBusy.add(`${t}|${id}`);
		try {
			const a = await api.get<BrowseSectionAnswer>(browseSectionPath(t, id));
			if (a.building) {
				setTimeout(() => { if (tilesFor === t) void loadSection(id); }, 2500);
				return;
			}
			if (tilesFor === t) sectionTiles = { ...sectionTiles, [id]: a.tiles ?? [] };
		} catch {
			if (tilesFor === t) sectionTiles = { ...sectionTiles, [id]: [] };
		} finally {
			sectionBusy.delete(`${t}|${id}`);
		}
	}
	// The sections shown: every loaded one, then the next to load (its sentinel)
	// once the one before shows all its rows.
	const shownSections = $derived.by(() => {
		const out: { s: BrowseSection; tiles: Tile[] | null }[] = [];
		for (const s of sections) {
			const got = sectionTiles[s.id];
			out.push({ s, tiles: got ?? null });
			if (got === undefined) break;
			const n = s.id === 'genres' ? (tiles?.length ?? 0) + (got ?? []).length : (got ?? []).length;
			if (shownCount(n, cols, rowsOf(s.id)) < n) break;
		}
		return out;
	});
	const genreTiles = $derived([...(tiles ?? []), ...(sectionTiles.genres ?? [])]);
	function near(node: HTMLElement, id: string) {
		if (typeof IntersectionObserver === 'undefined') {
			void loadSection(id);
			return;
		}
		const io = new IntersectionObserver((e) => { if (e.some((x) => x.isIntersecting)) void loadSection(id); }, { rootMargin: '0px 0px 900px 0px' });
		io.observe(node);
		return { destroy: () => io.disconnect() };
	}
	// People: TMDB's popular people, 20 a page, up to 150, loaded as the grid nears the bottom.
	// The Actors and Directors chips filter what is loaded, and keep loading until each has ~30.
	const PEOPLE_MAX = 150;
	const PEOPLE_PAGES = 8;
	const PEOPLE_WANT = 30;
	// Ahead of the popular list: From your Library and In what's hot right now, each loaded as it nears
	// and shown a few rows at a time; a section or chip with fewer than eight people stays hidden.
	let shelves = $state<ShelfMeta[] | null>(null);
	let shelfPeople = $state<Record<string, ShelfPerson[]>>({});
	let shelfRows = $state<Record<string, number>>({});
	const shelfBusy = new Set<string>();
	async function loadShelves() {
		try {
			shelves = (await api.get<{ shelves: ShelfMeta[] }>('/people/shelves')).shelves ?? [];
		} catch {
			shelves = [];
		}
	}
	async function loadShelf(id: string) {
		if (shelfBusy.has(id) || shelfPeople[id]) return;
		shelfBusy.add(id);
		try {
			const r = await api.get<{ people: ShelfPerson[] }>(`/people/shelves/${encodeURIComponent(id)}`);
			shelfPeople = { ...shelfPeople, [id]: (r.people ?? []).filter((p) => p.profile) };
		} catch {
			shelfPeople = { ...shelfPeople, [id]: [] };
		} finally {
			shelfBusy.delete(id);
		}
	}
	// The sections shown: every loaded one, then the next to load (its sentinel); the popular list follows once all are in.
	const shownShelves = $derived.by(() => {
		const out: { s: ShelfMeta; people: ShelfPerson[] | null }[] = [];
		for (const s of shelves ?? []) {
			const got = shelfPeople[s.id];
			out.push({ s, people: got ? byDept(got, chipDept(peopleFilter)) : null });
			if (!got) break;
		}
		return out;
	});
	const shelvesDone = $derived(shelves !== null && (shelves ?? []).every((s) => shelfPeople[s.id]));
	const shelfTiles = (people: ShelfPerson[], id: string) => people.slice(0, (shelfRows[id] ?? PEOPLE_ROWS) * cols).map((p) => ({ key: `p${p.id}`, name: p.name, note: p.note, href: to(`/people/${encodeURIComponent(p.id)}`), image: p.profile }));
	function nearShelf(node: HTMLElement, id: string) {
		const go = () => (shelfPeople[id] ? (shelfRows = { ...shelfRows, [id]: (shelfRows[id] ?? PEOPLE_ROWS) + PEOPLE_ROWS }) : void loadShelf(id));
		if (typeof IntersectionObserver === 'undefined') {
			go();
			return;
		}
		const io = new IntersectionObserver((e) => { if (e.some((x) => x.isIntersecting)) go(); }, { rootMargin: '0px 0px 600px 0px' });
		io.observe(node);
		return { destroy: () => io.disconnect() };
	}
	let peopleAll = $state<PersonSummary[] | null>(null);
	let peoplePage = 0;
	let peopleBusy = $state(false);
	let peopleEnd = $state(false);
	let peopleFilter = $state<PeopleFilter>('popular');
	let peopleSentinel = $state<HTMLElement>();
	const peopleChips = $derived(visibleChips(shelves ?? [], peopleAll ?? []));
	const peopleShown = $derived.by(() => {
		return byDept(peopleAll ?? [], chipDept(peopleFilter));
	});
	const peopleTiles = $derived<PlainTile[] | null>(peopleAll === null ? null : peopleShown.map((p) => ({ key: `p${p.id}`, name: p.name, href: to(`/people/${encodeURIComponent(p.id)}`), image: p.profile as string })));
	const peopleWantsMore = () => !peopleEnd && (peopleAll?.length ?? 0) < PEOPLE_MAX && peopleShown.length < (peopleFilter === 'popular' ? PEOPLE_MAX : PEOPLE_WANT);
	async function morePeople() {
		if (peopleBusy || peopleEnd) return;
		peopleBusy = true;
		try {
			while (peopleWantsMore() || peopleAll === null) {
				const r = await api.get<{ people: PersonSummary[]; page: number; total_pages: number }>(`/people/popular?page=${peoplePage + 1}`);
				peoplePage = r.page || peoplePage + 1;
				const have = new Set((peopleAll ?? []).map((p) => p.id));
				const fresh = r.people.filter((p) => p.profile && !have.has(p.id) && have.add(p.id));
				peopleAll = [...(peopleAll ?? []), ...fresh].slice(0, PEOPLE_MAX);
				if (peoplePage >= PEOPLE_PAGES || peoplePage >= (r.total_pages || PEOPLE_PAGES) || r.people.length === 0 || peopleAll.length >= PEOPLE_MAX) peopleEnd = true;
				if (peopleFilter === 'popular') break;
			}
		} catch {
			peopleEnd = true;
			peopleAll ??= [];
		} finally {
			peopleBusy = false;
		}
	}
	// The next page loads when the grid's end comes within a screen; a fresh observer after each load re-checks a short grid.
	$effect(() => {
		void peopleAll?.length;
		void peopleFilter;
		if (scope !== 'people' || !peopleSentinel || peopleEnd || typeof IntersectionObserver === 'undefined') return;
		const io = new IntersectionObserver((e) => { if (e.some((x) => x.isIntersecting)) void morePeople(); }, { rootMargin: '0px 0px 800px 0px' });
		io.observe(peopleSentinel);
		return () => io.disconnect();
	});
	// People and Music have their own tiles: popular people, and the music genres Explore Music names.
	async function loadPlain(s: 'people' | 'music') {
		if (plainFor === s) return;
		plainFor = s;
		plainTiles = null;
		try {
			if (s === 'people') {
				await loadShelves();
			} else {
				const r = await fetchMusicExplore('', 'global');
				if (plainFor === s && r.v2) musicGenres = r.rows.find((row) => row.shape === 'tile')?.items ?? [];
				if (plainFor === s) plainTiles = musicGenres.length ? [] : musicTiles(r, (g) => to(`/search?kind=music&mgenre=${encodeURIComponent(g)}`), musicImage);
			}
		} catch {
			if (plainFor === s) {
				plainTiles = [];
				plainFor = '';
			}
		}
	}
	$effect(() => {
		if (searchField.term.trim() || mood || mgenre || cat) return;
		if (scope === 'people' || scope === 'music') void loadPlain(scope);
		else void loadTiles(browseFor);
	});

	// A music genre view: that genre's Explore Music cards, as a grid.
	let mgenreFor = '';
	async function loadMgenre(g: string) {
		if (mgenreFor === g) return;
		mgenreFor = g;
		mgenreCards = null;
		try {
			const r = await fetchMusicExplore(g, 'global');
			if (mgenreFor !== g) return;
			const seen = new Set<string>();
			mgenreCards = r.rows.flatMap((row) => row.items.map((c) => ({ ...c, note: musicCardNote(row, c) }))).filter((c) => !seen.has(c.key) && !!seen.add(c.key));
		} catch {
			if (mgenreFor === g) {
				mgenreCards = [];
				mgenreFor = '';
			}
		}
	}
	$effect(() => {
		if (mgenre && !searchField.term.trim()) void loadMgenre(mgenre);
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
	// ── A category view: that Browse category's titles, as a grid ──────────
	let catFor = '';
	async function loadCat(c: CategoryView) {
		const k = `${c.type}|${c.id}`;
		if (catFor === k) return;
		catFor = k;
		catItems = null;
		try {
			const a = await api.get<{ name: string; items: ProviderItem[] }>(browseCategoryPath(c.type, c.id));
			if (catFor !== k) return;
			catName = a.name;
			catItems = a.items ?? [];
		} catch {
			if (catFor === k) {
				catItems = [];
				catFor = '';
			}
		}
	}
	$effect(() => {
		if (cat && !searchField.term.trim()) void loadCat(cat);
	});
	const catCards = $derived((catItems ?? []).map((item) => ({ item, c: discoverCard(item, 'search-category') })));
	const moodCards = $derived((moodItems ?? []).map((item) => ({ item, c: discoverCard(item, 'search-mood') })));

	// ── Back restores Search exactly ───────────────────────────────────────
	// The answers come from `kept` (no refetch, no entry animation); the
	// snapshot holds the spot: scroll, loaded Browse sections and rows, people
	// pages and the chip. The scroll comes back once the page is tall enough.
	const kept0 = kept && kept.key === entryKey() ? (kept.data as ReturnType<typeof keep>) : null;
	kept = null;
	let restored = !!kept0;
	function keep() {
		return $state.snapshot({ groups, lib, searched, tiles, tilesFor, sections, sectionTiles, sectionRows, plainTiles, plainFor, musicGenres, shelves, shelfPeople, shelfRows, peopleAll, peoplePage, peopleEnd, peopleFilter, catItems, catFor, catName, moodItems, moodFor, moodGenre, mgenreCards, mgenreFor });
	}
	if (kept0) {
		const k = kept0;
		groups = k.groups;
		lib = k.lib;
		searched = k.searched;
		tiles = k.tiles;
		tilesFor = k.tilesFor;
		sections = k.sections;
		sectionTiles = k.sectionTiles;
		sectionRows = k.sectionRows;
		plainTiles = k.plainTiles;
		musicGenres = k.musicGenres ?? [];
		plainFor = k.plainFor;
		shelves = k.shelves;
		shelfPeople = k.shelfPeople;
		shelfRows = k.shelfRows;
		peopleAll = k.peopleAll;
		peoplePage = k.peoplePage;
		peopleEnd = k.peopleEnd;
		peopleFilter = k.peopleFilter;
		catItems = k.catItems;
		catFor = k.catFor;
		catName = k.catName;
		moodItems = k.moodItems;
		moodFor = k.moodFor;
		moodGenre = k.moodGenre;
		mgenreCards = k.mgenreCards;
		mgenreFor = k.mgenreFor;
		revealing = false;
	}
	beforeNavigate(({ to: dest }) => {
		if (dest?.url.pathname === location.pathname) return;
		const key = entryKey();
		kept = key ? { key, data: keep() } : null;
	});
	type SearchReturn = { y: number; sections: string[]; rows: Record<string, number>; peoplePages: number; only: boolean };
	let backTo = $state<SearchReturn | null>(null);
	let backTries = 0;
	snapshot<SearchReturn>({
		id: 'search',
		capture: () => ({ y: Math.round(scrollY), sections: Object.keys(sectionTiles), rows: { ...sectionRows }, peoplePages: peoplePage, only: onlyLib }),
		restore: (v) => {
			if (!v) return;
			restored = true;
			revealing = false;
			onlyLib = v.only;
			sectionRows = { ...v.rows, ...sectionRows };
			if (v.y > 0) backTo = v;
		}
	});
	$effect(() => {
		const v = backTo;
		if (!v) return;
		void [lib, groups, tiles, sections, sectionTiles, sectionRows, plainTiles, shelves, shelfPeople, peopleAll, catItems, moodItems, mgenreCards];
		void tick().then(() => {
			if (backTo !== v) return;
			const short = document.documentElement.scrollHeight < v.y + innerHeight - 1;
			if (short && backTries++ < 20) {
				// Load what was loaded before: the next Browse section, or the next page of people.
				const next = v.sections.find((id) => sectionTiles[id] === undefined);
				if (next) void loadSection(next);
				else if (scope === 'people' && peoplePage < v.peoplePages) void morePeople();
				else setTimeout(() => backTo === v && (backTo = { ...v }), 150);
				return;
			}
			backTo = null;
			backTries = 0;
			scrollTo({ top: v.y, behavior: 'instant' });
		});
	});

	onMount(() => {
		try {
			onlyLib = localStorage.getItem(ONLY_KEY) === '1';
		} catch {
			/* a private window: the chip starts off */
		}
		loadRecent();
		const sync = () => (cols = browseCols(innerWidth));
		sync();
		addEventListener('resize', sync);
		if (manage) {
			api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
			api.get<MusicSettings>('/settings/music').then((r) => (musicDefaults = r)).catch(() => {});
		}
		if (searchField.term.trim() && !kept0) void run();
		searchHooks.submit = () => void run(new SubmitEvent('submit'));
		searchHooks.blur = blurred;
		searchHooks.exit = exit;
		if (!(document.activeElement instanceof HTMLInputElement)) focusField();
		playFieldEntry(calm || restored);
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
		cat = readCategory(dest.url.searchParams);
		mgenre = dest.url.searchParams.get('mgenre')?.trim() || null;
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
	/** The + on a movie or show card asks or adds as on Explore. */
	let flow: ReturnType<typeof QuickAsk> | undefined = $state();
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

{#snippet plus(c: ExploreCard, act: 'add' | 'ask')}
	<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void flow?.open(c, act)} />
{/snippet}

{#snippet discover(item: ProviderItem, raw: ExploreCard)}
	{@const c = flow ? flow.live(raw) : raw}
	{@const act = flow?.action(c) ?? null}
	<WideCard cover title={c.name} href={to(item.title_id ? `/library/${item.title_id}` : detailPath(item.kind, item.tmdb_id))} image={c.wide ?? c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} owned={ownedMark(c, undefined, who)} fresh={c.fresh} preview={{ kind: item.kind, tmdb: item.tmdb_id, year: item.year, titleId: c.titleId }} followTmdb={followsByTmdb(c)}>
		{#snippet growAction()}{#if act}{@render plus(c, act)}{/if}{/snippet}
	</WideCard>
{/snippet}

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
		{@const e = c.ec && flow ? flow.live(c.ec) : c.ec}
		{@const act = e ? (flow?.action(e) ?? null) : null}
		<WideCard cover title={c.name} href={c.href || undefined} image={c.image} kind={c.type === 'music' ? 'artist' : c.type === 'people' ? undefined : c.type} subtitle={c.subtitle} owned={e && e !== c.ec ? ownedMark(e, undefined, who) : c.owned} typed={scope === 'all'} preview={c.preview} followTmdb={!!e && followsByTmdb(e)}>
			{#snippet growAction()}{#if e && act}{@render plus(e, act)}{/if}{/snippet}
		</WideCard>
	{/if}
{/snippet}

{#snippet resultsSkeleton()}
	<!-- Results-shaped placeholder while a search runs: rows of wide cards where the results will land. -->
	<div class="results-sk" aria-busy="true" aria-label="Searching" data-testid="search-skeleton">
		{#if scope === 'all'}
			<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="140px" />
			<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="90px" />
			<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="110px" />
		{:else}
			<SkGrid kind="wide" min="var(--title-item)" count={12} />
		{/if}
	</div>
{/snippet}

{#snippet libraryRow()}
	{#if libCards.length}
		<div data-testid="search-library-row">
			<Shelf title="In your Library" item="var(--title-item)" placeholder="wide" lines={1}>
				{#each libCards as c (c.key)}<WideCard cover title={c.name} href={c.href} image={c.image} kind={c.type === 'music' ? 'artist' : c.type === 'people' ? undefined : c.type} subtitle={c.subtitle} owned={c.owned} typed={scope === 'all'} preview={c.preview} />{/each}
			</Shelf>
		</div>
	{/if}
{/snippet}

<section class="search" class:calm data-testid="search-page">
	<h1 class="sr-only">Search</h1>
	<div class="close-row"><Button variant="plain" icon="x" onclick={exit} data-testid="search-page-close">Close</Button></div>
	<div class="phone-field"><SearchField searchKey="" testid="search-box-phone" id="q-phone" closable={false} /></div>
	<MetadataNotice />

	{#if scopeOptions.length > 2}
		<div class="types" data-pin-pills>
			<SegmentedControl options={scopeOptions} value={scope} label="Look in" testid="search-scope" onchange={setScope} />
		</div>
	{/if}

	{#if searchField.term.trim()}
		<div class="results-head" data-testid="search-results-head">
			<p class="muted count" style:visibility={waiting || !searched ? 'hidden' : null}>{total} {total === 1 ? 'result' : 'results'}</p>
			<SegmentedControl size="sm" options={whereOptions} value={onlyLib ? 'library' : 'everywhere'} label="Show results from" testid="search-only-library" onchange={(v) => setOnlyLib(v === 'library')} />
		</div>
	{/if}

	<div class="results" class:holding aria-live="polite" aria-busy={busy}>
		{#if waiting}<p class="sr-only">Searching for “{searchField.term.trim()}”…</p>{/if}
		{#if !searchField.term.trim() && mgenre}
			<section class="mood" aria-labelledby="mgenre-h" data-testid="search-music-genre">
				<div class="mood-head">
					<Button variant="plain" icon="chevronLeft" href={to('/search?kind=music')}>Browse</Button>
					<h2 id="mgenre-h" class="h1">{mgenre.replace(/\b\w/g, (m) => m.toUpperCase())}</h2>
					<p class="muted">Music</p>
				</div>
				{#if mgenreCards === null}
					<div class="grid-sk" aria-busy="true" aria-label="Loading titles"><SkGrid kind="wide" min="var(--title-item)" count={12} /></div>
				{:else if mgenreCards.length === 0}
					<div class="empty"><h2 class="h2">Nothing here yet</h2><p class="muted">No {mgenre} music to show right now.</p></div>
				{:else}
					<ul class="grid">
						{#each mgenreCards as c (c.key)}
							<li><WideCard cover title={c.name} href={musicCardHref(c)} image={musicImage(c)} kind="artist" subtitle={c.note || c.artist} owned={c.title_id ? inLibrary : null} /></li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if !searchField.term.trim() && cat}
			<section class="mood" aria-labelledby="cat-h" data-testid="search-category">
				<div class="mood-head">
					<Button variant="plain" icon="chevronLeft" href={to(`/search${cat.type === 'movies' ? '?kind=movie' : cat.type === 'shows' ? '?kind=show' : ''}`)}>Browse</Button>
					<h2 id="cat-h" class="h1">{catName || 'Browse'}</h2>
					<p class="muted">{cat.type === 'shows' ? 'TV shows' : cat.type === 'all' ? 'Movies and TV shows' : 'Movies'}</p>
				</div>
				{#if catItems === null}
					<div class="grid-sk" aria-busy="true" aria-label="Loading titles"><SkGrid kind="wide" min="var(--title-item)" count={12} /></div>
				{:else if catCards.length === 0}
					<div class="empty"><h2 class="h2">Nothing here yet</h2><p class="muted">No titles fit this right now.</p></div>
				{:else}
					<ul class="grid">
						{#each catCards as { item, c } (c.key)}
							<li>{@render discover(item, c)}</li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if !searchField.term.trim() && mood}
			<section class="mood" aria-labelledby="mood-h" data-testid="search-mood">
				<div class="mood-head">
					<Button variant="plain" icon="chevronLeft" href={to(`/search${scope === 'all' ? '' : `?kind=${scope}`}`)}>Browse</Button>
					<h2 id="mood-h" class="h1">{mood.mood}</h2>
					<p class="muted">{moodGenre ? `${moodGenre} · ` : ''}{mood.type === 'shows' ? 'TV shows' : mood.type === 'all' ? 'Movies and TV shows' : 'Movies'}</p>
				</div>
				{#if moodItems === null}
					<div class="grid-sk" aria-busy="true" aria-label="Loading titles"><SkGrid kind="wide" min="var(--title-item)" count={12} /></div>
				{:else if moodCards.length === 0}
					<div class="empty"><h2 class="h2">Nothing here yet</h2><p class="muted">No {mood.type === 'shows' ? 'shows' : mood.type === 'all' ? 'titles' : 'movies'} fit this mood right now. Try the genre page instead.</p><Button variant="secondary" href={to(mood.type === 'all' ? `/explore?genre=${mood.genre}` : `/explore?type=${mood.type}&genre=${mood.genre}`)}>Open {moodGenre || 'the genre'}</Button></div>
				{:else}
					<ul class="grid">
						{#each moodCards as { item, c } (c.key)}
							<li>{@render discover(item, c)}</li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if waiting && !stale && slow}
			{@render resultsSkeleton()}
		{:else if !searchField.term.trim() || (waiting && !stale)}
			{#if recent.length}
				<section class="recent" aria-labelledby="recent-h">
					<div class="recent-head">
						<h2 id="recent-h" class="sec">Recent searches</h2>
						<button type="button" class="clear-all" onclick={clearRecent}>Clear<span class="sr-only"> recent searches</span></button>
					</div>
					<div class="chips">
						{#each recent as q (q)}<Chip label={q} icon="clock" onclick={() => pickRecent(q)} />{/each}
					</div>
				</section>
			{/if}
			<section class="browse" aria-labelledby="browse-h" data-testid="search-browse">
				<!-- On the sectioned tabs the section names head the page; "Browse" stays for screen readers only. -->
				<h2 id="browse-h" class={scope === 'music' ? 'h1' : 'sr-only'}>{scope === 'people' ? 'People' : 'Browse'}</h2>
				{#if scope === 'people'}
					<div class="chips people-chips" role="group" aria-label="Filter people">
						{#each peopleChips as c (c.value)}<Chip label={c.label} selected={peopleFilter === c.value} onclick={() => { peopleFilter = c.value; void morePeople(); }} />{/each}
					</div>
				{/if}
				{#if scope === 'people'}
					{#if shelves === null}
						<ul class="tiles" aria-hidden="true">
							{#each Array(cols * 2) as _, i (i)}<li class="ghost sk" use:landing={{ row: Math.floor(i / cols), col: i % cols }}></li>{/each}
						</ul>
					{:else}
						{#each shownShelves as { s, people } (s.id)}
							{#if people && people.length >= SHELF_MIN}
								{@const shown = shelfTiles(people, s.id)}
								<section class="browse-section" aria-labelledby={`people-${s.id}-h`} data-testid={`search-people-${s.id}`}>
									<h3 id={`people-${s.id}-h`} class="h1">{s.title}</h3>
									<ul class="tiles">
										{#each shown as t, i (t.key)}
											<li use:landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} note={t.note} href={t.href} image={t.image} hue="#555b66" plain row={Math.min(6, Math.floor(i / cols))} /></li>
										{/each}
									</ul>
									{#if shown.length < people.length}
										{#key shelfRows[s.id]}<div class="more" use:nearShelf={s.id}></div>{/key}
									{/if}
								</section>
							{/if}
							{#if people === null}
								<div class="more" use:nearShelf={s.id} aria-hidden="true"><ul class="tiles">{#each Array(cols) as _, i (i)}<li class="ghost sk"></li>{/each}</ul></div>
							{/if}
						{/each}
						{#if shelvesDone && (!peopleEnd || peopleShown.length >= SHELF_MIN)}
							<section class="browse-section" aria-labelledby="people-popular-h" data-testid="search-people-popular">
								<h3 id="people-popular-h" class="h1">Popular</h3>
								{#if peopleTiles && peopleTiles.length >= SHELF_MIN}
									<ul class="tiles">
										{#each peopleTiles as t, i (t.key)}
											<li use:landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={t.href} image={t.image} hue="#555b66" plain row={Math.min(6, Math.floor(i / cols))} /></li>
										{/each}
									</ul>
								{/if}
								<div bind:this={peopleSentinel} class="more" aria-busy={peopleBusy}>{#if peopleBusy}<Orbit />{/if}</div>
							</section>
						{:else if shelvesDone && !shownShelves.some((x) => (x.people?.length ?? 0) >= SHELF_MIN)}
							<p class="muted">Search for an actor, director or artist</p>
						{/if}
					{/if}
				{:else if scope === 'music'}
					{#if musicGenres.length}
						<div class="gtiles" data-testid="music-genre-tiles">
							{#each musicGenres as c (c.key)}<GenreTile name={c.name} href={genreHref(c)} hue={c.hue ?? 0} art={c.image ? (musicImage(c) ?? '') : ''} />{/each}
						</div>
					{:else if plainTiles === null}
						<ul class="tiles" aria-hidden="true">
							{#each Array(cols * 2) as _, i (i)}<li class="ghost sk" use:landing={{ row: Math.floor(i / cols), col: i % cols }}></li>{/each}
						</ul>
					{:else if plainTiles.length}
						<ul class="tiles">
							{#each plainTiles as t, i (t.key)}
								<li use:landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={t.href} image={t.image} hue={tileHue(t.name)} row={Math.min(6, Math.floor(i / cols))} /></li>
							{/each}
						</ul>
					{:else}
						<p class="muted">Music genres show up here once Explore has something to show.</p>
					{/if}
				{:else if tiles === null}
					<ul class="tiles" aria-hidden="true">
						{#each Array(cols * 2) as _, i (i)}<li class="ghost sk" use:landing={{ row: Math.floor(i / cols), col: i % cols }}></li>{/each}
					</ul>
				{:else if genreTiles.length || sections.length}
					{#each shownSections as { s, tiles: st } (s.id)}
						{@const all = s.id === 'genres' ? genreTiles : (st ?? [])}
						{@const list = all.slice(0, shownCount(all.length, cols, rowsOf(s.id)))}
						{#if list.length}
							<section class="browse-section" aria-labelledby={`browse-${s.id}-h`} data-testid={`search-browse-${s.id}`}>
								<h3 id={`browse-${s.id}-h`} class="h1">{s.title}</h3>
								<ul class="tiles">
									{#each list as t, i (`${t.category ?? ''}:${t.kind}:${t.genre}:${t.mood ?? ''}`)}
										<li use:landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={to(browseHref(t, browseFor))} image={t.backdrop} hue={tileHue(t.name)} row={Math.min(6, Math.floor(i / cols))} /></li>
									{/each}
								</ul>
								{#if list.length < all.length}<div class="more" use:moreRows={s.id}></div>{/if}
							</section>
						{/if}
						{#if st === null}
							<div class="more" use:near={s.id} aria-live="polite">
								{#if s.id !== 'genres'}
									<section class="browse-section" aria-label={`Loading ${s.title.toLowerCase()}`}>
										<h3 class="h1">{s.title}</h3>
										<ul class="tiles" aria-hidden="true">{#each Array(cols * BROWSE_ROWS) as _, i (i)}<li class="ghost sk"></li>{/each}</ul>
									</section>
								{/if}
							</div>
						{/if}
					{/each}
					{#if !sections.length}
						<ul class="tiles">
							{#each genreTiles as t, i (`${t.kind}:${t.genre}:${t.mood ?? ''}`)}
								<li use:landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={to(browseHref(t, browseFor))} image={t.backdrop} hue={tileHue(t.name)} row={Math.min(6, Math.floor(i / cols))} /></li>
							{/each}
						</ul>
					{/if}
				{:else}
					<p class="muted">Genres show up here once Explore has titles to show.</p>
				{/if}
			</section>
		{:else if noTMDB && total === 0}
			<div class="empty"><h2 class="h2">The metadata server is down</h2><p class="muted">Searches and new artwork come back when it is. Try again in a minute.</p></div>
		{:else if error && total === 0}
			<p class="notice bad" role="alert">{plainReason(error).text}</p>
		{:else if total === 0 && (pending || busy)}
			{@render resultsSkeleton()}
		{:else if total === 0 && searched}
			<div class="empty" data-testid="search-empty">
				<h2 class="h2">Nothing found for “{searched}”</h2>
				<ul class="tips">
					<li>Check the spelling, or use fewer words.</li>
					<li>Add the year, such as “Dune 2021”.</li>
					<li>Paste an id, such as tmdb:693134 or tvmaze:82.</li>
				</ul>
				{#if scope !== 'all'}<Button variant="secondary" onclick={() => setScope('all')}>Look everywhere</Button>{/if}
				{#if manage}<Button variant="primary" icon="search" href={to('/settings/sources')}>Search indexers for “{searched}”</Button>{/if}
			</div>
		{:else if scope === 'all'}
			{@render libraryRow()}
			{#if top.length}
				<Shelf title="Top matches" item="var(--title-item)" placeholder="wide" lines={1}>
					{#each top as c (c.key)}{@render card(c)}{/each}
				</Shelf>
			{/if}
			{#each order as t (t)}
				{@const g = wider[t]}
				{#if g?.length}
					<Shelf title={labels[t]} item="var(--title-item)" placeholder="wide" lines={1} onseeall={() => setScope(t)}>
						{#each g as c (c.key)}{@render card(c)}{/each}
					</Shelf>
				{/if}
			{/each}
		{:else}
			{@const g = wider[scope] ?? []}
			{@render libraryRow()}
			{#if g.length}
				<h2 class="sec">{labels[scope]}</h2>
				<ul class="grid">
					{#each g as c (c.key)}<li>{@render card(c)}</li>{/each}
				</ul>
			{/if}
		{/if}
	</div>
</section>

{#if revealing}<MosaicReveal ondone={() => (revealing = false)} />{/if}
{#if covering}<MosaicReveal exit ondone={left} />{/if}

<AddArtistDialog bind:this={addArtist} {folders} {rules} defaults={musicDefaults} onadded={artistAdded} />
<AskArtistDialog bind:this={askArtist} onasked={artistAsked} onrefused={artistRefused} />
<QuickAsk bind:this={flow} />

<style>
	/* Music genre tiles: 16:9, 5/3/2 columns, as on Explore → Music. */
	.gtiles { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; }
	@media (max-width: 1023px) { .gtiles { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
	@media (max-width: 640px) { .gtiles { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	.search { --title-item: max(200px, calc((100vw - 96px) / 5.16)); display: flex; flex-direction: column; gap: 24px; padding-bottom: 48px; min-width: 0; }
	@media (max-width: 1279px) { .search { --title-item: max(180px, calc((100vw - 64px) / 4.2)); } }
	@media (max-width: 767px) { .search { --title-item: calc((100vw - 40px) / 2.3); } }
	/* Desktop closes from the × in the top-bar field; phones (no top bar) keep this one. */
	.close-row { display: none; }
	@media (max-width: 767px) { .close-row { display: flex; justify-content: flex-end; margin-bottom: -16px; } }
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
	.results-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; margin-bottom: -12px; }
	.results-head .count { margin: 0; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.results-head :global(.ui-seg) { flex: none; }
	.results { display: flex; flex-direction: column; gap: 32px; min-width: 0; transition: opacity 160ms ease-out; }
	.results.holding { opacity: 0.6; }
	.h2 { margin: 0; font: var(--t-title3); }
	.sec { margin: 0; font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.h1 { margin: 0; font: var(--t-title1); letter-spacing: -0.02em; }
	/* Browse: tall tiles, 6 / 5 / 4 / 3 / 2 across (browseCols matches), 24 px gaps. */
	.browse { display: flex; flex-direction: column; gap: 16px; }
	.tiles { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 24px; margin: 0; padding: 0; list-style: none; }
	@media (max-width: 1439px) { .tiles { grid-template-columns: repeat(5, minmax(0, 1fr)); } }
	@media (max-width: 1279px) { .tiles { grid-template-columns: repeat(4, minmax(0, 1fr)); } }
	@media (max-width: 899px) { .tiles { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; } }
	@media (max-width: 599px) { .tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; } }
	.browse-section { display: flex; flex-direction: column; gap: 16px; margin-top: 16px; }
	.browse-section:first-of-type { margin-top: 0; }
	.browse-section h3 { margin: 0; }
	/* Ghost tiles use the shared Silhouette shimmer (tokens.css .sk), sized like BrowseTile. */
	.ghost { aspect-ratio: 3 / 4; border-radius: 16px; }
	.more .tiles { width: 100%; }
	.grid-sk { min-width: 0; }
	.results-sk { display: flex; flex-direction: column; gap: 32px; min-width: 0; }
	.mood-head { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; margin-bottom: 8px; }
	.mood-head p { margin: 0; }
	.mood { display: flex; flex-direction: column; gap: 16px; }
	.muted { color: var(--text-muted); }
	.recent { display: flex; flex-direction: column; gap: 12px; }
	/* The same head as a Shelf row: the name at the left, its action at the right edge like See all. */
	.recent-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.clear-all { display: inline-flex; align-items: center; min-height: 44px; flex: none; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--accent-text); font: var(--t-callout); font-weight: 500; cursor: pointer; }
	.clear-all:hover { text-decoration: underline; text-underline-offset: 3px; }
	.clear-all:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.people-chips { margin: 0 0 16px; }
	.more { min-height: 40px; display: grid; place-items: center; padding: 16px 0; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--title-item), 1fr)); gap: 16px 5px; margin: -16px 0 0; padding: 0; list-style: none; }
	.empty { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; max-width: 640px; }
	.tips { margin: 0; padding-left: 1.2em; list-style: disc; color: var(--text-muted); font: var(--t-body); display: grid; gap: 4px; }
	.tips li::marker { color: var(--text-faint, var(--text-muted)); }
	.notice { margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
</style>
