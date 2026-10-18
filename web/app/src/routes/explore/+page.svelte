<script lang="ts">
	import { onMount } from 'svelte';
	import { to } from '#lib/paths.ts';
	import { detailPath, remember } from '#lib/title-details.ts';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { ymd } from '#lib/calendar.ts';
	import {
		becauseYouHave,
		cardAction,
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
	import type { Ask, CalendarEntry, TitleView } from '#lib/types.ts';
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
		return askedHere.has(c.key) ? { ...c, mine: true, status: 'requested', statusLabel: 'Requested' } : c;
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

	async function ask(c: ExploreCard) {
		if (!c.ref) return;
		asking = c.key;
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { id: c.ref });
			askedHere = new Set([...askedHere, c.key]);
			if (r.ask.state === 'approved') toasts.push(`${r.ask.name} is approved; HolyArrs is looking for it.`, 'ok');
			else if (r.joined) toasts.push(`Someone asked for ${r.ask.name} already; you are on the ask too.`, 'ok');
			else toasts.push(`Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
			void refreshInbox();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			asking = '';
		}
	}

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
						<PosterCard
							title={c.name}
							href={cardHref(c)}
							image={c.image}
							kind={c.kind}
							subtitle={c.subtitle}
							badges={c.badges}
							status={c.status}
							statusLabel={c.statusLabel}
							vtName={c.vtName}
						>
							{#snippet action()}
								{#if act === 'add'}
									<IconButton icon="plus" label="Add {c.name}" variant="glass" size="lg" href={searchHref(c)} />
								{:else if act === 'ask' && c.kind === 'movie' && c.ref?.startsWith('tmdb:')}
									<IconButton icon="plus" label="Ask for {c.name}" variant="glass" size="lg" disabled={asking === c.key} onclick={() => ask(c)} />
								{:else if act === 'ask'}
									<IconButton icon="plus" label="Ask for {c.name}" variant="glass" size="lg" href={searchHref(c)} />
								{/if}
							{/snippet}
						</PosterCard>
					{/each}
				</Shelf>
			{/each}
		{/if}
	{/if}
</div>

<style>
	.explore { --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.explore > :global(.ui-head) { margin-bottom: -8px; }
	.loading { display: flex; flex-direction: column; gap: 14px; }
	.sk-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: var(--shelf-gap); max-height: 250px; overflow: hidden; margin-bottom: 16px; }
	@media (max-width: 767px) { .explore { --explore-item: 54vw; gap: 26px; } }
</style>
