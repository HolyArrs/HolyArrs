<script lang="ts">
	import { untrack } from 'svelte';
	import { to } from './paths.ts';
	import { api } from './api.ts';
	import { detailPath, remember } from './title-details.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';
	import Chip from './ui/Chip.svelte';
	import {
		discoverCard,
		matchOwned,
		ownedMark,
		ownedTitle,
		topChips,
		topPath,
		topSource,
		type ExploreCard,
		type ExploreType,
		type Genre,
		type OwnedIndex,
		type TopAnswer,
		type TopChip,
		type WatchService
	} from './explore.ts';
	import type { Audience } from './title-status.ts';

	// "Top on your services": one ranked 1-10 row with a chip per service the
	// person has. Netflix's chip is Netflix's weekly Top 10; the others (and
	// All my services) are TMDB's most popular this week on them.
	let {
		type,
		picked,
		used,
		shown,
		region,
		genre = null,
		owned,
		who = 'staff',
		ontoggle
	}: {
		type: ExploreType;
		/** The services picked on this device; null until the person picks once. */
		picked: number[] | null;
		/** The services the row is filled from: the picked ones, else the region's top few. */
		used: number[];
		/** The picker's services (the region's best known plus any picked). */
		shown: WatchService[];
		region: string;
		genre?: Genre | null;
		owned?: OwnedIndex;
		who?: Audience;
		ontoggle: (id: number) => void;
	} = $props();

	type Ranked = ExploreCard & { rank: number };
	let chip = $state<TopChip>('all');
	let answer = $state<TopAnswer | null>(null);
	let loading = $state(true);
	let editing = $state(false);
	const mine = $derived(used);
	const chips = $derived(topChips(mine, shown));
	const cards = $derived<Ranked[]>(
		(answer?.items ?? []).map((d) => ({ ...discoverCard(d, 'top_services'), key: `top-${d.kind}-${d.tmdb_id}`, rank: d.rank }))
	);
	const chipName = $derived(chip === 'all' ? 'your services' : (chips.find((c) => c.id === chip)?.name ?? 'this service'));
	const source = $derived(answer && answer.items.length ? topSource(answer, chipName) : '');
	const title = $derived(genre ? `Top ${genre.name.toLowerCase()} on your services` : 'Top on your services');

	/** Answers already read this visit, per chip, type and genre. */
	const memo = new Map<string, TopAnswer>();
	let gone: AbortController | undefined;
	async function load() {
		const q = { type, chip, services: mine, region, genre: genre?.id };
		const key = topPath(q);
		gone?.abort();
		const hit = memo.get(key);
		if (hit) {
			answer = hit;
			loading = false;
			return;
		}
		gone = new AbortController();
		loading = true;
		try {
			const a = await api.get<TopAnswer>(key, gone.signal);
			memo.set(key, a);
			answer = a;
		} catch {
			if (!gone.signal.aborted) answer = null;
		} finally {
			if (!gone.signal.aborted) loading = false;
		}
	}

	// A chip whose service was unpicked falls back to All my services.
	$effect(() => {
		if (chip !== 'all' && !mine.includes(chip)) chip = 'all';
	});
	$effect(() => {
		void [type, chip, mine.join('|'), region, genre?.id];
		untrack(() => void load());
		return () => gone?.abort();
	});

	function href(c: Ranked): string | undefined {
		if (c.titleId !== undefined) return to(`/library/${c.titleId}`);
		const tmdb = c.ref?.slice(5) ?? '';
		if (!tmdb || c.kind === 'artist') return undefined;
		remember(c.kind, tmdb, { name: c.name, poster: c.image });
		return to(detailPath(c.kind, tmdb));
	}
	const empty = $derived(!loading && cards.length === 0);
</script>

