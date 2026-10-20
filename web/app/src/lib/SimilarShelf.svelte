<script lang="ts">
	import { api } from './api.ts';
	import { to } from './paths.ts';
	import { artName, similarTitles } from './title-page.ts';
	import type { TitleView } from './types.ts';
	import Shelf from './ui/Shelf.svelte';
	import WideCard from './ui/WideCard.svelte';
	import PersonCircle from './ui/PersonCircle.svelte';
	import Sheet from './ui/Sheet.svelte';

	// Similar in your library: titles of the same kind that share genres,
	// people, language or era (see similarTitles). Hidden when empty.
	// Artists show as circles (Similar artists); movies and shows as the
	// Explore wide card at the page's one size.
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

{#snippet one(t: TitleView, sheet: boolean)}
	{#if t.kind === 'artist'}
		<PersonCircle name={t.name} role={t.year ? String(t.year) : undefined} image={t.artwork[0]} href={to(`/library/${t.id}`)} size={sheet ? 104 : 120} />
	{:else}
		<WideCard cover title={t.name} subtitle={t.year ? String(t.year) : undefined} kind={t.kind === 'show' ? 'show' : 'movie'} typed image={t.backdrop ?? t.artwork[0]} logo={t.logo} href={to(`/library/${t.id}`)} vtName={sheet ? undefined : artName(t.id)} owned={{ state: 'available', label: 'In your Library' }} />
	{/if}
{/snippet}

{#if list.length}
	{@const artist = title.kind === 'artist'}
	<Shelf title={artist ? 'Similar artists' : 'Similar in your library'} item={artist ? '136px' : 'var(--title-item)'} anchor="title-similar" placeholder={artist ? 'round' : 'wide'} lines={1} onseeall={() => (open = true)}>
		{#each list.slice(0, ROW) as t (t.id)}{@render one(t, false)}{/each}
	</Shelf>
	<Sheet bind:open title={artist ? 'Similar artists' : 'Similar in your library'} size="lg">
		<ul class="all-grid" class:round={artist}>
			{#each list as t (t.id)}<li>{@render one(t, true)}</li>{/each}
		</ul>
	</Sheet>
{/if}

<style>
	.all-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 18px 14px; margin: 0; padding: 0; list-style: none; }
	.all-grid.round { grid-template-columns: repeat(auto-fill, minmax(124px, 1fr)); }
</style>
