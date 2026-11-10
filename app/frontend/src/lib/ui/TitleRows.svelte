<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Shelves under a title: where to stream, reviews, collection, similar and more-from rows.
	import type { Snippet } from 'svelte';
	import IconButton from './IconButton.svelte';
	import Sheet from './Sheet.svelte';
	import Shelf from './Shelf.svelte';
	import TitleReviews from './TitleReviews.svelte';
	import WideCard from './WideCard.svelte';
	import { defaultRegion, serviceRegion } from './region.ts';
	import { discoverCard, followsByTmdb, ownedMark, type ExploreCard, type ProviderItem, type QuickAskSheet } from './title-cards.ts';
	import { readInfo, type InfoDetails, type InfoKind } from './title-info.ts';
	import { audienceOf } from './title-status.ts';
	import type { FilmographyPage } from './people.ts';
	import { api } from '../core/api.ts';
	import { detailPath, to } from '../core/paths.ts';
	import { session } from '../core/session.svelte.ts';
	import type { Ask } from '../core/types.ts';

	let {
		kind,
		tmdb,
		info = $bindable(null),
		middle,
		band = true,
		reviewsLast = false,
		quickAsk: QuickAsk
	}: {
		kind: InfoKind;
		tmdb: string | null | undefined;
		info?: InfoDetails | null;
		middle?: Snippet;
		band?: boolean;
		reviewsLast?: boolean;
		quickAsk: QuickAskSheet;
	} = $props();

	type Pair = { c: ExploreCard; item: ProviderItem };

	const region = serviceRegion(defaultRegion());

	let moreFrom = $state<{ person: string; items: ProviderItem[] } | null>(null);
	let withLead = $state<ProviderItem[]>([]);
	let allSimilar = $state(false);
	let flow: ReturnType<QuickAskSheet> | undefined = $state();

	const lead = $derived(info?.cast.find((p) => p.id) ?? null);
	const who = $derived(audienceOf(session.can));
	const collection = $derived(info?.collection ? pairsOf(info.collection.parts, 'collection') : []);
	// Titles already in the collection row are dropped so nothing shows twice.
	const similar = $derived.by(() => {
		const seen = new Set(collection.map((p) => p.c.key));
		return info ? pairsOf(info.similar, 'similar').filter((p) => !seen.has(p.c.key)) : [];
	});
	// A row with fewer than four cards looks broken, so it stays hidden.
	const more = $derived(moreFrom && moreFrom.items.length >= 4 ? pairsOf(moreFrom.items, 'more-from') : []);
	const withLeadCards = $derived.by(() => {
		const seen = new Set(more.map((p) => p.c.key.replace(/^more-from-/, '')));
		return pairsOf(withLead, 'with-lead').filter((p) => !seen.has(p.c.key.replace(/^with-lead-/, '')));
	});

	// Details load again whenever the title changes; the old request is cancelled.
	$effect(() => {
		const k = kind;
		const t = tmdb;
		info = null;
		if (!t) return;
		const ctl = new AbortController();
		api.get<Record<string, unknown>>(`/metadata/${k}/${encodeURIComponent(t)}?region=${encodeURIComponent(region)}`, ctl.signal)
			.then((raw) => (info = readInfo(k, t, raw)))
			.catch(() => {
			});
		return () => ctl.abort();
	});
	$effect(() => {
		const k = kind;
		const t = tmdb;
		moreFrom = null;
		if (!t) return;
		const ctl = new AbortController();
		api.get<{ person?: string; items?: ProviderItem[] }>(`/metadata/${k}/${encodeURIComponent(t)}/more-from`, ctl.signal)
			.then((r) => (moreFrom = { person: r.person ?? '', items: r.items ?? [] }))
			.catch(() => {
			});
		return () => ctl.abort();
	});
	$effect(() => {
		const id = lead?.id?.replace(/^tmdb:/, '');
		withLead = [];
		if (!id) return;
		const ctl = new AbortController();
		api.get<FilmographyPage>(`/people/${encodeURIComponent(id)}?role=actor&sort=popular&page_size=24&page=1`, ctl.signal)
			.then((r) => {
				// Known-for is the better list; fall back to all movies when it is empty.
				const list = r.known_for?.length ? r.known_for : r.movies;
				withLead = list.filter((m) => m.ids?.tmdb).map((m) => ({ tmdb_id: m.ids.tmdb, kind: 'movie', name: m.name, year: m.year || undefined, poster: m.poster, backdrop: m.backdrop, overview: m.overview, in_library: !!m.title_id, title_id: m.title_id, ask: m.ask ? { ...m.ask, state: m.ask.state as Ask['state'] } : null }));
			})
			.catch(() => {
			});
		return () => ctl.abort();
	});

	// Leaves out the current title itself.
	function pairsOf(items: ProviderItem[], row: string): Pair[] {
		return items.filter((i) => i.tmdb_id !== tmdb || i.kind !== kind).map((item) => ({ c: discoverCard(item, row), item }));
	}
	// Library titles open the library page, others open the detail page.
	const hrefOf = (item: ProviderItem) => (item.title_id ? to(`/library/${item.title_id}`) : to(detailPath(item.kind, item.tmdb_id)));
