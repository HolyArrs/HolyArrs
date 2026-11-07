<script lang="ts">
	import type { Snippet } from 'svelte';
	import { api } from '../core/api.ts';
	import { to } from '../core/paths.ts';
	import { session } from '../core/session.svelte.ts';
	import { readInfo, type InfoDetails, type InfoKind } from './title-info.ts';
	import { detailPath } from '../title-details.ts';
	import { discoverCard, followsByTmdb, ownedMark, serviceRegion, type ExploreCard, type ProviderItem } from '../explore.ts';
	import { defaultRegion } from './region.ts';
	import { audienceOf } from './title-status.ts';
	import type { FilmographyPage } from './people.ts';
	import type { Ask } from '../core/types.ts';
	import Shelf from './Shelf.svelte';
	import WideCard from './WideCard.svelte';
	import Sheet from './Sheet.svelte';
	import TitleReviews from './TitleReviews.svelte';
	import QuickAsk from '../QuickAsk.svelte';
	import IconButton from './IconButton.svelte';

	// The requester rows of a movie or show page, read from the details
	// answer (GET /metadata/{kind}/{tmdb}), in page order: Where to watch,
	// the cast (middle), Ratings & reviews, the Collection, More like this
	// and More from. The dates live in the page's About section. Every title row uses the
	// Explore wide card at one size (--title-item, title-page.css); owned and
	// asked titles carry their mark top-right. The current title never shows
	// in its own rows. Rows with nothing in them are left out.
	let {
		kind,
		tmdb,
		info = $bindable(null),
		middle,
		band = true,
		reviewsLast = false
	}: {
		kind: InfoKind;
		tmdb: string | null | undefined;
		/** The details answer, for the page (cast on a not-added title). */
		info?: InfoDetails | null;
		/** Between the band and the title rows (the cast). */
		middle?: Snippet;
		/** Where to stream as a band at the top (off when the page shows it elsewhere). */
		band?: boolean;
		/** Ratings & reviews after the title rows instead of before them. */
		reviewsLast?: boolean;
	} = $props();

	const region = serviceRegion(defaultRegion());
	$effect(() => {
		const k = kind;
		const t = tmdb;
		info = null;
		if (!t) return;
		const ctl = new AbortController();
		api.get<Record<string, unknown>>(`/metadata/${k}/${encodeURIComponent(t)}?region=${encodeURIComponent(region)}`, ctl.signal)
			.then((raw) => (info = readInfo(k, t, raw)))
			.catch(() => {
				/* The rows are extras: the page works without them. */
			});
		return () => ctl.abort();
	});

	// More from the director (movies) or the creator (shows): the server
	// answers at least four of their other titles, most popular first, or none.
	let moreFrom = $state<{ person: string; items: ProviderItem[] } | null>(null);
	$effect(() => {
		const k = kind;
		const t = tmdb;
		moreFrom = null;
		if (!t) return;
		const ctl = new AbortController();
		api.get<{ person?: string; items?: ProviderItem[] }>(`/metadata/${k}/${encodeURIComponent(t)}/more-from`, ctl.signal)
			.then((r) => (moreFrom = { person: r.person ?? '', items: r.items ?? [] }))
			.catch(() => {
				/* An extra row: hidden when it cannot be read. */
			});
		return () => ctl.abort();
	});

	// More with the lead actor: their best-known movies, from their person page's answer.
	const lead = $derived(info?.cast.find((p) => p.id) ?? null);
	let withLead = $state<ProviderItem[]>([]);
	$effect(() => {
		const id = lead?.id?.replace(/^tmdb:/, '');
		withLead = [];
		if (!id) return;
		const ctl = new AbortController();
		api.get<FilmographyPage>(`/people/${encodeURIComponent(id)}?role=actor&sort=popular&page_size=24&page=1`, ctl.signal)
			.then((r) => {
				const list = r.known_for?.length ? r.known_for : r.movies;
				withLead = list.filter((m) => m.ids?.tmdb).map((m) => ({ tmdb_id: m.ids.tmdb, kind: 'movie', name: m.name, year: m.year || undefined, poster: m.poster, backdrop: m.backdrop, overview: m.overview, in_library: !!m.title_id, title_id: m.title_id, ask: m.ask ? { ...m.ask, state: m.ask.state as Ask['state'] } : null }));
			})
			.catch(() => {
				/* An extra row: hidden when it cannot be read. */
			});
		return () => ctl.abort();
	});

	const who = $derived(audienceOf(session.can));
	type Pair = { c: ExploreCard; item: ProviderItem };
	const pairsOf = (items: ProviderItem[], row: string): Pair[] => items.filter((i) => i.tmdb_id !== tmdb || i.kind !== kind).map((item) => ({ c: discoverCard(item, row), item }));
	const collection = $derived(info?.collection ? pairsOf(info.collection.parts, 'collection') : []);
	const similar = $derived.by(() => {
		const seen = new Set(collection.map((p) => p.c.key));
		return info ? pairsOf(info.similar, 'similar').filter((p) => !seen.has(p.c.key)) : [];
	});
	const more = $derived(moreFrom && moreFrom.items.length >= 4 ? pairsOf(moreFrom.items, 'more-from') : []);
	const withLeadCards = $derived.by(() => {
		const seen = new Set(more.map((p) => p.c.key.replace(/^more-from-/, '')));
		return pairsOf(withLead, 'with-lead').filter((p) => !seen.has(p.c.key.replace(/^with-lead-/, '')));
	});
	const hrefOf = (item: ProviderItem) => (item.title_id ? to(`/library/${item.title_id}`) : to(detailPath(item.kind, item.tmdb_id)));
	let allSimilar = $state(false);
	/** The + on a card asks or adds as on Explore. */
	let flow: ReturnType<typeof QuickAsk> | undefined = $state();
