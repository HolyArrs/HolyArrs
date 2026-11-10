<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// History: every step from the ask to the media server, newest first, with a search bar and one Filter. Routine work is left out.
	import { libraryBrief } from '#lib/mediafiles/index.ts';
	import { HistoryPage as SearchLook } from '#lib/downloads/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { TitleView } from '#lib/core/types.ts';

	// The Library titles and member names, for search, posters and "who".
	let titles = $state<TitleView[]>([]);
	const titleById = $derived(new Map(titles.map((t) => [t.id, t])));
	let people = $state<Map<number, string>>(new Map());
	let me = $state<number | undefined>();
	onMount(() => {
		me = session.s.id;
		void libraryBrief()
			.then((r) => (titles = r))
			.catch(() => {});
		void api
			.get<{ users: { id: number; name: string }[] }>('/users')
			.then((r) => (people = new Map(r.users.map((u) => [u.id, u.name]))))
			.catch(() => {});
	});
</script>

<svelte:head><title>{t('downloads.page.historyTitle')}</title></svelte:head>

<div class="hist"><SearchLook {titleById} {people} {me} /></div>

<style>
	/* History has no shared top bar: its own row (search, Live, Filter) is pinned at the top instead, so the day headers and the details panel pin under that row (72px: a 52px capsule and 10px above and below). */
	.hist { --topbar: 72px; }
	@media (max-width: 767px) { .hist { --topbar: 0px; } }
</style>
