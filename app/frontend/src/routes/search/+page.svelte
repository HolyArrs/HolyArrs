<script module lang="ts">
	// Back to Search reuses what was on screen: the answers kept when Search was left, for that history entry and address only (a fresh visit loads anew).
	import { historyIndex, SearchField, BrowseTile, MusicBrowseSections, MosaicReveal, browseCols, browseHref, BROWSE_ROWS, readCategory, readMood, shownCount, tileHue, SHELF_MIN, live, libraryNote, rankTop, readScope, scopeAddress, type SearchScope, describes, found, plainPath, understood, type PlainAnswer } from '#lib/discover/index.ts';
	let kept: { key: string; data: unknown } | null = null;
	function entryKey(): string {
		if (typeof history === 'undefined') return '';
		const i = historyIndex();
		return i !== null ? `${i}|${location.pathname}${location.search}` : '';
	}
</script>

<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { createSearchEntry } from '#lib/discover/index.ts';
	import { artistCard, inLibrary, movieCard, personCard, showCard, type Card, type LibraryAnswer } from '#lib/discover/index.ts';
	import { createSearchArtists } from '#lib/discover/index.ts';
	import { createSearchRecent } from '#lib/discover/index.ts';
	import { createSearchBrowse } from '#lib/discover/index.ts';
	import { type IconName } from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import { Orbit, SkRail, SkGrid } from '#lib/ui/sk/index.ts';
	import type { PersonSummary } from '#lib/ui/people.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { discoverCard, followsByTmdb, ownedMark, type ExploreCard, type ProviderItem } from '#lib/ui/title-cards.ts';
	import { audienceOf } from '#lib/ui/title-status.ts';
	import { MusicTileGrid, musicCardHref, musicImage } from '#lib/music/index.ts';
	import { MetadataNotice, detailPath } from '#lib/metadata/index.ts';
	import type { LookupArtist, MusicSettings } from '#lib/music/index.ts';
	import { AddArtistDialog, AskArtistDialog, QuickAsk } from '#lib/requests/index.ts';
	import { createLatestOnly } from '#lib/downloads/index.ts';
	import { partOn, partsNow, refreshParts, posterStatus } from '#lib/mediafiles/index.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { afterNavigate, beforeNavigate, goto, snapshot } from '$app/navigation';
	import { page } from '$app/state';
	import { to } from '#lib/core/paths.ts';
	import { focusField, forgetReturn, playFieldEntry, searchField, searchHooks } from '#lib/core/search-field.svelte.ts';
	import { api, message, ApiError } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { reducedMotion } from '#lib/core/motion.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import type { Ask, LibraryFolder, LookupMovie, PickRulesView, TitleView } from '#lib/core/types.ts';
	import type { LookupShow } from '#lib/core/types.ts';

	// Search is a page; its one field is the top bar's (on phones, where the top bar is hidden, a copy sits at the page top). The words and the scope live in the address (?q=, ?kind=). Before typing it shows recent searches (this device, per user) and the Browse grid (genres and their moods, per tab); a mood tile opens a mood view here (?mood=&genre=&type=). Results come in as you type. Kid limits are applied by the server on every lookup.


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
	const browseCtx = {
		get scope() {
			return scope;
		},
		get cols() {
			return cols;
		},
	};
	const browse = createSearchBrowse(browseCtx);
	// The entry: a mosaic dissolve down the whole visible page, the tiles rising row by row behind it; under reduced motion the band alone. Timing: a 120 ms hold, the band builds to about 900 ms, the last visible row lands by about 1.1 s, the field's glow settles by about 1.4 s. Only transform and opacity move.
	const calm = reducedMotion();
	let revealing = $state(true);
	const entryCtx = {
		get revealing() {
			return revealing;
		},
		set revealing(v) {
			revealing = v;
		},
		get calm() {
			return calm;
		},
		get restored() {
			return restored;
		},
	};
	const entry = createSearchEntry(entryCtx);
	let cols = $state(6);
	const recentsCtx = {
		run,
	};
	const recents = createSearchRecent(recentsCtx);
	const beginSearch = createLatestOnly();
	const who = $derived(audienceOf(session.can));
	const manage = $derived(session.can('manage_library'));

	const scopeOptions = $derived(
		([
			{ value: 'all', label: t('discover.search.all'), icon: 'grid' },
			{ value: 'people', label: t('discover.search.people'), icon: 'people' },
			{ value: 'movie', label: t('discover.search.movies'), icon: 'film' },
			{ value: 'show', label: t('discover.search.tvShows'), icon: 'tv' },
			{ value: 'music', label: t('discover.search.music'), icon: 'music' }
		] as { value: SearchScope; label: string; icon: IconName }[]).filter((o) => (o.value === 'show' ? partOn('shows') : o.value === 'movie' ? partOn('movies') : o.value === 'music' ? partOn('music') : true))
	);
	const labels = $derived<Record<Card['type'], string>>({ show: t('discover.search.tvShows'), movie: t('discover.search.movies'), music: t('discover.search.music'), people: t('discover.search.people') });
	const order = $derived(scopeOptions.map((o) => o.value).filter((v): v is Card['type'] => v !== 'all'));


	let lib = $state<LibraryAnswer | null>(null);
	// Search in plain words: asked only when the words describe a title.
	let plainAns = $state<PlainAnswer | null>(null);
	const plainShown = $derived(found(plainAns) && plainAns?.query.text === searchField.term.trim());
	const plainCards = $derived((plainAns?.more ?? []).map((item) => ({ item, c: discoverCard(item, 'plain') })));
	const ONLY_KEY = 'holyarrs.search.only-library';
	let onlyLib = $state(false);
	function setOnlyLib(v: boolean) {
		onlyLib = v;
		try {
			localStorage.setItem(ONLY_KEY, v ? '1' : '0');
		} catch {
			/* A private window: the choice just is not remembered. */
		}
	}
	const whereOptions = $derived<{ value: 'everywhere' | 'library'; label: string }[]>([{ value: 'everywhere', label: t('discover.search.everywhere') }, { value: 'library', label: t('discover.search.inYourLibrary') }]);
	const markOf = (t: TitleView) => ownedMark({ key: String(t.id), kind: t.kind === 'show' ? 'show' : t.kind === 'artist' ? 'artist' : 'movie', name: t.name, titleId: t.id } as ExploreCard, t, who);
	// While a search runs and nothing has come back yet, the page keeps what it showed before.
	const freshTotal = $derived(order.reduce((n, t) => n + (groups[t]?.length ?? 0), 0) + (lib ? lib.titles.length + lib.albums.length + lib.people.length : 0));
	// Results land together: the skeleton stays until every group answered, so rows never push each other down. A slow group stops holding the rest after 2.5 s.
	let settleCap = $state(false);
	$effect(() => {
		void searchField.term;
		settleCap = false;
		const t = setTimeout(() => (settleCap = true), 2500);
		return () => clearTimeout(t);
	});
	const waiting = $derived(!!searchField.term.trim() && (order.some((t) => groups[t] === null) || busy || lib === null) && (freshTotal === 0 || !settleCap));
	const holding = $derived(waiting && !!stale);
	// A fresh search keeps the page you were on until its answer is slow: the skeleton shows once the request has been out 200 ms (the 250 ms typing pause does not count), so a quick answer swaps straight in.
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

	const ownAddresses = new Set<string>();
	function writeAddress() {
		const url = scopeAddress(page.url, scope, searchField.term);
		if (searchField.term.trim() || !(browse.mood || browse.mgenre || browse.cat)) for (const k of ['mood', 'genre', 'type', 'mgenre', 'cat']) url.searchParams.delete(k);
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

	// Typing never asks MusicBrainz while live search is on (or not known yet): the Music group waits for Enter (or a page opened with the words) and the dropdown shows the music matches meanwhile. Movies, shows and people follow typing as before.
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
		if (q) browse.mood = browse.mgenre = browse.cat = null;
		if (!q) {
			lib = null;
			stale = null;
			searched = '';
			busy = false;
			return;
		}
		if (e) recents.remember(q);
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
		plainAns = null;
		if (describes(q))
			api.get<PlainAnswer>(plainPath(q))
				.then((r) => {
					if (isLatest()) plainAns = r;
				})
				.catch(() => {});
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


	// Back restores Search exactly. The answers come from `kept` (no refetch, no entry animation); the snapshot holds the spot: scroll, loaded Browse sections and rows, people pages and the chip. The scroll comes back once the page is tall enough.
	const kept0 = kept && kept.key === entryKey() ? (kept.data as ReturnType<typeof keep>) : null;
	kept = null;
	let restored = !!kept0;
	function keep() {
		return $state.snapshot({ groups, lib, searched, tiles: browse.tiles, tilesFor: browse.tilesFor, sections: browse.sections, sectionTiles: browse.sectionTiles, sectionRows: browse.sectionRows, plainTiles: browse.plainTiles, plainFor: browse.plainFor, musicGenres: browse.musicGenres, musicSections: browse.musicSections, shelves: browse.shelves, shelfPeople: browse.shelfPeople, shelfRows: browse.shelfRows, peopleAll: browse.peopleAll, peoplePage: browse.peoplePage, peopleEnd: browse.peopleEnd, peopleFilter: browse.peopleFilter, catItems: browse.catItems, catFor: browse.catFor, catName: browse.catName, moodItems: browse.moodItems, moodFor: browse.moodFor, moodGenre: browse.moodGenre, mgenreCards: browse.mgenreCards, mgenreFor: browse.mgenreFor });
	}
	if (kept0) {
		const k = kept0;
		groups = k.groups;
		lib = k.lib;
		searched = k.searched;
		browse.tiles = k.tiles;
		browse.tilesFor = k.tilesFor;
		browse.sections = k.sections;
		browse.sectionTiles = k.sectionTiles;
		browse.sectionRows = k.sectionRows;
		browse.plainTiles = k.plainTiles;
		browse.musicGenres = k.musicGenres ?? [];
		browse.musicSections = k.musicSections ?? [];
		browse.plainFor = k.plainFor;
		browse.shelves = k.shelves;
		browse.shelfPeople = k.shelfPeople;
		browse.shelfRows = k.shelfRows;
		browse.peopleAll = k.peopleAll;
		browse.peoplePage = k.peoplePage;
		browse.peopleEnd = k.peopleEnd;
		browse.peopleFilter = k.peopleFilter;
		browse.catItems = k.catItems;
		browse.catFor = k.catFor;
		browse.catName = k.catName;
		browse.moodItems = k.moodItems;
		browse.moodFor = k.moodFor;
		browse.moodGenre = k.moodGenre;
		browse.mgenreCards = k.mgenreCards;
		browse.mgenreFor = k.mgenreFor;
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
		capture: () => ({ y: Math.round(scrollY), sections: Object.keys(browse.sectionTiles), rows: { ...browse.sectionRows }, peoplePages: browse.peoplePage, only: onlyLib }),
		restore: (v) => {
			if (!v) return;
			restored = true;
			revealing = false;
			onlyLib = v.only;
			browse.sectionRows = { ...v.rows, ...browse.sectionRows };
			if (v.y > 0) backTo = v;
		}
	});
	$effect(() => {
		const v = backTo;
		if (!v) return;
		void [lib, groups, browse.tiles, browse.sections, browse.sectionTiles, browse.sectionRows, browse.plainTiles, browse.shelves, browse.shelfPeople, browse.peopleAll, browse.catItems, browse.moodItems, browse.mgenreCards];
		void tick().then(() => {
			if (backTo !== v) return;
			const short = document.documentElement.scrollHeight < v.y + innerHeight - 1;
			if (short && backTries++ < 20) {
				// Load what was loaded before: the next Browse section, or the next page of people.
				const next = v.sections.find((id) => browse.sectionTiles[id] === undefined);
				if (next) void browse.loadSection(next);
				else if (scope === 'people' && browse.peoplePage < v.peoplePages) void browse.morePeople();
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
			/* A private window: the chip starts off. */
		}
		recents.loadRecent();
		const sync = () => (cols = browseCols(innerWidth));
		sync();
		addEventListener('resize', sync);
		if (manage) {
			api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (artists.folders = r.library_folders)).catch(() => {});
			api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (artists.rules = r.pick_rules)).catch(() => {});
			api.get<MusicSettings>('/settings/music').then((r) => (artists.musicDefaults = r)).catch(() => {});
		}
		if (searchField.term.trim() && !kept0) void run();
		searchHooks.submit = () => void run(new SubmitEvent('submit'));
		searchHooks.blur = blurred;
		searchHooks.exit = entry.exit;
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
			entry.cameFrom = from.url.pathname;
			entry.depth = 0;
			return;
		}
		if (type === 'popstate') entry.depth = Math.max(0, entry.depth - 1);
		else if (type === 'link') entry.depth++;
		if (ownAddresses.delete(dest.url.href)) return;
		const q = dest.url.searchParams.get('q') ?? '';
		const s = readScope(dest.url.searchParams.get('kind'));
		browse.mood = readMood(dest.url.searchParams);
		browse.cat = readCategory(dest.url.searchParams);
		browse.mgenre = dest.url.searchParams.get('mgenre')?.trim() || null;
		void tick().then(() => focusField());
		if (q === searchField.term && s === scope) return;
		searchField.term = lastTerm = q;
		scope = s;
		void run();
	});

	// Leaving the box after a search counts it as a recent one.
	function blurred() {
		if (searched && searched === searchField.term.trim()) recents.remember(searched);
	}

	const artistsCtx = {
		get groups() {
			return groups;
		},
		artistCard: artistCard,
	};
	const artists = createSearchArtists(artistsCtx);
	let addArtist: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let askArtist: ReturnType<typeof AskArtistDialog> | undefined = $state();
	/** The + on a movie or show card asks or adds as on Explore. */
	let flow: ReturnType<typeof QuickAsk> | undefined = $state();

