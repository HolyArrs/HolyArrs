<script lang="ts">
	import { api } from './api.ts';
	import { to } from './paths.ts';
	import { artName, similarTitles } from './title-page.ts';
	import type { TitleView } from './types.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';

	// Similar in your library: titles of the same kind that share genres,
	// people, language or era (see similarTitles). Hidden when empty.
	let { title }: { title: TitleView } = $props();
	let all = $state<TitleView[]>([]);

	$effect(() => {
		void title.id;
		let live = true;
		api.get<{ titles: TitleView[] }>('/titles')
			.then((r) => { if (live) all = r.titles ?? []; })
			.catch(() => { /* A side shelf: hidden when the list cannot be read. */ });
		return () => { live = false; };
	});
	const list = $derived(similarTitles(title, all, 14));
</script>

{#if list.length}
	<Shelf title="Similar in your library" item={title.kind === 'artist' ? '160px' : '150px'}>
		{#each list as t (t.id)}
			<PosterCard title={t.name} subtitle={t.year ? String(t.year) : undefined} image={t.artwork[0]} href={to(`/library/${t.id}`)} vtName={artName(t.id)} />
		{/each}
	</Shelf>
{/if}
