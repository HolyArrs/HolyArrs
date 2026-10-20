<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
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
		filterRows,
		parseType,
		pickerServices,
		readServices,
		serverRow,
		serviceRegion,
		SERVICES_KEY,
		SERVICES_SHOWN,
		tailPath,
		toggleService,
		writeServices,
		typeHref,
		type ExploreType,
		type ExploreCard,
		type ExploreRow,
		type HeadAnswer,
		type TailAnswer,
		type WatchService
	} from '#lib/explore.ts';
	import { defaultRegion, savedTop10 } from '#lib/top10.ts';
	import type { Ask, AskState, GateDefaults, LibraryFolder, LookupShow, PickRulesView, ShowSettings, TitleView } from '#lib/types.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { audienceOf } from '#lib/title-status.ts';
	import AddMovieDialog from '#lib/AddMovieDialog.svelte';
	import { savedDefaults } from '#lib/title-defaults.svelte.ts';
	import AddShowDialog from '#lib/AddShowDialog.svelte';
	import AskShowDialog from '#lib/AskShowDialog.svelte';
	import AskMovieDialog from '#lib/AskMovieDialog.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import MetadataNotice from '#lib/MetadataNotice.svelte';
	import Top10Row from '#lib/Top10Row.svelte';
	import MusicTrendingRows from '#lib/MusicTrendingRows.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import ExploreHero from '#lib/ui/ExploreHero.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import type { IconName } from '#lib/Icon.svelte';
	import { EAGER_ROWS, stableRows } from '#lib/row-window.ts';
	import Icon from '#lib/Icon.svelte';
	import MoreInfo from '#lib/ui/MoreInfo.svelte';
	import Theater from '#lib/ui/Theater.svelte';
	import { onTheaterExit } from '#lib/theater.svelte.ts';
	import { bigRows, CARD_OPENS_KEY, parseTitleParam, readCardOpens, withTitle, type CardOpens, type InfoRef } from '#lib/more-info.ts';

	let loaded = $state(false);
	let failed = $state(false);
	let okOnce = $state(false);
	let busy = $state(false);
	let titles = $state<TitleView[]>([]);
	/** The server-built page: the head in one answer, then tail pages as the page scrolls. */
	let head = $state<(ExploreRow & { wide: boolean; type: string })[]>([]);
	let tail = $state<(ExploreRow & { wide: boolean; type: string })[]>([]);
	let top10 = $state<HeadAnswer['top10']>(null);
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
		void goto(url, { replace: true, shallow: true, reset: false });
		void loadHead();
	}

	// The More info panel: ?title=movie:123 in the address (shallow), so Back
	// closes it and the link can be shared; the page keeps its place behind it.
	const shownUrl = $derived(page.shallow?.url ?? page.url);
	const info = $derived(parseTitleParam(shownUrl.searchParams.get('title')));
	let infoFrom = $state<DOMRect | null>(null);
	/** The panel was opened here (a history entry of ours), so closing it goes Back. */
	let pushed = false;
	$effect(() => {
		if (!info) pushed = false;
	});
	function openInfo(r: InfoRef, from: DOMRect | null) {
		infoFrom = from;
		const url = withTitle(shownUrl.href, r);
		if (info) return void goto(url, { shallow: true, replace: true, reset: false });
		pushed = true;
		void goto(url, { shallow: true, reset: false });
	}
	function closeInfo() {
		if (pushed) {
			pushed = false;
			history.back();
		} else void goto(withTitle(shownUrl.href, null), { shallow: true, replace: true, reset: false });
	}
	// A card click opens the title page, or the panel when this device prefers it.
	let cardOpens = $state<CardOpens>('page');
	function setCardOpens(v: CardOpens) {
		cardOpens = v;
		try {
			localStorage.setItem(CARD_OPENS_KEY, v);
		} catch {
			/* the choice is a convenience */
		}
	}
	/** infoHref is the panel's own address for a movie or show card (a real link: new tabs open the panel too). */
	function infoHref(c: ExploreCard): string | undefined {
		const tmdb = cardTmdb(c);
		if (!tmdb || (c.kind !== 'movie' && c.kind !== 'show')) return undefined;
		const u = withTitle(shownUrl.href, { kind: c.kind, tmdb });
		return u.pathname + u.search;
	}
	// Leaving the full-screen trailer lands in the title's panel (from the panel itself: back to it as it was).
	$effect(() => onTheaterExit((o) => {
		if (o.from !== 'panel') openInfo({ kind: o.kind, tmdb: o.tmdb }, null);
	}));

	// Music shows only music rows: the movie and show rows (Top 10, services, Coming soon, Best new releases) never appear there.
	const rows = $derived(type === 'music' ? [] : markTransitions(filterRows([...head, ...tail], type)));
	/** Big 4 shelves: the first personal row and Best new releases draw bigger cards. */
	const big = $derived(bigRows(rows));

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
			top10 = a.top10 ? { ...a.top10, kind: q.top10Kind } : null;
			hero = (a.hero ?? []).map((d) => ({ ...discoverCard(d, 'hero'), wide: d.backdrop ?? null }));
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

	async function loadTail() {
		if (tailBusy || !tailMore || type === 'music') return;
		tailBusy = true;
		const n = headAsk;
		try {
			const a = await api.get<TailAnswer>(tailPath(query(), tailPageNext), gone.signal);
			if (n !== headAsk) return;
			tail = [...tail, ...a.rows.map(serverRow)];
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
		// A card link to the panel (the device prefers it) opens it here, growing from the card.
		const link = el?.closest?.('a[href]') as HTMLAnchorElement | null;
		if (link && !e.defaultPrevented && e.button === 0 && !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey && link.closest('.row-wrap')) {
			const r = parseTitleParam(new URL(link.href, location.href).searchParams.get('title'));
			if (r) {
				e.preventDefault();
				openInfo(r, (link.querySelector('.art') ?? link).getBoundingClientRect());
			}
		}
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

	async function loadServices() {
		try {
			picked = readServices(region, localStorage.getItem(SERVICES_KEY));
		} catch {
			/* nothing saved */
		}
		try {
			services = (await api.get<{ services: WatchService[] }>(`/watch-services?region=${region}`, gone.signal)).services ?? [];
		} catch {
			services = [];
		}
	}

	const serviceCards = $derived(rows.find((r) => r.id === 'services')?.cards.slice(0, rowCap) ?? []);

	function tapService(id: number) {
		picked = toggleService(picked ?? [], id);
		try {
			localStorage.setItem(SERVICES_KEY, writeServices(region, picked, localStorage.getItem(SERVICES_KEY)));
		} catch {
			/* the choice is a convenience */
		}
		void loadHead();
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
		try {
			cardOpens = readCardOpens(localStorage.getItem(CARD_OPENS_KEY));
		} catch {
			/* nothing saved */
		}
		void load();
		void loadServices().then(() => void loadHead());
		return () => {
			clearTimeout(poll);
			gone.abort();
		};
	});
</script>

<svelte:head><title>Explore · HolyArrs</title></svelte:head>

{#snippet heroAction(c: ExploreCard)}
	{@const act = cardAction(c, { manage, ask: mayAsk })}
	{#if act && cardTmdb(c)}
		<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => void quick(c, act)} />
	{/if}
{/snippet}

{#snippet serviceTools()}
	<div class="svc" data-testid="services-picker">
		{#if picked === null}<p class="svc-hint">Pick the services you have</p>{/if}
		<div class="chips" role="group" aria-label="Your streaming services">
			{#each shownServices as sv (sv.id)}
				{@const on = picked !== null && picked.includes(sv.id)}
				<button type="button" class="svc-chip" class:on aria-pressed={on} onclick={() => tapService(sv.id)} title={sv.name}>
					{#if sv.logo}<img src={sv.logo} alt="" width="28" height="28" loading="lazy" />{/if}
					<span>{sv.name}</span>
				</button>
			{/each}
		</div>
		{#if serviceCards.length === 0}<p class="svc-hint">{usedServices.length ? 'Nothing found on these services.' : 'Pick a service to fill this row.'}</p>{/if}
	</div>
{/snippet}

<!-- A card opened from a row is counted (to measure only). -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="explore" onclick={opened}>
	<PageHeader title="Explore" subtitle="Find something to watch or listen to next." />
	<div class="types-mark" bind:this={typesMark} aria-hidden="true"></div>
	<div class="types" class:stuck={typesStuck}>
		<SegmentedControl options={typeOptions} value={type} label="Show" testid="explore-type" onchange={setType} />
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
		{:else if rows.length === 0}
			<Top10Row lock={type === 'movies' ? 'movie' : type === 'shows' ? 'show' : undefined} {owned} {who} initial={top10} />
			<EmptyState icon="compass" title="Explore is getting ready" text="Rows fill in within a few minutes." action={{ label: 'Search', href: to('/search'), icon: 'search' }} />
		{:else}
			{#if hero.length}
				{@const h = live(hero[heroAt % hero.length])}
				<ExploreHero kind={h.kind === 'show' ? 'show' : 'movie'} tmdb={cardTmdb(h)} title={h.name} href={cardHref(h)} backdrop={h.wide ?? h.image} onmore={cardTmdb(h) ? (from) => openInfo({ kind: h.kind === 'show' ? 'show' : 'movie', tmdb: cardTmdb(h) }, from) : undefined} subtitle={h.year ? `${h.year} · ${h.kind === 'show' ? 'Show' : 'Movie'}` : h.subtitle} settled={headLoaded && !busy}>
					{#snippet action()}{@render heroAction(h)}{/snippet}
				</ExploreHero>
			{/if}
			{#if !rows.some((r) => r.type === 'top10')}<Top10Row lock={type === 'movies' ? 'movie' : type === 'shows' ? 'show' : undefined} {owned} {who} initial={top10} />{/if}
			{#each rows as row, ri (row.id)}
				{#if row.type === 'top10'}
					<Top10Row lock={type === 'movies' ? 'movie' : type === 'shows' ? 'show' : undefined} {owned} {who} initial={top10} />
				{:else}
				<div class="row-wrap" data-row={row.type ?? row.id} data-title-layer={row.titleLayer}>
				<Shelf title={row.title} subtitle={row.subtitle} href={typeHref(row.href, type)} item={row.cards.some((c) => wideKind(c.kind)) ? (big.has(row.id) ? 'var(--title-item-big)' : 'var(--title-item)') : 'var(--explore-item)'} tools={row.id === 'services' ? serviceTools : undefined}
					items={row.cards} key={(c) => c.key} defer={ri >= EAGER_ROWS} placeholder={row.wide || row.cards.some((c) => wideKind(c.kind)) ? 'wide' : 'poster'} lines={row.cards.some((c) => c.subtitle) ? 2 : 1}>
					{#snippet card(raw: ExploreCard)}
						{@const c = live(raw)}
						{@const act = cardAction(c, { manage, ask: mayAsk })}
						{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
						{#if row.wide || wideKind(c.kind)}
							<WideCard cover big={big.has(row.id)} title={c.name} href={(cardOpens === 'panel' ? infoHref(c) : undefined) ?? cardHref(c)} onmore={cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? (from) => openInfo({ kind: c.kind as 'movie' | 'show', tmdb: cardTmdb(c) }, from) : undefined} image={c.wide ?? c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} badges={c.badges} badgesAt="bottom" vtName={c.vtName} fresh={c.fresh} owned={mark} typed row={row.type} preview={cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year } : undefined}>
								{#snippet growAction()}
									{#if act && cardTmdb(c)}
										<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => { count(row.type ?? row.id, 'ask', row.titleLayer); void quick(c, act); }} />
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
			{#if tailMore}<div class="sentinel" bind:this={sentinel} aria-hidden="true"></div>{/if}
		{/if}
	{/if}
</div>

{#snippet infoAction(c: ExploreCard)}
	{@const act = cardAction(c, { manage, ask: mayAsk })}
	{#if act && cardTmdb(c)}
		<button type="button" class="info-act" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} data-testid="info-act" onclick={() => { count('more_info', 'ask'); void quick(c, act); }}>
			<Icon name="plus" size={20} stroke={2.2} /><span>{act === 'add' ? 'Add' : c.kind === 'show' ? 'Ask for seasons…' : 'Ask'}</span>
		</button>
	{/if}
{/snippet}

{#snippet infoCard(c: ExploreCard)}
	<WideCard cover title={c.name} href={infoHref(c) ?? cardHref(c)} image={c.wide ?? c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} badges={c.badges} badgesAt="bottom" fresh={c.fresh} owned={ownedMark(c, ownedTitle(c, owned), who)} typed />
{/snippet}

{#if info}
	<MoreInfo ref={info} from={infoFrom} {region} {live} action={infoAction} card={infoCard} {cardOpens} onCardOpens={setCardOpens} onpick={openInfo} onclose={closeInfo} />
{/if}
<Theater />

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} onadded={(_m, t) => added(t)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} onadded={(_s, t) => added(t)} />
{:else if mayAsk}
	<AskMovieDialog bind:this={askMovie} onasked={(_m, r) => askedToast(r, r.ask.name)} onrefused={(_m, body) => refused(body)} />
	<AskShowDialog bind:this={askShow} onasked={(_s: LookupShow, r) => askedToast(r, `${r.ask.name} (${seasonWords(r.ask.seasons)})`)} onrefused={(_s: LookupShow, body: Record<string, unknown>) => refused(body)} />
{/if}

<style>
	.explore { container: explore / inline-size; --title-item: max(240px, calc((100cqi - 5 * var(--shelf-gap)) / 5.5)); --title-item-big: calc((100cqi - 4 * var(--shelf-gap)) / 4.2); --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.explore > :global(.ui-head) { margin-bottom: -8px; }
	/* The type switch: in the page under the title; once scrolled past it floats as a glass pill under the top bar. */
	.types-mark { height: 0; margin-top: -32px; }
	.types { position: sticky; top: calc(var(--topbar) + 10px); z-index: 20; display: flex; justify-content: center; margin-top: -12px; pointer-events: none; }
	.types :global(.ui-seg) { pointer-events: auto; transition: transform 320ms cubic-bezier(.2,.8,.2,1), box-shadow 320ms ease, background-color 320ms ease; transform-origin: top center; }
	.types.stuck :global(.ui-seg) { transform: scale(0.92); background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.types :global(.ui-seg) { border-radius: 999px; padding: 3px; }
	.types :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 999px; }
	.types :global(.ui-seg button) { min-height: 44px; padding: 0 20px; border-radius: 999px; }
	@media (prefers-reduced-motion: reduce) { .types :global(.ui-seg), .types :global(.ui-seg .thumb) { transition: none; } .types.stuck :global(.ui-seg) { transform: none; } }
	@media (max-width: 767px) { .types :global(.ui-seg) { display: grid; width: 100%; } .types :global(.ui-seg button) { padding: 0 8px; } }
	@media (max-width: 359px) {
		.types :global(.ui-seg button span) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	}
	.svc { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.svc-hint { color: var(--text-muted); font: var(--t-callout); }
	.chips { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-inline: calc(var(--gutter) * -1); padding: 2px var(--gutter); }
	.chips::-webkit-scrollbar { display: none; }
	.svc-chip { flex: none; display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 14px 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-callout); font-weight: 500; box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent); transition: background-color var(--dur-fast) ease, box-shadow var(--dur-fast) ease; }
	.svc-chip img { width: 28px; height: 28px; border-radius: 7px; object-fit: cover; }
	.svc-chip:hover { background: color-mix(in srgb, var(--text) 10%, var(--surface-2)); }
	.svc-chip.on { background: color-mix(in srgb, var(--accent) 18%, var(--surface-2)); box-shadow: inset 0 0 0 2px var(--accent); }
	.svc-chip:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sentinel { height: 1px; }
	.row-wrap { display: contents; }
	.loading { display: flex; flex-direction: column; gap: 14px; }
	.sk-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: var(--shelf-gap); max-height: 250px; overflow: hidden; margin-bottom: 16px; }
	@media (max-width: 767px) { .explore { --explore-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); --title-item-big: calc((100cqi - var(--shelf-gap)) / 1.08); gap: 26px; } }
</style>
