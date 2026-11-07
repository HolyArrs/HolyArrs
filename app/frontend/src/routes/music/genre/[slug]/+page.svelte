<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { ownedIndex } from '#lib/explore.ts';
	import MusicTrendingRows from '#lib/MusicTrendingRows.svelte';
	import type { LibraryFolder, PickRulesView } from '#lib/types.ts';

	// A Music genre page (Music only): the genre's band, then its rows —
	// Spotlight, new and top albums in the genre, your artists in it, what
	// is missing from them, its popular artists and what is coming.
	const slug = $derived(page.params.slug ?? '');
	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	const owned = ownedIndex([]);

	onMount(() => {
		if (session.can('manage_library')) {
			void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
		}
	});
</script>

<svelte:head><title>Music genre · HolyArrs</title></svelte:head>

<div class="genre" data-testid="music-genre-page">
	{#key slug}
		<MusicTrendingRows {owned} {manage} {mayAsk} {folders} {rules} {slug} />
	{/key}
</div>

<style>
	/* The same card sizes as Explore → Music. */
	.genre { --explore-item: 168px; display: flex; flex-direction: column; gap: 24px; padding-bottom: 48px; min-width: 0; }
	@media (max-width: 767px) { .genre { --explore-item: 54vw; } }
</style>
