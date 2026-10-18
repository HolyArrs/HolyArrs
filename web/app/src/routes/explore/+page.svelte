<script lang="ts">
	import { onMount } from 'svelte';
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
		cardPill,
		cardTmdb,
		comingSoon,
		discoverCard,
		discoverTitles,
		fetchProviderRow,
		fromPerson,
		genreRows,
		genresOf,
		keepFull,
		kindShelf,
		readyToWatch,
		stillLooking,
		markTransitions,
		missingEpisodes,
		popularInHousehold,
		recentlyAdded,
		titleCard,
		type ExploreCard,
		type ExploreRow,
		type Facets
	} from '#lib/explore.ts';
	import type { PersonSummary } from '#lib/people.ts';
	import type { Ask, CalendarEntry, GateDefaults, LibraryFolder, LookupShow, PickRulesView, ShowSettings, TitleDefaults, TitleView } from '#lib/types.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { audienceOf } from '#lib/title-status.ts';
	import AddMovieDialog from '#lib/AddMovieDialog.svelte';
	import AddShowDialog from '#lib/AddShowDialog.svelte';
	import AskShowDialog from '#lib/AskShowDialog.svelte';
	import AskMovieDialog from '#lib/AskMovieDialog.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';

	let loaded = $state(false);
	let failed = $state(false);
	let okOnce = $state(false);
	let busy = $state(false);
	let titles = $state<TitleView[]>([]);
	let dates = $state<CalendarEntry[]>([]);
	let asks = $state<Ask[]>([]);
	let people = $state(new Map<number, string[]>());
	let discovered = $state<ExploreRow[]>([]);
	/** Cards asked for on this page, so the card updates at once. */
	let askedHere = $state(new Set<string>());
	let asking = $state('');

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	const who = $derived(audienceOf(session.can));

	const facets = $derived(new Map<number, Facets>(titles.map((t) => [t.id, { genres: genresOf(t), people: people.get(t.id) ?? [] }])));

	const rows = $derived.by(() => {
		const because = becauseYouHave(titles, facets);
		const person = fromPerson(titles, facets);
		const [trending, popular, upcoming, similar] = ['trending', 'popular', 'upcoming', 'similar'].map((id) => discovered.find((r) => r.id === id));
		return markTransitions(
			keepFull([
				{ id: 'recent', title: 'Recently added', href: to('/library'), cards: recentlyAdded(titles).map((t) => titleCard(t)) },
				trending,
				{ id: 'ready', title: 'Ready to watch', subtitle: 'Every copy is in your Library', cards: readyToWatch(titles) },
				{ id: 'soon', title: 'Coming soon', subtitle: 'From your calendar', href: to('/calendar'), cards: comingSoon(dates, titles) },
				because && { id: 'because', title: `Because you have ${because.seed.name}`, cards: because.cards },
				similar,
				{ id: 'movies', title: 'Movies', href: to('/library?kind=movie'), cards: kindShelf(titles, 'movie') },
				{ id: 'shows', title: 'TV shows', href: to('/library?kind=show'), cards: kindShelf(titles, 'show') },
				...genreRows(titles, facets),
				popular,
				{ id: 'missing', title: 'Missing episodes', subtitle: 'Shows with aired episodes that have no file', href: to('/library?kind=show&show=missing&layout=list'), cards: missingEpisodes(titles) },
				{ id: 'looking', title: 'Still looking', subtitle: 'No copy in your Library yet', cards: stillLooking(titles) },
				{ id: 'music', title: 'Music', href: to('/library?kind=artist'), cards: kindShelf(titles, 'artist') },
				{ id: 'household', title: 'Popular in your household', subtitle: 'What people here asked for most', href: to('/wishlist'), cards: popularInHousehold(asks, titles) },
				person && { id: 'person', title: `From ${person.person}`, cards: person.cards },
				upcoming
			])
		);
	});

	function live(c: ExploreCard): ExploreCard {
		return askedHere.has(c.key) ? { ...c, mine: true, status: 'requested', statusLabel: undefined } : c;
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
	let titleDefaults = $state<TitleDefaults | null>(null);
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
		if (act === 'ask' && c.kind === 'movie') return askMovie?.open({ tmdb, name: c.name, year: c.year });
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
		if (current) askedHere = new Set([...askedHere, current.key]);
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
		api.get<TitleDefaults>('/settings/title-defaults').then((r) => (titleDefaults = r)).catch(() => {});
	});

	async function loadPeople(list: TitleView[]) {
		// A few of the newest movies only: each one asks TMDB.
		const movies = recentlyAdded(list.filter((t) => t.kind === 'movie' && t.ids?.tmdb), 6);
		const got = await Promise.allSettled(movies.map((t) => api.get<{ people: { person: PersonSummary }[] }>(`/titles/${t.id}/people`)));
		const next = new Map<number, string[]>();
		got.forEach((g, i) => {
			if (g.status === 'fulfilled') next.set(movies[i].id, (g.value.people ?? []).map((p) => p.person.name));
		});
		if (next.size) people = next;
	}

	async function loadProviderRows(list: TitleView[]) {
		const seed = recentlyAdded(list.filter((t) => t.kind === 'movie' || t.kind === 'show'), 1)[0];
		const asks = [
			{ row: 'trending' as const },
			{ row: 'popular' as const },
			{ row: 'upcoming' as const },
			...(seed ? [{ row: 'similar' as const, id: seed.id }] : [])
		];
		// Ask one row first: a server without discover rows answers 404, and
		// then the rest are not worth asking.
		const first = await fetchProviderRow(asks[0]).catch(() => null);
		if (!first) return;
		const got = [{ status: 'fulfilled' as const, value: first }, ...(await Promise.allSettled(asks.slice(1).map((q) => fetchProviderRow(q))))];
		const out: ExploreRow[] = [];
		got.forEach((g, i) => {
			if (g.status !== 'fulfilled' || !g.value) return;
			const q = asks[i];
			const title = q.row === 'similar' ? `More like ${seed!.name}` : (g.value.title ?? discoverTitles[q.row]);
			out.push({ id: q.row, title, cards: g.value.items.map((d) => discoverCard(d, q.row)) });
		});
		discovered = out;
	}

	async function load() {
		busy = true;
		const from = ymd(new Date());
		const until = ymd(new Date(Date.now() + 60 * 86400000));
		const [ti, ca, as] = await Promise.allSettled([
			api.get<{ titles: TitleView[] }>('/titles'),
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
		if (ti.status === 'fulfilled') {
			void loadPeople(titles);
			void loadProviderRows(titles).catch(() => {});
		}
	}

	onMount(() => {
		void load();
	});
</script>

<svelte:head><title>Explore · HolyArrs</title></svelte:head>

<div class="explore">
	<PageHeader title="Explore" subtitle="Find something to watch or listen to next." />

	{#if failed && !okOnce}
		<LoadError hasData={false} message="Could not load Explore." onretry={load} {busy} />
	{:else}
		{#if failed}<LoadError hasData={true} message="Could not refresh Explore." onretry={load} {busy} />{/if}
		{#if !loaded}
			<div class="loading" aria-busy="true" aria-label="Loading">
				{#each [0, 1] as r (r)}
					<Skeleton width="200px" />
					<div class="sk-row">{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<Skeleton shape="poster" />{/each}</div>
				{/each}
			</div>
		{:else if rows.length === 0}
			<EmptyState icon="compass" title="Nothing to explore yet" text="Rows fill in as your Library grows." action={{ label: 'Search', href: to('/search'), icon: 'search' }} />
		{:else}
			{#each rows as row (row.id)}
				<Shelf title={row.title} subtitle={row.subtitle} href={row.href} item="var(--explore-item)">
					{#each row.cards.map(live) as c (c.key)}
						{@const act = cardAction(c, { manage, ask: mayAsk })}
						{@const pill = cardPill(c, who)}
						<PosterCard
							title={c.name}
							href={cardHref(c)}
							image={c.image}
							kind={c.kind}
							subtitle={c.subtitle}
							badges={c.badges}
							status={pill?.kind}
							statusLabel={pill?.label}
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
					{/each}
				</Shelf>
			{/each}
		{/if}
	{/if}
</div>

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} {titleDefaults} onadded={(_m, t) => added(t)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} {titleDefaults} onadded={(_s, t) => added(t)} />
{:else if mayAsk}
	<AskMovieDialog bind:this={askMovie} onasked={(_m, r) => askedToast(r, r.ask.name)} />
	<AskShowDialog bind:this={askShow} onasked={(_s: LookupShow, r) => askedToast(r, `${r.ask.name} (${seasonWords(r.ask.seasons)})`)} />
{/if}

<style>
	.explore { --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.explore > :global(.ui-head) { margin-bottom: -8px; }
	.loading { display: flex; flex-direction: column; gap: 14px; }
	.sk-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: var(--shelf-gap); max-height: 250px; overflow: hidden; margin-bottom: 16px; }
	@media (max-width: 767px) { .explore { --explore-item: 54vw; gap: 26px; } }
</style>
