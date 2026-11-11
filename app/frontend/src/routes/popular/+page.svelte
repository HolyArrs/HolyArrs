<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { cardAction, cardTmdb, followsByTmdb, discoverCard, liveCard, matchOwned, ownedIndex, ownedMark, ownedTitle, type ExploreCard, type ExploreRow } from '#lib/ui/title-cards.ts';
	import { contextLine } from '#lib/ui/grow-lines.ts';
	import { defaultRegion } from '#lib/ui/region.ts';
	import { audienceOf } from '#lib/ui/title-status.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import type { IconName } from '#lib/ui/Icon.svelte';
	import { artistSheets } from '#lib/requests/index.ts';
	import { libraryTitles, ownedLibrary, partOff, partOn, savedDefaults } from '#lib/mediafiles/index.ts';
	import { detailPath, loadDetails, remember, MetadataNotice } from '#lib/metadata/index.ts';
	import { QuickAsk } from '#lib/requests/index.ts';
	import { serverRow, serviceRegion, type WatchService, cardServices, hasServiceChips, isRanked, newPath, newServicePath, parseNewType, pickService, type NewAnswer, type NewServiceAnswer, type NewType, type RailDay, type RailItem, PopularRail as NewRail, ExploreHero, whyShown, type Why } from '#lib/discover/index.ts';
	import type { GateDefaults } from '#lib/mediafiles/index.ts';
	import type { ShowSettings } from '#lib/shows/index.ts';
	import { seasonWords } from '#lib/shows/index.ts';
	import { MusicNewTab } from '#lib/music/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { proxied } from '#lib/core/tmdb-image.ts';
	import { api, message } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { refreshInbox } from '#lib/core/inbox.svelte.ts';
	import type { Ask, AskState, LibraryFolder, PickRulesView, TitleView } from '#lib/core/types.ts';
	import type { LookupShow } from '#lib/core/types.ts';
	import { buildingStatus } from '#lib/core/silhouette.ts';

	type Row = ExploreRow & { wide: boolean; type: string };

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	const who = $derived(audienceOf(session.can));

	let type = $state<NewType>(parseNewType(page.url.searchParams.get('type') ?? page.url.searchParams.get('kind')));
	const typeOptions = $derived(
		([
			{ value: 'all', label: t('discover.popular.all'), icon: 'grid' },
			{ value: 'shows', label: t('discover.popular.tvShows'), icon: 'tv' },
			{ value: 'movies', label: t('discover.popular.movies'), icon: 'film' },
			{ value: 'music', label: t('discover.popular.music'), icon: 'music' }
		] as { value: NewType; label: string; icon: IconName }[]).filter((o) => o.value !== 'music' || partOn('music'))
	);
	// Music turned off while picked falls back to All.
	$effect(() => {
		if (type === 'music' && partOff('music')) setType('all');
	});

	let rows = $state<Row[]>([]);
	let rail = $state<RailDay[]>([]);
	let hero = $state<ExploreCard[]>([]);
	let heroAt = $state(0);
	let loaded = $state(false);
	let failed = $state(false);
	let more = $state(false);
	let next = 0;
	let busy = false;
	let ask = 0;
	let poll: ReturnType<typeof setTimeout> | undefined;
	/** The try number while the server says the lists are still building (0 means not building). */
	let building = $state(0);
	const gone = new AbortController();

	// The pills float once their spot scrolls under the top bar (as on Explore).
	let typesMark = $state<HTMLElement>();
	let typesStuck = $state(false);
	$effect(() => {
		if (!typesMark) return;
		const top = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--topbar')) || 56;
		const io = new IntersectionObserver(([e]) => (typesStuck = !e.isIntersecting), { rootMargin: `-${top}px 0px 0px 0px` });
		io.observe(typesMark);
		return () => io.disconnect();
	});

	function setType(t: NewType) {
		type = t;
		svcLists = new Map();
		svcGen++;
		svcPicks = new Map();
		const url = new URL((page.shallow?.url ?? page.url).href);
		if (t === 'all') url.searchParams.delete('type');
		else url.searchParams.set('type', t);
		url.searchParams.delete('kind');
		void goto(url, { replace: true, shallow: true, reset: false });
		if (t !== 'music') void loadFirst();
	}

	/** Reads page 0; while lists are still building it asks again every 3 s (at most a minute). */
	async function loadFirst(tries = 0) {
		const n = ++ask;
		clearTimeout(poll);
		try {
			const a = await api.get<NewAnswer>(newPath(type, 0), gone.signal);
			if (n !== ask) return;
			rows = a.rows.map(serverRow);
			rail = a.rail ?? [];
			hero = (a.hero ?? []).map((d) => ({ ...discoverCard(d, 'np_hero'), wide: d.backdrop ?? null }));
			heroAt = nextHero(hero.length);
			next = 1;
			more = a.more;
			failed = false;
			building = a.building && tries < 20 ? tries + 1 : 0;
			if (building) poll = setTimeout(() => void loadFirst(tries + 1), 3000);
		} catch {
			if (n === ask) failed = true;
		} finally {
			if (n === ask) loaded = true;
		}
	}

	async function loadMore() {
		if (busy || !more) return;
		busy = true;
		const n = ask;
		try {
			const a = await api.get<NewAnswer>(newPath(type, next), gone.signal);
			if (n !== ask) return;
			const have = new Set(rows.map((r) => r.id));
			rows = [...rows, ...a.rows.map(serverRow).filter((r) => !have.has(r.id))];
			next++;
			more = a.more;
		} catch {
			more = false;
		} finally {
			busy = false;
		}
		requestAnimationFrame(() => {
			if (more && sentinel && sentinel.getBoundingClientRect().top < innerHeight * 2.5) void loadMore();
		});
	}

	let sentinel = $state<HTMLElement>();
	$effect(() => {
		if (!sentinel) return;
		const io = new IntersectionObserver(([e]) => e.isIntersecting && void loadMore(), { rootMargin: '150% 0px' });
		io.observe(sentinel);
		return () => io.disconnect();
	});

	// Service chips on the services and hot and new rows: one service at a time, or all (per row).
	let svcPicks = $state(new Map<string, number | null>());
	const svcPick = (row: string) => svcPicks.get(row) ?? null;
	// Each chip loads its service's own list (row type:service to cards; null while loading).
	let svcLists = $state(new Map<string, ExploreCard[] | null>());
	let svcGen = 0;
	function setPick(row: Row, s: number | null) {
		svcPicks = new Map([...svcPicks, [row.id, s]]);
		if (s === null) return;
		const k = `${row.type}:${s}`;
		if (svcLists.has(k)) return;
		svcLists = new Map([...svcLists, [k, null]]);
		const n = svcGen;
		api.get<NewServiceAnswer>(newServicePath(type, s, row.type), gone.signal)
			.then((a) => {
				if (n === svcGen) svcLists = new Map([...svcLists, [k, serverRow(a.row).cards]]);
			})
			.catch(() => {
				if (n !== svcGen) return;
				// Fall back to the shared row's cards for that service.
				svcLists = new Map([...svcLists, [k, pickService(row.cards, s)]]);
			});
	}
	const ghosts = (n: number): ExploreCard[] => Array.from({ length: n }, (_, i) => ({ key: `ghost-${i}`, kind: 'movie', name: '', badges: [] }) as ExploreCard);
	const region = serviceRegion(defaultRegion());
	let services = $state<WatchService[]>([]);
	const logo = (id: number | undefined) => proxied(services.find((s) => s.id === id)?.logo) || undefined; // Through the artwork cache.

	let titles = $state<TitleView[]>([]);
	const owned = $derived(ownedIndex(titles));
	let flow: ReturnType<typeof QuickAsk> | undefined = $state();
	function live(c: ExploreCard): ExploreCard {
		return matchOwned(flow ? flow.live(c) : c, owned);
	}
	const quick = (c: ExploreCard, act: 'add' | 'ask') => void flow?.open(c, act);
	async function loadTitles() {
		try {
			titles = (await ownedLibrary()).titles;
		} catch {
			/* Owned marks wait for the Library. */
		}
	}

	function cardHref(c: ExploreCard): string | undefined {
		if (c.titleId !== undefined) return to(`/library/${c.titleId}`);
		const tmdb = c.ref?.startsWith('tmdb:') ? c.ref.slice(5) : '';
		if (tmdb && (c.kind === 'movie' || c.kind === 'show')) {
			remember(c.kind, tmdb, { name: c.name, year: c.year, poster: c.image });
			return to(detailPath(c.kind, tmdb));
		}
		return undefined;
	}
	function searchHref(c: ExploreCard): string {
		return to(`/search?q=${encodeURIComponent(c.name)}${c.kind === 'show' ? '&kind=show' : ''}`);
	}

	onMount(() => {
		void loadTitles();
		if (type !== 'music') void loadFirst();
		api.get<{ services: WatchService[] }>(`/watch-services?region=${region}`, gone.signal).then((r) => (services = r.services ?? [])).catch(() => {});
		return () => {
			clearTimeout(poll);
			gone.abort();
		};
	});

	const wideKind = (k: string): boolean => k === 'movie' || k === 'show';
	function shown(row: Row): ExploreCard[] {
		if (!hasServiceChips(row.type)) return row.cards;
		const s = svcPick(row.id);
		if (s === null) return isRanked(row.type) ? row.cards.slice(0, 10) : row.cards;
		const got = svcLists.get(`${row.type}:${s}`);
		if (!got) return ghosts(isRanked(row.type) ? 6 : 8);
		return isRanked(row.type) ? got.slice(0, 10) : got;
	}
	/** A rail title as a row card (the same card and hover preview as every row). */
	const railCard = (it: RailItem): ExploreCard => ({ ...discoverCard(it, 'np_rail'), wide: it.backdrop ?? null });

	/** Rotates the hero through its titles, one step a visit. */
	function nextHero(n: number): number {
		if (n < 2) return 0;
		try {
			const at = (Number(sessionStorage.getItem('np-hero-at') ?? '-1') + 1) % n;
			sessionStorage.setItem('np-hero-at', String(at));
			return at;
		} catch {
			return 0;
		}
	}
	/** New season, the title's place on its service's chart, else New this week. */
	function heroWhy(h: ExploreCard): Why[] {
		const out = whyShown({ kind: h.kind, tmdb: cardTmdb(h) ?? '', fresh: h.fresh });
		const rank = h.streaming?.rank;
		if (rank && rank <= 10 && h.streaming && !out.some((w) => w.kind === 'top10')) out.push({ kind: 'top10', text: t('discover.popular.rankedOn', { rank, name: h.streaming.name }) });
		if (!out.length && h.fresh && h.fresh.kind !== 'coming') out.unshift({ kind: 'new_season', text: t('discover.popular.newThisWeek') });
		return out.slice(0, 2);
	}
