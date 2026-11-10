<!-- Row of outside rating scores, each with a short source mark. -->
<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import { scoreName, scoreText, type OutsideScore } from './outside-ratings.ts';

	let { scores = [] }: { scores?: OutsideScore[] } = $props();
	// Text fallback for sources with no drawn logo.
	const marks: Record<string, string> = { imdb: 'IMDb', rt_critics: 'RT', rt_audience: 'RT', metacritic: 'MC', letterboxd: 'LB', trakt: 'Trakt' };
</script>

{#if scores.length}
	<ul class="ratings" aria-label={t('ui.ratingsStrip.ratings')} data-testid="ratings-strip">
		{#each scores as s (s.source)}
			<li title={s.votes ? t('ui.ratingsStrip.withVotes', { name: scoreName(s.source), score: scoreText(s), votes: s.votes.toLocaleString(appLocale()) }) : t('ui.ratingsStrip.noVotes', { name: scoreName(s.source), score: scoreText(s) })}>
				<span class="mark {s.source}" aria-hidden="true">
					{#if s.source === 'rt_critics'}<svg width="11" height="11" viewBox="0 0 12 12"><circle cx="6" cy="6.6" r="4.8" fill="#fa320a" /><path d="M5 1.4c.6.6 1 .9 1.8.7" stroke="#00912d" stroke-width="1.4" fill="none" stroke-linecap="round" /></svg>
					{:else if s.source === 'rt_audience'}<svg width="11" height="11" viewBox="0 0 12 12"><path d="M2.2 4.6h7.6l-1 6.4H3.2z" fill="#fa320a" /><circle cx="4" cy="3.6" r="1.6" fill="#fff" /><circle cx="6" cy="3" r="1.6" fill="#fff" /><circle cx="8" cy="3.6" r="1.6" fill="#fff" /></svg>
					{:else}{marks[s.source]}{/if}
				</span>
				<span class="v">{scoreText(s)}</span>
				<span class="sr">{scoreName(s.source)}</span>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.ratings { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 10px; margin: 0; padding: 0; list-style: none; }
	li { display: inline-flex; align-items: center; gap: 5px; height: 22px; color: rgb(255 255 255 / 92%); font: 700 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.mark { display: inline-flex; align-items: center; justify-content: center; height: 16px; padding: 0 4px; border-radius: 3px; font: 800 10px/1 var(--font-ui); letter-spacing: .01em; }
	.mark.imdb { background: #f5c518; color: #000; }
	.mark.metacritic { background: #333; color: #ffcc34; border: 1px solid rgb(255 255 255 / 30%); }
	.mark.letterboxd { background: #14181c; color: #40bcf4; }
	.mark.trakt { background: #ed1c24; color: #fff; }
	.mark.rt_critics, .mark.rt_audience { padding: 0; }
	/* Screen-reader name of the source, hidden from view. */
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
</style>
