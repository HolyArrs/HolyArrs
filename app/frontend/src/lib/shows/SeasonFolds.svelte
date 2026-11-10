<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// Season pills with a shelf of episode cards for the chosen season.
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import { runtimeWords } from '#lib/ui/TitleHero.svelte';
	import { newSeason, newestSeason, seasonName } from './labels.ts';
	import { animeEpisode, animePath, type AnimeView } from './anime.ts';
	import { api } from '../core/api.ts';
	import { date } from '../core/format.ts';
	import { sized } from '../core/tmdb-image.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	type Ep = { number: number; name?: string; overview?: string; runtime?: number; still?: string; air_day?: string };

	let { tmdb, seasons, fallback }: { tmdb: string; seasons: { number: number; episodes: number; year?: number }[]; fallback?: string | null } = $props();

	let picked = $state<number | null>(null);
	let eps = $state<Record<number, Ep[] | 'error'>>({});
	// Best effort: the page works without anime data.
	let anime = $state<AnimeView | null>(null);
	// A failed picture falls back to the placeholder.
	let stillState = $state<Record<string, 'ok' | 'bad'>>({});

	// Specials sort last, the rest newest first.
	const ordered = $derived([...seasons].sort((a, b) => Number(a.number === 0) - Number(b.number === 0) || b.number - a.number));
	const fresh = $derived(newSeason(seasons));
	const shown = $derived(picked ?? newestSeason(seasons) ?? null);
	const cur = $derived(shown === null ? undefined : seasons.find((s) => s.number === shown));

	// A new show drops the old pick and cached episodes.
	$effect(() => {
		void tmdb;
		picked = null;
		eps = {};
	});
	$effect(() => {
		if (shown !== null && !eps[shown]) read(shown);
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

	const asked = new Set<string>();
	// The set stops the effect from refetching a season while its request is in flight.
	function read(n: number) {
		const id = tmdb;
		if (asked.has(`${id}:${n}`)) return;
		asked.add(`${id}:${n}`);
		api.get<{ episodes?: Ep[] }>(`/metadata/show/${encodeURIComponent(id)}/seasons/${n}`)
			.then((r) => id === tmdb && (eps[n] = r.episodes ?? []))
			.catch(() => {
				asked.delete(`${id}:${n}`);
				if (id === tmdb) eps[n] = 'error';
			});
	}

	function cards(n: number) {
		const list = eps[n];
		if (!Array.isArray(list)) return [];
		return list.map((e) => {
			const abs = anime ? animeEpisode(anime, n, e.number).absolute : undefined;
			const when = e.air_day ? date(e.air_day) : t('shows.why.noAirDateShort');
			return { key: e.number, name: `${e.number}. ${e.name || t('shows.list.episodeFallback', { n: e.number })}`, still: e.still || fallback, line: [e.runtime ? runtimeWords(e.runtime) : '', abs ? `${when} · ${t('shows.list.absolute', { n: abs })}` : when].filter(Boolean).join(' · '), overview: e.overview };
		});
	}
</script>

{#if seasons.length}
	<section class="tp-section seasons" aria-labelledby="h-seasons" data-testid="season-row">
		<h2 id="h-seasons" class="tp-h2">{t('shows.folds.seasons')}</h2>
		<div class="pills" role="group" aria-label={t('shows.picker.season')}>
			{#each ordered as s (s.number)}
				<button type="button" class="pill" aria-pressed={shown === s.number} onclick={() => (picked = s.number)}>{seasonName(s.number)}{#if fresh === s.number}<span class="dot" aria-label={t('shows.picker.new')}></span>{/if}</button>
			{/each}
		</div>
		{#if cur}<p class="xs muted line">{t(cur.episodes === 1 ? 'shows.list.countOne' : 'shows.list.countOther', { n: cur.episodes })}{cur.year ? ` · ${cur.year}` : ''}</p>{/if}
		{#if shown !== null}
			{#if eps[shown] === 'error'}
				<p class="small muted">{t('shows.list.readFailed', { season: seasonName(shown) })}</p>
			{:else if Array.isArray(eps[shown])}
				{#key shown}
					<Shelf title={t('shows.folds.seasonEpisodes', { season: seasonName(shown) })} item="260px" placeholder="wide" whole>
						{#each cards(shown) as c (c.key)}
							<article class="ep">
								<span class="still">
									{#if !c.still || stillState[`${shown}:${c.key}`] !== 'ok'}<ArtPlaceholder name={!c.still || stillState[`${shown}:${c.key}`] === 'bad' ? c.name.replace(/^\d+\.\s*/, '') : ''} shape="wide" loading={!!c.still && !stillState[`${shown}:${c.key}`]} />{/if}
									{#if c.still && stillState[`${shown}:${c.key}`] !== 'bad'}<img src={sized(c.still, 'backdrop', 300)} alt="" loading="lazy" decoding="async" onload={() => (stillState[`${shown}:${c.key}`] = 'ok')} onerror={() => (stillState[`${shown}:${c.key}`] = 'bad')} />{/if}
								</span>
								<b class="ep-name">{c.name}</b>
								<span class="xs muted">{serverWords(c.line)}</span>
								{#if c.overview}<p class="ep-text">{c.overview}</p>{/if}
							</article>
						{:else}
							<p class="small muted">{t('shows.list.none')}</p>
						{/each}
					</Shelf>
				{/key}
			{:else}
				<div class="sk" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton lines={3} />{/each}</div>
			{/if}
		{/if}
	</section>
{/if}

<style>
	.pills { display: flex; gap: 6px; overflow-x: auto; scrollbar-width: none; padding-bottom: 2px; }
	.pills::-webkit-scrollbar { display: none; }
	.pill { flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 34px; padding: 4px 14px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--text); font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.pill[aria-pressed='true'] { background: var(--text); color: var(--bg); border-color: var(--text); }
	.pill:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Touch targets grow by min-height because the scrolling row would clip extra padding. */
	@media (max-width: 767px) and (pointer: coarse) { .pill { min-height: 44px; } }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--accent); }
	.line { margin: 10px 0 2px; }
	/* Hidden from view but kept for screen readers; the pills already name the row. */
	.seasons :global(.ui-shelf header) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
	.ep { display: flex; flex-direction: column; gap: 4px; min-width: 0; align-self: start; }
	.still { position: relative; display: block; aspect-ratio: 16 / 9; margin-bottom: 6px; border-radius: 10px; overflow: hidden; background: var(--surface-2); }
	.still img { position: relative; width: 100%; height: 100%; object-fit: cover; }
	.ep-name { font: 600 15px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ep-text { margin: 2px 0 0; color: var(--text-muted); font: 400 13.5px/1.4 var(--font-ui); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 3; line-clamp: 3; overflow: hidden; }
	.sk { display: flex; gap: 12px; margin-top: 12px; }
	.sk > :global(*) { flex: 1; }
</style>
