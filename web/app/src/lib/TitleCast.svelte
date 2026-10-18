<script lang="ts">
	import { api } from './api.ts';
	import { to } from './paths.ts';
	import type { PersonSummary } from './people.ts';
	import Shelf from './ui/Shelf.svelte';
	import PersonCircle from './ui/PersonCircle.svelte';
	import Skeleton from './ui/Skeleton.svelte';

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
	const role = (c: Credit) => (c.characters?.length ? c.characters.join(', ') : c.roles.join(', '));
</script>

{#if phase === 'loading'}
	<section class="tp-section" aria-label="Cast and crew loading" aria-busy="true">
		<h2 class="tp-h2">Cast and crew</h2>
		<div class="sk-row">{#each [0, 1, 2, 3, 4] as i (i)}<Skeleton shape="circle" width="96px" height="96px" />{/each}</div>
	</section>
{:else if phase === 'ok' && people.length}
	<Shelf title="Cast and crew" item="112px">
		{#each people as c (c.person.id)}
			<PersonCircle name={c.person.name} role={role(c)} image={c.person.profile} href={to(`/people/${encodeURIComponent(c.person.id)}`)} />
		{/each}
	</Shelf>
{:else if phase === 'ok'}
	<section class="tp-section" aria-labelledby="h-cast">
		<h2 id="h-cast" class="tp-h2">Cast and crew</h2>
		<p class="small muted">No cast or crew credits were found.</p>
	</section>
{/if}

<style>
	.sk-row { display: flex; gap: 16px; overflow: hidden; }
</style>
