// Search's browse views: the genre and mood tiles, People shelves, Music genres and their pages.
import type { SearchScope } from './search-address.ts';
import { browseCategoryPath, browsePath, browseSectionPath, browseType, BROWSE_ROWS, musicTiles, readCategory, readMood, shownCount, strictTiles, type PlainTile, type BrowseAnswer, type BrowseSection, type BrowseSectionAnswer, type BrowseTile as Tile, type CategoryView, type MoodView } from './search-browse.ts';
import { byDept, chipDept, PEOPLE_ROWS, visibleChips, type PeopleFilter, type ShelfMeta, type ShelfPerson } from './people-shelves.ts';
import { genrePath, type GenreAnswer } from './genres.ts';
import type { PersonSummary } from '#lib/ui/people.ts';
import { discoverCard, type ProviderItem } from '#lib/ui/title-cards.ts';
import { fetchMusicExplore, genreHref, musicCardNote, musicImage, type MusicCardData } from '#lib/music/index.ts';
import { partOn } from '#lib/mediafiles/index.ts';
import { page } from '$app/state';
import { to } from '#lib/core/paths.ts';
import { searchField } from '#lib/core/search-field.svelte.ts';
import { api } from '#lib/core/api.ts';

export function createSearchBrowse(ctx: { scope: SearchScope; cols: number }) {
	let plainTiles = $state<PlainTile[] | null>(null);
	/** Music scope with the v2 rows: the genre tiles (hue and grey art), each opening its Music genre page. */
	let musicGenres = $state<MusicCardData[]>([]);
	/** The same genres as the shared Music tile: an album cover each, and none without one. */
	const musicGenreTiles = $derived(musicGenres.filter((c) => c.cover).map((c) => ({ key: c.key, name: c.name, href: genreHref(c), covers: [c.cover as string] })));
	// With music_lists_v1 row templates: the Browse sections (genres, moods, seasons, decades, activities).
	let musicSections = $state<{ id: string; title: string; tiles: { key: string; name: string; href: string; hue: number; covers: string[] }[] }[]>([]);
	let plainFor = '';
	let mgenre = $state<string | null>(page.url.searchParams.get('mgenre')?.trim() || null);
	let mgenreCards = $state<(MusicCardData & { note?: string })[] | null>(null);
	let tiles = $state<Tile[] | null>(null);
	let tilesFor = '';
	let mood = $state<MoodView | null>(readMood(page.url.searchParams));
	let moodItems = $state<ProviderItem[] | null>(null);
	let moodGenre = $state('');
	// Browse sections after the page's genre tiles (Moods, Stories), each loaded as the grid nears it; a category tile opens a category view here (?cat=&type=).
	let sections = $state<BrowseSection[]>([]);
	let sectionTiles = $state<Record<string, Tile[] | null>>({});
	let cat = $state<CategoryView | null>(readCategory(page.url.searchParams));
	let catItems = $state<ProviderItem[] | null>(null);
	let catName = $state('');
	const browseFor = $derived(browseType(ctx.scope));
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
	// A section loads once, in order (the server keeps backdrops unique across them). While the server is still building it, ghost tiles show and the page asks again.
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
	// The sections shown: every loaded one, then the next to load (its sentinel) once the one before shows all its rows.
	const shownSections = $derived.by(() => {
		const out: { s: BrowseSection; tiles: Tile[] | null }[] = [];
		for (const s of sections) {
			const got = sectionTiles[s.id];
			out.push({ s, tiles: got ?? null });
			if (got === undefined) break;
			const n = s.id === 'genres' ? (tiles?.length ?? 0) + (got ?? []).length : (got ?? []).length;
			if (shownCount(n, ctx.cols, rowsOf(s.id)) < n) break;
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
	// People: TMDB's popular people, 20 a page, up to 150, loaded as the grid nears the bottom. The Actors and Directors chips filter what is loaded, and keep loading until each has about 30.
	const PEOPLE_MAX = 150;
	const PEOPLE_PAGES = 8;
	const PEOPLE_WANT = 30;
	// Ahead of the popular list: From your Library and In what's hot right now, each loaded as it nears and shown a few rows at a time; a section or chip with fewer than eight people stays hidden.
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
	const shelfTiles = (people: ShelfPerson[], id: string) => people.slice(0, (shelfRows[id] ?? PEOPLE_ROWS) * ctx.cols).map((p) => ({ key: `p${p.id}`, name: p.name, note: p.note, href: to(`/people/${encodeURIComponent(p.id)}`), image: p.profile }));
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
		if (ctx.scope !== 'people' || !peopleSentinel || peopleEnd || typeof IntersectionObserver === 'undefined') return;
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
				const b = await api.get<{ sections: typeof musicSections }>(`/music/browse-tiles?${new URLSearchParams({ region: 'global' })}`).catch(() => null);
				if (plainFor === s && b?.sections?.length) {
					musicSections = b.sections;
					plainTiles = [];
					return;
				}
				const r = await fetchMusicExplore('', 'global');
				if (plainFor === s && r.v2) musicGenres = (r.rows.find((row) => row.shape === 'tile' || row.shape === 'grid')?.items ?? []).map((c) => (c.image || !c.cover ? c : { ...c, image: c.cover }));
				if (plainFor === s) plainTiles = musicGenres.some((c) => c.cover) ? [] : musicTiles(r, (g) => to(`/search?kind=music&mgenre=${encodeURIComponent(g)}`), musicImage);
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
		if (ctx.scope === 'people' || ctx.scope === 'music') void loadPlain(ctx.scope);
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

	return {
		get mood() {
			return mood;
		},
		set mood(v) {
			mood = v;
		},
		get mgenre() {
			return mgenre;
		},
		set mgenre(v) {
			mgenre = v;
		},
		get cat() {
			return cat;
		},
		set cat(v) {
			cat = v;
		},
		get tiles() {
			return tiles;
		},
		set tiles(v) {
			tiles = v;
		},
		get tilesFor() {
			return tilesFor;
		},
		set tilesFor(v) {
			tilesFor = v;
		},
		get sections() {
			return sections;
		},
		set sections(v) {
			sections = v;
		},
		get sectionTiles() {
			return sectionTiles;
		},
		set sectionTiles(v) {
			sectionTiles = v;
		},
		get sectionRows() {
			return sectionRows;
		},
		set sectionRows(v) {
			sectionRows = v;
		},
		get plainTiles() {
			return plainTiles;
		},
		set plainTiles(v) {
			plainTiles = v;
		},
		get plainFor() {
			return plainFor;
		},
		set plainFor(v) {
			plainFor = v;
		},
		get musicGenres() {
			return musicGenres;
		},
		set musicGenres(v) {
			musicGenres = v;
		},
		get musicSections() {
			return musicSections;
		},
		set musicSections(v) {
			musicSections = v;
		},
		get shelves() {
			return shelves;
		},
		set shelves(v) {
			shelves = v;
		},
		get shelfPeople() {
			return shelfPeople;
		},
		set shelfPeople(v) {
			shelfPeople = v;
		},
		get shelfRows() {
			return shelfRows;
		},
		set shelfRows(v) {
			shelfRows = v;
		},
		get peopleAll() {
			return peopleAll;
		},
		set peopleAll(v) {
			peopleAll = v;
		},
		get peoplePage() {
			return peoplePage;
		},
		set peoplePage(v) {
			peoplePage = v;
		},
		get peopleEnd() {
			return peopleEnd;
		},
		set peopleEnd(v) {
			peopleEnd = v;
		},
		get peopleFilter() {
			return peopleFilter;
		},
		set peopleFilter(v) {
			peopleFilter = v;
		},
		get catItems() {
			return catItems;
		},
		set catItems(v) {
			catItems = v;
		},
		get catFor() {
			return catFor;
		},
		set catFor(v) {
			catFor = v;
		},
		get catName() {
			return catName;
		},
		set catName(v) {
			catName = v;
		},
		get moodItems() {
			return moodItems;
		},
		set moodItems(v) {
			moodItems = v;
		},
		get moodFor() {
			return moodFor;
		},
		set moodFor(v) {
			moodFor = v;
		},
		get moodGenre() {
			return moodGenre;
		},
		set moodGenre(v) {
			moodGenre = v;
		},
		get mgenreCards() {
			return mgenreCards;
		},
		set mgenreCards(v) {
			mgenreCards = v;
		},
		get mgenreFor() {
			return mgenreFor;
		},
		set mgenreFor(v) {
			mgenreFor = v;
		},
		loadSection,
		morePeople,
		get catCards() {
			return catCards;
		},
		get moodCards() {
			return moodCards;
		},
		get peopleChips() {
			return peopleChips;
		},
		get shownShelves() {
			return shownShelves;
		},
		shelfTiles,
		get shelvesDone() {
			return shelvesDone;
		},
		get peopleShown() {
			return peopleShown;
		},
		get peopleTiles() {
			return peopleTiles;
		},
		get peopleSentinel() {
			return peopleSentinel;
		},
		set peopleSentinel(v) {
			peopleSentinel = v;
		},
		get peopleBusy() {
			return peopleBusy;
		},
		set peopleBusy(v) {
			peopleBusy = v;
		},
		get musicGenreTiles() {
			return musicGenreTiles;
		},
		get genreTiles() {
			return genreTiles;
		},
		get shownSections() {
			return shownSections;
		},
		rowsOf,
		get browseFor() {
			return browseFor;
		},
		nearShelf,
		moreRows,
		near,
	};
}
