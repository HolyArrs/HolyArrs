<script lang="ts">
	// History: every step from the ask to the media server, newest first,
	// with a search bar and one Filter. Routine work is left out.
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { libraryTitles } from '#lib/owned.ts';
	import { session } from '#lib/session.svelte.ts';
	import SearchLook from '#lib/history-looks/SearchLook.svelte';
	import type { TitleView } from '#lib/types.ts';

	// The Library titles and member names, for search, posters and "who".
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

<SearchLook {titleById} {people} {me} />
