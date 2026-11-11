<script lang="ts" module>
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	/** The rows already loaded when Explore was left, so Back shows them at once (no re-fetch flash). */
	let kept: { key: string; at: number; rows: unknown } | null = null;
</script>

<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { createExploreGenre } from '#lib/discover/index.ts';
	import { createExploreStrip } from '#lib/discover/index.ts';
	import { createExploreBanner } from '#lib/discover/index.ts';
	import { createExploreServices } from '#lib/discover/index.ts';
	import { cardAction, followsByTmdb, cardTmdb, discoverCard, matchOwned, ownedIndex, ownedMark, ownedTitle, cardIdentity, type ExploreCard, type ExploreRow } from '#lib/ui/title-cards.ts';
	import { contextLine } from '#lib/ui/grow-lines.ts';
	import { audienceOf } from '#lib/ui/title-status.ts';
	import Popover from '#lib/ui/Popover.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { SkCard, SkRail } from '#lib/ui/sk/index.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import type { IconName } from '#lib/ui/Icon.svelte';
	import { EAGER_ROWS, stableRows } from '#lib/ui/row-window.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { artistSheets } from '#lib/requests/index.ts';
	import { libraryTitles, ownedLibrary, partOff, partOn } from '#lib/mediafiles/index.ts';
	import { anchorDelta, canReturn, putOffsets, readAnchor, readOffsets, type ExploreReturn, headPath, serverRow, tailPath, type HeadAnswer, type TailAnswer, markTransitions, dedupeTop, filterRows, parseType, typeHref, enoughCards, genreRowPath, genreCardHref, genreRowAfter, GENRE_ROW_MAX, ROW_MIN, type GenreRowCard, type ExploreType, TopServicesRow, ExploreHero, CategoryStrip, GenreBanner, leaveOut, newHref, newServicesPath, type NewServicesAnswer, whyShown } from '#lib/discover/index.ts';
	import { detailPath, remember, MetadataNotice } from '#lib/metadata/index.ts';
	import { QuickAsk } from '#lib/requests/index.ts';
	import { MusicTrendingRows, musicCardHref, MusicGenreTiles } from '#lib/music/index.ts';
	import { onMount, tick } from 'svelte';
	import { page } from '$app/state';
	import { goto, snapshot } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { Ask, TitleView } from '#lib/core/types.ts';
	import { onTheaterExit } from '#lib/core/trailer/theater.svelte.ts';

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
			{ value: 'all', label: t('discover.explore.all'), icon: 'grid' },
			{ value: 'movies', label: t('discover.explore.movies'), icon: 'film' },
			{ value: 'shows', label: t('discover.explore.tvShows'), icon: 'tv' },
			{ value: 'music', label: t('discover.explore.music'), icon: 'music' }
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
			/* The choice is a convenience. */
		}
		const url = new URL((page.shallow?.url ?? page.url).href);
		if (t === 'all') url.searchParams.delete('type');
		else url.searchParams.set('type', t);
		url.searchParams.delete('genre');
		void goto(url, { replace: true, shallow: true, reset: false });
		// A new tab starts at its top, never at the last tab's scroll.
		settleCancel?.();
		back = null;
		scrollTo({ top: 0, behavior: 'instant' });
		void loadHead();
	}

	// Back from a title page: the page's scroll and each row's sideways offset come back once the rows have loaded far enough (more tail pages load first when the spot was further down).
	let back = $state<ExploreReturn | null>(null);
	snapshot<ExploreReturn>({
		id: 'explore',
		capture: () => ({ y: Math.round(scrollY), rows: readOffsets(document), anchor: readAnchor(document, document.querySelector('main')?.getBoundingClientRect().top ?? 0) }),
		restore: (v) => {
			if (v && v.y + Object.keys(v.rows).length > 0) back = v;
		}
	});
	let backTries = 0;
	// Rows above the spot may still be settling to their real heights: keep the row that was first on screen where it was for a moment, until the person scrolls.
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

	// Music shows only music rows: the movie and show rows (services, Best new releases) never appear there. A genre page (Movies or Shows, ?genre=<TMDB id>) swaps the head and tail for its own rows.
	const genreViewCtx = {
		get type() {
			return type;
		},
		set type(v) {
			type = v;
		},
		get gone() {
			return gone;
		},
		get heroAt() {
			return heroAt;
		},
		set heroAt(v) {
			heroAt = v;
		},
		get musicKept() {
			return musicKept;
		},
		get TYPE_KEY() {
			return TYPE_KEY;
		},
		loadHead,
		get back() {
			return back;
		},
		set back(v) {
			back = v;
		},
		get headLoaded() {
			return headLoaded;
		},
	};
	const genreView = createExploreGenre(genreViewCtx);
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

	// The category strip under the hero: the household's top Browse categories, from the Browse answers the server keeps warm. Hidden until they are ready (no ghosts).
	const stripViewCtx = {
		get head() {
			return head;
		},
		get viewerZone() {
			return viewerZone;
		},
		get gone() {
			return gone;
		},
		get type() {
			return type;
		},
		get headLoaded() {
			return headLoaded;
		},
		get genreOn() {
			return genreView.genreOn;
		},
	};
	const stripView = createExploreStrip(stripViewCtx);
	const bannerViewCtx = {
		get type() {
			return type;
		},
		get titles() {
			return titles;
		},
		get headLoaded() {
			return headLoaded;
		},
		get gone() {
			return gone;
		},
		live,
		get owned() {
			return owned;
		},
		get who() {
			return who;
		},
		get musicGenres() {
			return genreView.musicGenres;
		},
		get genreOn() {
			return genreView.genreOn;
		},
	};
	const bannerView = createExploreBanner(bannerViewCtx);
	// Hero badges: why the featured title shows (New season, Picked for you).
	let heroGenres = $state<Record<string, string[]>>({});
	let taste = $state<string[]>([]);
	const heroCards = $derived(genreView.genreOn ? genreView.genreHero : hero);
	/** The titles of Trending on streaming, as that row shows them now. */
	let servicesShown = $state<string[]>([]);
	// A title shows once in the first 8 rows, counting the hero and the services row.
	const taken = $derived([...heroCards.map(cardIdentity), ...servicesShown]);
	// New on your services: one row from New & Popular's kept lists, right after Trending on streaming, leaving out what the hero and that row show (hidden under 6).
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
	const newCards = $derived.by(() => {
		const c = newServices && !genreView.genreOn ? leaveOut(newServices.cards, taken) : [];
		return c.length >= ROW_MIN ? c : [];
	});
	const rows = $derived(type === 'music' ? [] : markTransitions(enoughCards(dedupeTop(genreView.genreOn ? (genreView.genreLoaded === genreView.genre?.id ? genreView.genreRows : []) : filterRows([...head, ...tail], type), taken))));
	/** Where Browse by genre goes: after this row index (-1: not shown). */
	const genreAt = $derived(genreView.genreOn ? -1 : genreRowAfter(rows));
	// Browse by genre: read once per tab after the head (the page the server already built).
	let genreCards = $state<GenreRowCard[]>([]);
	let genreCardsFor = '';
	$effect(() => {
		const t = type;
		if (!headLoaded || t === 'music' || genreCardsFor === t) return;
		genreCardsFor = t;
		genreCards = [];
		api.get<{ genres: GenreRowCard[] | null }>(genreRowPath(t), gone.signal).then(
			(a) => genreCardsFor === t && (genreCards = (a.genres ?? []).filter((g) => g.backdrop).slice(0, GENRE_ROW_MAX)),
			() => genreCardsFor === t && (genreCardsFor = '')
		);
	});
	/** Big 4 shelves: the first personal row and Best new releases draw bigger cards. */

	const viewerZone = (() => {
		try {
			return Intl.DateTimeFormat().resolvedOptions().timeZone || undefined;
		} catch {
			return undefined;
		}
	})();
	const query = () => ({ type, services: servicesView.picked ?? [], region: servicesView.region, tz: viewerZone });

	let headAsk = 0;
	/** The head rows' order as first shown for this query; a rebuilt pool never moves them. */
	let headOrder: string[] = [];
	let poll: ReturnType<typeof setTimeout> | undefined;
	/** Reads the head; while the server builds the pool it asks again every 5 s (at most 2 minutes). */
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

	/** Puts back the rows from the last visit (same query, under 10 minutes old). */
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

	/** A card opened or an ask started from a row, kept only to measure Explore. */
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
		return c.status === 'requested' ? to('/requests') : undefined;
	}

	function searchHref(c: ExploreCard): string {
		const kind = c.kind === 'show' ? '&kind=show' : c.kind === 'artist' ? '&kind=music' : '';
		return to(`/search?q=${encodeURIComponent(c.name)}${kind}`);
	}

	// Ask is one tap (no sheet); Add opens its sheet (folder and Pick rules).
	const quick = (c: ExploreCard, act: 'add' | 'ask') => void (act === 'ask' ? flow?.askNow(c) : flow?.open(c, act));

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
			/* Owned marks wait for the Library. */
		}
		loaded = true;
		busy = false;
	}

	// "On your services": the streaming services this person has, picked from the region's best known ones; remembered on this device.
	const servicesViewCtx = {
		get gone() {
			return gone;
		},
	};
	const servicesView = createExploreServices(servicesViewCtx);
	const gone = new AbortController();
	onMount(() => {
		if (!page.url.searchParams.has('type')) {
			try {
				const saved = parseType(localStorage.getItem(TYPE_KEY), musicKept);
				if (saved !== 'all') setType(saved);
			} catch {
				/* Nothing saved. */
			}
		}
		void load();
		// The head only needs the saved picks, not the service list: both go at once.
		servicesView.readPicked();
		if (!reuseKept()) void loadHead();
		void servicesView.loadServices();
		return () => {
			clearTimeout(poll);
			stripView.stripShown.flush();
			gone.abort();
			if (headLoaded && !failed && type !== 'music') kept = { key: JSON.stringify(query()), at: Date.now(), rows: $state.snapshot({ head, tail, hero, heroAt, tailPageNext, tailMore, headOrder }) };
		};
	});