</script>

<svelte:head><title>{t('discover.popular.newPopularHolyArrs')}</title></svelte:head>

{#snippet wideCard(raw: ExploreCard, rowType: string, rowTitle: string)}
	{@const c = live(raw)}
	{@const act = cardAction(c, { manage, ask: mayAsk })}
	{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
	{@const preview = cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
	<div class="logo-wrap">
		<WideCard cover title={c.name} href={cardHref(c)} image={c.wide} poster={c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} badges={c.badges} badgesAt="bottom" vtName={c.vtName} fresh={c.fresh} owned={mark} typed row={rowType} {preview}
			growService={c.streaming ? { name: c.streaming.name, logo: logo(c.streaming.service) } : null}
			growContext={contextLine({ state: mark?.state, label: mark?.label, mine: c.mine, fresh: c.fresh }, { rowType, rowTitle, fresh: c.fresh, streaming: c.streaming })}
			followTmdb={followsByTmdb(c)}>
			{#snippet growAction()}{@render hoverAction(c, act, mark)}{/snippet}
		</WideCard>
		{#if (rowType === 'np_services' || rowType === 'np_rail') && c.streaming && logo(c.streaming.service)}
			<img class="svc-logo" src={logo(c.streaming.service)} alt={t('discover.popular.on', { name: c.streaming.name })} title={c.streaming.name} loading="lazy" />
		{/if}
	</div>
{/snippet}

{#snippet hoverAction(c: ExploreCard, act: 'add' | 'ask' | null, mark: ReturnType<typeof ownedMark>)}
	{#if act && cardTmdb(c)}
		<IconButton icon="plus" label={act === 'add' ? t('discover.popular.add', { name: c.name }) : t('discover.popular.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void quick(c, act)} />
	{/if}
{/snippet}

<div class="np">
	<h1 class="sr-only">{t('discover.popular.newPopular')}</h1>
	<div class="types-mark" bind:this={typesMark} aria-hidden="true"></div>
	<div class="types" class:stuck={typesStuck} data-pin-pills>
		<SegmentedControl options={typeOptions} value={type} label={t('discover.popular.show')} testid="new-type" announce={(l) => t('discover.popular.showing', { label: l })} onchange={setType} />
	</div>
	<MetadataNotice />

	{#if type === 'music'}
		<MusicNewTab sheets={artistSheets} {manage} {mayAsk} folders={flow?.addChoices().folders ?? []} rules={flow?.addChoices().rules ?? []} />
	{:else if failed && rows.length === 0}
		<LoadError hasData={false} message={t('discover.popular.couldNotLoadNew')} onretry={() => void loadFirst()} busy={false} />
	{:else}
		<!-- Main data loading: the shared page Orbit. "Building" answers show the try count. -->
		<PageLoader loading={!loaded || (building > 0 && rows.length === 0)} label={t('discover.popular.loadingNewPopular')} status={building ? buildingStatus(building, t('discover.popular.newPopularName')) : ''} immediate={building > 0} onretry={() => void loadFirst()}>
		{#if rows.length === 0}
		<EmptyState icon="compass" title={t('discover.popular.newPopularIsGetting')} text={t('discover.popular.rowsFillInWithin')} action={{ label: t('discover.popular.explore'), href: to('/explore'), icon: 'compass' }} />
		{:else}
		{#if hero.length}
			{@const h = live(hero[heroAt % hero.length])}
			<ExploreHero kind={h.kind === 'show' ? 'show' : 'movie'} tmdb={cardTmdb(h)} title={h.name} href={cardHref(h)} backdrop={h.wide ?? h.image} poster={h.image} vtName={h.vtName} subtitle={h.year ? (h.kind === 'show' ? t('discover.popular.yearShow', { year: h.year }) : t('discover.popular.yearMovie', { year: h.year })) : h.subtitle} settled={loaded}>
				{#snippet action()}{@render hoverAction(h, cardAction(h, { manage, ask: mayAsk }), null)}{/snippet}
				{#snippet badge()}
					{@const why = heroWhy(h)}
					{#if why.length}<ul class="why" aria-label={t('discover.popular.whyItIsFeatured')}>{#each why as w (w.text)}<li class="why-{w.kind}">{w.text}</li>{/each}</ul>{/if}
				{/snippet}
			</ExploreHero>
		{/if}
		{#if rail.length > 0}
			<NewRail days={rail}>
				{#snippet card(it: RailItem)}{@render wideCard(railCard(it), 'np_rail', t('discover.popular.thisWeek'))}{/snippet}
			</NewRail>
		{/if}
		{#each rows as row (row.id)}
			{@const cards = shown(row)}
			{@const wide = !isRanked(row.type) && row.cards.some((c) => wideKind(c.kind))}
			{@const chips = hasServiceChips(row.type) ? cardServices(row.cards) : []}
			<div class="row-wrap" class:wide-row={wide} data-row={row.type} data-row-id={row.id}>
				{#snippet tools()}
					<div class="chips" role="group" aria-label={t('discover.popular.services')}>
						<Chip label={t('discover.popular.all')} selected={svcPick(row.id) === null} onclick={() => setPick(row, null)} />
						{#each chips as s (s.service)}<Chip label={s.name} selected={svcPick(row.id) === s.service} onclick={() => setPick(row, s.service)} />{/each}
					</div>
				{/snippet}
				<Shelf title={row.title} subtitle={row.subtitle} tools={chips.length > 1 ? tools : undefined}
					item={isRanked(row.type) ? 'calc(var(--explore-item) / 0.68)' : wide ? 'var(--title-item)' : 'var(--explore-item)'}
					items={cards} key={(c) => c.key} placeholder={wide ? 'wide' : 'poster'} lines={cards.some((c) => c.subtitle) ? 2 : 1}>
					{#snippet card(raw: ExploreCard)}
						{#if raw.key.startsWith('ghost-')}
							<div class="ghost" class:pc-ghost={isRanked(row.type)}><Skeleton shape={wide ? 'wide' : 'poster'} /><Skeleton width="70%" /></div>
						{:else}
						{@const c = live(raw)}
						{@const act = cardAction(c, { manage, ask: mayAsk })}
						{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
						{@const preview = cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
						{#if isRanked(row.type)}
							{@const rank = cards.indexOf(raw) + 1}
							<div class="ranked">
								<span class="rank" class:two={rank > 9} aria-hidden="true">{rank}</span>
								<div class="pc"><PosterCard title={c.name} href={cardHref(c)} image={c.image} kind={c.kind} owned={mark} fresh={c.fresh} actionOnHover {preview} followTmdb={followsByTmdb(c)}>
									{#snippet action()}{@render hoverAction(c, act, mark)}{/snippet}
								</PosterCard></div>
								<span class="sr-only">{t('discover.popular.number', { rank })}</span>
							</div>
						{:else if wide}
							{@render wideCard(raw, row.type, row.title)}
						{:else}
							<PosterCard title={c.name} href={cardHref(c)} image={c.image} kind={c.kind} subtitle={c.subtitle} badges={c.badges} typed row={row.type} owned={mark} fresh={c.fresh} actionOnHover vtName={c.vtName} {preview} followTmdb={followsByTmdb(c)}>
								{#snippet action()}
									{#if act && cardTmdb(c)}<IconButton icon="plus" label={act === 'add' ? t('discover.popular.add', { name: c.name }) : t('discover.popular.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" onclick={() => void quick(c, act)} />{/if}
								{/snippet}
							</PosterCard>
						{/if}
						{/if}
					{/snippet}
				</Shelf>
			</div>
		{/each}
		{#if more}<div class="sentinel" bind:this={sentinel} aria-hidden="true"></div>{/if}
		{/if}
		</PageLoader>
	{/if}
</div>

<QuickAsk bind:this={flow} onadded={() => void loadTitles()} />

<style>
	/* The same sizes and type as Explore. */
	.np { container: explore / inline-size; --title-item: max(200px, calc((100vw - 96px) / 5.16)); --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.types-mark { height: 0; margin-top: -32px; }
	.types { position: sticky; top: calc(var(--topbar) + 10px); z-index: 20; display: flex; justify-content: center; margin-top: -12px; pointer-events: none; }
	.types :global(.ui-seg) { pointer-events: auto; border-radius: 999px; padding: 3px; transition: transform var(--bar-glide), box-shadow var(--bar-glide), background-color var(--bar-glide); transform-origin: top center; }
	.types.stuck :global(.ui-seg) { transform: scale(0.9); background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	/* Scaled to 0.9 while stuck: the segments' hit area grows to stay 44px on touch phones. */
	.types.stuck { --seg-hit: 49px; }
	.types :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 999px; }
	.types :global(.ui-seg button) { min-height: 38px; padding: 0 16px; border-radius: 999px; font-size: 14px; }
	@media (prefers-reduced-motion: reduce) { .types :global(.ui-seg), .types :global(.ui-seg .thumb) { transition: none; } }
	@media (max-width: 767px) { .types :global(.ui-seg) { display: grid; width: 100%; } .types :global(.ui-seg button) { padding: 0 8px; } }
	/* Phones: the words win, the glyphs step aside, so no label is clipped. */
	@media (min-width: 360px) and (max-width: 479px) { .types :global(.ui-seg button svg) { display: none; } }
	.row-wrap { display: contents; }
	@media (min-width: 768px) { .row-wrap.wide-row { --shelf-gap: 5px; } }
	.chips { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-inline: calc(var(--gutter) * -1); padding: 2px var(--gutter); }
	.chips::-webkit-scrollbar { display: none; }
	.chips :global(.ui-chip) { flex: none; }
	.logo-wrap { position: relative; }
	.svc-logo { position: absolute; top: 8px; left: 8px; width: 28px; height: 28px; border-radius: 7px; object-fit: cover; box-shadow: 0 2px 8px rgb(0 0 0 / 0.35); pointer-events: none; z-index: 2; }
	.ranked { position: relative; container-type: inline-size; }
	.rank { position: absolute; left: 0; top: 0; width: 37%; height: 102cqw; display: flex; align-items: flex-end; justify-content: flex-end; font-size: 50cqw; font-weight: 800; line-height: 0.76; letter-spacing: -0.06em; color: var(--bg, #000); -webkit-text-stroke: 3px var(--text-muted); paint-order: stroke; user-select: none; z-index: 0; }
	.rank.two { font-size: 36cqw; letter-spacing: -0.12em; }
	.pc { position: relative; z-index: 1; margin-left: 32%; width: 68%; min-width: 0; }
	.sentinel { height: 1px; }
	/* Ghost cards while a chip's list loads, the same size as the cards they stand for. */
	.ghost { display: flex; flex-direction: column; gap: 8px; }
	.ghost.pc-ghost { margin-left: 32%; width: 68%; }
	/* The rail's cards are as wide as a Top 10 card. */
	.np { --rail-item: calc(var(--explore-item) / 0.68); }
	/* Hero badges (as on Explore): small glass pills, the chart one in the brand red. */
	.why { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0 0; padding: 0; list-style: none; }
	.why li { font: 600 12px/1 var(--font-ui); letter-spacing: .02em; padding: 6px 10px; border-radius: 999px; color: #fff; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 22%); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); }
	.why li.why-top10 { background: color-mix(in srgb, var(--accent) 80%, transparent); border-color: transparent; }
	@media (max-width: 767px) { .np { --explore-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); gap: 26px; } }
</style>
