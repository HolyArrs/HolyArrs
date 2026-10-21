<script lang="ts">
	import { api } from './api.ts';
	import { date } from './format.ts';
	import EpisodeList, { type EpisodeRow } from './EpisodeList.svelte';
	import SeasonPicker from './SeasonPicker.svelte';
	import Skeleton from './ui/Skeleton.svelte';
	import { newSeason, newestSeason, seasonName } from './shows.ts';
	import { animeEpisode, animePath, seasonAnimeLine, type AnimeView } from './anime.ts';

	// Episodes of a show that is not in the Library, from TMDB: the season
	// picker (newest season first, marked New while it airs) and the season's
	// episode list. Read only: nothing here is kept or looked for.
	let { tmdb, seasons, fallback }: { tmdb: string; seasons: { number: number; episodes: number; year?: number }[]; fallback?: string | null } = $props();

	let picked = $state<number | null>(null);
	const cur = $derived((picked !== null && seasons.some((s) => s.number === picked) ? picked : null) ?? newestSeason(seasons) ?? 0);
	const fresh = $derived(newSeason(seasons));
	type Ep = { number: number; name?: string; overview?: string; runtime?: number; still?: string; air_day?: string };
	let eps = $state<Record<number, Ep[] | 'error'>>({});
	$effect(() => {
		const n = cur;
		const id = tmdb;
		// No seasons yet (or a fallback 0 the show doesn't have): asking would 404.
		if (eps[n] || !seasons.some((s) => s.number === n)) return;
		const ctl = new AbortController();
		api.get<{ episodes?: Ep[] }>(`/metadata/show/${encodeURIComponent(id)}/seasons/${n}`, ctl.signal)
			.then((r) => (eps[n] = r.episodes ?? []))
			.catch(() => {
				if (!ctl.signal.aborted) eps[n] = 'error';
			});
		return () => ctl.abort();
	});
	// Anime: the AniList titles and how TMDB seasons map onto the cours
	// (absolute numbers, later cours inside one season). Best effort.
	let anime = $state<AnimeView | null>(null);
	$effect(() => {
		const id = tmdb;
		const ctl = new AbortController();
		anime = null;
		api.get<AnimeView>(animePath('show', id), ctl.signal)
			.then((v) => (anime = v.anime ? v : null))
			.catch(() => {});
		return () => ctl.abort();
	});
	const list = $derived(eps[cur]);
	const rows = $derived<EpisodeRow[]>(
		Array.isArray(list)
			? list.map((e) => {
				const abs = anime ? animeEpisode(anime, cur, e.number).absolute : undefined;
				const when = e.air_day ? date(e.air_day) : 'No air date yet';
				return { key: e.number, number: String(e.number), name: e.name || `Episode ${e.number}`, still: e.still, runtime: e.runtime, when: abs ? `${when} · Absolute ${abs}` : when, overview: e.overview };
			})
			: []
	);
	const curSeason = $derived(seasons.find((s) => s.number === cur));
	const animeLine = $derived(curSeason ? seasonAnimeLine(anime, cur, curSeason.episodes) : '');
</script>

{#if seasons.length}
	<section class="tp-section" aria-labelledby="h-seasons">
		<div class="tp-head">
			<h2 id="h-seasons" class="tp-h2">Episodes</h2>
			<div class="tp-head-end">
				<SeasonPicker seasons={seasons.map((s) => ({ number: s.number, name: seasonName(s.number) }))} value={cur} {fresh} onpick={(n) => (picked = n)} />
			</div>
		</div>
		{#if curSeason}<p class="xs muted" data-testid="season-line">{curSeason.episodes} {curSeason.episodes === 1 ? 'episode' : 'episodes'}{curSeason.year ? ` · ${curSeason.year}` : ''}</p>{/if}
		{#if anime?.alt_names.length}<p class="xs muted" data-testid="anime-names">Also known as {anime.alt_names.join(' · ')}</p>{/if}
		{#if animeLine}<p class="xs muted" data-testid="anime-season">{animeLine}</p>{/if}
		{#if list === 'error'}
			<p class="small muted">The episodes of {seasonName(cur)} could not be read; try again shortly.</p>
		{:else if list}
			<EpisodeList {rows} {fallback} />
		{:else}
			<div class="sk" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton lines={2} />{/each}</div>
		{/if}
	</section>
{/if}

<style>
	.sk { display: flex; flex-direction: column; gap: 12px; }
	[data-testid='season-line'], [data-testid='anime-names'], [data-testid='anime-season'] { margin: 0 0 4px; }
</style>