</script>

{#snippet card({ c: raw, item }: Pair)}
	{@const c = flow ? flow.live(raw) : raw}
	{@const act = flow?.action(c) ?? null}
	<WideCard cover title={c.name} href={hrefOf(item)} image={c.wide} poster={c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} owned={ownedMark(c, undefined, who)} typed preview={{ kind: item.kind, tmdb: item.tmdb_id, year: item.year, titleId: c.titleId }} followTmdb={followsByTmdb(c)}>
		{#snippet growAction()}
			{#if act}<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={flow?.busy(c) || undefined} onclick={() => void flow?.open(c, act)} />{/if}
		{/snippet}
	</WideCard>
{/snippet}

{#if band && info?.streaming.length}
	<section class="tp-band" aria-labelledby="title-where-h" id="title-where">
		<div class="band-col">
			<h2 class="tp-h3" id="title-where-h">Where to stream</h2>
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
	<Shelf title="More like this" item="var(--title-item)" anchor="title-similar" placeholder="wide" lines={1} whole onseeall={similar.length > 6 ? () => (allSimilar = true) : undefined}>
		{#each similar.slice(0, 18) as p (p.c.key)}{@render card(p)}{/each}
	</Shelf>
	<Sheet bind:open={allSimilar} title="More like this" size="lg">
		<ul class="all-wide">
			{#each similar as p (p.c.key)}<li>{@render card(p)}</li>{/each}
		</ul>
	</Sheet>
{/if}

{#if more.length >= 4}
	<Shelf title={moreFrom?.person ? `More from ${moreFrom.person}` : kind === 'show' ? 'More from the creator' : 'More from the director'} subtitle={moreFrom?.person ? (kind === 'show' ? 'Creator' : 'Director') : undefined} item="var(--title-item)" anchor="title-more-from" placeholder="wide" lines={1} whole>
		{#each more.slice(0, 18) as p (p.c.key)}{@render card(p)}{/each}
	</Shelf>
{/if}

{#if lead && withLeadCards.length >= 4}
	<Shelf title="More with {lead.name}" subtitle={lead.character ? `Lead actor · plays ${lead.character}` : 'Lead actor'} item="var(--title-item)" anchor="title-more-with" placeholder="wide" lines={1} whole>
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
