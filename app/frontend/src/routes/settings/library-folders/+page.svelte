<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { bytes, dateTime, folderHealth, tone } from '#lib/core/format.ts';
	import type { LibraryFolder } from '#lib/core/types.ts';

	// Grouped list with a plain line icon per folder.
	const kindWord = (k: string) => (k === 'show' ? 'Shows' : k === 'music' ? 'Music' : 'Movies');
	const kindIcon = (k: string) => (k === 'show' ? 'tv' : k === 'music' ? 'music' : 'film') as 'tv' | 'music' | 'film';
	// The last two parts of a path: enough to tell folders apart; the full path is in the tooltip.
	const shortPath = (p: string) => { const parts = p.split(/[\\/]/).filter(Boolean); return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : p; };
	const freeNum = (n: number) => bytes(n).split(' ');
	const byKind = $derived(['movie', 'show', 'music'].map((k) => ({ k, items: folders.filter((f) => (f.kind || 'movie') === k) })).filter((g) => g.items.length));

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

	// Extra import folders: where Import files by hand may also read.
	let extraText = $state('');
	let extraLoaded = $state(false);
	let extraError = $state('');
	let extraSaving = $state(false);
	const canSaveExtra = $derived(session.can('users'));

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
	async function loadExtra() {
		try {
			extraText = (await api.get<{ extra_folders: string[] }>('/settings/import-folders')).extra_folders.join('\n');
			extraLoaded = true;
		} catch {
			extraLoaded = false; // No settings permission: the card stays hidden.
		}
	}
	onMount(() => {
		void load();
		void loadExtra();
	});

	async function saveExtra(e: SubmitEvent) {
		e.preventDefault();
		extraError = '';
		extraSaving = true;
		try {
			const lines = extraText.split('\n').map((l) => l.trim()).filter(Boolean);
			extraText = (await api.put<{ extra_folders: string[] }>('/settings/import-folders', { extra_folders: lines })).extra_folders.join('\n');
			toasts.push('Import folders saved', 'ok');
		} catch (err) {
			extraError = message(err);
		} finally {
			extraSaving = false;
		}
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		if (!path.trim()) {
			formError = 'The path inside the container, if you use Docker.';
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
			toasts.push('Library folder added', f.health === 'ok' ? 'ok' : 'info', { body: f.health === 'ok' ? undefined : folderHealth(f.health) });
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
			toasts.push('Folder checked', n.health === 'ok' ? 'ok' : 'info', { body: folderHealth(n.health) });
		} catch (err) {
			toasts.push("Couldn't check folder", 'fail', { body: message(err) });
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
			toasts.push("Couldn't save", 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}

	async function remove(f: LibraryFolder) {
		if (!confirm(`Remove ${f.path}? Files stay on disk.`)) return;
		busyId = f.id;
		try {
			await api.del(`/library-folders/${f.id}`);
			folders = folders.filter((x) => x.id !== f.id);
			toasts.push('Library folder removed', 'ok');
		} catch (err) {
			toasts.push("Couldn't remove folder", 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}
</script>

<svelte:head><title>Library folders · HolyArrs</title></svelte:head>

<PageHeader title="Library folders">
	{#snippet actions()}
		{#if !adding && folders.length}<Button icon="plus" variant="primary" type="button" onclick={() => (adding = true)}>Add a folder</Button>{/if}
	{/snippet}
</PageHeader>

{#if adding}
	<form class="sc-form" onsubmit={add} aria-label="Add a Library folder">
		<SettingsCard title="Add a Library folder">
			<Field id="lf-kind" label="Holds" hint="Each kind needs its own folder">
				<select id="lf-kind" class="input" bind:value={kind}>
					<option value="movie">Movies</option>
					<option value="show">Shows</option>
					<option value="music">Music</option>
				</select>
			</Field>
			<Field id="lf-path" label="Folder path">
				<input id="lf-path" class="input mono" bind:value={path} placeholder={kind === 'show' ? '/shows' : kind === 'music' ? '/music' : '/movies'} autocomplete="off" spellcheck="false" />
			</Field>
			<div class="field">
				<label for="lf-label">Name <span class="muted">(optional)</span></label>
				<input id="lf-label" class="input" bind:value={label} placeholder={kind === 'show' ? 'Shows' : kind === 'music' ? 'Music' : 'Movies'} maxlength="64" />
			</div>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={saving} aria-busy={saving}>Add</Button>
				<Button variant="plain" type="button" onclick={() => ((adding = false), (formError = ''))}>Cancel</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={2} />
{:else if error}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if folders.length === 0 && !adding}
	<EmptyState icon="folder" title="No Library folders" text="Add a folder for movies (like /movies) and one for shows." action={{ label: 'Add a folder', icon: 'plus', onclick: () => (adding = true) }} />
{:else if folders.length}
	{#snippet row(f: LibraryFolder)}
		<li class="folder k-{f.kind || 'movie'}" title="{f.path}{f.health_checked_at ? ` · checked ${dateTime(f.health_checked_at)}` : ''}">
			<span class="ic"><Icon name="folder" size={20} /></span>
			<div class="grow">
				{#if editing === f.id}
					<form class="ed" onsubmit={(e) => (e.preventDefault(), saveLabel(f))}>
						<label class="sr-only" for="ed-{f.id}">Name</label>
						<input id="ed-{f.id}" class="input" bind:value={editLabel} maxlength="64" />
						<Button type="submit" size="sm" disabled={busyId === f.id}>Save</Button>
						<Button variant="plain" size="sm" type="button" onclick={() => (editing = null)}>Cancel</Button>
					</form>
				{:else}
					<div class="top"><span class="name">{f.label || f.path}</span><span class="kind" data-testid="folder-kind">{f.kind === 'show' ? 'For shows' : f.kind === 'music' ? 'For music' : 'For movies'}</span></div>
				{/if}
				<div class="where">{shortPath(f.path)}</div>
				{#if f.health_reason && tone(f.health) !== 'ok'}<div class="why {tone(f.health)}">{f.health_reason}</div>{/if}
			</div>
			<div class="free"><b>{freeNum(f.free_space)[0]}</b><small>{freeNum(f.free_space)[1] ?? ''} free</small></div>
			<span class="st {tone(f.health)}"><i></i><span>{folderHealth(f.health)}</span></span>
			<div class="acts">
				<IconButton icon="refresh" label="Check {f.path} again" type="button" tooltip={false} disabled={busyId === f.id} onclick={() => recheck(f)} />
				<Button variant="plain" size="sm" type="button" onclick={() => ((editing = f.id), (editLabel = f.label))}>Rename</Button>
				<IconButton icon="trash" label="Remove {f.path}" type="button" tooltip={false} disabled={busyId === f.id} onclick={() => remove(f)} />
			</div>
		</li>
	{/snippet}
	<div class="fl-groups">
		{#each byKind as g (g.k)}
			<section class="fl-group">
				<h3><Icon name={kindIcon(g.k)} size={15} />{kindWord(g.k)}<span class="cnt">{g.items.length}</span></h3>
				<ul class="fl" data-view="3">{#each g.items as f (f.id)}{@render row(f)}{/each}</ul>
			</section>
		{/each}
	</div>
{/if}

{#if extraLoaded}
	<form class="sc-form extra" onsubmit={saveExtra} aria-label="Extra import folders">
		<SettingsCard id="import-folders" title="Extra import folders" description="Import can also read from these">
			<Field id="if-paths" label="Folders" hint="One path per line, e.g. /data/imports. Up to 32." error={extraError}>
				<textarea id="if-paths" class="input mono" rows="3" bind:value={extraText} placeholder="/data/imports" autocomplete="off" spellcheck="false" readonly={!canSaveExtra}></textarea>
			</Field>
			{#if canSaveExtra}
				<div class="row" style="margin-top: var(--s-5)">
					<Button variant="primary" type="submit" disabled={extraSaving} aria-busy={extraSaving}>Save</Button>
				</div>
			{:else}
				<p class="hint">Owner and admins only</p>
			{/if}
		</SettingsCard>
	</form>
{/if}

<style>
	.extra {
		margin-top: var(--s-5);
	}
	textarea.input {
		height: auto;
		padding-top: 10px;
		width: 100%;
		min-width: 0;
	}
	/* The folder list: shared pieces. */
	.fl { list-style: none; margin: 0; padding: 0; }
	.folder { position: relative; display: grid; grid-template-columns: 40px minmax(0, 1fr) auto auto auto; align-items: center; column-gap: 16px; min-width: 0; }
	/* Kind tiles: glossy, the same family as the Settings icons. */
	.ic { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 11px; color: #fff; background: linear-gradient(160deg, #5aa4ff, #2563eb); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.35), 0 4px 10px -4px rgb(37 99 235 / 0.55); }
	.k-show .ic { background: linear-gradient(160deg, #b18cff, #7c3aed); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.35), 0 4px 10px -4px rgb(124 58 237 / 0.55); }
	.k-music .ic { background: linear-gradient(160deg, #ff8fb1, #e11d74); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.35), 0 4px 10px -4px rgb(225 29 116 / 0.55); }
	.grow { min-width: 0; }
	.top { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
	.name { font: 600 15px/1.3 var(--font-ui); letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.kind { flex: none; color: var(--text-muted); font: 500 12px/1 var(--font-ui); }
	.where { margin-top: 3px; color: var(--text-muted); font: 400 13px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.why { margin-top: 3px; font: 500 12px/1.3 var(--font-ui); color: var(--fail-text, var(--fail)); }
	.why.wait { color: var(--wait-text, var(--wait)); }
	.free { display: flex; align-items: baseline; gap: 4px; justify-content: flex-end; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.free b { font: 600 15px/1 var(--font-ui); }
	.free small { color: var(--text-muted); font: 400 12px/1 var(--font-ui); }
	.st { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 500 12px/1 var(--font-ui); white-space: nowrap; }
	.st i { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.st.wait i { background: var(--wait); box-shadow: 0 0 0 3px color-mix(in srgb, var(--wait) 22%, transparent); }
	.st.fail i { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.acts { display: flex; align-items: center; gap: 2px; }
	.ed { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
	.ed .input { max-width: 240px; min-height: 36px; }
	/* Glass sheet shared by the folder rows. */
	.fl {
		border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); border-radius: 20px;
		background: color-mix(in srgb, var(--surface-1) 60%, transparent);
		-webkit-backdrop-filter: blur(28px) saturate(1.6); backdrop-filter: blur(28px) saturate(1.6);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 24px 50px -32px rgb(0 0 0 / 0.45);
		overflow: hidden;
	}

	/* Grouped: Movies, Shows and Music each get their own sheet, one line per folder. */
	.fl .folder { padding: 14px 16px 14px 18px; min-height: 72px; }
	.fl .folder + .folder { box-shadow: inset 0 1px 0 var(--line); }
	.fl .folder:hover { background: color-mix(in srgb, var(--text) 3%, transparent); }
	.fl-groups { display: flex; flex-direction: column; gap: 18px; }
	.fl-group h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 8px 6px; color: var(--text-muted); font: 600 13px/1 var(--font-ui); letter-spacing: 0.02em; }
	.fl-group h3 .cnt { padding: 3px 8px; border-radius: var(--r-pill); background: var(--surface-2); font-size: 11px; font-variant-numeric: tabular-nums; }
	.kind { display: none; }

	@media (max-width: 760px) {
		.folder { grid-template-columns: 40px minmax(0, 1fr) auto; grid-template-areas: 'ic grow st' 'ic free acts'; row-gap: 6px; }
		.folder .ic { grid-area: ic; align-self: start; } .folder .grow { grid-area: grow; } .folder .st { grid-area: st; justify-self: end; }
		.folder .free { grid-area: free; justify-content: flex-start; } .folder .acts { grid-area: acts; justify-self: end; opacity: 1 !important; }
		.folder .kind { display: none; }
		.folder .name { white-space: normal; }
	}

	/* The folder mark: a plain line icon, no tile. */
	.fl .folder { grid-template-columns: 24px minmax(0, 1fr) auto auto auto; }
	.ic, .k-show .ic, .k-music .ic { width: 24px; height: 24px; background: none; box-shadow: none; border-radius: 0; color: color-mix(in srgb, var(--text) 55%, transparent); }
	@media (max-width: 760px) { .fl .folder { grid-template-columns: 24px minmax(0, 1fr) auto; } }
</style>
