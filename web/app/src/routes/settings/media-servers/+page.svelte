<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
	import { ago } from '#lib/format.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import type { MediaServerView } from '#lib/types.ts';

	type Kind = MediaServerView['kind'];
	const kindWord: Record<Kind, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };
	const portHint: Record<Kind, string> = { plex: 'http://192.168.1.10:32400', jellyfin: 'http://192.168.1.10:8096', emby: 'http://192.168.1.10:8096' };

	let servers = $state<MediaServerView[]>([]);
	let loaded = $state(false);
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

	async function load() {
		try {
			servers = (await api.get<{ media_servers: MediaServerView[] }>('/media-servers')).media_servers;
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
		form = { ...m, token: '' }; // empty keeps the stored token
		formError = '';
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		formError = '';
		try {
			if (isNew) await api.post('/media-servers', form);
			else await api.put(`/media-servers/${editing!.id}`, form);
			toasts.push(`${form.name} saved.`, 'ok');
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
			toasts.push(`${m.name} answered${r.found ? ': ' + r.found : ''}.`, 'ok');
		} catch (err) {
			toasts.push(`${m.name}: ${message(err)}`, 'fail');
		} finally {
			testing = 0;
			await load();
		}
	}

	async function remove(m: MediaServerView) {
		if (!confirm(`Remove ${m.name}? HolyArrs stops telling it about changes.`)) return;
		try {
			await api.del(`/media-servers/${m.id}`);
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	function moments(m: MediaServerView): string {
		const on = [m.on_import && 'imports', m.on_rename && 'renames', m.on_delete && 'deletes'].filter(Boolean);
		return on.length ? 'Told about ' + on.join(', ') : 'Told about nothing';
	}
</script>

<svelte:head><title>Media servers · HolyArrs</title></svelte:head>

<PageHeader title="Media servers" subtitle="After HolyArrs imports, renames or deletes a movie's files, it tells Plex, Jellyfin or Emby to look at that movie's folder again, so the change shows up without waiting for the server's own scan. Nothing is read back.">
	{#snippet actions()}
		{#if !editing}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>Add a media server</Button>{/if}
	{/snippet}
</PageHeader>

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? 'Add a media server' : `Change ${editing.name}`}>
		<SettingsCard title={isNew ? 'Add a media server' : `Change ${editing.name}`}>
			<div class="field" style="margin-top: var(--s-4)">
				<label for="ms-name">Name</label>
				<input id="ms-name" class="input" bind:value={form.name} maxlength="80" required />
			</div>
			{#if isNew}
				<div class="field">
					<label for="ms-kind">Kind</label>
					<select id="ms-kind" class="input" bind:value={form.kind}>
						<option value="plex">Plex</option>
						<option value="jellyfin">Jellyfin</option>
						<option value="emby">Emby</option>
					</select>
				</div>
			{/if}
			<div class="field">
				<label for="ms-url">Address</label>
				<input id="ms-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder={portHint[form.kind]} />
			</div>
			<div class="field">
				<label for="ms-token">{form.kind === 'plex' ? 'Plex token' : 'API key'}</label>
				<input id="ms-token" class="input mono" type="password" bind:value={form.token} autocomplete="new-password" />
				<span class="hint">
					{form.kind === 'plex' ? 'The token of your Plex server.' : 'Make one in the server: Dashboard, API keys.'} Kept encrypted and never shown again;
					leave empty to keep the stored one. A new address needs it typed again.
				</span>
			</div>
			<fieldset class="when">
				<legend class="label">Tell it after</legend>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_import} /><span><b>An import</b><br /><span class="hint">A new file or a better copy.</span></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_rename} /><span><b>A rename</b></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_delete} /><span><b>A delete</b><br /><span class="hint">Only when the delete removed files.</span></span></label>
			</fieldset>
			<div class="two">
				<div class="field">
					<label for="ms-from">HolyArrs path</label>
					<input id="ms-from" class="input mono" bind:value={form.path_from} placeholder="/data/movies" autocomplete="off" />
				</div>
				<div class="field">
					<label for="ms-to">Same folder on the server</label>
					<input id="ms-to" class="input mono" bind:value={form.path_to} placeholder="/movies" autocomplete="off" />
				</div>
			</div>
			<p class="hint">Only when the server sees your movies under another path (for example in another container). Leave both empty otherwise.</p>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>May reach the local network</b><br /><span class="hint">On by default: media servers are usually at home.</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.enabled} />
				<span><b>On</b><br /><span class="hint">Off keeps it but tells it nothing.</span></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim()} aria-busy={busy}>Save</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !loaded}
	<div class="sk" style="height: 120px"></div>
{:else if servers.length === 0 && !editing}
	<EmptyState icon="film" title="No media servers yet">
		<p>Add your Plex, Jellyfin or Emby server and new movies show up there by themselves.</p>
	</EmptyState>
{:else}
	<ul class="list">
		{#each servers as m (m.id)}
			<li class="card" data-testid="media-server">
				<div class="head">
					<h2 class="h3">{m.name}</h2>
					<span class="chip">{kindWord[m.kind]}</span>
					{#if !m.enabled}<span class="chip">Off</span>{/if}
				</div>
				<p class="small muted mono addr">{m.url}</p>
				<p class="small muted">{moments(m)}{m.path_from ? ` · ${m.path_from} is ${m.path_to} there` : ''}</p>
				{#if m.last_error}
					<p class="small bad-text" data-testid="media-result">Last try failed: {m.last_error}</p>
				{:else if m.last_ok_at}
					<p class="small muted" data-testid="media-result">Last answered {ago(m.last_ok_at)}</p>
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
	.two .field + .field {
		margin-top: 0;
	}
	.bad-text {
		color: var(--fail);
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
	}
</style>