</script>

{#snippet card({ c: raw, item }: Pair)}
	{@const c = flow ? flow.live(raw) : raw}
	{@const act = flow?.action(c) ?? null}
	<WideCard cover title={c.name} href={hrefOf(item)} image={c.wide} poster={c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} owned={ownedMark(c, undefined, who)} typed preview={{ kind: item.kind, tmdb: item.tmdb_id, year: item.year, titleId: c.titleId }} followTmdb={followsByTmdb(c)}>
		{#snippet growAction()}
			{#if act}<IconButton icon="plus" label={act === 'add' ? t('ui.titleRows.add', { name: c.name }) : t('ui.titleRows.askFor', { name: c.name })} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void flow?.open(c, act)} />{/if}
		{/snippet}
	</WideCard>
{/snippet}

{#if band && info?.streaming.length}
	<section class="tp-band" aria-labelledby="title-where-h" id="title-where">
		<div class="band-col">
			<h2 class="tp-h3" id="title-where-h">{t('ui.titleRows.whereToStream')}</h2>
			<ul class="stream">
				{#each info.streaming as sv (sv.id)}
					<li title={sv.name}>{#if sv.logo}<img src={sv.logo} alt="" width="32" height="32" loading="lazy" />{/if}<span>{sv.name}</span></li>
				{/each}
			</ul>
		</div>
	</section>
{/if}

{#if middle}{@render middle()}{/if}

{#if !reviewsLast}<TitleReviews {kind} {tmdb} />{/if}

{#if info?.collection && collection.length}
	<Shelf title={info.collection.name} item="var(--title-item)" anchor="title-collection" placeholder="wide" lines={1} whole>
		{#each collection as p (p.c.key)}{@render card(p)}{/each}
	</Shelf>
{/if}

{#if similar.length}
	<Shelf title={t('ui.titleRows.moreLikeThis')} item="var(--title-item)" anchor="title-similar" placeholder="wide" lines={1} whole onseeall={similar.length > 6 ? () => (allSimilar = true) : undefined}>
		{#each similar.slice(0, 18) as p (p.c.key)}{@render card(p)}{/each}
	</Shelf>
	<Sheet bind:open={allSimilar} title={t('ui.titleRows.moreLikeThis')} size="lg">
		<ul class="all-wide">
			{#each similar as p (p.c.key)}<li>{@render card(p)}</li>{/each}
		</ul>
	</Sheet>
{/if}

{#if more.length >= 4}
	<Shelf title={moreFrom?.person ? t('ui.titleRows.moreFrom', { name: moreFrom.person }) : kind === 'show' ? t('ui.titleRows.moreFromCreator') : t('ui.titleRows.moreFromDirector')} subtitle={moreFrom?.person ? (kind === 'show' ? t('ui.titleRows.creator') : t('ui.titleRows.director')) : undefined} item="var(--title-item)" anchor="title-more-from" placeholder="wide" lines={1} whole>
		{#each more.slice(0, 18) as p (p.c.key)}{@render card(p)}{/each}
	</Shelf>
{/if}

{#if lead && withLeadCards.length >= 4}
	<Shelf title={t('ui.titleRows.moreWith', { name: lead.name })} subtitle={lead.character ? t('ui.titleRows.leadActorPlays', { character: lead.character }) : t('ui.titleRows.leadActor')} item="var(--title-item)" anchor="title-more-with" placeholder="wide" lines={1} whole>
		{#each withLeadCards.slice(0, 18) as p (p.c.key)}{@render card(p)}{/each}
	</Shelf>
{/if}

{#if reviewsLast}<TitleReviews {kind} {tmdb} />{/if}

<QuickAsk bind:this={flow} />

<style>
	.stream { display: flex; flex-wrap: wrap; gap: 10px; margin: 0 0 6px; padding: 0; list-style: none; }
	.stream li { display: inline-flex; align-items: center; gap: 8px; min-height: 40px; padding: 4px 12px 4px 4px; border-radius: 999px; background: var(--surface-2); font: var(--t-callout); font-weight: 500; }
	.stream img { width: 32px; height: 32px; border-radius: 8px; object-fit: cover; }
	.all-wide { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 18px 14px; margin: 0; padding: 0; list-style: none; }
</style>
