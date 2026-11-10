<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import { discoverCard, matchOwned, ownedMark, followsByTmdb, ownedTitle, type ExploreCard, type OwnedIndex, cardIdentity } from '#lib/ui/title-cards.ts';
	import type { Audience } from '#lib/ui/title-status.ts';
	import { serviceChips, serviceLabel, serviceTopPath, topPath, type ServiceTopAnswer, type ServiceTopRow, type TopAnswer } from './explore-services.ts';
	import { type ExploreType } from './explore-type.ts';
	import { type Genre } from './genres.ts';
	import { detailPath, remember } from '../metadata/index.ts';
	import { untrack, type Snippet } from 'svelte';
	import { to } from '../core/paths.ts';
	import { api } from '../core/api.ts';

	// "Top on your services": one row with one chip per streaming service, one picked at a time. Every service's list comes in one answer, so switching chips never waits on the network.
	let {
		type,
		used,
		region,
		genre = null,
		owned,
		who = 'staff',
		shown = $bindable([]),
		seeAll,
		ask
	}: {
		type: ExploreType;
		/** The person's services, first among the chips. */
		used: number[];
		region: string;
		genre?: Genre | null;
		owned?: OwnedIndex;
		who?: Audience;
		/** The first chip's titles, so the rows below can skip them; switching chips never moves them. */
		shown?: string[];
		/** Where the row's See all goes (New & Popular). */
		seeAll?: string;
		/** The page's Ask or Add button for a card, the same one its other rows use. */
		ask?: Snippet<[ExploreCard]>;
	} = $props();

	type Ranked = ExploreCard & { rank: number };
	let answer = $state<ServiceTopAnswer | null>(null);
	let picked = $state<number | null>(null);
	let genreAnswer = $state<TopAnswer | null>(null);
	let loading = $state(true);
	const chips = $derived(serviceChips(answer?.services ?? [], used));
	const row = $derived<ServiceTopRow | undefined>(chips.find((c) => c.service === picked) ?? chips[0]);
	const ranked = $derived(!genre && !!row?.ranked);
	const items = $derived(genre ? (genreAnswer?.items ?? []) : (row?.items ?? []));
	const cards = $derived<Ranked[]>(items.map((d) => ({ ...discoverCard(d, 'top_services'), key: `top-${d.kind}-${d.tmdb_id}`, rank: d.rank })));
	const firstItems = $derived(genre ? (genreAnswer?.items ?? []) : (chips[0]?.items ?? []));
	$effect(() => {
		shown = firstItems.map((d) => cardIdentity(discoverCard(d, 'top_services')));
	});
	const source = $derived(!row ? '' : genre ? t('discover.topServicesRow.popularInGenre', { genre: genre.name.toLowerCase(), name: row.name }) : serviceLabel(row));
	const title = $derived(genre ? t('discover.topServicesRow.trendingGenre', { genre: genre.name.toLowerCase() }) : type === 'movies' ? t('discover.topServicesRow.trendingMovies') : type === 'shows' ? t('discover.topServicesRow.trendingShows') : t('discover.topServicesRow.trending'));

	let gone: AbortController | undefined;
	async function load() {
		gone?.abort();
		const mine = (gone = new AbortController());
		loading = true;
		try {
			answer = await api.get<ServiceTopAnswer>(serviceTopPath(type, region), mine.signal);
		} catch {
			if (!mine.signal.aborted) answer = null;
		} finally {
			if (!mine.signal.aborted) loading = false;
		}
	}
	$effect(() => {
		void [type, region];
		untrack(() => void load());
		return () => gone?.abort();
	});

	/** Genre lists already read this visit, per service. */
	const memo = new Map<string, TopAnswer>();
	let genreGone: AbortController | undefined;
	$effect(() => {
		const id = row?.service;
		const g = genre;
		void [type, region];
		if (!g || id === undefined) return;
		const path = topPath({ type, chip: id, services: [id], region, genre: g.id });
		const hit = memo.get(path);
		if (hit) {
			genreAnswer = hit;
			return;
		}
		untrack(() => {
			genreGone?.abort();
			const mine = (genreGone = new AbortController());
			api.get<TopAnswer>(path, mine.signal).then(
				(a) => {
					memo.set(path, a);
					if (!mine.signal.aborted) genreAnswer = a;
				},
				() => {
					if (!mine.signal.aborted) genreAnswer = null;
				}
			);
		});
	});

	function href(c: Ranked): string | undefined {
		if (c.titleId !== undefined) return to(`/library/${c.titleId}`);
		const tmdb = c.ref?.slice(5) ?? '';
		if (!tmdb || c.kind === 'artist') return undefined;
		remember(c.kind, tmdb, { name: c.name, poster: c.image });
		return to(detailPath(c.kind, tmdb));
	}
