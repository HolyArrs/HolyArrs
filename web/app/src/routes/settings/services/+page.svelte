<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { session } from '#lib/session.svelte.ts';
	import { defaultRegion } from '#lib/top10.ts';
	import { pickerServices, readServices, serviceRegion, SERVICES_KEY, toggleService, writeServices, type WatchService } from '#lib/explore.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Chip from '#lib/ui/Chip.svelte';

	// The streaming services this person has: Explore's "Trending on your
	// services" row leads with them. Kept in this browser; Explore sends them
	// along when it loads. A Kid cannot change them.
	const region = serviceRegion(defaultRegion());
	let services = $state<WatchService[]>([]);
	let picked = $state<number[]>([]);
	let loaded = $state(false);
	const shown = $derived(pickerServices(services, picked, 12));

	onMount(async () => {
		try {
			picked = readServices(region, localStorage.getItem(SERVICES_KEY)) ?? [];
		} catch {
			/* nothing saved */
		}
		try {
			services = (await api.get<{ services: WatchService[] }>(`/watch-services?region=${region}`)).services ?? [];
		} catch {
			services = [];
		}
		loaded = true;
	});

	function tap(id: number) {
		picked = toggleService(picked, id);
		try {
			localStorage.setItem(SERVICES_KEY, writeServices(region, picked, localStorage.getItem(SERVICES_KEY)));
		} catch {
			/* the choice is a convenience */
		}
	}
</script>

<svelte:head><title>Streaming services · HolyArrs</title></svelte:head>

<PageHeader title="Streaming services" subtitle="The services you stream on. Explore shows what is trending on them first." />

<SettingsCard id="services-card" title="Your services" description="Pick every service you have. The choice is kept in this browser and used by Explore." icon="tv">
	{#if session.kid}
		<p class="hint" data-testid="services-ask-owner">Ask your owner to pick services.</p>
	{:else if !loaded}
		<p class="hint">Loading services…</p>
	{:else if shown.length === 0}
		<p class="hint">No services are known for this region yet.</p>
	{:else}
		<div id="services" class="chips" role="group" aria-label="Streaming services">
			{#each shown as s (s.id)}
				<Chip label={s.name} selected={picked.includes(s.id)} onclick={() => tap(s.id)} />
			{/each}
		</div>
		<p class="hint">{picked.length === 0 ? 'None picked yet.' : `${picked.length} picked.`} <a href={to('/explore')}>Back to Explore</a></p>
	{/if}
</SettingsCard>

<style>
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2, 8px);
	}
</style>
