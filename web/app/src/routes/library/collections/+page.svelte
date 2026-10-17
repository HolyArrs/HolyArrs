<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import type { MovieCollection } from '#lib/collections.ts';

	let collections = $state<MovieCollection[]>([]);
	let loaded = $state(false);
	let error = $state('');

	onMount(async () => {
		try {
			collections = (await api.get<{ collections: MovieCollection[] }>('/collections')).collections;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	});
</script>

<svelte:head><title>Movie series · HolyArrs</title></svelte:head>

<a class="back small muted" href={to('/library')}>Library</a>
<div class="page-head">
	<div><p class="eyebrow">Library</p><h1>Movie series</h1><p class="muted">Review owned movies together without changing the series grouping by accident.</p></div>
</div>

{#if error}<div class="notice bad" role="alert">{error}</div>{/if}
{#if loaded && collections.length === 0}
	<div class="panel empty"><h2>No owned movie series</h2><p class="muted">Movie series appear after a details source supplies an exact group identity for movies already in your Library.</p></div>
{:else}
	<div class="stack">
		{#each collections as c (c.id)}
			<a class="panel collection" href={to(`/library/collections/${c.id}`)}>
				<span><b>{c.name}</b><small class="muted">Source identity {c.source_id}</small></span>
				<span class="chip">{c.owned_titles} owned</span>
			</a>
		{/each}
	</div>
{/if}

<style>
	.collection { display: flex; align-items: center; justify-content: space-between; gap: var(--s-4); padding: var(--s-3); color: inherit; text-decoration: none; border: 1px solid var(--line); border-radius: 6px; background: var(--surface-1); transition: border-color var(--dur-fast), background var(--dur-fast); }
	.collection:hover { border-color: var(--line-strong); background: var(--surface-2); }
	.collection span:first-child { display: grid; gap: .2rem; }
	.empty { text-align: center; padding: 3rem 1rem; }
</style>