</script>

<svelte:head><title>{t('discover.explore.exploreHolyArrs')}</title></svelte:head>

{#snippet newServicesRow(row: ExploreRow & { type: string }, cards: ExploreCard[])}
	<div class="row-wrap wide-row" data-row={row.type} data-row-id={row.id}>
		<Shelf title={serverWords(row.title)} href={to(newHref(type))} item="var(--title-item)" items={cards} key={(c) => c.key} placeholder="wide" lines={1}>
			{#snippet card(raw: ExploreCard)}
				{@const c = live(raw)}
				{@const act = cardAction(c, { manage, ask: mayAsk })}
				{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
				<WideCard cover title={c.name} href={cardHref(c)} image={c.wide ?? c.image} art={c.art} kind={c.kind} badges={c.badges} badgesAt="bottom" vtName={c.vtName} fresh={c.fresh} owned={mark} typed row={row.type} preview={cardTmdb(c) ? { kind: c.kind as 'movie' | 'show', tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
					growService={c.streaming ? { name: c.streaming.name, logo: servicesView.services.find((s) => s.id === c.streaming?.service)?.logo } : null}
					growContext={contextLine({ state: mark?.state, label: mark?.label, mine: c.mine, fresh: c.fresh }, { rowType: row.type, rowTitle: row.title, fresh: c.fresh, streaming: c.streaming })}
					followTmdb={!c.titleId && c.kind === 'show'}>
					{#snippet growAction()}
						{#if act && cardTmdb(c)}
							<IconButton icon="plus" label={act === 'add' ? t('discover.explore.add', { name: c.name }) : t('discover.explore.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void quick(c, act)} />
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
		<IconButton icon="plus" label={act === 'add' ? t('discover.explore.add', { name: c.name }) : t('discover.explore.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void quick(c, act)} />
	{/if}
{/snippet}

<!-- A card opened from a row is counted (to measure only). -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="explore" onclick={opened}>
	<!-- The sidebar and the pills say where you are; the page opens straight on its pills and hero. -->
	<h1 class="sr-only">{t('discover.explore.explore')}</h1>
	<div class="types-mark" bind:this={typesMark} aria-hidden="true"></div>
	<div class="types" class:stuck={typesStuck} data-pin-pills bind:this={typesBox} onpointerdowncapture={measurePick} onkeydowncapture={measurePick}>
		<SegmentedControl options={typeOptions} value={type} label={t('discover.explore.show')} testid="explore-type" announce={(l) => t('discover.explore.showing', { label: l })} onchange={setType} />
		{#if type === 'music' && genreView.musicGenres.length}
			<!-- Music: the same Genres button; a genre opens its page. -->
			<div class="genre-pick" data-testid="genre-picker">
				<Popover label={t('discover.explore.genres')} text={t('discover.explore.genres')} variant="button" align="end" width={pickW}>
					{#snippet children(close)}
						<div class="genre-tiles" role="group" aria-label={t('discover.explore.genres')}>
							<MusicGenreTiles genres={genreView.musicGenres} allHref={to('/search?kind=music')} compact all={false} onopen={close} />
						</div>
					{/snippet}
				</Popover>
			</div>
		{:else if genreView.genres.length}
			<div class="genre-pick" data-testid="genre-picker">
				<Popover label={t('discover.explore.genres')} text={genreView.genre ? genreView.genre.name : t('discover.explore.genresWord')} variant="button" align="end" width={pickW} onopen={() => void genreView.loadBrowseArt(type)}>
					{#snippet children(close)}
						<div class="genre-tiles" role="group" aria-label={t('discover.explore.genres')}>
							<MusicGenreTiles genres={genreView.genreTiles} allHref="" compact all={false} selected={genreView.genre ? String(genreView.genre.id) : ''} onpick={(k) => genreView.pickGenre(k === null ? null : Number(k), close)} />
						</div>
					{/snippet}
				</Popover>
			</div>
		{/if}
	</div>
	<MetadataNotice />
	{#if bannerView.musicBannerHold}
		<div class="music-banner hold" aria-hidden="true" data-testid="music-banner-hold">
			<GenreBanner layout={2} name={t('discover.explore.yourName', { name: bannerView.musicBannerName || t('discover.explore.music') })} eyebrow={t('discover.explore.pickedFromYourLibrary')} line={t('discover.explore.albumsAndArtistsFrom')} href={to('/explore?type=music')} art={[]} square>
				{#snippet picks()}
					{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<div class="pick-sk"><SkCard /></div>{/each}
				{/snippet}
			</GenreBanner>
		</div>
	{:else if bannerView.musicBannerShown && bannerView.musicFav}
		<div class="music-banner">
			<GenreBanner layout={2} name={t('discover.explore.yourName', { name: bannerView.musicFav.name })} eyebrow={t('discover.explore.pickedFromYourLibrary')} line={t('discover.explore.albumsAndArtistsFrom')} href={to(bannerView.musicFav.href)} art={[bannerView.musicPicks[0]?.image ?? bannerView.musicFav.art].filter(Boolean)} square>
				{#snippet picks()}
					{#if bannerView.musicPicksPending && bannerView.musicPicks.length === 0}
						{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<div class="pick-sk"><SkCard /></div>{/each}
					{/if}
					{#each bannerView.musicPicks as c (c.key)}
						<PosterCard title={c.name} href={to(musicCardHref(c))} image={c.image} kind={c.kind === 'artist' ? 'artist' : undefined} subtitle={c.artist} shape={c.kind === 'artist' ? 'round' : 'square'} />
					{/each}
				{/snippet}
			</GenreBanner>
		</div>
	{/if}
	{#if type === 'music'}<MusicTrendingRows sheets={artistSheets} bind:genres={genreView.musicGenres} bind:ready={bannerView.musicReady} {owned} {manage} {mayAsk} folders={flow?.addChoices().folders ?? []} rules={flow?.addChoices().rules ?? []} onadded={() => void load()} />{/if}

	{#if type === 'music'}
		<!-- Music: the music rows above only. -->
	{:else if failed && !okOnce}
		<LoadError hasData={false} message={t('discover.explore.couldNotLoadExplore')} onretry={load} {busy} />
	{:else}
		{#if failed}<LoadError hasData={true} message={t('discover.explore.couldNotRefreshExplore')} onretry={load} {busy} />{/if}
		<PageLoader loading={!headLoaded} label={t('discover.explore.loadingExplore')} onretry={load}>
		{#if genreView.genreOn && genreView.genreFailed}
			<LoadError hasData={false} message={t('discover.explore.couldNotLoadThis')} onretry={() => genreView.genre && void genreView.loadGenre(genreView.genre.id)} {busy} />
		{:else if genreView.genreOn && rows.length === 0}
			<div class="loading" aria-busy="true" aria-label={t('discover.explore.loadingExplore')} data-testid="explore-skeleton">
				<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="200px" />
				<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="160px" />
				<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="180px" />
			</div>
		{:else if rows.length === 0}
			<TopServicesRow {type} region={servicesView.region} {owned} {who} used={servicesView.usedServices} ask={rowAsk} />
			<EmptyState icon="compass" title={t('discover.explore.exploreIsGettingReady')} text={t('discover.explore.rowsFillInWithin')} action={{ label: t('discover.explore.search'), href: to('/search'), icon: 'search' }} />
		{:else}
			{#if heroCards.length}
				{@const h = live(heroCards[heroAt % heroCards.length])}
				<ExploreHero kind={h.kind === 'show' ? 'show' : 'movie'} tmdb={cardTmdb(h)} title={h.name} href={cardHref(h)} backdrop={h.wide ?? h.image} poster={h.image} vtName={h.vtName} subtitle={h.year ? (h.kind === 'show' ? t('discover.explore.yearShow', { year: h.year }) : t('discover.explore.yearMovie', { year: h.year })) : h.subtitle} settled={headLoaded && !busy}>
					{#snippet action()}{@render heroAction(h)}{/snippet}
					{#snippet badge()}
						{@const why = whyShown({ kind: h.kind, tmdb: cardTmdb(h) ?? '', genres: heroGenres[`${h.kind}:${cardTmdb(h)}`], fresh: h.fresh }, { taste })}
						{#if why.length}<ul class="why" aria-label={t('discover.explore.whyItIsFeatured')}>{#each why as w (w.kind)}<li class="why-{w.kind}">{w.text}</li>{/each}</ul>{/if}
					{/snippet}
				</ExploreHero>
			{/if}
			{#if bannerView.bannerOn && bannerView.favourite}
				<GenreBanner
					layout={bannerView.bannerLayout === 2 ? 2 : 1}
					name={t('discover.explore.yourName', { name: bannerView.favourite.name })}
					eyebrow={t('discover.explore.pickedFromYourLibrary')}
					line={t('discover.explore.librarySoFar', { have: bannerView.favourite.count, more: bannerView.bannerPicks.length })}
					count={bannerView.favourite.count}
					href={to(`/explore?type=${bannerView.favourite.kind}&genre=${bannerView.favourite.id}`)}
					art={bannerView.bannerPicks.map((c) => c.wide ?? c.image ?? '').filter(Boolean)}
				>
					{#snippet picks()}
						{#each bannerView.bannerPicks as c (c.key)}
							{@const act = cardAction(c, { manage, ask: mayAsk })}
							<PosterCard title={c.name} href={cardHref(c)} image={c.image} kind={c.kind} subtitle={c.subtitle} badges={c.badges} typed row="banner" fresh={c.fresh} actionOnHover vtName={undefined}
								preview={cardTmdb(c) ? { kind: c.kind as 'movie' | 'show', tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined} followTmdb={followsByTmdb(c)}>
								{#snippet action()}
									{#if act && cardTmdb(c)}<IconButton icon="plus" label={act === 'add' ? t('discover.explore.add', { name: c.name }) : t('discover.explore.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void quick(c, act)} />{/if}
								{/snippet}
							</PosterCard>
						{/each}
					{/snippet}
				</GenreBanner>
			{/if}
			{#if !genreView.genreOn && stripView.strip.length}<CategoryStrip tiles={stripView.strip} {type} onopen={stripView.stripOpened} />{/if}
			{#each rows as row, ri (row.id)}
				{#if row.type === 'top_services'}
					<TopServicesRow {type} region={servicesView.region} genre={genreView.genreOn ? genreView.genre : null} {owned} {who} used={servicesView.usedServices} bind:shown={servicesShown} seeAll={genreView.genreOn ? undefined : to(newHref(type))} ask={rowAsk} />
					{#if newServices && newCards.length}{@render newServicesRow(newServices, newCards)}{/if}
				{:else}
				<div class="row-wrap" class:wide-row={row.wide || row.cards.some((c) => wideKind(c.kind))} data-row={row.type ?? row.id} data-row-id={row.id} data-title-layer={row.titleLayer}>
				<Shelf title={serverWords(row.title)} subtitle={serverWords(row.subtitle)} href={typeHref(row.href, type)} item={row.cards.some((c) => wideKind(c.kind)) ? 'var(--title-item)' : 'var(--explore-item)'}
					items={row.cards} key={(c) => c.key} defer={ri >= EAGER_ROWS && !back} placeholder={row.wide || row.cards.some((c) => wideKind(c.kind)) ? 'wide' : 'poster'} lines={row.cards.some((c) => c.subtitle) ? 2 : 1}>
					{#snippet card(raw: ExploreCard)}
						{@const c = live(raw)}
						{@const act = cardAction(c, { manage, ask: mayAsk })}
						{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
						{#if row.wide || wideKind(c.kind)}
							<WideCard cover title={c.name} href={cardHref(c)} image={c.wide ?? c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} badges={c.badges} badgesAt="bottom" vtName={c.vtName} fresh={c.fresh} owned={mark} typed row={row.type} preview={cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
									growService={c.streaming ? { name: c.streaming.name, logo: servicesView.services.find((s) => s.id === c.streaming?.service)?.logo } : null}
									growContext={contextLine({ state: mark?.state, label: mark?.label, mine: c.mine, fresh: c.fresh }, { rowType: row.type, rowTitle: row.title, fresh: c.fresh, streaming: c.streaming })}
									followTmdb={followsByTmdb(c)}>
								{#snippet growAction()}
									{#if act && cardTmdb(c)}
										<IconButton icon="plus" label={act === 'add' ? t('discover.explore.add', { name: c.name }) : t('discover.explore.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => { count(row.type ?? row.id, 'ask', row.titleLayer); void quick(c, act); }} />
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
									<IconButton icon="plus" label={act === 'add' ? t('discover.explore.add', { name: c.name }) : t('discover.explore.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => { count(row.type ?? row.id, 'ask', row.titleLayer); void quick(c, act); }} />
								{:else if act}
									<IconButton icon="plus" label={act === 'add' ? t('discover.explore.add', { name: c.name }) : t('discover.explore.askFor', { name: c.name })} variant="glass" size="lg" href={searchHref(c)} />
								{/if}
							{/snippet}
						</PosterCard>
						{/if}
					{/snippet}
				</Shelf>
				</div>
				{/if}
				{#if ri === genreAt && genreCards.length >= ROW_MIN}
					<div class="row-wrap wide-row" data-row="genre_browse" data-row-id="genre_browse">
						<Shelf title={t('discover.explore.browseByGenre')} item="var(--title-item)" items={genreCards} key={(g) => `${g.kind}:${g.id}`} defer={!back} placeholder="wide" lines={1}>
							{#snippet card(g: GenreRowCard)}
								<WideCard cover title={serverWords(g.name)} href={to(genreCardHref(g))} image={g.backdrop} />
							{/snippet}
						</Shelf>
					</div>
				{/if}
			{/each}
			{#if tailMore && !genreView.genreOn}<div class="sentinel" bind:this={sentinel} aria-hidden="true"></div>{/if}
		{/if}
		</PageLoader>
	{/if}
</div>

<QuickAsk bind:this={flow} onadded={() => void load()} />

<style>
	.explore { container: explore / inline-size; /* One size for every title row: 279px cards at a 1536px window, sized from the window (the page has no sidebar), never under 200px. */
		--title-item: max(200px, calc((100vw - 96px) / 5.16)); --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.explore > :global(.ui-head) { margin-bottom: -8px; }
	/* The held banner keeps the real one's words for its height, unseen until the real one comes. */
	.music-banner.hold :global(.name) { visibility: hidden; }
	/* The type switch sits in the page under the title; once scrolled past it floats as a glass pill under the top bar. */
	.types-mark { height: 0; margin-top: -32px; }
	.types { position: sticky; top: calc(var(--topbar) + 10px); z-index: 20; display: flex; justify-content: center; margin-top: -12px; pointer-events: none; }
	.types :global(.ui-seg) { pointer-events: auto; transition: transform var(--bar-glide), box-shadow var(--bar-glide), background-color var(--bar-glide); transform-origin: top center; }
	.types.stuck :global(.ui-seg) { transform: scale(0.9); background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	/* Scaled to 0.9 while stuck: the segments' hit area grows to stay 44px on touch phones. */
	.types.stuck { --seg-hit: 49px; }
	.types :global(.ui-seg) { border-radius: 999px; padding: 3px; }
	.types :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 999px; }
	.types :global(.ui-seg button) { min-height: 38px; padding: 0 16px; border-radius: 999px; font-size: 14px; }
	/* Reduced motion: the same compact size, without the animation. */
	@media (prefers-reduced-motion: reduce) { .types :global(.ui-seg), .types :global(.ui-seg .thumb), .genre-pick :global(.trigger) { transition: none; } }
	@media (max-width: 767px) { .types :global(.ui-seg) { display: grid; width: 100%; } .types :global(.ui-seg button) { padding: 0 8px; } .music-banner { margin-top: 28px; } }
	/* Phones: the pills share the row with Genres (never wider than the screen); a long genre name ends in an ellipsis. */
	@media (max-width: 767px) { .types { max-width: 100%; } .types :global(.ui-seg) { flex: 1 1 auto; width: auto; min-width: 0; } .genre-pick { flex: none; min-width: 0; } .genre-pick :global(.trigger) { max-width: 36vw; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } }
	@media (max-width: 479px) { .types :global(.ui-seg button) { padding: 0 6px; font-size: 13px; min-width: 0; } .genre-pick { margin-left: 6px; } .genre-pick :global(.trigger) { padding: 0 12px; font-size: 13px; } }
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
	/* Minimal look: the floating pills are solid, with no blur and only a hairline edge. */
	:global(html[data-look='minimal']) .types.stuck :global(.ui-seg), :global(html[data-look='minimal']) .types.stuck .genre-pick :global(.trigger) { background: var(--surface-2); -webkit-backdrop-filter: none; backdrop-filter: none; box-shadow: 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
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
	:global(html[data-look='minimal']) .why li { -webkit-backdrop-filter: none; backdrop-filter: none; }
	.row-wrap { display: contents; }
	/* Wide title rows sit almost edge to edge: about 279px cards at a 1536px window with a 5px gap. */
	@media (min-width: 768px) { .row-wrap.wide-row { --shelf-gap: 5px; } }
	.loading { display: flex; flex-direction: column; gap: 32px; min-width: 0; }
	@media (min-width: 768px) { .loading { --shelf-gap: 5px; } }
	@media (max-width: 767px) { .explore { --explore-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); gap: 26px; } }
</style>
