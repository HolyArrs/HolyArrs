<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { to } from '#lib/paths.ts';
	import { checkMetadata, metadataHealth, METADATA_DOWN } from '#lib/metadata-health.svelte.ts';


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

<PageHeader title="Metadata" />

{#if !loaded}
	<SkSettings rows={2} />
{:else}
	<section class="card" id="metadata-card" tabindex="-1" aria-labelledby="h-server">
		<div class="card-head">
			<h2 id="h-server" class="h2">Metadata server</h2>
			<span class="chip {metadataHealth.up ? 'ok' : 'fail'}">{metadataHealth.up ? 'Answering' : 'Down'}</span>
		</div>
		{#if metadataHealth.up}
			<p class="small muted">Searches, details, Explore and artwork are ready.</p>
		{:else}
			<p class="notice warn" role="status">{METADATA_DOWN}</p>
		{/if}
		<Button variant="secondary" onclick={recheck} disabled={checking} aria-busy={checking} style="margin-top: var(--s-4)">Check again</Button>
	</section>

	<!-- Keep in step with docs/metadata-privacy.md (tmdb-privacy.test.ts checks both). -->
	<section class="card" id="metadata-sent" aria-labelledby="h-sent">
		<h2 id="h-sent" class="h2">What the metadata server gets</h2>
		<ul class="sent">
			<li><b>An install token.</b> It identifies this install, not a person.</li>
			<li><b>What you look up.</b> Search words, and the IDs of the movies, shows, people, artists and albums a page shows.</li>
			<li><b>Display choices.</b> The language and region, so posters, trailers and where to watch match.</li>
			<li><b>Small request headers.</b> <code>Accept</code>, <code>Cache-Control</code>, <code>If-None-Match</code> and <code>X-Fetch: ahead</code> when a page is loaded ahead of time.</li>
			<li><b>Install count by country.</b> Installs are counted per country. Turn off <a href={to('/settings/this-app')}>Count this install</a> and requests carry <code>X-Country-Count: off</code>.</li>
		</ul>
		<p class="small muted">Like any web request, the server sees the public IP address it comes from.</p>
		<h3 class="h3">Never sent</h3>
		<ul class="sent">
			<li>Your files, folder paths or file names.</li>
			<li>Your library: which titles you have, what is downloading, your history.</li>
			<li>Accounts: user names, passwords, sign-in sessions, who asked for what.</li>
			<li>Keys and addresses of your downloaders, sources, media servers and lists.</li>
		</ul>
	</section>
{/if}

<style>
	.sent { margin: var(--s-2) 0 var(--s-3); padding-left: 1.2em; display: grid; gap: 6px; }
	.sent code { font-size: .9em; overflow-wrap: anywhere; }
	#metadata-sent { margin-top: var(--s-4); }
	#metadata-sent .h3 { margin-top: var(--s-3); }
</style>
