<script lang="ts">
	// Season picker and lazy-loaded episode list for one show.
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import EpisodeList, { type EpisodeRow } from './EpisodeList.svelte';
	import SeasonPicker from './SeasonPicker.svelte';
	import { newSeason, newestSeason, seasonName } from './labels.ts';
	import { animeEpisode, animePath, seasonAnimeLine, type AnimeView } from './anime.ts';
	import { api } from '../core/api.ts';
	import { date } from '../core/format.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	type Ep = { number: number; name?: string; overview?: string; runtime?: number; still?: string; air_day?: string };

	let { tmdb, seasons, fallback }: { tmdb: string; seasons: { number: number; episodes: number; year?: number }[]; fallback?: string | null } = $props();

	let picked = $state<number | null>(null);
	let eps = $state<Record<number, Ep[] | 'error'>>({});
	// Best effort: the page works without anime data.
	let anime = $state<AnimeView | null>(null);

	// A pick that no longer exists falls back to the newest season.
	const cur = $derived((picked !== null && seasons.some((s) => s.number === picked) ? picked : null) ?? newestSeason(seasons) ?? 0);
	const fresh = $derived(newSeason(seasons));
	const list = $derived(eps[cur]);
	const rows = $derived<EpisodeRow[]>(
		Array.isArray(list)
			? list.map((e) => {
				const abs = anime ? animeEpisode(anime, cur, e.number).absolute : undefined;
				const when = e.air_day ? date(e.air_day) : t('shows.why.noAirDateShort');
				return { key: e.number, number: String(e.number), name: e.name || t('shows.list.episodeFallback', { n: e.number }), still: e.still, runtime: e.runtime, when: abs ? `${when} · ${t('shows.list.absolute', { n: abs })}` : when, overview: e.overview };
			})
			: []
	);
	const curSeason = $derived(seasons.find((s) => s.number === cur));
	const animeLine = $derived(curSeason ? seasonAnimeLine(anime, cur, curSeason.episodes) : '');

	// Each season loads once, on first view.
	$effect(() => {
		const n = cur;
		const id = tmdb;
		// Asking for a season the show lacks would 404.
		if (eps[n] || !seasons.some((s) => s.number === n)) return;
		const ctl = new AbortController();
		api.get<{ episodes?: Ep[] }>(`/metadata/show/${encodeURIComponent(id)}/seasons/${n}`, ctl.signal)
			.then((r) => (eps[n] = r.episodes ?? []))
			.catch(() => {
				if (!ctl.signal.aborted) eps[n] = 'error';
			});
		return () => ctl.abort();
	});
	$effect(() => {
		const id = tmdb;
		const ctl = new AbortController();
		anime = null;
		api.get<AnimeView>(animePath('show', id), ctl.signal)
			.then((v) => (anime = v.anime ? v : null))
			.catch(() => {});
		return () => ctl.abort();
	});
</script>

{#if seasons.length}
	<section class="tp-section" aria-labelledby="h-seasons">
		<div class="tp-head">
			<h2 id="h-seasons" class="tp-h2">{t('shows.list.episodes')}</h2>
			<div class="tp-head-end">
				<SeasonPicker seasons={seasons.map((s) => ({ number: s.number, name: seasonName(s.number) }))} value={cur} {fresh} onpick={(n) => (picked = n)} />
			</div>
		</div>
		{#if curSeason}<p class="xs muted" data-testid="season-line">{t(curSeason.episodes === 1 ? 'shows.list.countOne' : 'shows.list.countOther', { n: curSeason.episodes })}{curSeason.year ? ` · ${curSeason.year}` : ''}</p>{/if}
		{#if anime?.alt_names.length}<p class="xs muted" data-testid="anime-names">{t('shows.list.alsoKnown', { names: anime.alt_names.join(' · ') })}</p>{/if}
		{#if animeLine}<p class="xs muted" data-testid="anime-season">{animeLine}</p>{/if}
		{#if list === 'error'}
			<p class="small muted">{t('shows.list.readFailed', { season: seasonName(cur) })}</p>
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
