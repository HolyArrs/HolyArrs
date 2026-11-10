<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { ownedIndex } from '#lib/ui/title-cards.ts';
	import { artistSheets } from '#lib/requests/index.ts';
	import { MusicTrendingRows } from '#lib/music/index.ts';
	import type { MusicPageKind } from '#lib/music/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { LibraryFolder, PickRulesView } from '#lib/core/types.ts';

	// A Music Browse page for a mood, season, decade or activity: the same band, Spotlight and rows as a genre page, from the row templates.
	const kind = $derived((page.params.kind ?? 'mood') as MusicPageKind);
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

<svelte:head><title>{t('music.music')} · HolyArrs</title></svelte:head>

<div class="genre" data-testid="music-browse-page">
	{#key `${kind}/${slug}`}
		<MusicTrendingRows sheets={artistSheets} {owned} {manage} {mayAsk} {folders} {rules} {slug} {kind} />
	{/key}
</div>

<style>
	/* The same card sizes as Explore > Music. */
	.genre { --explore-item: 168px; display: flex; flex-direction: column; gap: 24px; padding-bottom: 48px; min-width: 0; }
	@media (max-width: 767px) { .genre { --explore-item: 54vw; } }
</style>
