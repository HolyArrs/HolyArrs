<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import FilmographyList from '#lib/FilmographyList.svelte';
	import Poster from '#lib/Poster.svelte';
	import { actionableSelection, normalizeSelection, runReviewed, type FilmographyPage, type PersonMovie, type PersonRole, type ReviewedResult } from '#lib/people.ts';
	import type { AskLimit } from '#lib/types.ts';

	const id = page.params.id ?? '';
	let role = $state<PersonRole>('all');
	let current = $state(1);
	let data = $state<FilmographyPage | null>(null);
	let selected = $state<string[]>([]);
	let reviewed = $state<PersonMovie[] | null>(null);
	let results = $state<ReviewedResult[]>([]);
	let limit = $state<AskLimit | null>(null);
	let busy = $state(false);
	let submitting = $state(false);
	let error = $state('');
	const canSubmit = $derived(session.can('manage_library') || session.can('ask'));
	const action = $derived(session.can('manage_library') ? 'add' : 'ask');

	async function load(pageNumber = current) {
		busy = true;
		error = '';
		try {
			data = await api.get<FilmographyPage>(`/people/${encodeURIComponent(id)}?role=${role}&page=${pageNumber}&page_size=24`);
			current = pageNumber;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	function select(tmdb: string, checked: boolean) {
		selected = checked ? normalizeSelection([...selected, tmdb]) : selected.filter((id) => id !== tmdb);
		reviewed = null;
		results = [];
	}

	async function changeRole(next: PersonRole) {
		role = next;
		selected = [];
		reviewed = null;
		results = [];
		await load(1);
	}

	async function review() {
		busy = true;
		error = '';
		try {
			const r = await api.post<{ movies: PersonMovie[]; automatic_future_credits: boolean }>(`/people/${encodeURIComponent(id)}/movies/review`, { tmdb_ids: selected });
			reviewed = r.movies;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function submitReviewed() {
		if (!reviewed) return;
		submitting = true;
		results = await runReviewed((path, body) => api.post(path, body), action, actionableSelection(reviewed));
		submitting = false;
		if (action === 'ask') void api.get<AskLimit>('/asks/limit').then((r) => (limit = r)).catch(() => {});
		await load(current);
	}

	onMount(() => {
		void load();
		if (session.can('ask') && !session.can('manage_library')) void api.get<AskLimit>('/asks/limit').then((r) => (limit = r)).catch(() => {});
	});
</script>

<svelte:head><title>{data?.person.name ?? 'Person'} · HolyArrs</title></svelte:head>

{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
{#if busy && !data}<div class="sk hero" aria-busy="true"></div>{/if}
{#if data}
	<section class="workspace">
	<header class="person-head">
		<div class="portrait"><Poster src={data.person.profile} name={data.person.name} /></div>
		<div>
			<h1>{data.person.name}</h1>
			<p>{data.person.department || 'Department unavailable'}{#if data.person.birthplace} · {data.person.birthplace}{/if}</p>
			{#if data.person.biography}<p class="bio">{data.person.biography}</p>{/if}
		</div>
	</header>

	<div class="toolbar">
		<div class="seg" role="group" aria-label="Credit role">
			{#each [['all', 'All'], ['actor', 'Actor'], ['director', 'Director']] as option}
				<button type="button" aria-pressed={role === option[0]} onclick={() => changeRole(option[0] as PersonRole)}>{option[1]}</button>
			{/each}
		</div>
		{#if selected.length}<button class="btn btn-red" type="button" onclick={review} disabled={busy}>Review {selected.length} selected</button>{/if}
	</div>

	{#if !data.complete}<p class="notice">The provider returns this filmography as one unpaged list and does not promise it is complete.</p>{/if}
	{#if limit && action === 'ask' && !limit.no_limit && !limit.movies.no_limit}
		<p class="small muted">Movie asks this rolling week: {limit.movies.used} used, {limit.movies.left} left.</p>
	{/if}

	<FilmographyList movies={data.movies} {selected} selectable={canSubmit} onselect={select} />
	{#if !data.movies.length}<p class="notice">No movies match this role on this page.</p>{/if}

	<nav class="pages" aria-label="Filmography pages">
		<button class="btn btn-ghost" type="button" disabled={busy || current <= 1} onclick={() => load(current - 1)}>Previous</button>
		<span>Page {current}{#if data.total_pages} of {data.total_pages}{/if}</span>
		<button class="btn btn-ghost" type="button" disabled={busy || current >= data.total_pages} onclick={() => load(current + 1)}>Next</button>
	</nav>

	{#if reviewed}
		<section class="card review" aria-labelledby="review-title">
			<h2 id="review-title">Review {reviewed.length} movie{reviewed.length === 1 ? '' : 's'}</h2>
			<p>Only these movies will be {action === 'add' ? 'added' : 'asked for'}. Future credits are not followed automatically.</p>
			<ul>{#each reviewed as movie (movie.ids.tmdb)}<li>{movie.name}{#if movie.year} ({movie.year}){/if}</li>{/each}</ul>
			<button class="btn btn-red" type="button" disabled={submitting || !actionableSelection(reviewed).length} aria-busy={submitting} onclick={submitReviewed}>{action === 'add' ? 'Add reviewed movies' : 'Ask for reviewed movies'}</button>
		</section>
	{/if}
	{#if results.length}
		<section class="card" aria-labelledby="results-title">
			<h2 id="results-title">Results</h2>
			<ul class="results">{#each results as result}<li class:failed={!result.ok}>{result.tmdb_id}: {result.ok ? 'Submitted' : result.error}</li>{/each}</ul>
		</section>
	{/if}
	<p class="small muted attribution">{data.attribution}</p>
	</section>
{/if}

<style>
	.workspace { overflow: hidden; border: 1px solid var(--line); border-radius: 16px; background: var(--surface-1); }
	.person-head { display: grid; grid-template-columns: 132px minmax(0, 1fr); gap: var(--s-5); align-items: start; margin: 0; padding: var(--s-5); border: 0; border-bottom: 1px solid var(--line); border-radius: 0; background: var(--surface-1); }
	.portrait { width: 132px; }
	h1 { font: var(--weight-semibold) var(--text-2xl) / var(--leading-tight) var(--font-display); }
	.person-head p { margin-top: var(--s-2); color: var(--text-muted); }
	.bio { max-width: 76ch; line-height: 1.6; text-wrap: pretty; }
	.toolbar, .pages { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--s-3); margin: 0; padding: var(--s-3) var(--s-5); border-bottom: 1px solid var(--line); }
	.toolbar { position: sticky; top: 0; z-index: 3; background: color-mix(in srgb, var(--surface-1) 96%, transparent); backdrop-filter: blur(10px); }
	.workspace :global(.films) { grid-template-columns: repeat(auto-fit, minmax(min(100%, 360px), 1fr)); gap: 0; }
	.workspace :global(.films > li) { border: 0; border-right: 1px solid var(--line); border-bottom: 1px solid var(--line); border-radius: 0; }
	.review { margin: 0; border: 0; border-top: 1px solid var(--line); border-radius: 0; box-shadow: none; }
	.review p, .review ul, .review button, .results { margin-top: var(--s-3); }
	.review ul, .results { padding-left: var(--s-5); }
	.failed { color: var(--fail); }
	.attribution { margin: 0; padding: var(--s-4) var(--s-5); }
	.workspace > .card:not(.review) { margin: 0; border: 0; border-top: 1px solid var(--line); border-radius: 0; box-shadow: none; }
	.workspace > :global(.notice), .workspace > .small.muted { margin: var(--s-3) var(--s-5); }
	.hero { min-height: 220px; }
	@media (max-width: 560px) { .person-head { grid-template-columns: 88px minmax(0, 1fr); gap: var(--s-3); padding: var(--s-4); } .portrait { width: 88px; } .toolbar { position: static; } .toolbar > .btn { width: 100%; min-height: var(--tap); } .pages { justify-content: center; } }
</style>
