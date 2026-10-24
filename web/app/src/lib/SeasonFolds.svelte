<script lang="ts">
	import { api } from './api.ts';
	import { date } from './format.ts';
	import Icon from './Icon.svelte';
	import EpisodeList, { type EpisodeRow } from './EpisodeList.svelte';
	import Skeleton from './ui/Skeleton.svelte';
	import { newSeason, seasonName } from './shows.ts';
	import { animeEpisode, animePath, type AnimeView } from './anime.ts';

	// Seasons of a show that is not in the Library, one folded row each
	// (newest first, Specials last), the way request apps list them: the name,
	// the episode count and year; opening a row reads its episodes from TMDB.
	let { tmdb, seasons, fallback }: { tmdb: string; seasons: { number: number; episodes: number; year?: number }[]; fallback?: string | null } = $props();

	const ordered = $derived([...seasons].sort((a, b) => Number(a.number === 0) - Number(b.number === 0) || b.number - a.number));
	const fresh = $derived(newSeason(seasons));
	let open = $state<Record<number, boolean>>({});
	type Ep = { number: number; name?: string; overview?: string; runtime?: number; still?: string; air_day?: string };
	let eps = $state<Record<number, Ep[] | 'error'>>({});

	$effect(() => {
		void tmdb;
		open = {};
		eps = {};
	});

	function toggle(n: number) {
		open[n] = !open[n];
		if (open[n] && !eps[n]) read(n);
	}
	function read(n: number) {
		const id = tmdb;
		api.get<{ episodes?: Ep[] }>(`/metadata/show/${encodeURIComponent(id)}/seasons/${n}`)
			.then((r) => id === tmdb && (eps[n] = r.episodes ?? []))
			.catch(() => id === tmdb && (eps[n] = 'error'));
	}

	// Anime: absolute episode numbers beside the air date. Best effort.
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
	function rows(n: number): EpisodeRow[] {
		const list = eps[n];
		if (!Array.isArray(list)) return [];
		return list.map((e) => {
			const abs = anime ? animeEpisode(anime, n, e.number).absolute : undefined;
			const when = e.air_day ? date(e.air_day) : 'No air date yet';
			return { key: e.number, number: String(e.number), name: e.name || `Episode ${e.number}`, still: e.still, runtime: e.runtime, when: abs ? `${when} · Absolute ${abs}` : when, overview: e.overview };
		});
	}
</script>

{#if seasons.length}
	<section class="tp-section" aria-labelledby="h-seasons" data-testid="season-folds">
		<h2 id="h-seasons" class="tp-h2">Seasons</h2>
		<ul class="folds">
			{#each ordered as s (s.number)}
				{@const on = !!open[s.number]}
				<li class="fold" class:on>
					<button type="button" class="head" aria-expanded={on} aria-controls="season-{s.number}" onclick={() => toggle(s.number)}>
						<span class="name">{seasonName(s.number)}</span>
						<span class="count">{s.episodes} {s.episodes === 1 ? 'episode' : 'episodes'}</span>
						{#if fresh === s.number}<span class="new">New</span>{/if}
						<span class="end">
							{#if s.year}<span class="yr">{s.year}</span>{/if}
							<span class="chev"><Icon name="chevronDown" size={18} /></span>
						</span>
					</button>
					<div class="body" id="season-{s.number}" inert={!on}>
						<div class="in">
							{#if eps[s.number] === 'error'}
								<p class="small muted">The episodes of {seasonName(s.number)} could not be read; try again shortly.</p>
							{:else if eps[s.number]}
								<EpisodeList rows={rows(s.number)} {fallback} />
							{:else if on}
								<div class="sk" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton lines={2} />{/each}</div>
							{/if}
						</div>
					</div>
				</li>
			{/each}
		</ul>
	</section>
{/if}

<style>
	.folds { display: flex; flex-direction: column; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.fold { border-radius: var(--r-card); background: var(--surface-1); border: 1px solid var(--line); overflow: hidden; transition: background var(--dur) var(--ease-out); }
	.head { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 54px; padding: 10px 16px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.head:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.head:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.name { font: 600 17px/1.2 var(--font-ui); letter-spacing: -0.01em; }
	.count { padding: 3px 8px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: 600 12px/1 var(--font-ui); white-space: nowrap; }
	.new { padding: 3px 8px; border-radius: 999px; background: var(--accent-fill); color: var(--text-on-red, #fff); font: 700 11px/1 var(--font-ui); letter-spacing: .02em; }
	.end { display: inline-flex; align-items: center; gap: 12px; margin-left: auto; color: var(--text-muted); font: 500 14px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.chev { display: inline-flex; transition: transform 320ms cubic-bezier(0.34, 1.56, 0.64, 1); }
	.on .chev { transform: rotate(180deg); }
	/* The fold opens by its own height: grid rows from 0fr to 1fr. */
	.body { display: grid; grid-template-rows: 0fr; transition: grid-template-rows 340ms var(--ease-out); }
	.on .body { grid-template-rows: 1fr; }
	.in { min-height: 0; overflow: hidden; padding: 0 16px; opacity: 0; transition: opacity 240ms ease-out, padding 340ms var(--ease-out); }
	.on .in { padding: 4px 16px 14px; opacity: 1; transition-delay: 60ms; }
	.sk { display: flex; flex-direction: column; gap: 12px; }
	@media (prefers-reduced-motion: reduce) { .body, .in, .chev { transition: none; } }
	@media (max-width: 600px) { .head { padding: 10px 12px; } .on .in { padding: 4px 12px 12px; } .yr { display: none; } }
</style>
