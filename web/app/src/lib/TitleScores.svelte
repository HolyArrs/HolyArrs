<script lang="ts">
	import { scoreName, scoreText, type OutsideScore } from './outside-ratings.ts';

	// A title's scores on the page (not over art): TMDB's star score first,
	// then the outside ratings. chips: one line of marks and numbers; tiles:
	// big numbers with a short name under each; list: name left, score right;
	// mono: the chips line in one quiet color, for a facts row.
	let { scores = [], tmdb, variant = 'chips' }: { scores?: OutsideScore[]; tmdb?: number | null; variant?: 'chips' | 'tiles' | 'list' | 'mono' } = $props();
	const marks: Record<string, string> = { imdb: 'IMDb', metacritic: 'MC', letterboxd: 'LB', trakt: 'Trakt' };
	const short: Record<string, string> = { imdb: 'IMDb', rt_critics: 'Critics', rt_audience: 'Audience', metacritic: 'Metacritic', letterboxd: 'Letterboxd', trakt: 'Trakt' };
	const tmdbText = $derived(tmdb ? (Math.round(tmdb * 10) / 10).toFixed(1) : '');
	const count = $derived(scores.length + (tmdbText ? 1 : 0));
</script>

{#snippet monoMark(source: string)}
	<!-- One-color marks drawn in the text color: a tomato, a popcorn bucket, a star, a circled M, three dots, a circled tick. -->
	{#if source === 'rt_critics'}<svg width="15" height="15" viewBox="0 0 12 12"><ellipse cx="6" cy="7.3" rx="4.9" ry="4.2" fill="currentColor" /><path d="M6 3.4C5.7 2.2 4.8 1.5 3.5 1.5c.6 1 1.4 1.6 2.5 1.9zm0 0c.3-1.2 1.2-1.9 2.5-1.9-.6 1-1.4 1.6-2.5 1.9z" fill="currentColor" /></svg>
	{:else if source === 'rt_audience'}<svg width="15" height="15" viewBox="0 0 12 12"><circle cx="3.9" cy="3.9" r="1.7" fill="currentColor" /><circle cx="6" cy="3.2" r="1.8" fill="currentColor" /><circle cx="8.1" cy="3.9" r="1.7" fill="currentColor" /><path d="M2.3 5.4h7.4l-1 6.1H3.3z" fill="currentColor" /><path d="M5 6.2v4.6M7 6.2v4.6" stroke="var(--bg)" stroke-width=".8" /></svg>
	{:else if source === 'tmdb'}<svg width="15" height="15" viewBox="0 0 12 12"><path d="M6 .8l1.6 3.3 3.6.5-2.6 2.5.6 3.6L6 9l-3.2 1.7.6-3.6L.8 4.6l3.6-.5z" fill="currentColor" /></svg>
	{:else if source === 'metacritic'}<svg width="15" height="15" viewBox="0 0 12 12"><circle cx="6" cy="6" r="5.1" fill="none" stroke="currentColor" stroke-width="1.2" /><path d="M3.7 8.1V4.3L6 6.9l2.3-2.6v3.8" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" /></svg>
	{:else if source === 'letterboxd'}<svg width="17" height="15" viewBox="0 0 14 12"><circle cx="3" cy="6" r="2.4" fill="currentColor" opacity=".55" /><circle cx="7" cy="6" r="2.4" fill="currentColor" /><circle cx="11" cy="6" r="2.4" fill="currentColor" opacity=".55" /></svg>
	{:else if source === 'trakt'}<svg width="15" height="15" viewBox="0 0 12 12"><circle cx="6" cy="6" r="5.1" fill="none" stroke="currentColor" stroke-width="1.2" /><path d="M3.8 6.1l1.6 1.6 3-3.3" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" /></svg>
	{:else}{marks[source]}{/if}
{/snippet}

{#snippet mark(source: string)}
	<span class="mark {source}" aria-hidden="true">
		{#if variant === 'mono'}{@render monoMark(source)}
		{:else if source === 'rt_critics'}<svg width="14" height="14" viewBox="0 0 12 12"><circle cx="6" cy="6.6" r="4.8" fill="#fa320a" /><path d="M5 1.4c.6.6 1 .9 1.8.7" stroke="#00912d" stroke-width="1.4" fill="none" stroke-linecap="round" /></svg>
		{:else if source === 'rt_audience'}<svg width="14" height="14" viewBox="0 0 12 12"><path d="M2.2 4.6h7.6l-1 6.4H3.2z" fill="#fa320a" /><circle cx="4" cy="3.6" r="1.6" fill="#f5e6c8" /><circle cx="6" cy="3" r="1.6" fill="#f5e6c8" /><circle cx="8" cy="3.6" r="1.6" fill="#f5e6c8" /></svg>
		{:else if source === 'tmdb'}<svg width="14" height="14" viewBox="0 0 12 12"><path d="M6 .8l1.6 3.3 3.6.5-2.6 2.5.6 3.6L6 9l-3.2 1.7.6-3.6L.8 4.6l3.6-.5z" fill="#f5c518" /></svg>
		{:else}{marks[source]}{/if}
	</span>
{/snippet}

{#if count}
	<ul class="scores {variant}" aria-label="Ratings" data-testid="title-scores">
		{#if tmdbText}
			<li title="TMDB: {tmdbText} out of 10">{@render mark('tmdb')}<span class="v">{tmdbText}</span><span class="n">TMDB</span></li>
		{/if}
		{#each scores as s (s.source)}
			<li title={`${scoreName(s.source)}: ${scoreText(s)}${s.votes ? ` (${s.votes.toLocaleString()} votes)` : ''}`}>{@render mark(s.source)}<span class="v">{scoreText(s)}</span><span class="n">{variant === 'list' ? scoreName(s.source) : short[s.source]}</span></li>
		{/each}
	</ul>
{/if}

<style>
	.scores { margin: 0; padding: 0; list-style: none; }
	li { color: var(--text); font-variant-numeric: tabular-nums; }
	.mark { display: inline-flex; align-items: center; justify-content: center; flex: none; height: 18px; min-width: 18px; padding: 0 4px; border-radius: 4px; font: 800 10.5px/1 var(--font-ui); letter-spacing: .01em; }
	.mark.imdb { background: #f5c518; color: #000; }
	.mark.metacritic { background: #333; color: #ffcc34; }
	.mark.letterboxd { background: #14181c; color: #40bcf4; }
	.mark.trakt { background: #ed1c24; color: #fff; }
	.mark.rt_critics, .mark.rt_audience, .mark.tmdb { padding: 0; }
	.v { font: 700 15px/1 var(--font-ui); }
	.n { color: var(--text-muted); font: 500 12px/1.2 var(--font-ui); }

	/* chips: one line, Overseerr-like. */
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; }
	.chips li { display: inline-flex; align-items: center; gap: 6px; }
	.chips .n { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }

	/* mono: the chips without brand colors, joining the facts row around them
	   (each score is an item of that row, with the same · and spacing). */
	.mono { display: contents; }
	.mono li { display: inline-flex; align-items: center; gap: 5px; color: var(--text-muted); }
	.mono li + li { margin-left: 8px; }
	.mono li + li::before { content: '·'; margin-right: 3px; color: var(--text-faint); }
	.mono .v { font-size: 14px; font-weight: 600; color: var(--text); }
	.mono .n { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
	.mono .mark { height: 16px; min-width: 0; padding: 0; background: none; color: var(--text-muted); }
	.mono .mark.imdb { padding: 0 4px; border-radius: 3px; background: var(--text-muted); color: var(--bg); font-size: 9.5px; letter-spacing: 0; }
	.mono .mark.trakt:not(:has(svg)) { padding: 0 4px; border-radius: 3px; box-shadow: inset 0 0 0 1px currentColor; }

	/* tiles: big numbers with the name under them. */
	.tiles { display: flex; flex-wrap: wrap; gap: 14px 28px; }
	.tiles li { display: grid; grid-template-columns: auto auto; grid-template-rows: auto auto; align-items: center; gap: 4px 7px; }
	.tiles .v { font-size: 22px; letter-spacing: -0.01em; }
	.tiles .n { grid-column: 1 / -1; text-transform: uppercase; letter-spacing: .06em; font-size: 11px; font-weight: 600; }

	/* list: name left, score right. */
	.list { display: flex; flex-direction: column; }
	.list li { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 10px; padding: 9px 0; }
	.list li + li { border-top: 1px solid var(--line); }
	.list .n { grid-column: 2; grid-row: 1; color: var(--text); font-size: 14px; }
	.list .v { grid-column: 3; }
</style>