</script>

{#snippet tools()}
	<div class="top-tools">
		<div class="chips" role="group" aria-label={t('discover.topServicesRow.services')}>
			{#each chips as c (c.service)}
				<Chip label={c.name} selected={row?.service === c.service} onclick={() => (picked = c.service)} />
			{/each}
		</div>
	</div>
{/snippet}

<!-- With no list at all (a cold metadata server) the row is left out, never shown empty. -->
{#if loading || chips.length > 0}
	<section class="top" data-testid="top-services-row" data-row="top_services">
		<!-- A new list starts at rank 1, not where the last one was scrolled to. -->
		{#key `${row?.service ?? ''}|${type}|${genre?.id ?? ''}`}
			<!-- While loading, the source line and chips keep their space so nothing below moves. -->
			<Shelf {title} subtitle={source || (loading ? ' ' : undefined)} {tools} href={seeAll} item={ranked ? 'calc(var(--explore-item, 180px) / 0.68)' : 'var(--explore-item, 180px)'} items={cards} key={(c) => c.key} placeholder="poster">
				{#snippet card(raw: Ranked)}
					{@const c = owned ? { ...matchOwned(raw, owned), rank: raw.rank } : raw}
					{@const mark = ownedMark(c, owned ? ownedTitle(c, owned) : undefined, who)}
					{@const preview = c.ref?.startsWith('tmdb:') && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: c.ref.slice(5), year: c.year, titleId: c.titleId } : undefined}
					{#snippet cardAsk()}{@render ask?.(c)}{/snippet}
					{#if ranked}
						<div class="ranked">
							<span class="rank" class:two={c.rank > 9} aria-hidden="true">{c.rank}</span>
							<div class="pc"><PosterCard title={c.name} href={href(c)} image={c.image ?? c.art?.art ?? null} kind={c.kind} owned={mark} fresh={c.fresh} {preview} action={ask ? cardAsk : undefined} actionOnHover followTmdb={followsByTmdb(c)} /></div>
							<span class="sr">{t('discover.topServicesRow.number', { rank: c.rank })}</span>
						</div>
					{:else}
						<PosterCard title={c.name} href={href(c)} image={c.image ?? c.art?.art ?? null} kind={c.kind} owned={mark} fresh={c.fresh} {preview} action={ask ? cardAsk : undefined} actionOnHover followTmdb={followsByTmdb(c)} />
					{/if}
				{/snippet}
			</Shelf>
		{/key}
	</section>
{/if}

<style>
	.top { position: relative; }
	.top-tools { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.chips { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-inline: calc(var(--gutter) * -1); padding: 2px var(--gutter); }
	.chips { min-height: 36px; }
	@media (pointer: coarse) { .chips { min-height: 48px; } }
	.chips::-webkit-scrollbar { display: none; }
	.chips :global(.ui-chip) { flex: none; }
	/* The column is the poster (68%, as wide as other rows' posters) plus the number (32%); the poster overlaps the number's right edge a little. */
	.ranked { position: relative; container-type: inline-size; }
	.rank { position: absolute; left: 0; top: 0; width: 37%; height: 102cqw; display: flex; align-items: flex-end; justify-content: flex-end; font-size: 50cqw; font-weight: 800; line-height: 0.76; letter-spacing: -0.06em; color: var(--bg, #000); -webkit-text-stroke: 3px var(--text-muted); paint-order: stroke; user-select: none; z-index: 0; }
	.rank.two { font-size: 36cqw; letter-spacing: -0.12em; }
	.pc { position: relative; z-index: 1; margin-left: 32%; width: 68%; min-width: 0; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
</style>
