<script lang="ts">
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { checkMetadata, metadataHealth, METADATA_DOWN } from '#lib/metadata-health.svelte.ts';

	const NOTICE = 'This product uses the TMDB API but is not endorsed or certified by TMDB.';

	let loaded = $state(false);
	let checking = $state(false);

	onMount(async () => {
		await checkMetadata();
		loaded = true;
	});

	async function recheck() {
		checking = true;
		await checkMetadata(true);
		checking = false;
	}
</script>

<svelte:head><title>Metadata · HolyArrs</title></svelte:head>

<PageHeader title="Metadata" subtitle="Movie, show and music names, dates and pictures come from the HolyArrs metadata server. Nothing to set up." />

{#if !loaded}
	<div class="sk" style="height: 120px"></div>
{:else}
	<section class="card" id="metadata-card" tabindex="-1" aria-labelledby="h-server">
		<div class="card-head">
			<h2 id="h-server" class="h2">Metadata server</h2>
			<span class="chip {metadataHealth.up ? 'ok' : ''}">{metadataHealth.up ? 'Answering' : 'Down'}</span>
		</div>
		{#if metadataHealth.up}
			<p class="small muted">Searches, details, Explore and artwork are ready.</p>
		{:else}
			<p class="notice warn" role="status">{METADATA_DOWN}</p>
		{/if}
		<Button variant="secondary" onclick={recheck} disabled={checking} aria-busy={checking} style="margin-top: var(--s-4)">Check again</Button>
	</section>
{/if}

<p class="xs muted notice-line">
	Movie data and pictures from <a href="https://www.themoviedb.org/" rel="noopener noreferrer" target="_blank">TMDB</a>. {NOTICE}
</p>

<style>
	.notice-line {
		margin-top: var(--s-6);
	}
	.notice-line a {
		text-decoration: underline;
	}
</style>
