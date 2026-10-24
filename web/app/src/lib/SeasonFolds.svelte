<script lang="ts" module>
	/** How the seasons are laid out while Luis compares them: tiles in a grid, pills over a row of episodes, or folds in two columns. */
	export type SeasonLayout = 'grid' | 'row' | 'cols';
</script>

<script lang="ts">
	import { api } from './api.ts';
	import { date } from './format.ts';
	import Icon from './Icon.svelte';
	import EpisodeList, { type EpisodeRow } from './EpisodeList.svelte';
	import Skeleton from './ui/Skeleton.svelte';
	import Shelf from './ui/Shelf.svelte';
	import { sized } from './tmdb-image.ts';
	import { runtimeWords } from './ui/TitleHero.svelte';
	import { newSeason, newestSeason, seasonName } from './shows.ts';
	import { animeEpisode, animePath, type AnimeView } from './anime.ts';

	// Seasons of a show that is not in the Library (newest first, Specials
	// last), read only. Opening a season reads its episodes from the server.
	let {
		tmdb,
		seasons,
		fallback,
		layout = 'grid'
	}: { tmdb: string; seasons: { number: number; episodes: number; year?: number }[]; fallback?: string | null; layout?: SeasonLayout } = $props();

	const ordered = $derived([...seasons].sort((a, b) => Number(a.number === 0) - Number(b.number === 0) || b.number - a.number));
	const fresh = $derived(newSeason(seasons));
	/** Folds (two columns): any number open. */
	let open = $state<Record<number, boolean>>({});
	/** Grid and row: the one season shown. */
	let one = $state<number | null>(null);
	type Ep = { number: number; name?: string; overview?: string; runtime?: number; still?: string; air_day?: string };
	let eps = $state<Record<number, Ep[] | 'error'>>({});

	$effect(() => {
		void tmdb;
		open = {};
		one = null;
		eps = {};
	});
	// The row layout always shows a season: the newest, until another is picked.
	const shown = $derived(layout === 'row' ? (one ?? newestSeason(seasons) ?? null) : one);
	$effect(() => {
		if (shown !== null && !eps[shown]) read(shown);
	});

	function toggle(n: number) {
		open[n] = !open[n];
		if (open[n] && !eps[n]) read(n);
	}
	function pick(n: number) {
		one = one === n && layout === 'grid' ? null : n;
	}
	const asked = new Set<string>();
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
	const count = (n: number) => `${n} ${n === 1 ? 'episode' : 'episodes'}`;

	// Grid: the episodes open in a panel under the row of tiles the season is in.
	let grid = $state<HTMLElement>();
	let cols = $state(1);
	$effect(() => {
		const el = grid;
		if (!el) return;
		const measure = () => (cols = Math.max(1, getComputedStyle(el).gridTemplateColumns.split(' ').filter(Boolean).length));
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		return () => ro.disconnect();
	});
	const openAt = $derived(one === null ? -1 : ordered.findIndex((s) => s.number === one));
	const panelOrder = $derived(openAt < 0 ? 0 : Math.min((Math.floor(openAt / cols) + 1) * cols - 1, ordered.length - 1) * 2 + 1);

	// Two columns: newest half on the left, the rest on the right.
	const half = $derived(Math.ceil(ordered.length / 2));
</script>

