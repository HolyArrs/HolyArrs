<script lang="ts">
	import Poster from './Poster.svelte';
	import { to } from './paths.ts';
	import StatusPill from './ui/StatusPill.svelte';
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
						<h3>{movie.name}{#if movie.year}{' '}<span>({movie.year})</span>{/if}</h3>
						<p class="small muted">{movie.roles?.join(' · ')}{#if movie.characters?.length} · {movie.characters.join(', ')}{/if}</p>
					</div>
				</div>
				{#if movie.title_id}
					<a class="state" href={to(`/library/${movie.title_id}`)}><StatusPill kind="in_library" label="In Library" /></a>
				{:else if movie.never_add}
					<span class="state"><StatusPill kind="turned_down" label="Never-add" /></span>
				{:else if movie.ask}
					<span class="state"><StatusPill kind="requested" label={movie.ask.state_label} /></span>
				{:else}
					<span class="state"><StatusPill kind="missing" label="Missing" /></span>
				{/if}
			</div>
		</li>
	{/each}
</ul>

<style>
	.films { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr)); gap: 12px; }
	li { display: grid; grid-template-columns: 64px minmax(0, 1fr); gap: 12px; padding: 12px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: var(--shadow-card); min-width: 0; }
	.poster { width: 64px; border-radius: 8px; overflow: hidden; }
	.grow { min-width: 0; }
	.line { display: flex; gap: 12px; align-items: start; }
	input { margin-top: 2px; width: 20px; height: 20px; flex: none; accent-color: var(--accent); }
	h3 { margin: 0; font: var(--t-headline); overflow-wrap: anywhere; }
	h3 span { color: var(--text-muted); font-weight: 400; }
	p { margin: 2px 0 0; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.state { display: inline-flex; align-items: center; min-height: 32px; margin-top: 8px; border-radius: 999px; }
	a.state:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 560px) { li { grid-template-columns: 56px minmax(0, 1fr); } .poster { width: 56px; } }
</style>
