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
	import { ymd } from '#lib/calendar.ts';
	import {
		becauseYouHave,
		cardAction,
		cardTmdb,
		comingSoon,
		discoverCard,
		discoverTitles,
		fetchProviderRow,
		fromPerson,
		genreRows,
		genresOf,
		keepFull,
		liveCard,
		matchOwned,
		ownedIndex,
		ownedMark,
		ownedTitle,
		markTransitions,
		popularInHousehold,
		recentlyAdded,
		filterRows,
		parseType,
		typeHref,
		typeKindOf,
		type ExploreType,
		type ExploreCard,
		type ExploreRow,
		type Facets
	} from '#lib/explore.ts';
	import type { ProviderRowQuery } from '#lib/explore.ts';
	import type { PersonSummary } from '#lib/people.ts';
	import type { Ask, AskState, CalendarEntry, GateDefaults, LibraryFolder, LookupShow, PickRulesView, ShowSettings, TitleView } from '#lib/types.ts';
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
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import type { IconName } from '#lib/Icon.svelte';

	let loaded = $state(false);
	let failed = $state(false);
	let okOnce = $state(false);
	let busy = $state(false);
	let titles = $state<TitleView[]>([]);
	let dates = $state<CalendarEntry[]>([]);
	let asks = $state<Ask[]>([]);
	let people = $state(new Map<number, string[]>());
	let discovered = $state<ExploreRow[]>([]);
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
		void loadKindRows(t);
	}

	/** Provider rows asked for one kind, so a Movies or Shows row is full rather than half of a mixed one. */
	let kindRowsGot = $state<Record<'movie' | 'show', ExploreRow[]>>({ movie: [], show: [] });
	const kindAsked = new Set<string>();
	async function loadKindRows(t: ExploreType) {
		const kind = typeKindOf(t);
		if ((kind !== 'movie' && kind !== 'show') || kindAsked.has(kind) || !discovered.length) return;
		kindAsked.add(kind);
		await Promise.all(
			(['trending', 'popular', 'upcoming'] as const).map((row) =>
				fetchProviderRow({ row, kind }, api, gone.signal)
					.then((got) => {
						if (!got || gone.signal.aborted) return;
						const r = { id: row, title: got.title ?? discoverTitles[row], cards: got.items.slice(0, rowCap).map((d) => discoverCard(d, row)) };
						kindRowsGot = { ...kindRowsGot, [kind]: [...kindRowsGot[kind].filter((x) => x.id !== row), r] };
					})
					.catch(() => {})
			)
		);
	}
	// The mixed rows came in after a type was picked (or the page opened on one).
	$effect(() => {
		if (discovered.length && type !== 'all') void loadKindRows(type);
	});

	const facets = $derived(new Map<number, Facets>(titles.map((t) => [t.id, { genres: genresOf(t), people: people.get(t.id) ?? [] }])));

	const rows = $derived.by(() => {
		const because = becauseYouHave(titles, facets);
		const person = fromPerson(titles, facets);
		const k = typeKindOf(type);
		const own = k === 'movie' || k === 'show' ? kindRowsGot[k] : [];
		const [trending, popular, upcoming, similar] = ['trending', 'popular', 'upcoming', 'similar'].map((id) => own.find((r) => r.id === id) ?? discovered.find((r) => r.id === id));
		return markTransitions(
			filterRows(keepFull([
				trending,
				{ id: 'soon', title: 'Coming soon', subtitle: 'From your calendar', href: to('/calendar'), cards: comingSoon(dates, titles) },
				because && { id: 'because', title: `Because you have ${because.seed.name}`, cards: because.cards },
				similar,
				...genreRows(titles, facets),
				popular,
				{ id: 'household', title: 'Popular in your household', subtitle: 'What people here asked for most', href: to('/wishlist'), cards: popularInHousehold(asks, titles) },
				person && { id: 'person', title: `From ${person.person}`, cards: person.cards },
				upcoming
			]), type)
		);
	});

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

	async function loadPeople(list: TitleView[]) {
		// A few of the newest movies only: each one asks TMDB.
		const movies = recentlyAdded(list.filter((t) => t.kind === 'movie' && t.ids?.tmdb), 6);
		const got = await Promise.allSettled(movies.map((t) => api.get<{ people: { person: PersonSummary }[] }>(`/titles/${t.id}/people`, gone.signal)));
		const next = new Map<number, string[]>();
		got.forEach((g, i) => {
			if (g.status === 'fulfilled') next.set(movies[i].id, (g.value.people ?? []).map((p) => p.person.name));
		});
		if (next.size && !gone.signal.aborted) people = next;
	}

	/** A discover row shows at most this many cards; a longer answer is cut. */
	const rowCap = 20;

	async function loadProviderRows(list: Promise<TitleView[]>) {
		const asks = [{ row: 'trending' as const }, { row: 'popular' as const }, { row: 'upcoming' as const }];
		const show = (q: ProviderRowQuery, got: Awaited<ReturnType<typeof fetchProviderRow>>, title?: string) => {
			if (!got || gone.signal.aborted) return;
			const row = { id: q.row, title: title ?? got.title ?? (discoverTitles as Record<string, string>)[q.row] ?? '', cards: got.items.slice(0, rowCap).map((d) => discoverCard(d, q.row)) };
			discovered = [...discovered.filter((r) => r.id !== q.row), row];
		};
		// Ask one row first: a server without discover rows answers 404, and
		// then the rest are not worth asking. Each row shows as soon as it comes.
		const first = await fetchProviderRow(asks[0], api, gone.signal).catch(() => null);
		if (!first) return;
		show(asks[0], first);
		const rest = asks.slice(1).map((q) => fetchProviderRow(q, api, gone.signal).then((g) => show(q, g)).catch(() => {}));
		// "More like" needs the Library, which may still be on its way.
		const seed = recentlyAdded((await list).filter((t) => t.kind === 'movie' || t.kind === 'show'), 1)[0];
		if (seed) {
			const q = { row: 'similar' as const, id: seed.id };
			rest.push(fetchProviderRow(q, api, gone.signal).then((g) => show(q, g, `More like ${seed.name}`)).catch(() => {}));
		}
		await Promise.all(rest);
	}

	async function load() {
		busy = true;
		const from = ymd(new Date());
		const until = ymd(new Date(Date.now() + 60 * 86400000));
		// Every ask starts at once; discover rows do not wait for the Library.
		const library = api.get<{ titles: TitleView[] }>('/titles');
		if (!discovered.length) void loadProviderRows(library.then((r) => r.titles ?? []).catch(() => [])).catch(() => {});
		const [ti, ca, as] = await Promise.allSettled([
			library,
			api.get<{ dates: CalendarEntry[] | null }>(`/calendar?from=${from}&to=${until}`),
			mayAsk || session.can('approve') ? api.get<{ asks: Ask[] }>('/asks') : Promise.resolve(null)
		]);
		failed = ti.status === 'rejected';
		if (ti.status === 'fulfilled') {
			titles = ti.value.titles ?? [];
			okOnce = true;
		}
		if (ca.status === 'fulfilled') dates = ca.value.dates ?? [];
		if (as.status === 'fulfilled' && as.value) asks = as.value.asks ?? [];
		loaded = true;
		busy = false;
		if (ti.status === 'fulfilled') void loadPeople(titles);
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
		return () => gone.abort();
	});
