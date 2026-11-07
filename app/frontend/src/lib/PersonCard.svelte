<script lang="ts">
	import Poster from './ui/Poster.svelte';
	import { to } from './core/paths.ts';
	import { press } from './core/motion.ts';
	import type { PersonSummary } from './ui/people.ts';

	let { person }: { person: PersonSummary } = $props();
	const known = $derived(person.known_for.map((m) => (m.year ? `${m.name} (${m.year})` : m.name)).slice(0, 3).join(', '));
</script>

<a class="person-card" href={to(`/people/${person.id}`)} use:press={'card'}>
	<div class="portrait"><Poster src={person.profile} name={person.name} /></div>
	<div class="text">
		<h2>{person.name}</h2>
		<p class="muted">{person.department || 'Department unavailable'}</p>
		{#if known}<p class="known">Known for {known}</p>{/if}
	</div>
</a>

<style>
	.person-card {
		display: grid;
		grid-template-columns: 72px minmax(0, 1fr);
		gap: 14px;
		align-items: center;
		min-height: 108px;
		padding: 12px;
		border-radius: var(--r-card);
		background: var(--surface-1);
		box-shadow: var(--shadow-card);
		color: var(--text);
		text-decoration: none;
		transition: box-shadow var(--dur) var(--ease-out), transform var(--dur) var(--ease-out);
	}
	.person-card:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.person-card:hover { transform: translateY(-3px); box-shadow: var(--shadow-lift); }
	}
	.portrait { width: 72px; border-radius: 8px; overflow: hidden; }
	.text { min-width: 0; }
	h2 { margin: 0; font: var(--t-headline); overflow-wrap: anywhere; }
	p { margin: 2px 0 0; overflow-wrap: anywhere; font: var(--t-subhead); font-weight: 400; }
	.muted { color: var(--text-muted); }
	@media (max-width: 520px) {
		.person-card { grid-template-columns: 60px minmax(0, 1fr); min-height: 96px; }
		.portrait { width: 60px; }
	}
</style>
