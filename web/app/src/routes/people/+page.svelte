<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import PersonCard from '#lib/PersonCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Icon from '#lib/Icon.svelte';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
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
<PageHeader title="People" subtitle="Find an actor or director, then see their movies and which ones are already in your Library." back={{ href: to('/library'), label: 'Library' }} />

<form class="searchbar" role="search" onsubmit={search}>
	<span class="search-ic" aria-hidden="true"><Icon name="search" size={18} /></span>
	<label class="sr-only" for="person-query">Person name</label>
	<input id="person-query" type="search" bind:value={query} placeholder="Actor or director name" autocomplete="off" spellcheck="false" />
	<Button variant="primary" type="submit" disabled={!query.trim()} {busy}>Search</Button>
</form>

{#if error}<p class="notice bad" role="alert"><Icon name="alert" size={17} /><span>{plainReason(error).text}</span></p>{/if}
{#if busy && !result}
	<div class="grid" aria-busy="true">{#each Array(4) as _, i (i)}<div class="sk-card"><Skeleton shape="poster" /><Skeleton lines={3} /></div>{/each}</div>
{:else if result}
	<p class="count">{result.total_results} {result.total_results === 1 ? 'person' : 'people'} found. Pick the right one by what they do and the movies they are known for.</p>
	{#if result.people.length}
		<div class="grid">
			{#each result.people as person (person.id)}<PersonCard {person} />{/each}
		</div>
	{:else}
		<EmptyState compact icon="people" title="No matching people were returned." />
	{/if}
	<nav class="pages" aria-label="People search pages">
		<Button icon="chevronLeft" disabled={busy || current <= 1} onclick={() => search(undefined, current - 1)}>Previous</Button>
		<span class="count">Page {current}{#if result.total_pages} of {result.total_pages}{/if}</span>
		<Button iconAfter="chevronRight" disabled={busy || current >= result.total_pages} onclick={() => search(undefined, current + 1)}>Next</Button>
	</nav>
	<p class="attribution">{result.attribution}</p>
{:else if !error}
	<EmptyState icon="people" title="Look for a person" text="Type a name above to see their movies." />
{/if}
</section>

<style>
	.workspace { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.searchbar { position: relative; display: flex; align-items: center; gap: 8px; max-width: 720px; }
	.search-ic { position: absolute; left: 14px; top: 50%; translate: 0 -50%; display: grid; color: var(--text-muted); pointer-events: none; }
	.searchbar input { flex: 1; min-width: 0; height: 44px; padding: 0 12px 0 40px; border: 1px solid transparent; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-body); }
	.searchbar input:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.notice { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr)); gap: 12px; }
	.sk-card { display: grid; grid-template-columns: 72px minmax(0, 1fr); gap: 12px; padding: 12px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.count { margin: 0; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.attribution { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.pages { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	@media (max-width: 520px) { .searchbar { flex-wrap: wrap; } .searchbar :global(.ui-btn) { width: 100%; } }
</style>
