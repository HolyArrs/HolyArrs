<script lang="ts">
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
			toasts.push(`${form.name} saved`, 'ok');
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
			results = { ...results, [m.id]: { ok: true, text: libs ? '' : (r.found ?? '').trim().split(/\s+/).pop() ?? '', hover: r.found ?? '', extra: libs ? `${libs} ${libs === 1 ? 'library' : 'libraries'}` : '' } };
		} catch (err) {
			results = { ...results, [m.id]: { ok: false, text: message(err) } };
		} finally {
			testing = 0;
			await load();
		}
	}

	async function remove(m: MediaServerView) {
		if (!confirm(`Remove ${m.name}? It stops getting updates.`)) return;
		try {
			await api.del(`/media-servers/${m.id}`);
			await load();
		} catch (err) {
			toasts.push("Couldn't remove Media server", 'fail', { body: message(err) });
		}
	}

	function moments(m: MediaServerView): string {
		const on = [m.on_import && 'imports', m.on_rename && 'renames', m.on_delete && 'deletes'].filter(Boolean);
		return on.length ? 'Told about ' + on.join(', ') : 'Told about nothing';
	}
</script>

<svelte:head><title>Media servers · HolyArrs</title></svelte:head>

<PageHeader title="Media servers">
	{#snippet actions()}
		{#if !editing && servers.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>Add a media server</Button>{/if}
	{/snippet}
</PageHeader>

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? 'Add a media server' : `Change ${editing.name}`}>
		<SettingsCard title={isNew ? 'Add a media server' : `Change ${editing.name}`}>
			<Field id="ms-name" label="Name" style="margin-top: var(--s-4)">
				<input id="ms-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			{#if isNew}
				<Field id="ms-kind" label="Kind">
					<select id="ms-kind" class="input" bind:value={form.kind}>
						<option value="plex">Plex</option>
						<option value="jellyfin">Jellyfin</option>
						<option value="emby">Emby</option>
					</select>
				</Field>
			{/if}
			<Field id="ms-url" label="Address">
				<input id="ms-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder={portHint[form.kind]} />
			</Field>
			<Field id="ms-token" label={form.kind === 'plex' ? 'Plex token' : 'API key'}>
				<input id="ms-token" class="input mono" type="password" bind:value={form.token} autocomplete="new-password" />
				<span class="hint">
					{form.kind === 'plex' ? 'The token of your Plex server.' : 'Make one in the server: Dashboard, API keys.'} Kept encrypted and never shown again;
					leave empty to keep the stored one. A new address needs it typed again.
				</span>
			</Field>
			<fieldset class="when">
				<legend class="label">Update it after</legend>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_import} /><span><b>An import</b></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_rename} /><span><b>A rename</b></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_delete} /><span><b>A delete</b><br /><span class="hint">Only when files were removed</span></span></label>
			</fieldset>
			<div class="two">
				<Field id="ms-from" label="HolyArrs path">
					<input id="ms-from" class="input mono" bind:value={form.path_from} placeholder="/data/movies" autocomplete="off" />
				</Field>
				<Field id="ms-to" label="Same folder on the server">
					<input id="ms-to" class="input mono" bind:value={form.path_to} placeholder="/movies" autocomplete="off" />
				</Field>
			</div>
			<p class="hint">Only if the server sees your files under another path</p>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>Reach the local network</b></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.enabled} />
				<span><b>Send updates</b></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim()} aria-busy={busy}>Save</Button>
				<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
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
	<EmptyState icon="tv" title="No media servers" text="New titles show up in Plex, Jellyfin or Emby by themselves." action={{ label: 'Add a media server', onclick: startAdd, icon: 'plus' }} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<ul class="list">
		{#each servers as m (m.id)}
			<li class="card" data-testid="media-server">
				<div class="head">
					<h2 class="h3">{m.name}</h2>
					<span class="chip">{kindWord[m.kind]}</span>
					{#if !m.enabled}<span class="chip">Off</span>{/if}
					{#if results[m.id]}<TestTag ok={results[m.id].ok} text={results[m.id].text} hover={results[m.id].hover} app={kindWord[m.kind]} extra={results[m.id].extra} testid="media-test" />{/if}
				</div>
				<p class="small muted mono addr">{m.url}</p>
				<p class="small muted">{moments(m)}{m.path_from ? ` · ${m.path_from} is ${m.path_to} there` : ''}</p>
				{#if results[m.id] && !results[m.id].ok}
					<p class="small bad-text" data-testid="media-result">{results[m.id].text}</p>
				{:else if m.last_error}
					<p class="small bad-text" data-testid="media-result">Last try failed: {m.last_error}</p>
				{:else if m.last_ok_at}
					<p class="small muted" data-testid="media-result">Last checked {ago(m.last_ok_at)}</p>
				{/if}
				<div class="row actions">
					<Button type="button" disabled={testing === m.id} aria-busy={testing === m.id} onclick={() => test(m)}>Test</Button>
					<Button variant="plain" type="button" onclick={() => startChange(m)}>Change</Button>
					<Button variant="plain" type="button" onclick={() => remove(m)} aria-label="Remove {m.name}">Remove</Button>
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
