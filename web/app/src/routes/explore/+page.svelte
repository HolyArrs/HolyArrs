<script lang="ts" module>
	/** The rows already loaded when Explore was left, so Back shows them at once (no re-fetch flash). */
	let kept: { key: string; at: number; rows: unknown } | null = null;
</script>

<script lang="ts">
	import { libraryTitles, ownedLibrary } from '#lib/owned.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { anchorDelta, canReturn, putOffsets, readAnchor, readOffsets, type ExploreReturn } from '#lib/explore-return.ts';
	import { page } from '$app/state';
	import { goto, snapshot } from '$app/navigation';
	import { partOff, partOn } from '#lib/parts.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { detailPath, loadDetails, remember } from '#lib/title-details.ts';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import {
		cardAction,
		followsByTmdb,
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
	import { defaultRegion } from '#lib/region.ts';
	import type { Ask, AskState, GateDefaults, LibraryFolder, LookupShow, PickRulesView, ShowSettings, TitleView } from '#lib/types.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { audienceOf } from '#lib/title-status.ts';
	import QuickAsk from '#lib/QuickAsk.svelte';
	import MetadataNotice from '#lib/MetadataNotice.svelte';
	import TopServicesRow from '#lib/TopServicesRow.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import MusicTrendingRows from '#lib/MusicTrendingRows.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import ExploreHero from '#lib/ui/ExploreHero.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import CategoryStrip from '#lib/ui/CategoryStrip.svelte';
	import GenreBanner from '#lib/ui/GenreBanner.svelte';
	import { GENRES } from '#lib/explore.ts';
	import { fetchMusicGenre, musicCardHref, type MusicCardData } from '#lib/music/music-trending.ts';
	import MusicGenreTiles from '#lib/ui/MusicGenreTiles.svelte';
	import { leaveOut, newHref, newServicesPath, type NewServicesAnswer } from '#lib/new-popular.ts';
	import { browsePath, browseSectionPath, tileHue, type BrowseAnswer, type BrowseSectionAnswer, type BrowseTile } from '#lib/search-browse.ts';
	import { fitTile } from '#lib/tmdb-image.ts';
	import { fromStrip, shownBatcher, stripKey, stripPath, stripRowNames, type StripAnswer } from '#lib/category-strip.ts';
	import { whyShown } from '#lib/why-shown.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { SkCard, SkRail } from '#lib/ui/sk/index.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
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
	/** The Add and Ask sheets, and the cards asked for or added on this page. */
	let flow: ReturnType<typeof QuickAsk> | undefined = $state();

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
	// The tab waits for the parts answer; a saved Music pick is kept until Music is known off.
	const musicOn = $derived(partOn('music'));
	const musicKept = $derived(!partOff('music'));
	let type = $state<ExploreType>(parseType(page.url.searchParams.get('type')));
	const typeOptions = $derived(
		([
			{ value: 'all', label: 'All', icon: 'grid' },
			{ value: 'movies', label: 'Movies', icon: 'film' },
			{ value: 'shows', label: 'TV shows', icon: 'tv' },
			{ value: 'music', label: 'Music', icon: 'music' }
		] as { value: ExploreType; label: string; icon: IconName }[]).filter((o) => o.value !== 'music' || musicOn)
	);
	// Music turned off while picked falls back to All.
	$effect(() => {
		if (type === 'music' && !musicKept) setType('all');
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
		const done = () => {
			removeEventListener('wheel', stop);
			removeEventListener('touchstart', stop);
			removeEventListener('keydown', stop);
		};
		fix();
		const timers = [120, 400, 900].map((ms) => setTimeout(fix, ms));
		timers.push(setTimeout(done, 1000));
		// Leaving the page inside that second must not nudge the next page's scroll.
		settleCancel = () => {
			timers.forEach(clearTimeout);
			done();
		};
	}
	let settleCancel: (() => void) | null = null;
	$effect(() => () => settleCancel?.());
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
	$effect(() => onTheaterExit((o) => (o.kind === 'artist' ? undefined : goto(to(detailPath(o.kind, o.tmdb))))));

	// Music shows only music rows: the movie and show rows (services, Best new releases) never appear there.
	// A genre page (Movies or Shows, ?genre=<TMDB id>) swaps the head and tail for its own rows.
	const genre = $derived(parseGenre((page.shallow?.url ?? page.url).searchParams.get('genre'), type));
	const genres = $derived(genresFor(type));
	let musicGenres = $state<{ key: string; name: string; href: string; art: string; images?: string[]; hue: number }[]>([]);
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
			const t = parseType(u.searchParams.get('type'), musicKept);
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

	// The Genres picker for Movies and TV: the Music tile look, each genre over the
	// backdrop Search's Browse grid uses for it (read when the picker first opens).
	let browseArt = $state<Record<string, Record<string, string>>>({});
	async function loadBrowseArt(t: typeof type) {
		if (t !== 'movies' && t !== 'shows') return;
		if (browseArt[t]) return;
		browseArt = { ...browseArt, [t]: {} };
		try {
			// The same tiles Search's Browse grid shows: its first answer, then its Genres section.
			// Worlds is read too: it holds the pictures the combined TV genres fall back on.
			const [a, more, worlds] = await Promise.all([api.get<BrowseAnswer>(browsePath(t)), api.get<BrowseSectionAnswer>(browseSectionPath(t, 'genres')).catch(() => null), api.get<BrowseSectionAnswer>(browseSectionPath(t, 'worlds')).catch(() => null)]);
			const art: Record<string, string> = {};
			for (const x of [...(a.tiles ?? []), ...(more?.tiles ?? []), ...(worlds?.tiles ?? [])]) {
				if (x.mood || !x.backdrop) continue;
				const src = fitTile(x.backdrop, 130, false);
				art[`id:${x.genre}`] ??= src;
				art[`name:${x.genre_name.toLowerCase()}`] ??= src;
				art[`name:${x.name.toLowerCase()}`] ??= src;
			}
			browseArt = { ...browseArt, [t]: art };
		} catch {
			// No art: the tiles keep their colour.
		}
	}
	// Genres Browse has no tile of their own (TMDB's combined TV genres) borrow a
	// picture from the closest Browse tiles, tried in order.
	const genreArtAlias: Record<string, string[]> = {
		'action & adventure': ['action', 'adventure', 'treasure hunts'],
		'sci-fi & fantasy': ['sci-fi', 'science fiction', 'fantasy', 'space', 'fantasy worlds', 'parallel worlds'],
		'science fiction': ['sci-fi', 'space'],
		fantasy: ['fantasy worlds', 'medieval'],
		'war & politics': ['war', 'politics', 'world war ii', 'cold war'],
		war: ['world war ii', 'cold war'],
		'tv movie': ['drama'],
		history: ['victorian era', 'ancient worlds', 'medieval'],
	};
	function genreArt(art: Record<string, string> | undefined, id: number, name: string): string {
		if (!art) return '';
		const n = name.toLowerCase();
		for (const k of [`id:${id}`, `name:${n}`, ...(genreArtAlias[n] ?? []).map((a) => `name:${a}`)]) if (art[k]) return art[k];
		return '';
	}
	const genreTiles = $derived(genres.map((g) => ({ key: String(g.id), name: g.name, href: '', art: genreArt(browseArt[type], g.id, g.name), hue: 0, tint: tileHue(g.name) })));
	// The picker spans from the left edge of the type pills to the Genres button's right edge.
	let typesBox = $state<HTMLElement>();
	let pickW = $state(340);
	function measurePick() {
		// The left edge of the first pill (All), the right edge of the Genres button.
		const first = typesBox?.querySelector('.ui-seg button');
		const pick = typesBox?.querySelector('.genre-pick .trigger');
		if (!first || !pick) return;
		// The pill row shrinks (scale .9) once it floats; the panel sits inside that scale, so undo it.
		const box = typesBox!.querySelector<HTMLElement>('.genre-pick')!;
		const k = box.getBoundingClientRect().width / (box.offsetWidth || 1) || 1;
		pickW = Math.round((pick.getBoundingClientRect().right - first.getBoundingClientRect().left) / k);
	}
	$effect(() => {
		if (!typesBox) return;
		measurePick();
		const ro = new ResizeObserver(measurePick);
		ro.observe(typesBox);
		addEventListener('resize', measurePick);
		return () => { ro.disconnect(); removeEventListener('resize', measurePick); };
	});

	// The category strip under the hero: the household's top Browse categories, from the
	// Browse answers the server keeps warm. Hidden until they are ready (no ghosts).
	let strip = $state<(BrowseTile & { section: string })[]>([]);
	let stripFor = '';
	async function loadStrip(t: ExploreType) {
		if (t === 'music' || stripFor === t) return;
		stripFor = t;
		strip = [];
		try {
			// Picked for this member on the server from the Browse sections it has ready; the rows below are passed so it skips what they show.
			const a = await api.get<StripAnswer>(stripPath(t, stripRowNames(head), viewerZone), gone.signal);
			if (stripFor !== t) return;
			strip = fromStrip(a.tiles ?? []);
			stripShown.add(strip.map(stripKey));
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
	// The strip learns from its own counts: the tiles shown (batched) and the one opened.
	const stripShown = shownBatcher((categories) => void api.post('/explore/strip/counts', { event: 'shown', categories }).catch(() => {}));
	function stripOpened(t: BrowseTile) {
		stripShown.flush();
		void api.post('/explore/strip/counts', { event: 'open', categories: [stripKey(t)] }).catch(() => {});
	}

	// The genre banner: the words on the left, the picks in a row on the right.
	const bannerLayout: number = 2;
	const bannerNorm = (n: string) => n.toLowerCase().replace(/\s*&\s*/g, ' & ');
	/** The household's favourite genre: the most common among Library titles (asked for ones included). */
	const favourite = $derived.by(() => {
		if (type === 'music') return null;
		const tally = new Map<string, { n: number; kind: 'movies' | 'shows'; id: number; name: string }>();
		for (const t of titles) {
			const k = t.kind === 'movie' ? 'movies' : t.kind === 'show' ? 'shows' : null;
			if (!k || (type !== 'all' && type !== k)) continue;
			const list = GENRES[k === 'movies' ? 'movie' : 'show'];
			for (const gn of t.genres ?? []) {
				const g = list.find((x) => bannerNorm(x.name) === bannerNorm(gn) || bannerNorm(x.name).split(' & ').includes(bannerNorm(gn)));
				if (!g) continue;
				const key = `${k}:${g.id}`;
				const e = tally.get(key) ?? { n: 0, kind: k, id: g.id, name: g.name };
				e.n++;
				tally.set(key, e);
			}
		}
		// Movies and shows count as one genre by name: the larger side opens its page.
		const byName = new Map<string, { n: number; best: { n: number; kind: 'movies' | 'shows'; id: number; name: string } }>();
		for (const e of tally.values()) {
			const cur = byName.get(e.name) ?? { n: 0, best: e };
			cur.n += e.n;
			if (e.n > cur.best.n) cur.best = e;
			byName.set(e.name, cur);
		}
		const top = [...byName.values()].sort((a, b) => b.n - a.n)[0];
		return top && top.n >= 2 ? { ...top.best, count: top.n } : null;
	});
	let bannerCards = $state<ExploreCard[]>([]);
	let bannerFor = '';
	$effect(() => {
		const f = favourite;
		if (bannerLayout === 0 || !f || !headLoaded) return;
		const key = `${f.kind}:${f.id}`;
		if (bannerFor === key) return;
		bannerFor = key;
		bannerCards = [];
		// The app's own genre answer (the genre page's), never an outside call.
		api.get<GenreAnswer>(genrePath(f.kind, f.id), gone.signal).then(
			(a) => {
				if (bannerFor !== key) return;
				const got = [...(a.hero ?? []).map((d) => ({ ...discoverCard(d, 'banner'), wide: d.backdrop ?? null })), ...a.rows.flatMap((r) => serverRow(r).cards)];
				const seen = new Set<string>();
				bannerCards = got.filter((c) => (seen.has(cardIdentity(c)) ? false : (seen.add(cardIdentity(c)), true)));
			},
			() => {}
		);
	});
	const bannerPicks = $derived(bannerCards.map(live).filter((c) => (c.kind === 'movie' || c.kind === 'show') && c.image && !ownedMark(c, ownedTitle(c, owned), who) && c.titleId === undefined).slice(0, 7));
	// Music: the household's top music genre (first of the genre tiles the
	// music rows answer, built from the Library's artists) and its albums and
	// artists not in the Library yet, from the app's own genre page answer.
	let musicPicks = $state<MusicCardData[]>([]);
	/** The picks are on their way: the banner keeps its place (placeholders) so the rows below never jump. */
	let musicPicksPending = $state(false);
	let musicPicksFor = '';
	const musicFav = $derived(type === 'music' ? (musicGenres[0] ?? null) : null);
	/** The music rows' first answer is in (musicGenres says all it will). */
	let musicReady = $state(false);
	/**
	 * Until the music rows answer, the "Your genre" banner's place is held (the same banner
	 * with placeholders, so the exact height) when the last visit showed it, so the rows
	 * never jump down when it arrives. Remembered per browser; first visit holds it.
	 */
	let musicBannerHint = $state(true);
	/** The last banner's genre name: the held banner's name wraps the same, so the same height. */
	let musicBannerName = $state('');
	try {
		const kept = localStorage.getItem('holyarrs-music-banner');
		musicBannerHint = kept !== '0';
		musicBannerName = kept && kept !== '0' && kept !== '1' ? kept : '';
	} catch {
		/* storage off: hold the place */
	}
	$effect(() => {
		if (type !== 'music') musicReady = false;
	});
	const musicBannerShown = $derived(type === 'music' && !!musicFav && (musicPicksPending || musicPicks.length >= 4));
	const musicBannerHold = $derived(type === 'music' && !musicFav && !musicReady && musicBannerHint);
	$effect(() => {
		if (type !== 'music' || !musicReady || musicPicksPending) return;
		try {
			localStorage.setItem('holyarrs-music-banner', musicBannerShown && musicFav ? musicFav.name : '0');
		} catch {
			/* storage off */
		}
	});
	$effect(() => {
		const g = musicFav;
		if (!g) return;
		const slug = g.href.split('/music/genre/')[1]?.split('?')[0] ?? '';
		if (!slug || musicPicksFor === slug) return;
		musicPicksFor = slug;
		musicPicks = [];
		musicPicksPending = true;
		void fetchMusicGenre(decodeURIComponent(slug), 'US', api, gone.signal).then(
			(a) => {
				if (musicPicksFor !== slug) return;
				const seen = new Set<string>();
				musicPicks = [...(a.hero ? [a.hero] : []), ...a.rows.flatMap((r) => r.items ?? [])]
					.filter((c) => (c.kind === 'album' || c.kind === 'artist') && c.image && !c.title_id && !seen.has(c.key) && (seen.add(c.key), true))
					.slice(0, 7);
			},
			() => {}
		).finally(() => musicPicksFor === slug && (musicPicksPending = false));
	});
	const bannerOn = $derived(bannerLayout > 0 && !genreOn && type !== 'music' && !!favourite && bannerPicks.length >= 5);

	// Hero badges: why the featured title shows (New season, Picked for you).
	let heroGenres = $state<Record<string, string[]>>({});
	let taste = $state<string[]>([]);
	const heroCards = $derived(genreOn ? genreHero : hero);
	/** The titles of Trending on streaming, as that row shows them now. */
	let servicesShown = $state<string[]>([]);
	// A title shows once in the first 8 rows, counting the hero and the services row.
	const taken = $derived([...heroCards.map(cardIdentity), ...servicesShown]);
	// New on your services: one row from New & Popular's kept lists, right after Trending on
	// streaming, leaving out what the hero and that row show (hidden under 6).
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
	const newCards = $derived(newServices && !genreOn ? leaveOut(newServices.cards, taken) : []);
	const rows = $derived(type === 'music' ? [] : markTransitions(dedupeTop(genreOn ? (genreLoaded === genre?.id ? genreRows : []) : filterRows([...head, ...tail], type), taken)));
	/** Where Browse by genre goes: after this row index (-1: not shown). */
	/** Big 4 shelves: the first personal row and Best new releases draw bigger cards. */

	const viewerZone = (() => {
		try {
			return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
		} catch {
			return undefined;
		}
	})();
	const query = () => ({ type, services: picked ?? [], region, tz: viewerZone });

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
		return matchOwned(flow ? flow.live(c) : c, owned);
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

	const quick = (c: ExploreCard, act: 'add' | 'ask') => void flow?.open(c, act);

	/** Movies and shows draw wide title cards; artists keep their square cards. */
	const wideKind = (k: string): boolean => k === 'movie' || k === 'show';

	/** A row shows at most this many cards. */
	const rowCap = 20;

	async function load() {
		busy = true;
		try {
			const o = await ownedLibrary();
			titles = o.titles;
			// The genre banner reads Library genres; an answer without them borrows the full list.
			if (o.light && type !== 'music' && !o.titles.some((t) => t.genres?.length)) titles = await libraryTitles();
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
				const saved = parseType(localStorage.getItem(TYPE_KEY), musicKept);
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
			stripShown.flush();
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
							<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void quick(c, act)} />
						{/if}
					{/snippet}
				</WideCard>
			{/snippet}
		</Shelf>
	</div>
{/snippet}

<!-- A row outside this page (Trending on streaming) asks with the card as this page last saw it, so the button goes once asked. -->
{#snippet rowAsk(c: ExploreCard)}{@render heroAction(live(c))}{/snippet}

{#snippet heroAction(c: ExploreCard)}
	{@const act = cardAction(c, { manage, ask: mayAsk })}
	{#if act && cardTmdb(c)}
		<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void quick(c, act)} />
	{/if}
{/snippet}


<!-- A card opened from a row is counted (to measure only). -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="explore" onclick={opened}>
	<!-- The sidebar and the pills say where you are; the page opens straight on its pills and hero. -->
	<h1 class="sr-only">Explore</h1>
	<div class="types-mark" bind:this={typesMark} aria-hidden="true"></div>
	<div class="types" class:stuck={typesStuck} data-pin-pills bind:this={typesBox} onpointerdowncapture={measurePick} onkeydowncapture={measurePick}>
		<SegmentedControl options={typeOptions} value={type} label="Show" testid="explore-type" announce={(l) => `Showing ${l}`} onchange={setType} />
		{#if type === 'music' && musicGenres.length}
			<!-- Music: the same Genres button; a genre opens its page. -->
			<div class="genre-pick" data-testid="genre-picker">
				<Popover label="Genres" text="Genres" variant="button" align="end" width={pickW}>
					{#snippet children(close)}
						<div class="genre-tiles" role="group" aria-label="Genres">
							<MusicGenreTiles genres={musicGenres} allHref={to('/search?kind=music')} compact all={false} onopen={close} />
						</div>
					{/snippet}
				</Popover>
			</div>
		{:else if genres.length}
			<div class="genre-pick" data-testid="genre-picker">
				<Popover label="Genres" text={genre ? genre.name : 'Genres'} variant="button" align="end" width={pickW} onopen={() => void loadBrowseArt(type)}>
					{#snippet children(close)}
						<div class="genre-tiles" role="group" aria-label="Genres">
							<MusicGenreTiles genres={genreTiles} allHref="" compact all={false} selected={genre ? String(genre.id) : ''} onpick={(k) => pickGenre(k === null ? null : Number(k), close)} />
						</div>
					{/snippet}
				</Popover>
			</div>
		{/if}
	</div>
	<MetadataNotice />
	{#if musicBannerHold}
		<div class="music-banner hold" aria-hidden="true" data-testid="music-banner-hold">
			<GenreBanner layout={2} name={`Your ${musicBannerName || 'Music'}`} eyebrow="Picked from your library" line="Albums and artists from it you could ask for." href={to('/explore?type=music')} art={[]} square>
				{#snippet picks()}
					{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<div class="pick-sk"><SkCard /></div>{/each}
				{/snippet}
			</GenreBanner>
		</div>
	{:else if musicBannerShown && musicFav}
		<div class="music-banner">
			<GenreBanner layout={2} name={`Your ${musicFav.name}`} eyebrow="Picked from your library" line="Albums and artists from it you could ask for." href={to(musicFav.href)} art={[musicPicks[0]?.image ?? musicFav.art].filter(Boolean)} square>
				{#snippet picks()}
					{#if musicPicksPending && musicPicks.length === 0}
						{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<div class="pick-sk"><SkCard /></div>{/each}
					{/if}
					{#each musicPicks as c (c.key)}
						<PosterCard title={c.name} href={to(musicCardHref(c))} image={c.image} kind={c.kind === 'artist' ? 'artist' : undefined} subtitle={c.artist} shape={c.kind === 'artist' ? 'round' : 'square'} />
					{/each}
				{/snippet}
			</GenreBanner>
		</div>
	{/if}
	{#if type === 'music'}<MusicTrendingRows bind:genres={musicGenres} bind:ready={musicReady} {owned} {manage} {mayAsk} folders={flow?.addChoices().folders ?? []} rules={flow?.addChoices().rules ?? []} onadded={() => void load()} />{/if}

	{#if type === 'music'}
		<!-- Music: the music rows above only. -->
	{:else if failed && !okOnce}
		<LoadError hasData={false} message="Could not load Explore." onretry={load} {busy} />
	{:else}
		{#if failed}<LoadError hasData={true} message="Could not refresh Explore." onretry={load} {busy} />{/if}
		<PageLoader loading={!headLoaded} label="Loading Explore" onretry={load}>
		{#if genreOn && genreFailed}
			<LoadError hasData={false} message="Could not load this genre." onretry={() => genre && void loadGenre(genre.id)} {busy} />
		{:else if genreOn && rows.length === 0}
			<div class="loading" aria-busy="true" aria-label="Loading Explore" data-testid="explore-skeleton">
				<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="200px" />
				<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="160px" />
				<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="180px" />
			</div>
		{:else if rows.length === 0}
			<TopServicesRow {type} {region} {owned} {who} used={usedServices} ask={rowAsk} />
			<EmptyState icon="compass" title="Explore is getting ready" text="Rows fill in within a few minutes." action={{ label: 'Search', href: to('/search'), icon: 'search' }} />
		{:else}
			{#if heroCards.length}
				{@const h = live(heroCards[heroAt % heroCards.length])}
				<ExploreHero kind={h.kind === 'show' ? 'show' : 'movie'} tmdb={cardTmdb(h)} title={h.name} href={cardHref(h)} backdrop={h.wide ?? h.image} vtName={h.vtName} subtitle={h.year ? `${h.year} · ${h.kind === 'show' ? 'Show' : 'Movie'}` : h.subtitle} settled={headLoaded && !busy}>
					{#snippet action()}{@render heroAction(h)}{/snippet}
					{#snippet badge()}
						{@const why = whyShown({ kind: h.kind, tmdb: cardTmdb(h) ?? '', genres: heroGenres[`${h.kind}:${cardTmdb(h)}`], fresh: h.fresh }, { taste })}
						{#if why.length}<ul class="why" aria-label="Why it is featured">{#each why as w (w.kind)}<li class="why-{w.kind}">{w.text}</li>{/each}</ul>{/if}
					{/snippet}
				</ExploreHero>
			{/if}
			{#if bannerOn && favourite}
				<GenreBanner
					layout={bannerLayout === 2 ? 2 : 1}
					name={`Your ${favourite.name}`}
					eyebrow="Picked from your library"
					line={`${favourite.count} in your library so far. ${bannerPicks.length} more you could ask for.`}
					count={favourite.count}
					href={to(`/explore?type=${favourite.kind}&genre=${favourite.id}`)}
					art={bannerPicks.map((c) => c.wide ?? c.image ?? '').filter(Boolean)}
				>
					{#snippet picks()}
						{#each bannerPicks as c (c.key)}
							{@const act = cardAction(c, { manage, ask: mayAsk })}
							<PosterCard title={c.name} href={cardHref(c)} image={c.image} kind={c.kind} subtitle={c.subtitle} badges={c.badges} typed row="banner" fresh={c.fresh} actionOnHover vtName={undefined}
								preview={cardTmdb(c) ? { kind: c.kind as 'movie' | 'show', tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined} followTmdb={followsByTmdb(c)}>
								{#snippet action()}
									{#if act && cardTmdb(c)}<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void quick(c, act)} />{/if}
								{/snippet}
							</PosterCard>
						{/each}
					{/snippet}
				</GenreBanner>
			{/if}
			{#if !genreOn && strip.length}<CategoryStrip tiles={strip} {type} onopen={stripOpened} />{/if}
			{#each rows as row, ri (row.id)}
				{#if row.type === 'top_services'}
					<TopServicesRow {type} {region} genre={genreOn ? genre : null} {owned} {who} used={usedServices} bind:shown={servicesShown} seeAll={genreOn ? undefined : to(newHref(type))} ask={rowAsk} />
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
									followTmdb={followsByTmdb(c)}>
								{#snippet growAction()}
									{#if act && cardTmdb(c)}
										<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => { count(row.type ?? row.id, 'ask', row.titleLayer); void quick(c, act); }} />
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
							needsArt={c.kind === 'artist'}
							actionOnHover
							vtName={c.vtName}
							preview={cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
							followTmdb={followsByTmdb(c)}
						>
							{#snippet action()}
								{#if act && cardTmdb(c)}
									<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => { count(row.type ?? row.id, 'ask', row.titleLayer); void quick(c, act); }} />
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
		</PageLoader>
	{/if}
</div>

<QuickAsk bind:this={flow} onadded={() => void load()} />

<style>
	.explore { container: explore / inline-size; /* One size for every title row, measured on Netflix (2026-10-20): 279px cards at a 1536px window, sized from the window like Netflix (whose page has no sidebar), so cards match it; never under 200px. */
		--title-item: max(200px, calc((100vw - 96px) / 5.16)); --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.explore > :global(.ui-head) { margin-bottom: -8px; }
	/* The held banner keeps the real one's words for its height, unseen until the real one comes. */
	.music-banner.hold :global(.name) { visibility: hidden; }
	/* The type switch: in the page under the title; once scrolled past it floats as a glass pill under the top bar. */
	.types-mark { height: 0; margin-top: -32px; }
	.types { position: sticky; top: calc(var(--topbar) + 10px); z-index: 20; display: flex; justify-content: center; margin-top: -12px; pointer-events: none; }
	.types :global(.ui-seg) { pointer-events: auto; transition: transform var(--bar-glide), box-shadow var(--bar-glide), background-color var(--bar-glide); transform-origin: top center; }
	.types.stuck :global(.ui-seg) { transform: scale(0.9); background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.types :global(.ui-seg) { border-radius: 999px; padding: 3px; }
	.types :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 999px; }
	.types :global(.ui-seg button) { min-height: 38px; padding: 0 16px; border-radius: 999px; font-size: 14px; }
	/* Reduced motion: the same compact size, without the animation. */
	@media (prefers-reduced-motion: reduce) { .types :global(.ui-seg), .types :global(.ui-seg .thumb), .genre-pick :global(.trigger) { transition: none; } }
	@media (max-width: 767px) { .types :global(.ui-seg) { display: grid; width: 100%; } .types :global(.ui-seg button) { padding: 0 8px; } .music-banner { margin-top: 28px; } }
	/* Phones: the words win, the glyphs step aside, so no label is clipped. */
	@media (min-width: 360px) and (max-width: 479px) { .types :global(.ui-seg button svg) { display: none; } }
	@media (max-width: 359px) {
		.types :global(.ui-seg button span) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	}
	.pick-sk { --poster-ratio: 1; }
	.genre-pick { pointer-events: auto; margin-left: 8px; display: flex; align-items: center; }
	/* Genres matches the tab pill: same height, type and glass. */
	.genre-pick :global(.trigger) { min-height: 44px; border-radius: 999px; padding: 0 16px; font: 600 14px/1 var(--font-ui); background: var(--surface-2); color: var(--text); }
	.genre-pick :global(.trigger) { transition: transform var(--bar-glide); transform-origin: top center; }
	.types.stuck .genre-pick :global(.trigger) { transform: scale(0.9); }
	.types.stuck .genre-pick :global(.trigger) { background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	/* The picker panel is the one scroll: a thin themed bar, never taller than the screen allows. */
	.genre-pick :global(.panel) { max-height: min(calc(100dvh - var(--topbar, 64px) - 96px), 560px); padding: 0; overscroll-behavior: contain; scrollbar-width: thin; scrollbar-color: color-mix(in srgb, var(--text) 28%, transparent) transparent; }
	.genre-pick :global(.panel::-webkit-scrollbar) { width: 8px; }
	.genre-pick :global(.panel::-webkit-scrollbar-thumb) { border-radius: 8px; background: color-mix(in srgb, var(--text) 28%, transparent); }
	.genre-tiles { padding: 14px; }
	@media (max-width: 767px) { .genre-pick :global(.panel) { max-width: calc(100vw - 16px); } .genre-tiles { padding: 10px; } }
	.sentinel { height: 1px; }
	/* Hero badges, next to the facts line: small glass pills. */
	.why { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0 0; padding: 0; list-style: none; }
	.why li { font: 600 12px/1 var(--font-ui); letter-spacing: .02em; padding: 6px 10px; border-radius: 999px; color: #fff; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 22%); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); }
	.row-wrap { display: contents; }
	/* Wide title rows sit almost edge to edge, as on Netflix: about 279px cards at a 1536px window with a 5px gap. */
	@media (min-width: 768px) { .row-wrap.wide-row { --shelf-gap: 5px; } }
	.loading { display: flex; flex-direction: column; gap: 32px; min-width: 0; }
	@media (min-width: 768px) { .loading { --shelf-gap: 5px; } }
	@media (max-width: 767px) { .explore { --explore-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); gap: 26px; } }
</style>
