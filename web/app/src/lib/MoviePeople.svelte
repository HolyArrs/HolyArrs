<script lang="ts">
	import { api, message } from './api.ts';
	import { to } from './paths.ts';
	import type { PersonSummary } from './people.ts';

	let { titleId }: { titleId: number } = $props();
	let people = $state<{ person: PersonSummary; roles: string[]; characters: string[] }[]>([]);
	let error = $state('');
	let loaded = $state(false);
	let busy = $state(false);
	let revision = 0;

	$effect(() => {
		const id = titleId;
		revision++;
		people = []; loaded = false; error = ''; busy = false;
		return () => { revision++; };
	});
	async function load() {
		const run = ++revision;
		busy = true; error = '';
		try {
			const answer = await api.get<{ people: typeof people }>(`/titles/${titleId}/people`);
			if (run === revision) { people = answer.people; loaded = true; }
		} catch (err) { if (run === revision) error = message(err); }
		finally { if (run === revision) busy = false; }
	}
</script>

<section class="card" aria-labelledby="h-movie-people">
	<h2 id="h-movie-people" class="h2">Cast and directors</h2>
	<p class="small muted">Browse their movies and review missing titles. Credits may be incomplete.</p>
	{#if !loaded}<button class="btn btn-ghost" type="button" onclick={load} disabled={busy} aria-busy={busy}>{busy ? 'Loading credits…' : 'Show credits'}</button>{/if}
	{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
	{#if loaded && people.length === 0}<p class="small muted">No supported cast or director credits were returned.</p>{/if}
	<ul>
		{#each people as credit (credit.person.id)}
			<li><a href={to(`/people/${encodeURIComponent(credit.person.id)}`)}>{credit.person.name}</a><span class="small muted"> · {credit.roles.join(', ')}{credit.characters?.length ? ` · ${credit.characters.join(', ')}` : ''}</span></li>
		{/each}
	</ul>
</section>

<style>
	ul { padding-left: var(--s-5); }
	li { margin-top: var(--s-3); overflow-wrap: anywhere; }
</style>
