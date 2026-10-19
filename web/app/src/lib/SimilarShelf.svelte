<script lang="ts">
	import { api } from './api.ts';
	import { to } from './paths.ts';
	import { artName, similarTitles } from './title-page.ts';
	import type { TitleView } from './types.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';
	import Sheet from './ui/Sheet.svelte';

	// Similar in your library: titles of the same kind that share genres,
	// people, language or era (see similarTitles). Hidden when empty.
	let { title }: { title: TitleView } = $props();
	let all = $state<TitleView[]>([]);

	// Read the Library once per title: a live update hands the page a new
	// title object with the same id, which must not fetch the whole list again.
	const id = $derived(title.id);
	$effect(() => {
		void id;
		let live = true;
		api.get<{ titles: TitleView[] }>('/titles')
			.then((r) => { if (live) all = r.titles ?? []; })
			.catch(() => { /* A side shelf: hidden when the list cannot be read. */ });
		return () => { live = false; };
	});
	// One row on the page; See all opens the longer list in a sheet.
	const ROW = 12;
	const list = $derived(similarTitles(title, all, 36));
	let open = $state(false);
	// Opening another title from the sheet closes it.
	$effect(() => {
		void id;
		open = false;
	});
</script>

{#if list.length}
	<Shelf title="Similar in your library" item={title.kind === 'artist' ? '160px' : '150px'} anchor="title-similar" onseeall={() => (open = true)}>
		{#each list.slice(0, ROW) as t (t.id)}
			<PosterCard title={t.name} subtitle={t.year ? String(t.year) : undefined} image={t.artwork[0]} href={to(`/library/${t.id}`)} vtName={artName(t.id)} />
		{/each}
	</Shelf>
	<Sheet bind:open title="Similar in your library" size="lg">
		<ul class="all-grid">
			{#each list as t (t.id)}
				<li><PosterCard title={t.name} subtitle={t.year ? String(t.year) : undefined} image={t.artwork[0]} href={to(`/library/${t.id}`)} /></li>
			{/each}
		</ul>
	</Sheet>
{/if}

<style>
	.all-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 18px 14px; margin: 0; padding: 0; list-style: none; }
</style>
