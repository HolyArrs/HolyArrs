<script lang="ts" module>
	/** One row of a season's episode list. */
	export interface EpisodeRow {
		key: string | number;
		/** The episode number in the season (or its code, such as S01E02). */
		number: string;
		name: string;
		still?: string | null;
		runtime?: number;
		/** The air date in words, or "No air date yet". */
		when?: string;
		overview?: string;
		/** A requester's mark: the episode is here to watch. */
		here?: boolean;
	}
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import { sized } from './tmdb-image.ts';
	import { runtimeWords } from './ui/TitleHero.svelte';

	// A season as a vertical list, the pattern streaming apps use for long
	// seasons: the number, a still, the name, runtime and air date, and two
	// lines of synopsis. Library pages add a status and an actions menu per row.
	let {
		rows,
		fallback,
		status,
		action,
		after
	}: {
		rows: EpisodeRow[];
		/** The picture for episodes without a still (the show's backdrop). */
		fallback?: string | null;
		/** Beside the name: the row's status (managers). */
		status?: Snippet<[number]>;
		/** At the row's end: its actions menu. */
		action?: Snippet<[number]>;
		/** Under the synopsis: why it is or is not looked for. */
		after?: Snippet<[number]>;
	} = $props();
	let failed = $state<Record<string, boolean>>({});
</script>

<ol class="eps">
	{#each rows as r, i (r.key)}
		{@const src = r.still || fallback}
		<li class="ep" data-testid="episode">
			<span class="num" aria-hidden="true">{r.number}</span>
			<div class="still">
				{#if src && !failed[r.key]}<img src={sized(src, 'backdrop', 240)} alt="" loading="lazy" decoding="async" onerror={() => (failed[r.key] = true)} />{:else}<span class="ph"><Icon name="tv" size={20} /></span>{/if}
				{#if r.here}<span class="here" title="In your Library"><Icon name="check" size={14} /><span class="sr-only">In your Library</span></span>{/if}
			</div>
			<div class="body">
				<div class="top">
					<h3 class="name"><span class="sr-only">Episode {r.number}: </span>{r.name}</h3>
					{#if action}<span class="end">{@render action(i)}</span>{/if}
				</div>
				<p class="meta">{[runtimeWords(r.runtime), r.when].filter(Boolean).join(' · ')}{#if status}{' '}{@render status(i)}{/if}</p>
				{#if r.overview}<p class="syn">{r.overview}</p>{/if}
				{#if after}{@render after(i)}{/if}
			</div>
		</li>
	{:else}
		<li class="small muted">No episodes listed yet.</li>
	{/each}
</ol>

<style>
	.eps { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.ep { display: grid; grid-template-columns: 2.2em minmax(120px, 200px) minmax(0, 1fr); gap: 16px; align-items: center; padding: 16px 0; min-width: 0; }
	.ep + .ep { border-top: 1px solid var(--line); }
	.num { color: var(--text-muted); font: 600 22px/1 var(--font-display); text-align: center; font-variant-numeric: tabular-nums; }
	.still { position: relative; aspect-ratio: 16 / 9; border-radius: 8px; overflow: hidden; background: var(--surface-2); }
	.still img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.ph { position: absolute; inset: 0; display: grid; place-items: center; color: var(--text-muted); }
	.here { position: absolute; top: 6px; right: 6px; display: grid; place-items: center; width: 22px; height: 22px; border-radius: 999px; background: var(--on-art-scrim-mid, rgb(0 0 0 / 50%)); color: var(--on-art, #fff); }
	.body { display: flex; flex-direction: column; gap: 4px; min-width: 0; align-self: start; }
	.top { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; min-width: 0; }
	.name { margin: 0; font: 600 16px/1.3 var(--font-ui); overflow-wrap: anywhere; }
	.end { flex: none; margin: -6px -6px 0 0; }
	.meta { margin: 0; color: var(--text-muted); font: var(--t-footnote); display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; }
	.syn { margin: 2px 0 0; color: var(--text-muted); font: var(--t-subhead); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; max-width: 75ch; }
	@media (max-width: 600px) {
		.ep { grid-template-columns: minmax(0, 132px) minmax(0, 1fr); gap: 8px 12px; }
		.num { display: none; }
		.syn { grid-column: 1 / -1; }
	}
</style>
