<script lang="ts">
	import Poster from './Poster.svelte';
	import { to } from './paths.ts';
	import type { PersonMovie } from './people.ts';

	let {
		movies,
		selected,
		selectable,
		onselect
	}: { movies: PersonMovie[]; selected: string[]; selectable: boolean; onselect: (id: string, checked: boolean) => void } = $props();
</script>

<ul class="films">
	{#each movies as movie (movie.ids.tmdb)}
		<li>
			<div class="poster"><Poster src={movie.poster} name={movie.name} /></div>
			<div class="grow">
				<div class="line">
					{#if selectable && !movie.title_id && !movie.never_add && !movie.ask}
						<input aria-label={`Select ${movie.name}`} type="checkbox" checked={selected.includes(movie.ids.tmdb)} onchange={(e) => onselect(movie.ids.tmdb, e.currentTarget.checked)} />
					{/if}
					<div>
						<h3>{movie.name}{#if movie.year} <span>({movie.year})</span>{/if}</h3>
						<p class="small muted">{movie.roles?.join(' · ')}{#if movie.characters?.length} · {movie.characters.join(', ')}{/if}</p>
					</div>
				</div>
				{#if movie.title_id}
					<a class="state" href={to(`/library/${movie.title_id}`)}>In Library</a>
				{:else if movie.never_add}
					<span class="state">Never-add</span>
				{:else if movie.ask}
					<span class="state">{movie.ask.state_label}</span>
				{:else}
					<span class="state missing">Missing</span>
				{/if}
			</div>
		</li>
	{/each}
</ul>

<style>
	.films { list-style: none; display: grid; gap: var(--s-3); }
	li { display: grid; grid-template-columns: 64px 1fr; gap: var(--s-4); padding: var(--s-3); border: 1px solid var(--line); border-radius: var(--r-card); background: var(--surface-1); }
	.poster { width: 64px; }
	.line { display: flex; gap: var(--s-3); align-items: start; }
	input { margin-top: 4px; width: 18px; height: 18px; }
	h3 { font-size: var(--text-md); }
	h3 span { color: var(--text-muted); font-weight: var(--weight-regular); }
	.state { display: inline-block; margin-top: var(--s-2); font-size: var(--text-sm); color: var(--text-muted); }
	.state.missing { color: var(--wait); }
	@media (max-width: 560px) { li { grid-template-columns: 52px 1fr; padding: var(--s-3); } .poster { width: 52px; } }
</style>