{#snippet episodes(n: number)}
	{#if eps[n] === 'error'}
		<p class="small muted">The episodes of {seasonName(n)} could not be read; try again shortly.</p>
	{:else if eps[n]}
		<EpisodeList rows={rows(n)} {fallback} />
	{:else}
		<div class="sk" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton lines={2} />{/each}</div>
	{/if}
{/snippet}

{#snippet fold(s: { number: number; episodes: number; year?: number })}
	{@const on = !!open[s.number]}
	<li class="fold" class:on>
		<button type="button" class="head" aria-expanded={on} aria-controls="season-{s.number}" onclick={() => toggle(s.number)}>
			<span class="name">{seasonName(s.number)}</span>
			<span class="count">{count(s.episodes)}</span>
			{#if fresh === s.number}<span class="new">New</span>{/if}
			<span class="end">
				{#if s.year}<span class="yr">{s.year}</span>{/if}
				<span class="chev"><Icon name="chevronDown" size={18} /></span>
			</span>
		</button>
		<div class="body" id="season-{s.number}" inert={!on}>
			<div class="in">{#if on || eps[s.number]}{@render episodes(s.number)}{/if}</div>
		</div>
	</li>
{/snippet}

{#if seasons.length}
	<section class="tp-section seasons l-{layout}" aria-labelledby="h-seasons" data-testid="season-folds">
		<h2 id="h-seasons" class="tp-h2">Seasons</h2>
		{#if layout === 'grid'}
			<div class="sgrid" bind:this={grid}>
				{#each ordered as s, i (s.number)}
					{@const on = one === s.number}
					<button type="button" class="tile" class:on style="order: {i * 2}" aria-expanded={on} aria-controls="season-panel" onclick={() => pick(s.number)}>
						<span class="tname">{seasonName(s.number)}{#if fresh === s.number}<span class="new">New</span>{/if}</span>
						<span class="tmeta">{count(s.episodes)}{s.year ? ` · ${s.year}` : ''}</span>
						<span class="chev"><Icon name="chevronDown" size={18} /></span>
					</button>
				{/each}
				{#if one !== null}
					<div class="panel" id="season-panel" style="order: {panelOrder}">
						{#key one}<div class="panel-in">{@render episodes(one)}</div>{/key}
					</div>
				{/if}
			</div>
		{:else if layout === 'row'}
			<div class="pills" role="group" aria-label="Season">
				{#each ordered as s (s.number)}
					<button type="button" class="pill" aria-pressed={shown === s.number} onclick={() => pick(s.number)}>{seasonName(s.number)}{#if fresh === s.number}<span class="dot" aria-label="New"></span>{/if}</button>
				{/each}
			</div>
			{#if shown !== null}
				{@const cur = seasons.find((s) => s.number === shown)}
				{#if cur}<p class="xs muted line">{count(cur.episodes)}{cur.year ? ` · ${cur.year}` : ''}</p>{/if}
				{#if eps[shown] === 'error'}
					<p class="small muted">The episodes of {seasonName(shown)} could not be read; try again shortly.</p>
				{:else if Array.isArray(eps[shown])}
					{#key shown}
						<Shelf title="{seasonName(shown)} episodes" item="min(300px, 78vw)" placeholder="wide">
							{#each rows(shown) as r (r.key)}
								<article class="ep">
									<span class="still">{#if r.still || fallback}<img src={sized(r.still || fallback, 'backdrop', 300)} alt="" loading="lazy" decoding="async" />{/if}</span>
									<b class="ep-name">{r.number}. {r.name}</b>
									<span class="xs muted">{[r.runtime ? runtimeWords(r.runtime) : '', r.when].filter(Boolean).join(' · ')}</span>
									{#if r.overview}<p class="ep-text">{r.overview}</p>{/if}
								</article>
							{/each}
						</Shelf>
					{/key}
				{:else}
					<div class="sk row-sk" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton lines={3} />{/each}</div>
				{/if}
			{/if}
		{:else}
			<div class="cols">
				<ul class="folds">{#each ordered.slice(0, half) as s (s.number)}{@render fold(s)}{/each}</ul>
				<ul class="folds">{#each ordered.slice(half) as s (s.number)}{@render fold(s)}{/each}</ul>
			</div>
		{/if}
	</section>
{/if}

<style>
	.new { margin-left: 8px; padding: 3px 8px; border-radius: 999px; background: var(--accent-fill); color: var(--text-on-red, #fff); font: 700 11px/1 var(--font-ui); letter-spacing: .02em; vertical-align: middle; }
	.chev { display: inline-flex; color: var(--text-muted); transition: transform 320ms cubic-bezier(0.34, 1.56, 0.64, 1); }
	.sk { display: flex; flex-direction: column; gap: 12px; }

	/* Grid: compact tiles; the open season's episodes in a panel under its row. */
	.sgrid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 200px), 1fr)); gap: 8px; }
	.tile { display: grid; grid-template-columns: 1fr auto; grid-template-rows: auto auto; align-items: center; gap: 4px 10px; min-height: 64px; padding: 12px 14px; border: 1px solid var(--line); border-radius: var(--r-card); background: var(--surface-1); color: var(--text); text-align: left; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), transform var(--dur-fast) cubic-bezier(0.34, 1.56, 0.64, 1); }
	.tile:hover { background: var(--surface-2); transform: translateY(-1px); }
	.tile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tile.on { border-color: var(--text); background: var(--surface-2); }
	.tile.on .chev { transform: rotate(180deg); }
	.tname { font: 600 16px/1.2 var(--font-ui); letter-spacing: -0.01em; }
	.tmeta { grid-column: 1; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); font-variant-numeric: tabular-nums; }
	.tile .chev { grid-column: 2; grid-row: 1 / span 2; }
	.panel { grid-column: 1 / -1; padding: 6px 16px 14px; border-radius: var(--r-card); background: var(--surface-1); border: 1px solid var(--line); animation: drop 300ms var(--ease-out) both; }
	.panel-in { animation: fade 240ms ease-out 60ms both; }
	/* A wide panel lays the episodes out in two columns, so rows are not mostly empty. */
	@media (min-width: 1100px) {
		.sgrid .panel :global(.eps) { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: 32px; }
		.sgrid .panel :global(.eps > .ep) { border-top: 1px solid var(--line); }
		.sgrid .panel :global(.eps > .ep:nth-child(-n + 2)) { border-top: 0; }
	}
	@keyframes drop { from { opacity: 0; transform: translateY(-6px); } }
	@keyframes fade { from { opacity: 0; } }

	/* Row: season pills over one row of episode cards. */
	.pills { display: flex; gap: 6px; overflow-x: auto; scrollbar-width: none; padding-bottom: 2px; }
	.pills::-webkit-scrollbar { display: none; }
	.pill { flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 34px; padding: 4px 14px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--text); font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.pill[aria-pressed='true'] { background: var(--text); color: var(--bg); border-color: var(--text); }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--accent); }
	.line { margin: 10px 0 2px; }
	.l-row :global(.ui-shelf header) { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
	.ep { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.still { display: block; aspect-ratio: 16 / 9; margin-bottom: 6px; border-radius: 10px; overflow: hidden; background: var(--surface-2); }
	.still img { width: 100%; height: 100%; object-fit: cover; }
	.ep-name { font: 600 15px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ep-text { margin: 2px 0 0; color: var(--text-muted); font: 400 13.5px/1.4 var(--font-ui); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 3; line-clamp: 3; overflow: hidden; }
	.row-sk { flex-direction: row; margin-top: 12px; }
	.row-sk > :global(*) { flex: 1; }

	/* Two columns of folds; each column grows on its own. */
	.cols { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; align-items: start; }
	.folds { display: flex; flex-direction: column; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.fold { border-radius: var(--r-card); background: var(--surface-1); border: 1px solid var(--line); overflow: hidden; }
	.head { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 54px; padding: 10px 16px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.head:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.head:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.name { font: 600 16px/1.2 var(--font-ui); letter-spacing: -0.01em; }
	.count { padding: 3px 8px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: 600 12px/1 var(--font-ui); white-space: nowrap; }
	.end { display: inline-flex; align-items: center; gap: 12px; margin-left: auto; color: var(--text-muted); font: 500 14px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.on .chev { transform: rotate(180deg); }
	/* The fold opens by its own height: grid rows from 0fr to 1fr. */
	.body { display: grid; grid-template-rows: 0fr; transition: grid-template-rows 340ms var(--ease-out); }
	.on .body { grid-template-rows: 1fr; }
	.in { min-height: 0; overflow: hidden; padding: 0 16px; opacity: 0; transition: opacity 240ms ease-out, padding 340ms var(--ease-out); }
	.on .in { padding: 4px 16px 14px; opacity: 1; transition-delay: 60ms; }

	@media (max-width: 899px) { .cols { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 600px) { .sgrid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .head { padding: 10px 12px; } .on .in { padding: 4px 12px 12px; } .yr { display: none; } .panel { padding: 4px 12px 12px; } }
	@media (prefers-reduced-motion: reduce) { .body, .in, .chev, .tile { transition: none; } .panel, .panel-in { animation: none; } }
</style>
