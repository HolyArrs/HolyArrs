<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import type { MovieCollection } from '#lib/collections.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import ListRow from '#lib/ui/ListRow.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import { plainReason } from '#lib/plain.ts';

	let collections = $state<MovieCollection[]>([]);
	let loaded = $state(false);
	let error = $state('');

	onMount(load);
	async function load() {
		try {
			collections = (await api.get<{ collections: MovieCollection[] }>('/collections')).collections;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
</script>

<svelte:head><title>Movie series · HolyArrs</title></svelte:head>

<div class="page">
	<PageHeader title="Movie series" subtitle="Movies in your Library that belong to the same series." back={{ href: to('/library'), label: 'Library' }} />

	{#if error}
		<LoadError hasData={collections.length > 0} message={plainReason(error).text} onretry={load} />
	{/if}
	{#if !loaded}
		<div class="rows">{#each Array(4) as _, i (i)}<Skeleton height="56px" />{/each}</div>
	{:else if !error && collections.length === 0}
		<EmptyState icon="film" title="No movie series yet" text="Movie series show up once movies in your Library are known to belong together." />
	{:else if collections.length}
		<ul class="rows">
			{#each collections as c (c.id)}
				<li>
					<ListRow title={c.name} icon="film" href={to(`/library/collections/${c.id}`)}>
						{#snippet trailing()}<Badge label="{c.owned_titles} in library" />{/snippet}
					</ListRow>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; }
	.rows { display: flex; flex-direction: column; gap: 4px; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.rows > li + li { border-top: 1px solid var(--line); }
</style>
