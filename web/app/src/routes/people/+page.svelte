<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import PersonCard from '#lib/PersonCard.svelte';
	import type { PersonSearchPage } from '#lib/people.ts';

	let query = $state(page.url.searchParams.get('q') ?? '');
	let result = $state<PersonSearchPage | null>(null);
	let busy = $state(false);
	let error = $state('');
	let current = $state(1);

	async function search(e?: SubmitEvent, pageNumber = 1) {
		e?.preventDefault();
		const q = query.trim();
		if (!q) return;
		busy = true;
		error = '';
		const url = new URL(page.url.href);
		url.searchParams.set('q', q);
		void goto(url, { replace: true, shallow: true });
		try {
			result = await api.get<PersonSearchPage>(`/people/search?q=${encodeURIComponent(q)}&page=${pageNumber}`);
			current = pageNumber;
		} catch (err) {
			result = null;
			error = message(err);
		} finally {
			busy = false;
		}
	}

	onMount(() => { if (query.trim()) void search(); });
</script>

<svelte:head><title>People · HolyArrs</title></svelte:head>

<section class="workspace">
<header class="page-head">
	<div>
		<h1>People</h1>
		<p>Find an actor or director, then see their movies and which ones are already in your Library.</p>
	</div>
</header>

<form class="searchbar" role="search" onsubmit={search}>
	<label class="sr-only" for="person-query">Person name</label>
	<input id="person-query" type="search" bind:value={query} placeholder="Actor or director name" autocomplete="off" spellcheck="false" />
	<button class="btn btn-red" type="submit" disabled={busy || !query.trim()} aria-busy={busy}>Search</button>
</form>

{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
{#if busy && !result}
	<div class="grid" aria-busy="true">{#each Array(4) as _}<div class="sk"></div>{/each}</div>
{:else if result}
	<p class="small muted count">{result.total_results} provider result{result.total_results === 1 ? '' : 's'}. Pick the matching identity by department and known movies.</p>
	<div class="grid">
		{#each result.people as person (person.id)}<PersonCard {person} />{/each}
	</div>
	{#if !result.people.length}<p class="notice">No matching people were returned.</p>{/if}
		<nav class="pages" aria-label="People search pages">
			<button class="btn btn-ghost" type="button" disabled={busy || current <= 1} onclick={() => search(undefined, current - 1)}>Previous</button>
			<span>Page {current}{#if result.total_pages} of {result.total_pages}{/if}</span>
			<button class="btn btn-ghost" type="button" disabled={busy || current >= result.total_pages} onclick={() => search(undefined, current + 1)}>Next</button>
		</nav>
	<p class="small muted attribution">{result.attribution}</p>
{/if}
</section>

<style>
	.workspace { overflow: hidden; border: 1px solid var(--line); border-radius: 16px; background: var(--surface-1); }
	.workspace .page-head { margin: 0; padding: var(--s-5); border-bottom: 1px solid var(--line); }
	.searchbar { display: grid; grid-template-columns: 1fr auto; gap: var(--s-3); max-width: none; margin: 0; padding: var(--s-4) var(--s-5); border-bottom: 1px solid var(--line); }
	.searchbar input { min-width: 0; min-height: var(--tap); padding: 0 var(--s-3); border: 1px solid var(--line-strong); border-radius: 10px; background: var(--surface-1); }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 360px), 1fr)); gap: 0; }
	.grid :global(.person-card) { border: 0; border-right: 1px solid var(--line); border-bottom: 1px solid var(--line); border-radius: 0; background: var(--surface-1); transform: none; }
	.grid :global(.person-card:hover) { background: var(--surface-2); transform: none; }
	.sk { min-height: 112px; }
	.count { margin: 0; padding: var(--s-3) var(--s-5); border-bottom: 1px solid var(--line); }
	.attribution { margin: 0; padding: var(--s-4) var(--s-5); }
	.pages { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); margin: 0; padding: var(--s-4) var(--s-5); border-top: 1px solid var(--line); }
	.workspace > :global(.notice) { margin: var(--s-4) var(--s-5); }
	@media (max-width: 520px) { .searchbar { grid-template-columns: 1fr; } .searchbar button { width: 100%; min-height: var(--tap); } .pages { justify-content: center; flex-wrap: wrap; } }
</style>