</script>

<svelte:head><title>Explore · HolyArrs</title></svelte:head>

<div class="explore">
	<PageHeader title="Explore" subtitle="Find something to watch or listen to next." />
	<div class="types-mark" bind:this={typesMark} aria-hidden="true"></div>
	<div class="types" class:stuck={typesStuck}>
		<SegmentedControl options={typeOptions} value={type} label="Show" testid="explore-type" onchange={setType} />
	</div>
	<MetadataNotice />

	{#if failed && !okOnce}
		<LoadError hasData={false} message="Could not load Explore." onretry={load} {busy} />
	{:else}
		{#if failed}<LoadError hasData={true} message="Could not refresh Explore." onretry={load} {busy} />{/if}
		{#if !loaded && discovered.length === 0}
			<div class="loading" aria-busy="true" aria-label="Loading">
				{#each [0, 1] as r (r)}
					<Skeleton width="200px" />
					<div class="sk-row">{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<Skeleton shape="poster" />{/each}</div>
				{/each}
			</div>
		{:else if rows.length === 0}
			<EmptyState icon="compass" title="Nothing to explore yet" text="Rows fill in as your Library grows." action={{ label: 'Search', href: to('/search'), icon: 'search' }} />
		{:else}
			{#if type !== 'music'}<Top10Row lock={type === 'movies' ? 'movie' : type === 'shows' ? 'show' : undefined} />{/if}
			{#each rows as row (row.id)}
				<Shelf title={row.title} subtitle={row.subtitle} href={typeHref(row.href, type)} item={row.id === 'soon' ? 'var(--release-item)' : 'var(--explore-item)'}>
					{#each row.cards.map(live) as c (c.key)}
						{@const act = cardAction(c, { manage, ask: mayAsk })}
						{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
						{#if row.id === 'soon'}
							<WideCard title={c.name} href={cardHref(c)} image={c.wide} kind={c.kind} subtitle={c.subtitle} badges={c.badges} badgesAt="bottom" vtName={c.vtName} />
						{:else}
						<PosterCard
							title={c.name}
							href={cardHref(c)}
							image={c.image}
							kind={c.kind}
							subtitle={c.subtitle}
							badges={c.badges}
							typePill={type === 'all' ? c.kind : undefined}
							owned={mark}
							actionOnHover
							vtName={c.vtName}
						>
							{#snippet action()}
								{#if act && cardTmdb(c)}
									<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => void quick(c, act)} />
								{:else if act}
									<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" href={searchHref(c)} />
								{/if}
							{/snippet}
						</PosterCard>
						{/if}
					{/each}
				</Shelf>
			{/each}
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
	.explore { --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
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
	.loading { display: flex; flex-direction: column; gap: 14px; }
	.sk-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: var(--shelf-gap); max-height: 250px; overflow: hidden; margin-bottom: 16px; }
	@media (max-width: 767px) { .explore { --explore-item: 54vw; gap: 26px; } }
</style>
