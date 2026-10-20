<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { to } from './paths.ts';
	import { detailPath, remember } from './title-details.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';
	import { defaultRegion, fetchTop10, top10Cards, type Top10Answer, type Top10Card, type Top10Kind } from './top10.ts';
	import { matchOwned, ownedMark, ownedTitle, type OwnedIndex } from './explore.ts';
	import type { Audience } from './title-status.ts';

	// "Top 10 on Netflix": Netflix's weekly list with big rank numbers. Hidden
	// when the metadata server has no list.
	const KEY = 'holyarrs.top10';
	/** lock: Explore's type switch picked Movies or Shows, so this row follows it and hides its own toggle. */
	/** owned and who: the Library, so the posters carry the same badges as the other rows; typePill shows the type pill (Explore's All). */
	/** initial: the list the Explore head answer brought (kind and region as saved here), so the first paint needs no request of its own. */
	let { lock, owned, who = 'staff', typePill = false, initial = null }: { lock?: Top10Kind; owned?: OwnedIndex; who?: Audience; typePill?: boolean; initial?: (Top10Answer & { kind?: Top10Kind }) | null } = $props();
	let own = $state<Top10Kind>('movie');
	const kind = $derived(lock ?? own);
	let region = $state('global');
	let regions = $state<string[]>([]);
	let cards = $state<Top10Card[]>([]);
	let week = $state('');
	let gone: AbortController | undefined;

	const names = typeof Intl !== 'undefined' && 'DisplayNames' in Intl ? new Intl.DisplayNames(undefined, { type: 'region' }) : null;
	function regionName(r: string): string {
		if (r === 'global') return 'Worldwide';
		try {
			return names?.of(r) ?? r;
		} catch {
			return r;
		}
	}

	async function load() {
		gone?.abort();
		gone = new AbortController();
		const signal = gone.signal;
		const given = initial && initial.items.length && initial.region === region && (initial.kind ?? kind) === kind ? initial : null;
		const a = given ?? (await fetchTop10(kind, region, undefined, signal));
		if (signal.aborted) return;
		cards = a ? top10Cards(a) : [];
		week = a?.week ?? '';
		if (a) {
			regions = a.regions;
			if (a.region !== region) region = a.region;
		}
	}

	function pick() {
		try {
			localStorage.setItem(KEY, JSON.stringify({ kind, region }));
		} catch {
			/* the choice is a convenience */
		}
		void load();
	}

	onMount(() => {
		region = defaultRegion();
		try {
			const saved = JSON.parse(localStorage.getItem(KEY) ?? 'null');
			if (saved?.kind === 'movie' || saved?.kind === 'show') own = saved.kind;
			if (typeof saved?.region === 'string') region = saved.region;
		} catch {
			/* nothing saved */
		}
		ready = true;
		return () => gone?.abort();
	});

	// Loads once mounted, and again when the type switch changes the kind.
	let ready = $state(false);
	$effect(() => {
		void lock;
		if (ready) untrack(() => void load());
	});

	function href(c: Top10Card): string | undefined {
		if (c.titleId !== undefined) return to(`/library/${c.titleId}`);
		const tmdb = c.ref?.slice(5) ?? '';
		if (!tmdb || c.kind === 'artist') return undefined;
		remember(c.kind, tmdb, { name: c.name, poster: c.image });
		return to(detailPath(c.kind, tmdb));
	}

	const weekText = $derived(week ? `Week ending ${new Date(`${week}T12:00:00Z`).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}` : '');
</script>

{#if cards.length}
	<section class="top10" data-testid="top10-row">
		<div class="pick">
			{#if !lock}
				<div class="seg" role="group" aria-label="Movies or shows">
					<button type="button" aria-pressed={kind === 'movie'} onclick={() => { own = 'movie'; pick(); }}>Movies</button>
					<button type="button" aria-pressed={kind === 'show'} onclick={() => { own = 'show'; pick(); }}>Shows</button>
				</div>
			{/if}
			{#if regions.length > 1}
				<select aria-label="Region" bind:value={region} onchange={pick}>
					{#each regions as r (r)}<option value={r}>{regionName(r)}</option>{/each}
				</select>
			{/if}
		</div>
		<!-- A new list starts at rank 1, not where the last one was scrolled to. -->
		{#key `${kind}|${region}|${week}`}
		<Shelf title="Top 10 on Netflix" subtitle={[regionName(region), weekText].filter(Boolean).join(' · ')} item="calc(var(--explore-item, 180px) / 0.68)">
			{#each cards as raw (raw.key)}
				{@const c = owned ? { ...matchOwned(raw, owned), rank: raw.rank } : raw}
				{@const mark = ownedMark(c, owned ? ownedTitle(c, owned) : undefined, who)}
				<div class="ranked">
					<span class="rank" class:two={c.rank > 9} aria-hidden="true">{c.rank}</span>
					<div class="pc"><PosterCard title={c.name} href={href(c)} image={c.image} kind={c.kind} subtitle={c.subtitle} typePill={typePill ? c.kind : undefined} owned={mark} fresh={c.fresh} top10Row preview={c.ref?.startsWith('tmdb:') && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: c.ref.slice(5) } : undefined} /></div>
					<span class="sr">Number {c.rank}</span>
				</div>
			{/each}
		</Shelf>
		{/key}
	</section>
{/if}

<style>
	.top10 { position: relative; }
	.pick { position: absolute; right: 0; top: 0; z-index: 1; display: flex; gap: 8px; align-items: center; }
	.seg { display: inline-flex; border-radius: 999px; background: var(--surface-2); padding: 2px; }
	.seg button { border: 0; background: transparent; color: var(--text-2, var(--text)); font: inherit; font-size: 13px; padding: 4px 12px; border-radius: 999px; cursor: pointer; }
	.seg button[aria-pressed='true'] { background: var(--surface, var(--bg)); color: var(--text); box-shadow: var(--shadow-card); }
	select { font: inherit; font-size: 13px; padding: 4px 8px; border-radius: 8px; border: 1px solid var(--border, transparent); background: var(--surface-2); color: var(--text); max-width: 160px; }
	/* The column is the poster (68%, the same width as the other rows' posters) plus the number (32%); the poster overlaps the number's right edge a little. */
	.ranked { position: relative; container-type: inline-size; }
	.rank { position: absolute; left: 0; top: 0; width: 37%; height: 102cqw; display: flex; align-items: flex-end; justify-content: flex-end; font-size: 50cqw; font-weight: 800; line-height: 0.76; letter-spacing: -0.06em; color: var(--bg, #000); -webkit-text-stroke: 3px var(--text-2, #9a9a9a); paint-order: stroke; user-select: none; z-index: 0; }
	.rank.two { font-size: 36cqw; letter-spacing: -0.12em; }
	.pc { position: relative; z-index: 1; margin-left: 32%; width: 68%; min-width: 0; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
	@media (max-width: 640px) {
		.pick { position: static; margin-bottom: 8px; }
	}
</style>
