<script lang="ts">
	// Plugins: the signed plugin list, what each may reach, and install, start, stop, remove. Owner only.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { limitLines, notices, pluginName, pluginState, shownVersion, stateLabel, type Plugin, type PluginOverview } from '#lib/plugins/index.ts';

	const owner = $derived(session.s.role === 'owner');
	let ov = $state<PluginOverview | null>(null);
	let loaded = $state(false);
	let error = $state('');
	let busy = $state('');
	let reaching = $state<Plugin | null>(null);
	let reachOpen = $state(false);
	let removing = $state<Plugin | null>(null);
	let removeOpen = $state(false);

	const notes = $derived(ov ? notices(ov) : []);
	const limits = $derived(ov ? limitLines(ov.limits) : []);
	const canInstall = $derived(!!ov && ov.key_set && ov.can_run);
	const reach = $derived(
		reaching
			? [
					{ label: t('plugins.hosts'), list: reaching.access?.hosts ?? [] },
					{ label: t('plugins.folders'), list: reaching.access?.folders ?? [] },
					{ label: t('plugins.actions'), list: reaching.access?.actions ?? [] }
				]
			: []
	);

	async function load() {
		try {
			ov = await api.get<PluginOverview>('/plugins');
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(() => {
		if (owner) void load();
		else loaded = true;
	});

	async function act(p: Plugin, run: () => Promise<unknown>, done: string) {
		if (busy) return;
		busy = p.id;
		try {
			await run();
			toasts.push(t(done, { name: pluginName(p.id) }), 'ok');
			await load();
		} catch (e) {
			toasts.push(t('plugins.failed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	const path = (p: Plugin) => `/plugins/${encodeURIComponent(p.id)}`;

	function askInstall(p: Plugin) {
		reaching = p;
		reachOpen = true;
	}
	function install() {
		const p = reaching;
		reachOpen = false;
		if (p) void act(p, () => api.post(`${path(p)}/install`), 'plugins.installed');
	}
	function askRemove(p: Plugin) {
		removing = p;
		removeOpen = true;
	}
	function remove(keep: boolean) {
		const p = removing;
		removeOpen = false;
		if (p) void act(p, () => api.del(path(p) + (keep ? '?keep=1' : '')), 'plugins.removed');
	}
</script>

<svelte:head><title>{t('plugins.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('plugins.title')} />

{#if !owner}
	<EmptyState icon="shield" title={t('plugins.title')} text={t('plugins.ownerOnly')} />
{:else if !loaded}
	<SkSettings rows={2} />
{:else if !ov}
	<LoadError hasData={false} message={error} onretry={load} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#each notes as n (n)}
		<p class="notice warn" role="status" data-testid="plugins-notice"><Icon name="info" size={18} /><span>{n}</span></p>
	{/each}

	<section class="card" aria-labelledby="h-plugins">
		<div class="card-head"><h2 id="h-plugins" class="h2">{t('plugins.title')}</h2></div>
		{#if ov.plugins.length === 0}
			<p class="small muted">{t('plugins.none')}</p>
		{:else}
			<ul class="rows">
				{#each ov.plugins as p (p.id)}
					{@const s = pluginState(p)}
					<li data-testid="plugin-row" data-id={p.id}>
						<div class="grow">
							<div class="pname">{pluginName(p.id)}</div>
							<div class="xs muted">{t('plugins.byLine')} · <span class="mono">{shownVersion(p)}</span></div>
							<div class="state"><Badge label={stateLabel(p)} /></div>
						</div>
						<div class="acts">
							{#if s === 'not_installed'}
								<Button variant="primary" disabled={!canInstall || !!busy} busy={busy === p.id} onclick={() => askInstall(p)}>{t('plugins.install')}</Button>
							{:else}
								{#if s === 'running'}
									<Button disabled={!!busy} busy={busy === p.id} onclick={() => act(p, () => api.post(`${path(p)}/stop`), 'plugins.stopped')}>{t('plugins.stop')}</Button>
								{:else if s === 'stopped'}
									<Button disabled={!ov.can_run || !!busy} busy={busy === p.id} onclick={() => act(p, () => api.post(`${path(p)}/start`), 'plugins.started')}>{t('plugins.start')}</Button>
								{/if}
								<Button variant="plain" disabled={!!busy} onclick={() => askRemove(p)}>{t('plugins.remove')}</Button>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<SettingsCard id="plugin-limits" title={t('plugins.limits')} description={t('plugins.limitsAbout')}>
		{#if limits.length}
			<ul class="facts small">
				{#each limits as l (l)}<li>{l}</li>{/each}
			</ul>
		{:else}
			<p class="small muted facts-none">{t('plugins.limitsNone')}</p>
		{/if}
	</SettingsCard>
{/if}

<Sheet bind:open={reachOpen} size="sm" title={reaching ? t('plugins.reachTitle', { name: pluginName(reaching.id) }) : t('plugins.install')} description={t('plugins.reachAbout')}>
	<dl class="reach" data-testid="plugin-reach">
		{#each reach as r (r.label)}
			<dt>{r.label}</dt>
			<dd>
				{#if r.list.length}
					<ul>{#each r.list as v (v)}<li class="mono small">{v}</li>{/each}</ul>
				{:else}<span class="small muted">{t('plugins.nothing')}</span>{/if}
			</dd>
		{/each}
	</dl>
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (reachOpen = false)}>{t('plugins.cancel')}</Button>
		<Button variant="primary" disabled={!canInstall} onclick={install}>{t('plugins.install')}</Button>
	</div>
</Sheet>

<Sheet bind:open={removeOpen} size="sm" title={removing ? t('plugins.removeTitle', { name: pluginName(removing.id) }) : t('plugins.remove')} description={t('plugins.removeAbout')}>
	<div class="confirm-actions">
		<Button variant="plain" onclick={() => (removeOpen = false)}>{t('plugins.cancel')}</Button>
		<Button onclick={() => remove(true)}>{t('plugins.removeKeep')}</Button>
		<Button variant="destructive" onclick={() => remove(false)}>{t('plugins.removeAll')}</Button>
	</div>
</Sheet>

<style>
	.notice { margin-bottom: var(--s-4); }
	.pname { font-weight: var(--weight-semibold); }
	.state { margin-top: 4px; }
	.acts { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
	.facts { margin: var(--s-3) 0 0; padding-left: 1.2em; display: grid; gap: 4px; }
	.facts-none { margin-top: var(--s-3); }
	.reach { display: grid; gap: 4px; margin: 0; }
	.reach dt { font-weight: var(--weight-semibold); margin-top: var(--s-2); }
	.reach dd { margin: 0; }
	.reach ul { margin: 0; padding-left: 1.2em; }
	.confirm-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; margin-top: var(--s-4); }
	@media (max-width: 560px) {
		.rows > li { flex-wrap: wrap; }
		.acts { width: 100%; justify-content: flex-start; }
	}
</style>
