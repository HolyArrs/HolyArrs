<script lang="ts">
	import { api } from '../core/api.ts';
	import { to } from '../core/paths.ts';
	import type { PersonSummary } from './people.ts';
	import Shelf from './Shelf.svelte';
	import PersonCircle from './PersonCircle.svelte';
	import Skeleton from './Skeleton.svelte';
	import Sheet from './Sheet.svelte';

	// Cast and crew of a title as a PersonCircle shelf; each opens the person's page.
	let { titleId }: { titleId: number } = $props();
	type Credit = { person: PersonSummary; roles: string[]; characters: string[] };
	let people = $state<Credit[]>([]);
	let phase = $state<'loading' | 'ok' | 'error'>('loading');

	$effect(() => {
		const id = titleId;
		let live = true;
		phase = 'loading';
		people = [];
		api.get<{ people: Credit[] }>(`/titles/${id}/people`)
			.then((r) => { if (live) { people = r.people ?? []; phase = 'ok'; } })
			.catch(() => { if (live) { phase = 'error'; } });
		return () => { live = false; };
	});
	// One row on the page; See all opens everyone in a sheet.
	const ROW = 12;
	let all = $state(false);
	const role = (c: Credit) => (c.characters?.length ? c.characters.join(', ') : c.roles.join(', '));
</script>

{#if phase === 'loading'}
	<section class="tp-section" id="title-cast" aria-label="Cast and crew loading" aria-busy="true">
		<h2 class="tp-h2">Cast and crew</h2>
		<div class="sk-row">{#each [0, 1, 2, 3, 4] as i (i)}<Skeleton shape="circle" width="96px" height="96px" />{/each}</div>
	</section>
{:else if phase === 'ok' && people.length}
	<Shelf title="Cast and crew" item="112px" anchor="title-cast" whole onseeall={() => (all = true)}>
		{#each people.slice(0, ROW) as c (c.person.id)}
			<PersonCircle name={c.person.name} role={role(c)} image={c.person.profile} href={to(`/people/${encodeURIComponent(c.person.id)}`)} />
		{/each}
	</Shelf>
	<Sheet bind:open={all} title="Cast and crew" size="lg">
		<ul class="all-grid">
			{#each people as c (c.person.id)}
				<li><PersonCircle name={c.person.name} role={role(c)} image={c.person.profile} href={to(`/people/${encodeURIComponent(c.person.id)}`)} /></li>
			{/each}
		</ul>
	</Sheet>
{:else if phase === 'ok'}
	<section class="tp-section" id="title-cast" aria-labelledby="h-cast">
		<h2 id="h-cast" class="tp-h2">Cast and crew</h2>
		<p class="small muted">No cast or crew credits were found.</p>
	</section>
{/if}

<style>
	.sk-row { display: flex; gap: 16px; overflow: hidden; }
	.all-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(104px, 1fr)); gap: 16px 12px; margin: 0; padding: 0; list-style: none; }
</style>