{#snippet tools()}
	<div class="top-tools">
		<div class="chips" role="group" aria-label="Services">
			{#each chips as c (c.id)}
				<Chip label={c.name} selected={chip === c.id} onclick={() => (chip = c.id)} />
			{/each}
			<Chip label={mine.length ? 'Edit services' : 'Pick your services'} icon={mine.length ? 'tools' : 'plus'} selected={editing ? true : undefined} onclick={() => (editing = !editing)} />
		</div>
		{#if editing || picked === null}
			<div class="svc" data-testid="services-picker" role="group" aria-label="Your streaming services">
				{#each shown as sv (sv.id)}
					{@const on = mine.includes(sv.id)}
					<button type="button" class="svc-chip" class:on aria-pressed={on} onclick={() => ontoggle(sv.id)} title={sv.name}>
						{#if sv.logo}<img src={sv.logo} alt="" width="28" height="28" loading="lazy" />{/if}
						<span>{sv.name}</span>
					</button>
				{/each}
			</div>
		{/if}
		{#if empty}<p class="hint">{mine.length ? 'Nothing ranked here this week.' : 'Pick the services you have to fill this row.'}</p>{/if}
	</div>
{/snippet}

<section class="top" data-testid="top-services-row" data-row="top_services">
	<!-- A new list starts at rank 1, not where the last one was scrolled to. -->
	{#key `${chip}|${type}|${genre?.id ?? ''}|${answer?.week ?? ''}`}
		<Shelf {title} subtitle={source || undefined} {tools} item="calc(var(--explore-item, 180px) / 0.68)" items={cards} key={(c) => c.key} placeholder="poster">
			{#snippet card(raw: Ranked)}
				{@const c = owned ? { ...matchOwned(raw, owned), rank: raw.rank } : raw}
				{@const mark = ownedMark(c, owned ? ownedTitle(c, owned) : undefined, who)}
				<div class="ranked">
					<span class="rank" class:two={c.rank > 9} aria-hidden="true">{c.rank}</span>
					<div class="pc"><PosterCard title={c.name} href={href(c)} image={c.image} kind={c.kind} owned={mark} fresh={c.fresh} top10Row preview={c.ref?.startsWith('tmdb:') && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: c.ref.slice(5) } : undefined} /></div>
					<span class="sr">Number {c.rank}</span>
				</div>
			{/snippet}
		</Shelf>
	{/key}
</section>

<style>
	.top { position: relative; }
	.top-tools { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.chips, .svc { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-inline: calc(var(--gutter) * -1); padding: 2px var(--gutter); }
	.chips::-webkit-scrollbar, .svc::-webkit-scrollbar { display: none; }
	.chips :global(.ui-chip) { flex: none; }
	.hint { color: var(--text-muted); font: var(--t-callout); }
	.svc-chip { flex: none; display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 14px 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-callout); font-weight: 500; box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent); transition: background-color var(--dur-fast) ease, box-shadow var(--dur-fast) ease; }
	.svc-chip img { width: 28px; height: 28px; border-radius: 7px; object-fit: cover; }
	.svc-chip:hover { background: color-mix(in srgb, var(--text) 10%, var(--surface-2)); }
	.svc-chip.on { background: color-mix(in srgb, var(--accent) 18%, var(--surface-2)); box-shadow: inset 0 0 0 2px var(--accent); }
	.svc-chip:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* The column is the poster (68%, the same width as the other rows' posters) plus the number (32%); the poster overlaps the number's right edge a little. */
	.ranked { position: relative; container-type: inline-size; }
	.rank { position: absolute; left: 0; top: 0; width: 37%; height: 102cqw; display: flex; align-items: flex-end; justify-content: flex-end; font-size: 50cqw; font-weight: 800; line-height: 0.76; letter-spacing: -0.06em; color: var(--bg, #000); -webkit-text-stroke: 3px var(--text-2, #9a9a9a); paint-order: stroke; user-select: none; z-index: 0; }
	.rank.two { font-size: 36cqw; letter-spacing: -0.12em; }
	.pc { position: relative; z-index: 1; margin-left: 32%; width: 68%; min-width: 0; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
</style>
