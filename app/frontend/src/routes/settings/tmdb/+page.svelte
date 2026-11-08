<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import { checkMetadata, metadataHealth, METADATA_DOWN } from '#lib/metadata/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';

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
	<SettingsCard id="metadata-card" title="Metadata server">
		<div class="srow">
			<div class="txt"><span class="label">{metadataHealth.up ? 'Connected' : 'Not answering'}</span>{#if !metadataHealth.up}<span class="hint" role="status">{METADATA_DOWN}</span>{/if}</div>
			<div class="ctl state">
				<span class="st" class:fail={!metadataHealth.up}><i></i>{metadataHealth.up ? 'Answering' : 'Down'}</span>
				<Button variant="secondary" size="sm" onclick={recheck} disabled={checking} aria-busy={checking}>Check again</Button>
			</div>
		</div>
	</SettingsCard>

	<!-- Keep in step with the metadata privacy doc (tmdb-privacy.test.ts checks both). -->
	<SettingsCard id="metadata-sent" title="What we send" description="Your public IP address goes too, like any web request">
		<ul class="sent">
			<li><b>An install token.</b> Identifies this install, not you.</li>
			<li><b>What you look up.</b> Search words and IDs of what a page shows.</li>
			<li><b>Display choices.</b> Language and region.</li>
			<li><b>Small request headers.</b> <code>Accept</code>, <code>Cache-Control</code>, <code>If-None-Match</code> and <code>X-Fetch: ahead</code> when a page is loaded ahead of time.</li>
			<li><b>Install count by country.</b> Installs are counted per country. Turn off <a href={to('/settings/this-app') + '#count-install'}>Count this install</a> and requests carry <code>X-Country-Count: off</code>.</li>
		</ul>
	</SettingsCard>

	<SettingsCard id="metadata-never" title="Never sent">
		<ul class="sent">
			<li>Your files, folder paths or file names.</li>
			<li>Your library: which titles you have, what is downloading, your history.</li>
			<li>Accounts: user names, passwords, sign-in sessions, who requested what.</li>
			<li>Keys and addresses of your Downloaders, Sources, media servers and lists.</li>
		</ul>
	</SettingsCard>
{/if}

<style>
	.sent { margin: 0; padding-block: 0 !important; list-style: none; box-shadow: none !important; }
	.sent > li { padding: 13px 0; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; }
	.sent > li + li { box-shadow: inset 0 1px 0 var(--line); }
	.sent b { color: var(--text); font-weight: 600; }
	.sent code { font-size: .9em; overflow-wrap: anywhere; }
	.state { display: flex; align-items: center; gap: 14px; }
	.st { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 500 13px/1 var(--font-ui); }
	.st i { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.st.fail i { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
</style>
