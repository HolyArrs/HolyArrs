<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import type { MediaServerView } from '#lib/mediaservers/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	type Kind = MediaServerView['kind'];
	const kindWord: Record<Kind, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };
	const portHint: Record<Kind, string> = { plex: 'http://192.168.1.10:32400', jellyfin: 'http://192.168.1.10:8096', emby: 'http://192.168.1.10:8096' };

	let servers = $state<MediaServerView[]>([]);
	let loaded = $state(false);
	let hasData = $state(false);
	let error = $state('');
	let editing = $state<MediaServerView | null>(null);
	let isNew = $state(false);
	const blank = () => ({
		name: '',
		kind: 'jellyfin' as Kind,
		url: '',
		token: '',
		enabled: true,
		on_import: true,
		on_rename: true,
		on_delete: true,
		path_from: '',
		path_to: '',
		allow_local_network: true
	});
	let form = $state(blank());
	let busy = $state(false);
	let formError = $state('');
	let testing = $state(0);
	// The last Test of each server, shown as a tag on its card.
	let results = $state<Record<number, { ok: boolean; text: string; extra?: string; hover?: string }>>({});

	async function load() {
		try {
			servers = (await api.get<{ media_servers: MediaServerView[] }>('/media-servers')).media_servers;
			hasData = true;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	function startAdd() {
		isNew = true;
		editing = { ...blank(), id: 0 } as MediaServerView;
		form = blank();
		formError = '';
	}

	function startChange(m: MediaServerView) {
		isNew = false;
		editing = m;
		form = { ...m, token: '' }; // Empty keeps the stored token.
		formError = '';
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		formError = '';
		try {
			if (isNew) await api.post('/media-servers', form);
			else {
				await api.put(`/media-servers/${editing!.id}`, form);
				const id = editing!.id;
				results = Object.fromEntries(Object.entries(results).filter(([k]) => Number(k) !== id));
			}
			toasts.push(t('settings.mediaServers.saved', { name: form.name }), 'ok');
			editing = null;
			await load();
		} catch (err) {
			formError = message(err);
		} finally {
			busy = false;
		}
	}

	async function test(m: MediaServerView) {
		testing = m.id;
		try {
			const r = await api.post<{ found: string }>(`/media-servers/${m.id}/test`);
			// Plex names its libraries instead of a version: the tag counts them.
			const libs = /^Libraries:/i.test(r.found ?? '') ? r.found.slice(10).split(',').filter((x) => x.trim()).length : 0;
			// Jellyfin and Emby answer "<server name> <version>": only the last word is read for the version (a name like "HTPC 2.0" is not).
			results = { ...results, [m.id]: { ok: true, text: libs ? '' : (r.found ?? '').trim().split(/\s+/).pop() ?? '', hover: r.found ?? '', extra: libs ? t(libs === 1 ? 'settings.mediaServers.libraryOne' : 'settings.mediaServers.libraryOther', { n: libs }) : '' } };
		} catch (err) {
			results = { ...results, [m.id]: { ok: false, text: message(err) } };
		} finally {
			testing = 0;
			await load();
		}
	}

	async function remove(m: MediaServerView) {
		if (!confirm(t('settings.mediaServers.confirmRemove', { name: m.name }))) return;
		try {
			await api.del(`/media-servers/${m.id}`);
			await load();
		} catch (err) {
			toasts.push(t('settings.mediaServers.couldntRemove'), 'fail', { body: message(err) });
		}
	}

	function moments(m: MediaServerView): string {
		const on = [m.on_import && t('settings.mediaServers.imports'), m.on_rename && t('settings.mediaServers.renames'), m.on_delete && t('settings.mediaServers.deletes')].filter(Boolean);
		return on.length ? t('settings.mediaServers.toldAbout', { list: on.join(', ') }) : t('settings.mediaServers.toldNothing');
	}
</script>

<svelte:head><title>{t('settings.mediaServers.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.mediaServers.title')}>
	{#snippet actions()}
		{#if !editing && servers.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>{t('settings.mediaServers.add')}</Button>{/if}
	{/snippet}
</PageHeader>

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? t('settings.mediaServers.add') : t('settings.mediaServers.change', { name: editing.name })}>
		<SettingsCard title={isNew ? t('settings.mediaServers.add') : t('settings.mediaServers.change', { name: editing.name })}>
			<Field id="ms-name" label={t('settings.mediaServers.name')} style="margin-top: var(--s-4)">
				<input id="ms-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			{#if isNew}
				<Field id="ms-kind" label={t('settings.mediaServers.kind')}>
					<select id="ms-kind" class="input" bind:value={form.kind}>
						<option value="plex">Plex</option>
						<option value="jellyfin">Jellyfin</option>
						<option value="emby">Emby</option>
					</select>
				</Field>
			{/if}
			<Field id="ms-url" label={t('settings.mediaServers.address')}>
				<input id="ms-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder={portHint[form.kind]} />
			</Field>
			<Field id="ms-token" label={form.kind === 'plex' ? t('settings.mediaServers.plexTokenLabel') : t('settings.mediaServers.apiKeyLabel')}>
				<input id="ms-token" class="input mono" type="password" bind:value={form.token} autocomplete="new-password" />
				<span class="hint">
					{form.kind === 'plex' ? t('settings.mediaServers.plexTokenHint') : t('settings.mediaServers.apiKeyHint')}
				</span>
			</Field>
			<fieldset class="when">
				<legend class="label">{t('settings.mediaServers.after')}</legend>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_import} /><span><b>{t('settings.mediaServers.anImport')}</b></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_rename} /><span><b>{t('settings.mediaServers.aRename')}</b></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_delete} /><span><b>{t('settings.mediaServers.aDelete')}</b><br /><span class="hint">{t('settings.mediaServers.aDeleteHint')}</span></span></label>
			</fieldset>
			<div class="two">
				<Field id="ms-from" label={t('settings.mediaServers.pathFrom')}>
					<input id="ms-from" class="input mono" bind:value={form.path_from} placeholder="/data/movies" autocomplete="off" />
				</Field>
				<Field id="ms-to" label={t('settings.mediaServers.pathTo')}>
					<input id="ms-to" class="input mono" bind:value={form.path_to} placeholder="/movies" autocomplete="off" />
				</Field>
			</div>
			<p class="hint">{t('settings.mediaServers.pathHint')}</p>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>{t('settings.mediaServers.local')}</b></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.enabled} />
				<span><b>{t('settings.mediaServers.send')}</b></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim()} aria-busy={busy}>{t('settings.mediaServers.save')}</Button>
				<Button variant="plain" type="button" onclick={() => (editing = null)}>{t('settings.mediaServers.cancel')}</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={2} />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if servers.length === 0 && !editing}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<EmptyState icon="tv" title={t('settings.mediaServers.none')} text={t('settings.mediaServers.noneText')} action={{ label: t('settings.mediaServers.add'), onclick: startAdd, icon: 'plus' }} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<ul class="list">
		{#each servers as m (m.id)}
			<li class="card" data-testid="media-server">
				<div class="head">
					<h2 class="h3">{m.name}</h2>
					<span class="chip">{kindWord[m.kind]}</span>
					{#if !m.enabled}<span class="chip">{t('settings.mediaServers.off')}</span>{/if}
					{#if results[m.id]}<TestTag ok={results[m.id].ok} text={results[m.id].text} hover={results[m.id].hover} app={kindWord[m.kind]} extra={results[m.id].extra} testid="media-test" />{/if}
				</div>
				<p class="small muted mono addr">{m.url}</p>
				<p class="small muted">{moments(m)}{m.path_from ? ` · ${t('settings.mediaServers.pathMap', { from: m.path_from, to: m.path_to })}` : ''}</p>
				{#if results[m.id] && !results[m.id].ok}
					<p class="small bad-text" data-testid="media-result">{results[m.id].text}</p>
				{:else if m.last_error}
					<p class="small bad-text" data-testid="media-result">{t('settings.mediaServers.lastFailed', { error: m.last_error })}</p>
				{:else if m.last_ok_at}
					<p class="small muted" data-testid="media-result">{t('settings.mediaServers.lastChecked', { ago: ago(m.last_ok_at) })}</p>
				{/if}
				<div class="row actions">
					<Button type="button" disabled={testing === m.id} aria-busy={testing === m.id} onclick={() => test(m)}>{t('settings.mediaServers.test')}</Button>
					<Button variant="plain" type="button" onclick={() => startChange(m)}>{t('settings.mediaServers.changeButton')}</Button>
					<Button variant="plain" type="button" onclick={() => remove(m)} aria-label={t('settings.mediaServers.removeName', { name: m.name })}>{t('settings.mediaServers.remove')}</Button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.list {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.list .card + .card {
		margin-top: 0;
	}
	.head {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		flex-wrap: wrap;
	}
	.addr {
		overflow-wrap: anywhere;
	}
	.actions {
		margin-top: var(--s-3);
		flex-wrap: wrap;
	}
	.when {
		border: 0;
		padding: 0;
		margin: var(--s-5) 0 0;
	}
	.two {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.two :global(.field + .field) {
		margin-top: 0;
	}
	.bad-text {
		color: var(--fail-text);
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
	}
</style>
