<script lang="ts">
	import Poster from './Poster.svelte';
	import { to } from './paths.ts';
	import type { PersonSummary } from './people.ts';

	let { person }: { person: PersonSummary } = $props();
	const known = $derived(person.known_for.map((m) => (m.year ? `${m.name} (${m.year})` : m.name)).slice(0, 3).join(', '));
</script>

<a class="person-card" href={to(`/people/${person.id}`)}>
	<div class="portrait"><Poster src={person.profile} name={person.name} /></div>
	<div>
		<h2>{person.name}</h2>
		<p class="muted">{person.department || 'Department unavailable'}</p>
		{#if known}<p class="small">Known for {known}</p>{/if}
	</div>
</a>

<style>
	.person-card {
		display: grid;
		grid-template-columns: 72px 1fr;
		gap: var(--s-4);
		align-items: center;
		padding: var(--s-4);
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
		color: inherit;
		text-decoration: none;
	}
	.person-card:hover { border-color: var(--line-strong); }
	.portrait { width: 72px; }
	h2 { font: var(--weight-semibold) var(--text-md) / var(--leading-tight) var(--font-display); }
	p { margin-top: var(--s-1); }
</style>
