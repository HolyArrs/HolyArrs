<script lang="ts">
	// Settings > Remote access: a card per forwarding plugin installed, else a pointer to the Plugins page. Owner only.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { to } from '#lib/core/paths.ts';
	import { RemoteAccess, forwards, type PluginOverview } from '#lib/plugins/index.ts';
	import { page } from '$app/state';

	let ov = $state<PluginOverview | null>(null);
	let loaded = $state(false);
	let error = $state('');
	// Every installed plugin the owner gave network.forward (Tailscale, NetBird, a tunnel); ?plugin= picks one.
	const forwarders = $derived((ov?.plugins ?? []).filter((p) => p.installed && forwards(p)));

	async function load() {
		try {
			ov = await api.get<PluginOverview>('/plugins');
			error = '';
		} catch (e) {
			// No plugins on this server: the same pointer as "not installed".
			if (e instanceof ApiError && e.status === 404) ov = null;
			else error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);
</script>

<svelte:head><title>{t('plugins.remoteTitle')} · HolyArrs</title></svelte:head>

<PageHeader title={t('plugins.remoteTitle')} />

{#if !loaded}
	<SkSettings rows={2} />
{:else if error && !ov}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if ov && forwarders.length}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#each forwarders.filter((p) => !page.url.searchParams.get('plugin') || p.id === page.url.searchParams.get('plugin')) as p (p.id)}
		<RemoteAccess plugin={p} canRun={ov.can_run} onchange={load} />
	{/each}
{:else}
	<section class="card" data-testid="remote-needs-plugin">
		<p class="small">{t('plugins.remoteNeedsPlugin')}</p>
		<Button href={to('/plugins')} icon="plug">{t('plugins.remoteOpenPlugins')}</Button>
	</section>
{/if}

<style>
	[data-testid='remote-needs-plugin'] { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--s-3); }
	[data-testid='remote-needs-plugin'] p { margin: 0; }
</style>