</script>

<svelte:head><title>{searched ? t('discover.search.searchTitle', { searched }) : t('discover.search.search')} · HolyArrs</title></svelte:head>

{#snippet plus(c: ExploreCard, act: 'add' | 'ask')}
	<IconButton icon="plus" label={act === 'add' ? t('discover.search.addName', { name: c.name }) : t('discover.search.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void flow?.open(c, act)} />
{/snippet}

{#snippet discover(item: ProviderItem, raw: ExploreCard)}
	{@const c = flow ? flow.live(raw) : raw}
	{@const act = flow?.action(c) ?? null}
	<WideCard cover title={c.name} href={to(item.title_id ? `/library/${item.title_id}` : detailPath(item.kind, item.tmdb_id))} image={c.wide} poster={c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} owned={ownedMark(c, undefined, who)} fresh={c.fresh} preview={{ kind: item.kind, tmdb: item.tmdb_id, year: item.year, titleId: c.titleId }} followTmdb={followsByTmdb(c)}>
		{#snippet growAction()}{#if act}{@render plus(c, act)}{/if}{/snippet}
	</WideCard>
{/snippet}

{#snippet card(c: Card)}
	{@const a = c.artist}
	{#if a && !a.title_id && !a.never_add && (manage || (session.can('ask') && !a.ask?.mine))}
		<WideCard cover title={c.name} image={c.image} kind="artist" subtitle={c.subtitle} owned={c.owned} typed={scope === 'all'}>
			{#snippet action()}
				{#if manage}<Button variant="primary" icon="plus" onclick={() => addArtist?.open(a)}>{t('discover.search.add')}</Button>
				{:else}<Button variant="primary" icon="wish" onclick={() => askArtist?.open(a)}>{a.ask ? t('discover.search.askToo') : t('discover.search.ask')}</Button>{/if}
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
	<div class="results-sk" aria-busy="true" aria-label={t('discover.search.searching')} data-testid="search-skeleton">
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
			<Shelf title={t('discover.search.inYourLibrary')} item="var(--title-item)" placeholder="wide" lines={1}>
				{#each libCards as c (c.key)}<WideCard cover title={c.name} href={c.href} image={c.image} kind={c.type === 'music' ? 'artist' : c.type === 'people' ? undefined : c.type} subtitle={c.subtitle} owned={c.owned} typed={scope === 'all'} preview={c.preview} />{/each}
			</Shelf>
		</div>
	{/if}
{/snippet}

<section class="search" class:calm data-testid="search-page">
	<h1 class="sr-only">{t('discover.search.search')}</h1>
	<div class="close-row"><Button variant="plain" icon="x" onclick={entry.exit} data-testid="search-page-close">{t('discover.search.close')}</Button></div>
	<div class="phone-field"><SearchField searchKey="" testid="search-box-phone" id="q-phone" closable={false} /></div>
	<MetadataNotice />

	{#if scopeOptions.length > 2}
		<div class="types" data-pin-pills>
			<SegmentedControl options={scopeOptions} value={scope} label={t('discover.search.lookIn')} testid="search-scope" onchange={setScope} />
		</div>
	{/if}

	{#if searchField.term.trim()}
		<div class="results-head" data-testid="search-results-head">
			<p class="muted count" style:visibility={waiting || !searched ? 'hidden' : null}>{t(total === 1 ? 'discover.search.resultCount.one' : 'discover.search.resultCount.other', { n: total })}</p>
			<SegmentedControl size="sm" options={whereOptions} value={onlyLib ? 'library' : 'everywhere'} label={t('discover.search.showResultsFrom')} testid="search-only-library" onchange={(v) => setOnlyLib(v === 'library')} />
		</div>
	{/if}

	<div class="results" class:holding aria-live="polite" aria-busy={busy}>
		{#if waiting}<p class="sr-only">{t('discover.search.searchingFor', { v: searchField.term.trim() })}</p>{/if}
		{#if plainShown && plainAns}
			<section class="plain" aria-labelledby="plain-h" data-testid="search-plain">
				<h2 id="plain-h" class="h2">{t('discover.search.matchesForYourDescription')}</h2>
				<p class="muted understood">{t('discover.search.understoodAs', { v: understood(plainAns.query) })}</p>
				{#if plainAns.titles.length}
					<Shelf title={t('discover.search.inYourLibrary')} item="var(--title-item)" placeholder="wide" lines={1}>
						{#each plainAns.titles as t (t.id)}
							<WideCard cover title={t.name} href={to(`/library/${t.id}`)} image={t.backdrop ?? t.artwork[0]} kind={t.kind === 'artist' ? 'artist' : t.kind === 'show' ? 'show' : 'movie'} subtitle={t.year ? String(t.year) : undefined} owned={inLibrary} />
						{/each}
					</Shelf>
				{/if}
				{#if plainCards.length}
					<Shelf title={t('discover.search.moreLikeThat')} item="var(--title-item)" placeholder="wide" lines={1}>
						{#each plainCards as { item, c } (c.key)}{@render discover(item, c)}{/each}
					</Shelf>
				{/if}
			</section>
		{/if}
		{#if !searchField.term.trim() && browse.mgenre}
			<section class="mood" aria-labelledby="mgenre-h" data-testid="search-music-genre">
				<div class="mood-head">
					<Button variant="plain" icon="chevronLeft" href={to('/search?kind=music')}>{t('discover.search.browse')}</Button>
					<h2 id="mgenre-h" class="h1">{browse.mgenre.replace(/\b\w/g, (m) => m.toUpperCase())}</h2>
					<p class="muted">{t('discover.search.music')}</p>
				</div>
				{#if browse.mgenreCards === null}
					<div class="grid-sk" aria-busy="true" aria-label={t('discover.search.loadingTitles')}><SkGrid kind="wide" min="var(--title-item)" count={12} /></div>
				{:else if browse.mgenreCards.length === 0}
					<div class="empty"><h2 class="h2">{t('discover.search.nothingHereYet')}</h2><p class="muted">{t('discover.search.noMusicToShow', { mgenre: browse.mgenre })}</p></div>
				{:else}
					<ul class="grid">
						{#each browse.mgenreCards as c (c.key)}
							<li><WideCard cover title={c.name} href={musicCardHref(c)} image={musicImage(c)} kind="artist" subtitle={c.note || c.artist} owned={c.title_id ? inLibrary : null} /></li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if !searchField.term.trim() && browse.cat}
			<section class="mood" aria-labelledby="cat-h" data-testid="search-category">
				<div class="mood-head">
					<Button variant="plain" icon="chevronLeft" href={to(`/search${browse.cat.type === 'movies' ? '?kind=movie' : browse.cat.type === 'shows' ? '?kind=show' : ''}`)}>{t('discover.search.browse')}</Button>
					<h2 id="cat-h" class="h1">{browse.catName || t('discover.search.browse2')}</h2>
					<p class="muted">{browse.cat.type === 'shows' ? t('discover.search.tvShows') : browse.cat.type === 'all' ? t('discover.search.moviesAndTvShows') : t('discover.search.movies')}</p>
				</div>
				{#if browse.catItems === null}
					<div class="grid-sk" aria-busy="true" aria-label={t('discover.search.loadingTitles')}><SkGrid kind="wide" min="var(--title-item)" count={12} /></div>
				{:else if browse.catCards.length === 0}
					<div class="empty"><h2 class="h2">{t('discover.search.nothingHereYet')}</h2><p class="muted">{t('discover.search.noTitlesFitThis')}</p></div>
				{:else}
					<ul class="grid">
						{#each browse.catCards as { item, c } (c.key)}
							<li>{@render discover(item, c)}</li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if !searchField.term.trim() && browse.mood}
			<section class="mood" aria-labelledby="mood-h" data-testid="search-mood">
				<div class="mood-head">
					<Button variant="plain" icon="chevronLeft" href={to(`/search${scope === 'all' ? '' : `?kind=${scope}`}`)}>{t('discover.search.browse')}</Button>
					<h2 id="mood-h" class="h1">{browse.mood.mood}</h2>
					<p class="muted">{browse.moodGenre ? `${browse.moodGenre} · ` : ''}{browse.mood.type === 'shows' ? t('discover.search.tvShows') : browse.mood.type === 'all' ? t('discover.search.moviesAndTvShows') : t('discover.search.movies')}</p>
				</div>
				{#if browse.moodItems === null}
					<div class="grid-sk" aria-busy="true" aria-label={t('discover.search.loadingTitles')}><SkGrid kind="wide" min="var(--title-item)" count={12} /></div>
				{:else if browse.moodCards.length === 0}
					<div class="empty"><h2 class="h2">{t('discover.search.nothingHereYet')}</h2><p class="muted">{browse.mood.type === 'shows' ? t('discover.search.noShowsMood') : browse.mood.type === 'all' ? t('discover.search.noTitlesMood') : t('discover.search.noMoviesMood')}</p><Button variant="secondary" href={to(browse.mood.type === 'all' ? `/explore?genre=${browse.mood.genre}` : `/explore?type=${browse.mood.type}&genre=${browse.mood.genre}`)}>{browse.moodGenre ? t('discover.search.openGenre', { genre: browse.moodGenre }) : t('discover.search.openTheGenre')}</Button></div>
				{:else}
					<ul class="grid">
						{#each browse.moodCards as { item, c } (c.key)}
							<li>{@render discover(item, c)}</li>
						{/each}
					</ul>
				{/if}
			</section>
		{:else if waiting && !stale && slow}
			{@render resultsSkeleton()}
		{:else if !searchField.term.trim() || (waiting && !stale)}
			{#if recents.recent.length}
				<section class="recent" aria-labelledby="recent-h">
					<div class="recent-head">
						<h2 id="recent-h" class="sec">{t('discover.search.recentSearches')}</h2>
						<button type="button" class="clear-all" onclick={recents.clearRecent}>{t('discover.search.clear')}<span class="sr-only"> {t('discover.search.recentSearches2')}</span></button>
					</div>
					<div class="chips">
						{#each recents.recent as q (q)}<Chip label={q} icon="clock" onclick={() => recents.pickRecent(q)} />{/each}
					</div>
				</section>
			{/if}
			<section class="browse" aria-labelledby="browse-h" data-testid="search-browse">
				<!-- On the sectioned tabs the section names head the page; "Browse" stays for screen readers only. -->
				<h2 id="browse-h" class={scope === 'music' ? 'h1' : 'sr-only'}>{scope === 'people' ? t('discover.search.people') : t('discover.search.browse2')}</h2>
				{#if scope === 'people'}
					<div class="chips people-chips" role="group" aria-label={t('discover.search.filterPeople')}>
						{#each browse.peopleChips as c (c.value)}<Chip label={c.label} selected={browse.peopleFilter === c.value} onclick={() => { browse.peopleFilter = c.value; void browse.morePeople(); }} />{/each}
					</div>
				{/if}
				{#if scope === 'people'}
					{#if browse.shelves === null}
						<ul class="tiles" aria-hidden="true">
							{#each Array(cols * 2) as _, i (i)}<li class="ghost sk" use:entry.landing={{ row: Math.floor(i / cols), col: i % cols }}></li>{/each}
						</ul>
					{:else}
						{#each browse.shownShelves as { s, people } (s.id)}
							{#if people && people.length >= SHELF_MIN}
								{@const shown = browse.shelfTiles(people, s.id)}
								<section class="browse-section" aria-labelledby={`people-${s.id}-h`} data-testid={`search-people-${s.id}`}>
									<h3 id={`people-${s.id}-h`} class="h1">{s.title}</h3>
									<ul class="tiles">
										{#each shown as t, i (t.key)}
											<li use:entry.landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} note={t.note} href={t.href} image={t.image} hue="#555b66" plain row={Math.min(6, Math.floor(i / cols))} /></li>
										{/each}
									</ul>
									{#if shown.length < people.length}
										{#key browse.shelfRows[s.id]}<div class="more" use:browse.nearShelf={s.id}></div>{/key}
									{/if}
								</section>
							{/if}
							{#if people === null}
								<div class="more" use:browse.nearShelf={s.id} aria-hidden="true"><ul class="tiles">{#each Array(cols) as _, i (i)}<li class="ghost sk"></li>{/each}</ul></div>
							{/if}
						{/each}
						{#if browse.shelvesDone && (!browse.peopleEnd || browse.peopleShown.length >= SHELF_MIN)}
							<section class="browse-section" aria-labelledby="people-popular-h" data-testid="search-people-popular">
								<h3 id="people-popular-h" class="h1">{t('discover.search.popular')}</h3>
								{#if browse.peopleTiles && browse.peopleTiles.length >= SHELF_MIN}
									<ul class="tiles">
										{#each browse.peopleTiles as t, i (t.key)}
											<li use:entry.landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={t.href} image={t.image} hue="#555b66" plain row={Math.min(6, Math.floor(i / cols))} /></li>
										{/each}
									</ul>
								{/if}
								<div bind:this={browse.peopleSentinel} class="more" aria-busy={browse.peopleBusy}>{#if browse.peopleBusy}<Orbit />{/if}</div>
							</section>
						{:else if browse.shelvesDone && !browse.shownShelves.some((x) => (x.people?.length ?? 0) >= SHELF_MIN)}
							<p class="muted">{t('discover.search.searchForAnActor')}</p>
						{/if}
					{/if}
				{:else if scope === 'music'}
					{#if browse.musicSections.length}
						<MusicBrowseSections sections={browse.musicSections} />
					{:else if browse.musicGenreTiles.length}
						<MusicTileGrid tiles={browse.musicGenreTiles} />
					{:else if browse.plainTiles === null}
						<MusicTileGrid tiles={[]} ghosts={12} />
					{:else if browse.plainTiles.length}
						<MusicTileGrid tiles={browse.plainTiles.map((t) => ({ key: t.key, name: t.name, href: t.href, covers: [t.image] }))} />
					{:else}
						<p class="muted">{t('discover.search.musicGenresShowUp')}</p>
					{/if}
				{:else if browse.tiles === null}
					<ul class="tiles" aria-hidden="true">
						{#each Array(cols * 2) as _, i (i)}<li class="ghost sk" use:entry.landing={{ row: Math.floor(i / cols), col: i % cols }}></li>{/each}
					</ul>
				{:else if browse.genreTiles.length || browse.sections.length}
					{#each browse.shownSections as { s, tiles: st } (s.id)}
						{@const all = s.id === 'genres' ? browse.genreTiles : (st ?? [])}
						{@const list = all.slice(0, shownCount(all.length, cols, browse.rowsOf(s.id)))}
						{#if list.length}
							<section class="browse-section" aria-labelledby={`browse-${s.id}-h`} data-testid={`search-browse-${s.id}`}>
								<h3 id={`browse-${s.id}-h`} class="h1">{s.title}</h3>
								<ul class="tiles">
									{#each list as t, i (`${t.category ?? ''}:${t.kind}:${t.genre}:${t.mood ?? ''}`)}
										<li use:entry.landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={to(browseHref(t, browse.browseFor))} image={t.backdrop} hue={tileHue(t.name)} row={Math.min(6, Math.floor(i / cols))} /></li>
									{/each}
								</ul>
								{#if list.length < all.length}<div class="more" use:browse.moreRows={s.id}></div>{/if}
							</section>
						{/if}
						{#if st === null}
							<div class="more" use:browse.near={s.id} aria-live="polite">
								{#if s.id !== 'genres'}
									<section class="browse-section" aria-label={t('discover.search.loadingSection', { title: s.title.toLowerCase() })}>
										<h3 class="h1">{s.title}</h3>
										<ul class="tiles" aria-hidden="true">{#each Array(cols * BROWSE_ROWS) as _, i (i)}<li class="ghost sk"></li>{/each}</ul>
									</section>
								{/if}
							</div>
						{/if}
					{/each}
					{#if !browse.sections.length}
						<ul class="tiles">
							{#each browse.genreTiles as t, i (`${t.kind}:${t.genre}:${t.mood ?? ''}`)}
								<li use:entry.landing={{ row: Math.floor(i / cols), col: i % cols }}><BrowseTile name={t.name} href={to(browseHref(t, browse.browseFor))} image={t.backdrop} hue={tileHue(t.name)} row={Math.min(6, Math.floor(i / cols))} /></li>
							{/each}
						</ul>
					{/if}
				{:else}
					<p class="muted">{t('discover.search.genresShowUpHere')}</p>
				{/if}
			</section>
		{:else if noTMDB && total === 0}
			<div class="empty"><h2 class="h2">{t('discover.search.theMetadataServerIs')}</h2><p class="muted">{t('discover.search.searchesAndNewArtwork')}</p></div>
		{:else if error && total === 0}
			<p class="notice bad" role="alert">{plainReason(error).text}</p>
		{:else if total === 0 && (pending || busy)}
			{@render resultsSkeleton()}
		{:else if total === 0 && searched && !plainShown}
			<div class="empty" data-testid="search-empty">
				<h2 class="h2">{t('discover.search.nothingFoundFor', { searched })}</h2>
				<ul class="tips">
					<li>{t('discover.search.checkTheSpellingOr')}</li>
					<li>{t('discover.search.addTheYearSuch')}</li>
					<li>{t('discover.search.pasteAnIdSuch')}</li>
					<li>{t('discover.search.orDescribeItSuch')}</li>
				</ul>
				{#if scope !== 'all'}<Button variant="secondary" onclick={() => setScope('all')}>{t('discover.search.lookEverywhere')}</Button>{/if}
				{#if manage}<Button variant="primary" icon="search" href={to('/settings/sources')}>{t('discover.search.searchIndexersFor', { searched })}</Button>{/if}
			</div>
		{:else if scope === 'all'}
			{@render libraryRow()}
			{#if top.length}
				<Shelf title={t('discover.search.topMatches')} item="var(--title-item)" placeholder="wide" lines={1}>
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
{#if entry.covering}<MosaicReveal exit ondone={entry.left} />{/if}

<AddArtistDialog bind:this={addArtist} folders={artists.folders} rules={artists.rules} defaults={artists.musicDefaults} onadded={artists.artistAdded} />
<AskArtistDialog bind:this={askArtist} onasked={artists.artistAsked} onrefused={artists.artistRefused} />
<QuickAsk bind:this={flow} />

<style>
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
	.results-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; margin-bottom: -12px; }
	.results-head .count { margin: 0; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.results-head :global(.ui-seg) { flex: none; }
	.results { display: flex; flex-direction: column; gap: 32px; min-width: 0; transition: opacity var(--dur-fast) var(--ease-out); }
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
	/* Ghost tiles use the shared skeleton shimmer (tokens.css .sk), sized like BrowseTile. */
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
	.plain { display: grid; gap: 8px; margin-bottom: 24px; min-width: 0; }
	.plain .understood { margin: 0; font: var(--t-callout); overflow-wrap: anywhere; }
</style>
