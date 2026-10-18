<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes, dateTime, folderHealth, tone } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/EmptyState.svelte';
	import type { LibraryFolder } from '#lib/types.ts';

	let folders = $state<LibraryFolder[]>([]);
	let loaded = $state(false);
	let error = $state('');

	let adding = $state(false);
	let path = $state('');
	let label = $state('');
	let kind = $state<'movie' | 'show' | 'music'>('movie');
	let formError = $state('');
	let saving = $state(false);

	let editing = $state<number | null>(null);
	let editLabel = $state('');
	let busyId = $state<number | null>(null);

	async function load() {
		try {
			folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	async function add(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		if (!path.trim()) {
			formError = 'Type the folder path, as HolyArrs sees it (inside the container if you use Docker).';
			return;
		}
		saving = true;
		try {
			const f = await api.post<LibraryFolder>('/library-folders', { path: path.trim(), kind, label: label.trim() });
			folders = [...folders, f];
			path = '';
			label = '';
			kind = 'movie';
			adding = false;
			toasts.push(f.health === 'ok' ? 'Library folder added. It is healthy.' : `Library folder added: ${folderHealth(f.health)}.`, f.health === 'ok' ? 'ok' : 'info');
		} catch (err) {
			formError = message(err);
		} finally {
			saving = false;
		}
	}

	async function recheck(f: LibraryFolder) {
		busyId = f.id;
		try {
			const n = await api.post<LibraryFolder>(`/library-folders/${f.id}/check`);
			folders = folders.map((x) => (x.id === f.id ? n : x));
			toasts.push(`Checked: ${folderHealth(n.health)}.`, n.health === 'ok' ? 'ok' : 'info');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busyId = null;
		}
	}

	async function saveLabel(f: LibraryFolder) {
		busyId = f.id;
		try {
			const n = await api.put<LibraryFolder>(`/library-folders/${f.id}`, { label: editLabel.trim() });
			folders = folders.map((x) => (x.id === f.id ? n : x));
			editing = null;
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busyId = null;
		}
	}

	async function remove(f: LibraryFolder) {
		if (!confirm(`Remove the Library folder ${f.path}? The files in it stay where they are.`)) return;
		busyId = f.id;
		try {
			await api.del(`/library-folders/${f.id}`);
			folders = folders.filter((x) => x.id !== f.id);
			toasts.push('Library folder removed.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busyId = null;
		}
	}
</script>

<svelte:head><title>Library folders · HolyArrs</title></svelte:head>

<PageHeader title="Library folders" subtitle="Where finished movies and show episodes are placed. HolyArrs checks each folder: that it exists, that it can write there, and whether files can be hardlinked from your Downloaders.">
	{#snippet actions()}
		{#if !adding}<Button icon="plus" variant="primary" type="button" onclick={() => (adding = true)}>Add a folder</Button>{/if}
	{/snippet}
</PageHeader>

{#if adding}
	<form class="sc-form" onsubmit={add} aria-label="New Library folder">
		<SettingsCard title="New Library folder">
			<div class="field">
				<label for="lf-kind">Holds</label>
				<select id="lf-kind" class="input" bind:value={kind}>
					<option value="movie">Movies</option>
					<option value="show">Shows</option>
					<option value="music">Music</option>
				</select>
				<span class="hint">Movies, shows and music each get their own folders.</span>
			</div>
			<div class="field">
				<label for="lf-path">Folder path</label>
				<input id="lf-path" class="input mono" bind:value={path} placeholder={kind === 'show' ? '/shows' : kind === 'music' ? '/music' : '/movies'} autocomplete="off" spellcheck="false" />
			</div>
			<div class="field">
				<label for="lf-label">Name <span class="muted">(optional)</span></label>
				<input id="lf-label" class="input" bind:value={label} placeholder={kind === 'show' ? 'Shows' : kind === 'music' ? 'Music' : 'Movies'} maxlength="64" />
			</div>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={saving} aria-busy={saving}>Add and check</Button>
				<Button variant="plain" type="button" onclick={() => ((adding = false), (formError = ''))}>Cancel</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<div class="sk" style="height: 120px"></div>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if folders.length === 0 && !adding}
	<EmptyState icon="folder" title="No Library folder yet">
		<p>Add the folder where your movies should live, for example <code>/movies</code>, and one for shows, for example <code>/shows</code>. HolyArrs needs one before it can add a movie or a show.</p>
		{#snippet actions()}<Button variant="primary" type="button" onclick={() => (adding = true)}>Add a folder</Button>{/snippet}
	</EmptyState>
{:else if folders.length}
	<ul class="card rows">
		{#each folders as f (f.id)}
			<li class="folder">
				<span class="ic"><Icon name="folder" /></span>
				<div class="grow">
					{#if editing === f.id}
						<form class="row" onsubmit={(e) => (e.preventDefault(), saveLabel(f))}>
							<label class="sr-only" for="ed-{f.id}">Name</label>
							<input id="ed-{f.id}" class="input" style="max-width: 260px" bind:value={editLabel} maxlength="64" />
							<Button type="submit" disabled={busyId === f.id}>Save</Button>
							<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
						</form>
					{:else}
						<div class="name">{f.label || f.path}</div>
						<div class="xs muted" data-testid="folder-kind">{f.kind === 'show' ? 'For shows' : f.kind === 'music' ? 'For music' : 'For movies'}</div>
					{/if}
					<div class="mono xs muted ellipsis">{f.path} · {bytes(f.free_space)} free</div>
					<div class="health">
						<span class="chip {tone(f.health)}">{folderHealth(f.health)}</span>
						{#if f.health_reason}<span class="xs muted">{f.health_reason}</span>{/if}
						{#if f.health_checked_at}<span class="xs muted">· checked {dateTime(f.health_checked_at)}</span>{/if}
					</div>
				</div>
				<div class="acts">
					<IconButton icon="refresh" label="Check {f.path} again" type="button" title="Check again" disabled={busyId === f.id} onclick={() => recheck(f)} />
					<Button variant="plain" type="button" onclick={() => ((editing = f.id), (editLabel = f.label))}>Rename</Button>
					<IconButton icon="trash" label="Remove {f.path}" type="button" title="Remove" disabled={busyId === f.id} onclick={() => remove(f)} />
				</div>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.folder {
		align-items: flex-start !important;
	}
	.folder > .grow {
		flex-basis: 0;
	}
	.ic {
		width: 40px;
		height: 40px;
		display: grid;
		place-items: center;
		border-radius: var(--r-btn);
		background: var(--surface-3);
		flex: none;
	}
	.name {
		font-weight: var(--weight-semibold);
	}
	.health {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-2);
		margin-top: var(--s-2);
	}
	.acts {
		display: flex;
		align-items: center;
		gap: var(--s-1);
	}
	code {
		font-family: var(--font-mono);
	}
	@media (max-width: 600px) {
		.folder {
			flex-wrap: wrap;
		}
		.acts {
			width: 100%;
			justify-content: flex-end;
		}
	}
</style>
