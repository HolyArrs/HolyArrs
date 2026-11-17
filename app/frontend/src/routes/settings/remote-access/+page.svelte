<script lang="ts">
	// Settings > Remote access: the Tailscale plugin's card when it is installed, else a pointer to the Plugins page. Owner only.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { to } from '#lib/core/paths.ts';
	import { RemoteAccess, type PluginOverview } from '#lib/plugins/index.ts';

	let ov = $state<PluginOverview | null>(null);
	let loaded = $state(false);
	let error = $state('');
	const tailscale = $derived(ov?.plugins.find((p) => p.id === 'tailscale' && p.installed) ?? null);

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
{:else if ov && tailscale}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<RemoteAccess plugin={tailscale} canRun={ov.can_run} onchange={load} />
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
