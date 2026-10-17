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
		grid-template-columns: 84px minmax(0, 1fr);
		gap: var(--s-3);
		align-items: center;
		min-height: 112px;
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-radius: 6px;
		background: color-mix(in srgb, var(--surface-1) 88%, transparent);
		color: inherit;
		text-decoration: none;
		transition: border-color var(--dur-fast), background var(--dur-fast), transform var(--dur-fast);
	}
	.person-card:hover {
		border-color: color-mix(in srgb, var(--ha-red) 55%, var(--line-strong));
		background: var(--surface-2);
		transform: translateY(-1px);
	}
	.person-card:active { transform: translateY(0); }
	.portrait { width: 84px; }
	h2 { font: var(--weight-semibold) var(--text-md) / var(--leading-tight) var(--font-display); }
	p { margin-top: var(--s-1); overflow-wrap: anywhere; }
	@media (max-width: 520px) {
		.person-card { grid-template-columns: 68px minmax(0, 1fr); min-height: 96px; }
		.portrait { width: 68px; }
	}
</style>
