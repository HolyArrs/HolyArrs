<script lang="ts">
	// History. While its new look is chosen, ?look= picks one of four side by
	// side: today's page (now), the refined feed, a dense table and a story
	// per title. The pill at the bottom switches between them.
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api } from '#lib/api.ts';
	import { libraryTitles } from '#lib/owned.ts';
	import { session } from '#lib/session.svelte.ts';
	import HistoryNow from '#lib/history-looks/HistoryNow.svelte';
	import DirFeed from '#lib/history-looks/DirFeed.svelte';
	import DirTable from '#lib/history-looks/DirTable.svelte';
	import DirStory from '#lib/history-looks/DirStory.svelte';
	import LookPills from '#lib/history-looks/LookPills.svelte';
	import { lookOf } from '#lib/history-looks/dirs.ts';
	import type { TitleView } from '#lib/types.ts';

	const look = $derived(lookOf(page.url.searchParams.get('look')));

	// The new looks share the Library titles and member names (today's page reads its own).
	let titles = $state<TitleView[]>([]);
	const titleById = $derived(new Map(titles.map((t) => [t.id, t])));
	let people = $state<Map<number, string>>(new Map());
	let me = $state<number | undefined>();
	onMount(() => {
		me = session.s.id;
		void libraryTitles()
			.then((r) => (titles = r))
			.catch(() => {});
		void api
			.get<{ users: { id: number; name: string }[] }>('/users')
			.then((r) => (people = new Map(r.users.map((u) => [u.id, u.name]))))
			.catch(() => {});
	});
</script>

<svelte:head><title>History · HolyArrs</title></svelte:head>

<div class="looks" data-look={look}>
	{#if look === 'feed'}
		<DirFeed {titles} {titleById} {people} {me} />
	{:else if look === 'table'}
		<DirTable {titles} {titleById} {people} {me} />
	{:else if look === 'story'}
		<DirStory {titles} {titleById} {people} {me} />
	{:else}
		<HistoryNow />
	{/if}
</div>

<LookPills />

<style>
	/* Room under the last rows for the floating look switch. */
	.looks { padding-bottom: calc(72px + env(safe-area-inset-bottom, 0px)); }
</style>
