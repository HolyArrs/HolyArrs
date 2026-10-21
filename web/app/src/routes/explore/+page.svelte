<script lang="ts" module>
	/** The rows already loaded when Explore was left, so Back shows them at once (no re-fetch flash). */
	let kept: { key: string; at: number; rows: unknown } | null = null;
</script>

<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import { anchorDelta, canReturn, putOffsets, readAnchor, readOffsets, type ExploreReturn } from '#lib/explore-return.ts';
	import { page } from '$app/state';
	import { goto, snapshot } from '$app/navigation';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { detailPath, loadDetails, remember } from '#lib/title-details.ts';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import {
		cardAction,
		cardTmdb,
		discoverCard,
		headPath,
		liveCard,
		matchOwned,
		ownedIndex,
		ownedMark,
		ownedTitle,
		markTransitions,
		cardIdentity,
		dedupeTop,
		filterRows,
		parseType,
		pickerServices,
		readServices,
		serverRow,
		serviceRegion,
		SERVICES_KEY,
		SERVICES_SHOWN,
		tailPath,
		typeHref,
		genrePath,
		genresFor,
		parseGenre,
		type GenreAnswer,
		type ExploreType,
		type ExploreCard,
		type ExploreRow,
		type HeadAnswer,
		type TailAnswer,
		type WatchService
	} from '#lib/explore.ts';
	import { contextLine } from '#lib/grow-lines.ts';
	import { defaultRegion, savedTop10 } from '#lib/top10.ts';
	import type { Ask, AskState, GateDefaults, LibraryFolder, LookupShow, PickRulesView, ShowSettings, TitleView } from '#lib/types.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { audienceOf } from '#lib/title-status.ts';
	import AddMovieDialog from '#lib/AddMovieDialog.svelte';
	import { savedDefaults } from '#lib/title-defaults.svelte.ts';
	import AddShowDialog from '#lib/AddShowDialog.svelte';
	import AskShowDialog from '#lib/AskShowDialog.svelte';
	import AskMovieDialog from '#lib/AskMovieDialog.svelte';
	import MetadataNotice from '#lib/MetadataNotice.svelte';
	import TopServicesRow from '#lib/TopServicesRow.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import MusicTrendingRows from '#lib/MusicTrendingRows.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import ExploreHero from '#lib/ui/ExploreHero.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import CategoryStrip from '#lib/ui/CategoryStrip.svelte';
	import { leaveOut, newHref, newServicesPath, type NewServicesAnswer } from '#lib/new-popular.ts';
	import { browsePath, browseSectionPath, type BrowseAnswer, type BrowseSectionAnswer, type BrowseTile } from '#lib/search-browse.ts';
	import { dayKey, stripTiles, STRIP_ORDER } from '#lib/category-strip.ts';
	import { top10Map, whyShown } from '#lib/why-shown.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import type { IconName } from '#lib/Icon.svelte';
	import { EAGER_ROWS, stableRows } from '#lib/row-window.ts';
	import Icon from '#lib/Icon.svelte';
	import { onTheaterExit } from '#lib/theater.svelte.ts';

	let loaded = $state(false);
	let failed = $state(false);
	let okOnce = $state(false);
	let busy = $state(false);
	let titles = $state<TitleView[]>([]);
	/** The server-built page: the head in one answer, then tail pages as the page scrolls. */
	let head = $state<(ExploreRow & { wide: boolean; type: string })[]>([]);
	let tail = $state<(ExploreRow & { wide: boolean; type: string })[]>([]);
	let hero = $state<ExploreCard[]>([]);
	let heroAt = $state(0);
	let headLoaded = $state(false);
	let tailPageNext = 0;
	let tailMore = $state(false);
	let tailBusy = false;
	/** Cards asked for (with the ask state returned) or found in the Library on this page, so the card updates at once. */
	let askedHere = $state(new Map<string, { ask?: AskState; titleId?: number }>());
	let asking = $state('');

	// The switch floats once its spot scrolls under the top bar.
	let typesMark = $state<HTMLElement>();
	let typesStuck = $state(false);
	$effect(() => {
		if (!typesMark) return;
		const top = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--topbar')) || 56;
		const io = new IntersectionObserver(([e]) => (typesStuck = !e.isIntersecting), { rootMargin: `-${top}px 0px 0px 0px` });
		io.observe(typesMark);
		return () => io.disconnect();
	});
	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	const who = $derived(audienceOf(session.can));

	// The type switch: in the URL (?type=, none for All) and remembered on this device.
	const TYPE_KEY = 'holyarrs.explore.type';
	const musicOn = $derived(partsNow.answer?.parts.find((p) => p.part === 'music')?.on !== false);
	let type = $state<ExploreType>(parseType(page.url.searchParams.get('type')));
	const typeOptions = $derived(
		([
			{ value: 'all', label: 'All', icon: 'grid' },
			{ value: 'movies', label: 'Movies', icon: 'film' },
			{ value: 'shows', label: 'Shows', icon: 'tv' },
			{ value: 'music', label: 'Music', icon: 'music' }
		] as { value: ExploreType; label: string; icon: IconName }[]).filter((o) => o.value !== 'music' || musicOn)
	);
	// Music turned off while picked falls back to All.
	$effect(() => {
		if (type === 'music' && !musicOn) setType('all');
	});

	function setType(t: ExploreType) {
		type = t;
		try {
			localStorage.setItem(TYPE_KEY, t);
		} catch {
			/* the choice is a convenience */
		}
		const url = new URL((page.shallow?.url ?? page.url).href);
		if (t === 'all') url.searchParams.delete('type');
		else url.searchParams.set('type', t);
		url.searchParams.delete('genre');
		void goto(url, { replace: true, shallow: true, reset: false });
		void loadHead();
	}

	// Back from a title page: the page's scroll and each row's sideways offset
	// come back once the rows have loaded far enough (more tail pages load
	// first when the spot was further down).
	let back = $state<ExploreReturn | null>(null);
	snapshot<ExploreReturn>({
		id: 'explore',
		capture: () => ({ y: Math.round(scrollY), rows: readOffsets(document), anchor: readAnchor(document, document.querySelector('main')?.getBoundingClientRect().top ?? 0) }),
		restore: (v) => {
			if (v && v.y + Object.keys(v.rows).length > 0) back = v;
		}
	});
	let backTries = 0;
	// Rows above the spot may still be settling to their real heights: keep the
	// row that was first on screen where it was for a moment, until the person scrolls.
	function settleOn(v: ExploreReturn) {
		let moved = false;
		const stop = () => (moved = true);
		const opts = { passive: true, once: true } as const;
		addEventListener('wheel', stop, opts);
		addEventListener('touchstart', stop, opts);
		addEventListener('keydown', stop, opts);
		const fix = () => {
			const d = moved ? null : anchorDelta(document, v.anchor);
			if (d) scrollBy({ top: d, behavior: 'instant' });
		};
		fix();
		for (const ms of [120, 400, 900]) setTimeout(fix, ms);
		setTimeout(() => {
			removeEventListener('wheel', stop);
			removeEventListener('touchstart', stop);
			removeEventListener('keydown', stop);
		}, 1000);
	}
	$effect(() => {
		const v = back;
		if (!v || !headLoaded) return;
		void rows.length;
		void tick().then(() => {
			if (back !== v) return;
			const more = tailMore && backTries++ < 12;
			if (!canReturn(document.documentElement.scrollHeight, v.y, innerHeight, more)) {
				void loadTail();
				return;
			}
			back = null;
			backTries = 0;
			scrollTo({ top: v.y, behavior: 'instant' });
			putOffsets(document, v.rows);
			settleOn(v);
		});
	});
	// A full-screen trailer (hero or hover card) that plays to its end lands on the title's page; Esc only closes it.
	$effect(() => onTheaterExit((o) => goto(to(detailPath(o.kind, o.tmdb)))));

	// Music shows only music rows: the movie and show rows (Top 10, services, Best new releases) never appear there.
	// A genre page (Movies or Shows, ?genre=<TMDB id>) swaps the head and tail for its own rows.
	const genre = $derived(parseGenre((page.shallow?.url ?? page.url).searchParams.get('genre'), type));
	const genres = $derived(genresFor(type));
	let genreRows = $state<(ExploreRow & { wide: boolean; type: string })[]>([]);
	let genreHero = $state<ExploreCard[]>([]);
	let genreLoaded = $state<number | null>(null);
	let genreFailed = $state(false);
	let genreAsk = 0;
	async function loadGenre(id: number, again = 0) {
		const n = again ? genreAsk : ++genreAsk;
		genreFailed = false;
		try {
			const a = await api.get<GenreAnswer>(genrePath(type, id), gone.signal);
			if (n !== genreAsk) return;
			const got = a.rows.map(serverRow);
			// Top on your services, in the genre, is the second row.
			const marker = { id: 'top_services', type: 'top_services', title: 'Trending on streaming', cards: [], keep: true, wide: false };
			genreRows = [...got.slice(0, 1), marker, ...got.slice(1)];
			genreHero = (a.hero ?? []).map((d) => ({ ...discoverCard(d, 'hero'), wide: d.backdrop ?? null }));
			if (!again) heroAt = 0;
			genreLoaded = id;
			// A cold page answers with its first rows; the lower ones follow on the next ask.
			if (a.partial && again < 6) setTimeout(() => n === genreAsk && void loadGenre(id, again + 1), 1000);
		} catch {
			if (n === genreAsk && !again) genreFailed = true;
		}
	}
	$effect(() => {
		const g = genre;
		if (g && genreLoaded !== g.id) void loadGenre(g.id);
	});
	// Back from a genre page lands on the tab and the spot it was opened from: the address
	// (tab included) maps to where the page stood when a genre was opened from it.
	const spots = new Map<string, ExploreReturn>();
	function rememberSpot() {
		spots.set(new URL((page.shallow?.url ?? page.url).href).href, {
			y: Math.round(scrollY),
			rows: readOffsets(document),
			anchor: readAnchor(document, document.querySelector('main')?.getBoundingClientRect().top ?? 0)
		});
	}
	$effect(() => {
		const u = page.shallow?.url ?? page.url;
		untrack(() => {
			const t = parseType(u.searchParams.get('type'), musicOn);
			if (t !== type) {
				type = t;
				try {
					localStorage.setItem(TYPE_KEY, t);
				} catch {
					/* the choice is a convenience */
				}
				void loadHead();
			}
			const spot = spots.get(u.href);
			if (spot && !u.searchParams.has('genre')) {
				spots.delete(u.href);
				back = spot;
			}
		});
	});
	/** pickGenre opens a genre page (a new history entry, so Back returns); null clears it. */
	function pickGenre(id: number | null, close?: () => void) {
		close?.();
		const url = new URL((page.shallow?.url ?? page.url).href);
		if (id !== null) rememberSpot();
		if (id === null) url.searchParams.delete('genre');
		else url.searchParams.set('genre', String(id));
		genreLoaded = null;
		void goto(url, { shallow: true, reset: false });
		scrollTo({ top: 0, behavior: 'instant' });
	}
	const genreOn = $derived(!!genre && type !== 'music');

	// The category strip under the hero: the household's top Browse categories, from the
	// Browse answers the server keeps warm. Hidden until they are ready (no ghosts).
	let strip = $state<(BrowseTile & { section: string })[]>([]);
	let stripFor = '';
	async function loadStrip(t: ExploreType) {
		if (t === 'music' || stripFor === t) return;
		stripFor = t;
		strip = [];
		try {
			// The fast genre tiles and every section the server has ready; a section still building is left out.
			const [b, ...got] = await Promise.all([
				api.get<BrowseAnswer>(browsePath(t), gone.signal),
				...STRIP_ORDER.map((id) => api.get<BrowseSectionAnswer>(browseSectionPath(t, id), gone.signal).catch(() => null))
			]);
			if (stripFor !== t) return;
			const sections: Record<string, BrowseTile[]> = {};
			STRIP_ORDER.forEach((id, i) => {
				const a = got[i];
				sections[id] = a && !a.building ? (a.tiles ?? []) : [];
			});
			sections.genres = [...(b.tiles ?? []), ...sections.genres];
			strip = stripTiles(sections, t, `${session.s.name}|${dayKey(new Date())}`);
		} catch {
			if (stripFor === t) stripFor = '';
		}
	}
	$effect(() => {
		const t = type;
		if (!headLoaded || genreOn || t === 'music') return;
		const id = setTimeout(() => void loadStrip(t), 200);
		return () => clearTimeout(id);
	});
	// Hero badges: why the featured title shows (New season, its Top 10 rank, Picked for you).
	let heroGenres = $state<Record<string, string[]>>({});
	let taste = $state<string[]>([]);
	let top10Ranks = $state<Map<string, number>>(new Map());
	const heroCards = $derived(genreOn ? genreHero : hero);
	/** The titles of Trending on streaming, as that row shows them now. */
	let servicesShown = $state<string[]>([]);
	// A title shows once in the first 8 rows, counting the hero and the services row.
	const taken = $derived([...heroCards.map(cardIdentity), ...servicesShown]);
	// New on your services: one row from New & Popular's kept lists, right after Trending on
	// streaming, leaving out what the hero, that row and Top 10 show (hidden under 6).
	let newServices = $state<(ExploreRow & { wide: boolean; type: string }) | null>(null);
	let newFor = '';
	$effect(() => {
		const t = type;
		if (!headLoaded || t === 'music' || newFor === t) return;
		newFor = t;
		newServices = null;
		api.get<NewServicesAnswer>(newServicesPath(t), gone.signal).then(
			(a) => newFor === t && (newServices = serverRow(a.row)),
			() => {}
		);
	});
	const newCards = $derived(newServices && !genreOn ? leaveOut(newServices.cards, [...taken, ...top10Ranks.keys()]) : []);
	const rows = $derived(type === 'music' ? [] : markTransitions(dedupeTop(genreOn ? (genreLoaded === genre?.id ? genreRows : []) : filterRows([...head, ...tail], type), taken)));
	/** Where Browse by genre goes: after this row index (-1: not shown). */
	/** Big 4 shelves: the first personal row and Best new releases draw bigger cards. */

	function readTop10Pick() {
		try {
			return savedTop10(localStorage.getItem('holyarrs.top10'));
		} catch {
			return savedTop10(null);
		}
	}
	const viewerZone = (() => {
		try {
			return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
		} catch {
			return undefined;
		}
	})();
	const query = () => {
		const t = readTop10Pick();
		return { type, services: picked ?? [], region, top10Kind: type === 'shows' ? ('show' as const) : type === 'movies' ? ('movie' as const) : t.kind, top10Region: t.region, tz: viewerZone };
	};

	let headAsk = 0;
	/** The head rows' order as first shown for this query; a rebuilt pool never moves them. */
	let headOrder: string[] = [];
	let poll: ReturnType<typeof setTimeout> | undefined;
	/** loadHead reads the head; while the server builds the pool it asks again every 5 s (at most 2 minutes). */
	async function loadHead(tries = 0) {
		if (type === 'music') return;
		const n = ++headAsk;
		clearTimeout(poll);
		const q = query();
		try {
			const a = await api.get<HeadAnswer>(headPath(q), gone.signal);
			if (n !== headAsk) return;
			const kept = stableRows(tries > 0 ? headOrder : [], a.head.map(serverRow), (r) => r.id);
			headOrder = kept.order;
			head = kept.rows;
			hero = (a.hero ?? []).map((d) => ({ ...discoverCard(d, 'hero'), wide: d.backdrop ?? null }));
			heroGenres = Object.fromEntries((a.hero ?? []).map((d) => [`${d.kind}:${d.tmdb_id}`, d.genres ?? []]));
			taste = a.taste ?? [];
			top10Ranks = top10Map([a.top10, ...Object.entries(a.top10?.by_kind ?? {}).map(([kind, l]) => (l ? { ...l, kind } : null))]);
			heroAt = 0;
			tail = [];
			tailPageNext = 0;
			tailMore = a.tail_rows > 0;
			failed = false;
			okOnce = true;
			if (a.building && tries < 24) poll = setTimeout(() => void loadHead(tries + 1), 5000);
		} catch {
			if (n === headAsk) failed = true;
		} finally {
			if (n === headAsk) headLoaded = true;
		}
	}

	/** reuseKept puts back the rows from the last visit (same query, under 10 minutes old). */
	function reuseKept() {
		const k = kept;
		kept = null;
		if (!k || Date.now() - k.at > 600_000 || k.key !== JSON.stringify(query())) return false;
		const r = k.rows as { head: typeof head; tail: typeof tail; hero: typeof hero; heroAt: number; tailPageNext: number; tailMore: boolean; headOrder: string[] };
		({ head, tail, hero, heroAt, tailPageNext, tailMore, headOrder } = r);
		okOnce = true;
		headLoaded = true;
		return true;
	}

	async function loadTail() {
		if (tailBusy || !tailMore || type === 'music') return;
		tailBusy = true;
		const n = headAsk;
		try {
			const a = await api.get<TailAnswer>(tailPath(query(), tailPageNext), gone.signal);
			if (n !== headAsk) return;
			// A page laid out again between two asks may repeat a row: keep the first.
			const have = new Set([...head, ...tail].map((r) => r.id));
			tail = [...tail, ...a.rows.map(serverRow).filter((r) => !have.has(r.id))];
			tailPageNext++;
			tailMore = a.more;
		} catch {
			tailMore = false;
		} finally {
			tailBusy = false;
		}
		// Still within 1.5 screens of the end (a tall screen): the next page too.
		requestAnimationFrame(() => {
			if (tailMore && sentinel && sentinel.getBoundingClientRect().top < innerHeight * 2.5) void loadTail();
		});
	}

	// The tail loads when its sentinel comes within 1.5 screens.
	let sentinel = $state<HTMLElement>();
	$effect(() => {
		if (!sentinel) return;
		const io = new IntersectionObserver(([e]) => e.isIntersecting && void loadTail(), { rootMargin: '150% 0px' });
		io.observe(sentinel);
		return () => io.disconnect();
	});

	function opened(e: MouseEvent) {
		const el = e.target as Element | null;
		const wrap = el?.closest?.('[data-row]');
		const row = wrap?.getAttribute('data-row');
		if (row && el?.closest('a')) count(row, 'open', wrap?.getAttribute('data-title-layer') ?? undefined);
	}

	/** count: a card opened or an ask started from a row, kept only to measure Explore. */
	function count(rowType: string, event: 'open' | 'ask', titleLayer?: string) {
		void api.post('/explore/counts', { row_type: rowType, event, ...(titleLayer ? { title_layer: titleLayer } : {}) }).catch(() => {});
	}

	/** Every card is matched against the Library (by TMDB or MusicBrainz id), so owned titles read as owned in every row. */
	const owned = $derived(ownedIndex(titles));
	function live(c: ExploreCard): ExploreCard {
		return matchOwned(liveCard(c, askedHere.get(c.key)), owned);
	}

	/** refused: the server says the title is in the Library already (409), so the card says so. */
	function refused(body: Record<string, unknown>) {
		if (current && typeof body.title_id === 'number') askedHere = new Map([...askedHere, [current.key, { titleId: body.title_id }]]);
	}

	// Library titles open their page; other movies and shows open their details.
	function cardHref(c: ExploreCard): string | undefined {
		if (c.titleId !== undefined) return to(`/library/${c.titleId}`);
		const tmdb = c.ref?.startsWith('tmdb:') ? c.ref.slice(5) : '';
		if (tmdb && (c.kind === 'movie' || c.kind === 'show')) {
			remember(c.kind, tmdb, { name: c.name, year: c.year, poster: c.image });
			return to(detailPath(c.kind, tmdb));
		}
		return c.status === 'requested' ? to('/wishlist') : undefined;
	}

	function searchHref(c: ExploreCard): string {
		const kind = c.kind === 'show' ? '&kind=show' : c.kind === 'artist' ? '&kind=music' : '';
		return to(`/search?q=${encodeURIComponent(c.name)}${kind}`);
	}

	// The round + never adds or asks in one click: it opens a confirm sheet
	// (seasons for shows; Library folder and Pick rules for people who manage).
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let gateDefault = $state('released');
	let showDefaults = $state<ShowSettings | null>(null);
	let addMovie: ReturnType<typeof AddMovieDialog> | undefined = $state();
	let addShow: ReturnType<typeof AddShowDialog> | undefined = $state();
	let askShow: ReturnType<typeof AskShowDialog> | undefined = $state();
	let askMovie: ReturnType<typeof AskMovieDialog> | undefined = $state();
	/** Which card the sheet is for, so the card updates once it is asked or added. */
	let current: ExploreCard | null = null;

	async function quick(c: ExploreCard, act: 'add' | 'ask') {
		const tmdb = cardTmdb(c);
		if (!tmdb || asking) return;
		current = c;
		if (act === 'ask' && c.kind === 'movie') return askMovie?.open({ tmdb, name: c.name, year: c.year, ask: c.ask });
		asking = c.key;
		try {
			const d = await loadDetails(api.get, c.kind as 'movie' | 'show', tmdb);
			if (!d) return void toasts.push(`Could not find ${c.name}. Try Search.`, 'fail');
			if (d.titleId) return void toasts.push(`${c.name} is in your Library already.`, 'ok');
			if (act === 'add' && d.movie) addMovie?.open(d.movie);
			else if (act === 'add' && d.show) addShow?.open(d.show);
			else if (d.show) askShow?.open(d.show);
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			asking = '';
		}
	}

	function askedToast(r: { ask: Ask; joined: boolean }, what: string) {
		if (current) askedHere = new Map([...askedHere, [current.key, { ask: r.ask.state }]]);
		if (r.ask.state === 'approved') toasts.push(`${what} is approved; HolyArrs is looking for it.`, 'ok');
		else if (r.joined) toasts.push(`Someone asked for ${what} already; you are on the ask too.`, 'ok');
		else toasts.push(`Asked for ${what}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
	}

	function added(t: TitleView) {
		toasts.push(`${t.name} is in your Library.`, 'ok');
		void load();
	}

	$effect(() => {
		if (!manage) return;
		api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
		api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
		api.get<GateDefaults>('/settings/release-gate').then((r) => (gateDefault = r.release_gate)).catch(() => {});
		api.get<ShowSettings>('/settings/shows').then((r) => (showDefaults = r)).catch(() => {});
		// The add dialogs wait for the saved defaults; fetched once for this visit.
		savedDefaults.forget();
		void savedDefaults.load();
	});

	/** Movies and shows draw wide title cards; artists keep their square cards. */
	const wideKind = (k: string): boolean => k === 'movie' || k === 'show';

	/** A row shows at most this many cards. */
	const rowCap = 20;

	async function load() {
		busy = true;
		try {
			titles = (await api.get<{ titles: TitleView[] }>('/titles')).titles ?? [];
		} catch {
			/* owned marks wait for the Library */
		}
		loaded = true;
		busy = false;
	}

	// "On your services": the streaming services this person has, picked
	// from the region's best known ones; remembered on this device.
	const region = serviceRegion(defaultRegion());
	let services = $state<WatchService[]>([]);
	/** null until the person picks once (the row then asks them to). */
	let picked = $state<number[] | null>(null);
	const shownServices = $derived(pickerServices(services, picked ?? []));
	/** The ids the row is filled from: the picked ones, else the region's top few. */
	const usedServices = $derived(picked ?? shownServices.slice(0, SERVICES_SHOWN).map((s) => s.id));

	function readPicked() {
		try {
			picked = readServices(region, localStorage.getItem(SERVICES_KEY));
		} catch {
			/* nothing saved */
		}
	}

	async function loadServices() {
		try {
			services = (await api.get<{ services: WatchService[] }>(`/watch-services?region=${region}`, gone.signal)).services ?? [];
		} catch {
			services = [];
		}
	}

	// Leaving Explore stops the TMDB-backed asks still on their way.
	const gone = new AbortController();
	onMount(() => {
		if (!page.url.searchParams.has('type')) {
			try {
				const saved = parseType(localStorage.getItem(TYPE_KEY), musicOn);
				if (saved !== 'all') setType(saved);
			} catch {
				/* nothing saved */
			}
		}
		void load();
		// The head only needs the saved picks, not the service list: both go at once.
		readPicked();
		if (!reuseKept()) void loadHead();
		void loadServices();
		return () => {
			clearTimeout(poll);
			gone.abort();
			if (headLoaded && !failed && type !== 'music') kept = { key: JSON.stringify(query()), at: Date.now(), rows: $state.snapshot({ head, tail, hero, heroAt, tailPageNext, tailMore, headOrder }) };
		};
	});
</script>

<svelte:head><title>Explore · HolyArrs</title></svelte:head>


{#snippet newServicesRow(row: ExploreRow & { type: string }, cards: ExploreCard[])}
	<div class="row-wrap wide-row" data-row={row.type} data-row-id={row.id}>
		<Shelf title={row.title} href={to(newHref(type))} item="var(--title-item)" items={cards} key={(c) => c.key} placeholder="wide" lines={1}>
			{#snippet card(raw: ExploreCard)}
				{@const c = live(raw)}
				{@const act = cardAction(c, { manage, ask: mayAsk })}
				{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
				<WideCard cover title={c.name} href={cardHref(c)} image={c.wide ?? c.image} art={c.art} kind={c.kind} badges={c.badges} badgesAt="bottom" vtName={c.vtName} fresh={c.fresh} owned={mark} typed row={row.type} preview={cardTmdb(c) ? { kind: c.kind as 'movie' | 'show', tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
					growService={c.streaming ? { name: c.streaming.name, logo: services.find((s) => s.id === c.streaming?.service)?.logo } : null}
					growContext={contextLine({ state: mark?.state, label: mark?.label, mine: c.mine, fresh: c.fresh }, { rowType: row.type, rowTitle: row.title, fresh: c.fresh, streaming: c.streaming })}
					followTmdb={!c.titleId && c.kind === 'show'}>
					{#snippet growAction()}
						{#if act && cardTmdb(c)}
							<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => void quick(c, act)} />
						{:else}
							<IconButton icon="play" label="Open {c.name}" variant="glass" size="lg" href={cardHref(c) ?? searchHref(c)} />
						{/if}
					{/snippet}
				</WideCard>
			{/snippet}
		</Shelf>
	</div>
{/snippet}

{#snippet heroAction(c: ExploreCard)}
	{@const act = cardAction(c, { manage, ask: mayAsk })}
	{#if act && cardTmdb(c)}
		<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => void quick(c, act)} />
	{/if}
{/snippet}


<!-- A card opened from a row is counted (to measure only). -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="explore" onclick={opened}>
	<!-- The sidebar and the pills say where you are; the page opens straight on its pills and hero. -->
	<h1 class="sr-only">Explore</h1>
	<div class="types-mark" bind:this={typesMark} aria-hidden="true"></div>
	<div class="types" class:stuck={typesStuck} data-pin-pills>
		<SegmentedControl options={typeOptions} value={type} label="Show" testid="explore-type" announce={(l) => `Showing ${l}`} onchange={setType} />
		{#if genres.length}
			<div class="genre-pick" data-testid="genre-picker">
				<Popover label="Genres" text={genre ? genre.name : 'Genres'} variant="button" align="end" width={340}>
					{#snippet children(close)}
						<div class="genre-list" role="group" aria-label="Genres">
							<Chip label="All genres" selected={!genre} onclick={() => pickGenre(null, close)} />
							{#each genres as g (g.id)}
								<Chip label={g.name} selected={genre?.id === g.id} onclick={() => pickGenre(g.id, close)} />
							{/each}
						</div>
					{/snippet}
				</Popover>
			</div>
		{/if}
	</div>
	<MetadataNotice />
	{#if type === 'music'}<MusicTrendingRows {owned} {manage} {mayAsk} {folders} {rules} onadded={() => void load()} />{/if}

	{#if type === 'music'}
		<!-- Music: the music rows above only. -->
	{:else if failed && !okOnce}
		<LoadError hasData={false} message="Could not load Explore." onretry={load} {busy} />
	{:else}
		{#if failed}<LoadError hasData={true} message="Could not refresh Explore." onretry={load} {busy} />{/if}
		{#if !headLoaded}
			<div class="loading" aria-busy="true" aria-label="Loading">
				{#each [0, 1] as r (r)}
					<Skeleton width="200px" />
					<div class="sk-row">{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<Skeleton shape="poster" />{/each}</div>
				{/each}
			</div>
		{:else if genreOn && genreFailed}
			<LoadError hasData={false} message="Could not load this genre." onretry={() => genre && void loadGenre(genre.id)} {busy} />
		{:else if genreOn && rows.length === 0}
			<div class="loading" aria-busy="true" aria-label="Loading">
				{#each [0, 1] as r (r)}
					<Skeleton width="200px" />
					<div class="sk-row">{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<Skeleton shape="poster" />{/each}</div>
				{/each}
			</div>
		{:else if rows.length === 0}
			<TopServicesRow {type} {region} {owned} {who} used={usedServices} />
			<EmptyState icon="compass" title="Explore is getting ready" text="Rows fill in within a few minutes." action={{ label: 'Search', href: to('/search'), icon: 'search' }} />
		{:else}
			{#if heroCards.length}
				{@const h = live(heroCards[heroAt % heroCards.length])}
				<ExploreHero kind={h.kind === 'show' ? 'show' : 'movie'} tmdb={cardTmdb(h)} title={h.name} href={cardHref(h)} backdrop={h.wide ?? h.image} vtName={h.vtName} subtitle={h.year ? `${h.year} · ${h.kind === 'show' ? 'Show' : 'Movie'}` : h.subtitle} settled={headLoaded && !busy}>
					{#snippet action()}{@render heroAction(h)}{/snippet}
					{#snippet badge()}
						{@const why = whyShown({ kind: h.kind, tmdb: cardTmdb(h) ?? '', genres: heroGenres[`${h.kind}:${cardTmdb(h)}`], fresh: h.fresh }, { top10: top10Ranks, top10Source: 'Netflix', taste })}
						{#if why.length}<ul class="why" aria-label="Why it is featured">{#each why as w (w.kind)}<li class="why-{w.kind}">{w.text}</li>{/each}</ul>{/if}
					{/snippet}
				</ExploreHero>
			{/if}
			{#if !genreOn && strip.length}<CategoryStrip tiles={strip} {type} />{/if}
			{#each rows as row, ri (row.id)}
				{#if row.type === 'top_services'}
					<TopServicesRow {type} {region} genre={genreOn ? genre : null} {owned} {who} used={usedServices} bind:shown={servicesShown} seeAll={genreOn ? undefined : to(newHref(type))} />
					{#if newServices && newCards.length}{@render newServicesRow(newServices, newCards)}{/if}
				{:else}
				<div class="row-wrap" class:wide-row={row.wide || row.cards.some((c) => wideKind(c.kind))} data-row={row.type ?? row.id} data-row-id={row.id} data-title-layer={row.titleLayer}>
				<Shelf title={row.title} subtitle={row.subtitle} href={typeHref(row.href, type)} item={row.cards.some((c) => wideKind(c.kind)) ? 'var(--title-item)' : 'var(--explore-item)'}
					items={row.cards} key={(c) => c.key} defer={ri >= EAGER_ROWS} placeholder={row.wide || row.cards.some((c) => wideKind(c.kind)) ? 'wide' : 'poster'} lines={row.cards.some((c) => c.subtitle) ? 2 : 1}>
					{#snippet card(raw: ExploreCard)}
						{@const c = live(raw)}
						{@const act = cardAction(c, { manage, ask: mayAsk })}
						{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
						{#if row.wide || wideKind(c.kind)}
							<WideCard cover title={c.name} href={cardHref(c)} image={c.wide ?? c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} badges={c.badges} badgesAt="bottom" vtName={c.vtName} fresh={c.fresh} owned={mark} typed row={row.type} preview={cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
									growService={c.streaming ? { name: c.streaming.name, logo: services.find((s) => s.id === c.streaming?.service)?.logo } : null}
									growContext={contextLine({ state: mark?.state, label: mark?.label, mine: c.mine, fresh: c.fresh }, { rowType: row.type, rowTitle: row.title, fresh: c.fresh, streaming: c.streaming })}
									followTmdb={!c.titleId && (c.kind === 'show' || c.fresh?.kind === 'coming' || (!!c.year && c.year > new Date().getFullYear()))}>
								{#snippet growAction()}
									{#if act && cardTmdb(c)}
										<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => { count(row.type ?? row.id, 'ask', row.titleLayer); void quick(c, act); }} />
									{:else if mark?.state === 'available' || mark?.state === 'partial' || mark?.state === 'downloading'}
										<IconButton icon="check" label="In Library" variant="glass" size="lg" href={cardHref(c) ?? searchHref(c)} />
									{:else if mark?.state === 'requested'}
										<IconButton icon="check" label="Asked" variant="glass" size="lg" href={cardHref(c) ?? searchHref(c)} />
									{:else}
										<IconButton icon="play" label="Open {c.name}" variant="glass" size="lg" href={cardHref(c) ?? searchHref(c)} />
									{/if}
								{/snippet}
							</WideCard>
						{:else}
						<PosterCard
							title={c.name}
							href={cardHref(c)}
							image={c.image}
							kind={c.kind}
							subtitle={c.subtitle}
							badges={c.badges}
							typed
							row={row.type}
							owned={mark}
							fresh={c.fresh}
							actionOnHover
							vtName={c.vtName}
							preview={cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year } : undefined}
						>
							{#snippet action()}
								{#if act && cardTmdb(c)}
									<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => { count(row.type ?? row.id, 'ask', row.titleLayer); void quick(c, act); }} />
								{:else if act}
									<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" href={searchHref(c)} />
								{/if}
							{/snippet}
						</PosterCard>
						{/if}
					{/snippet}
				</Shelf>
				</div>
				{/if}
			{/each}
			{#if tailMore && !genreOn}<div class="sentinel" bind:this={sentinel} aria-hidden="true"></div>{/if}
		{/if}
	{/if}
</div>

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} onadded={(_m, t) => added(t)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} onadded={(_s, t) => added(t)} />
{:else if mayAsk}
	<AskMovieDialog bind:this={askMovie} onasked={(_m, r) => askedToast(r, r.ask.name)} onrefused={(_m, body) => refused(body)} />
	<AskShowDialog bind:this={askShow} onasked={(_s: LookupShow, r) => askedToast(r, `${r.ask.name} (${seasonWords(r.ask.seasons)})`)} onrefused={(_s: LookupShow, body: Record<string, unknown>) => refused(body)} />
{/if}

<style>
	.explore { container: explore / inline-size; /* One size for every title row, measured on Netflix (2026-10-20): 279px cards at a 1536px window, sized from the window like Netflix (whose page has no sidebar), so cards match it; never under 200px. */
		--title-item: max(200px, calc((100vw - 96px) / 5.16)); --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.explore > :global(.ui-head) { margin-bottom: -8px; }
	/* The type switch: in the page under the title; once scrolled past it floats as a glass pill under the top bar. */
	.types-mark { height: 0; margin-top: -32px; }
	.types { position: sticky; top: calc(var(--topbar) + 10px); z-index: 20; display: flex; justify-content: center; margin-top: -12px; pointer-events: none; }
	.types :global(.ui-seg) { pointer-events: auto; transition: transform 320ms cubic-bezier(.2,.8,.2,1), box-shadow 320ms ease, background-color 320ms ease; transform-origin: top center; }
	.types.stuck :global(.ui-seg) { transform: scale(0.9); background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.types :global(.ui-seg) { border-radius: 999px; padding: 3px; }
	.types :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 999px; }
	.types :global(.ui-seg button) { min-height: 38px; padding: 0 16px; border-radius: 999px; font-size: 14px; }
	/* Reduced motion: the same compact size, without the animation. */
	@media (prefers-reduced-motion: reduce) { .types :global(.ui-seg), .types :global(.ui-seg .thumb), .genre-pick { transition: none; } }
	@media (max-width: 767px) { .types :global(.ui-seg) { display: grid; width: 100%; } .types :global(.ui-seg button) { padding: 0 8px; } }
	@media (max-width: 359px) {
		.types :global(.ui-seg button span) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	}
	.genre-pick { pointer-events: auto; margin-left: 8px; display: flex; align-items: center; transition: transform 320ms cubic-bezier(.2,.8,.2,1); transform-origin: top center; }
	/* Genres matches the tab pill: same height, type and glass. */
	.genre-pick :global(.trigger) { min-height: 44px; border-radius: 999px; padding: 0 16px; font: 600 14px/1 var(--font-ui); background: var(--surface-2); color: var(--text); }
	.types.stuck .genre-pick { transform: scale(0.9); }
	.types.stuck .genre-pick :global(.trigger) { background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.genre-list { display: flex; flex-wrap: wrap; gap: 8px; padding: 12px; }
	.sentinel { height: 1px; }
	/* Hero badges, next to the facts line: small glass pills, the Top 10 one in the brand red. */
	.why { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0 0; padding: 0; list-style: none; }
	.why li { font: 600 12px/1 var(--font-ui); letter-spacing: .02em; padding: 6px 10px; border-radius: 999px; color: #fff; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 22%); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); }
	.why li.why-top10 { background: color-mix(in srgb, var(--accent) 80%, transparent); border-color: transparent; }
	.row-wrap { display: contents; }
	/* Wide title rows sit almost edge to edge, as on Netflix: about 279px cards at a 1536px window with a 5px gap. */
	@media (min-width: 768px) { .row-wrap.wide-row { --shelf-gap: 5px; } }
	.loading { display: flex; flex-direction: column; gap: 14px; }
	.sk-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: var(--shelf-gap); max-height: 250px; overflow: hidden; margin-bottom: 16px; }
	@media (max-width: 767px) { .explore { --explore-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); gap: 26px; } }
</style>
