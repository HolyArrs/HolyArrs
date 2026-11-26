<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { PlexPlayback, playbackSources, refreshPlaybackSources } from '#lib/mediaservers/index.ts';
	import { to } from '#lib/core/paths.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';

	const owner = $derived(session.s.role === 'owner');
	const selected = $derived(playbackSources.servers.find((m) => String(m.id) === page.url.searchParams.get('server')) ?? playbackSources.servers[0]);
	const settingsPath = '/settings/media-servers#playback-monitoring';

	function pickServer(e: Event) {
		const select = e.currentTarget;
		if (!(select instanceof HTMLSelectElement)) return;
		const url = new URL(page.url.href);
		url.searchParams.set('server', select.value);
		void goto(url);
	}
</script>

<svelte:head><title>{t('settings.mediaServers.playback.title')} · HolyArrs</title></svelte:head>

<div class="page">
	<PageHeader title={t('settings.mediaServers.playback.title')}>
		{#snippet actions()}
			{#if owner}<Button size="sm" variant="secondary" href={to(settingsPath)}>{t('settings.mediaServers.title')}</Button>{/if}
		{/snippet}
	</PageHeader>

	{#if owner}
		{#if playbackSources.error && !selected}
			<LoadError hasData={false} message={playbackSources.error} onretry={refreshPlaybackSources} />
		{:else}
			<PageLoader loading={!playbackSources.loaded}>
				{#if selected}
					<div class="source">
						<Field id="playback-source" label={t('downloads.tv.server')}>
							<select class="input" id="playback-source" value={String(selected.id)} onchange={pickServer}>
								{#each playbackSources.servers as source (source.id)}
									<option value={String(source.id)}>{source.name}</option>
								{/each}
							</select>
						</Field>
					</div>
					{#key selected.id}<PlexPlayback server={selected.id} />{/key}
				{:else}
					<p class="muted">{t('settings.mediaServers.playback.monitor')} · {t('settings.mediaServers.off')}</p>
				{/if}
			</PageLoader>
		{/if}
	{:else}
		<p class="muted">{t('server.mediaservers.sign-in-as-the-owner-to')}</p>
	{/if}
</div>

<style>
	.page { padding: var(--s-5) max(var(--s-4), 3vw) var(--s-6); max-width: 1280px; margin-inline: auto; }
	.source { max-width: 20rem; margin-bottom: var(--s-5); }
</style>
