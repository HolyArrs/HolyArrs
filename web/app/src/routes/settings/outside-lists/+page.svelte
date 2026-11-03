<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { live } from '#lib/live.svelte.ts';
	import { dateTime } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { every, listKinds, listLink } from '#lib/lists.ts';
	import type { LibraryFolder, NeverAddEntry, OutsideList, PickRulesView } from '#lib/types.ts';

	let lists = $state<OutsideList[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let never = $state<NeverAddEntry[]>([]);
	let loaded = $state(false);
	let hasData = $state(false);
	let error = $state('');
	let busyId = $state<number | null>(null);

	// The list form, for a new list (editId null) or a change.
	let formOpen = $state(false);
	let editId = $state<number | null>(null);
	let name = $state('');
	let kind = $state('tmdb_list');
	let ref = $state('');
	let folderId = $state(0);
	let rulesId = $state(0);
	let keep = $state(true);
	let searchOnAdd = $state(false);
	let enabled = $state(true);
	let hours = $state(12);
	let formError = $state('');
	let saving = $state(false);

	// Trakt app key
	let traktMasked = $state('');
	let traktKey = $state('');
	let traktError = $state('');
	let traktSaving = $state(false);

	// Never-add list
	let naId = $state('');
	let naName = $state('');
	let naReason = $state('');
	let naError = $state('');

	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	const movieRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('movie')));
	const hasTrakt = $derived(lists.some((l) => l.kind === 'trakt_list'));

	async function loadLists() {
		lists = (await api.get<{ outside_lists: OutsideList[] }>('/outside-lists')).outside_lists;
	}

	async function load() {
		try {
			await loadLists();
			const [f, r, n] = await Promise.all([
				api.get<{ library_folders: LibraryFolder[] }>('/library-folders'),
				api.get<{ pick_rules: PickRulesView[] }>('/pick-rules'),
				api.get<{ never_add: NeverAddEntry[] }>('/never-add')
			]);
			folders = f.library_folders;
			rules = r.pick_rules;
			never = n.never_add;
			traktMasked = (await api.get<{ key: string }>('/settings/trakt-key')).key;
			hasData = true;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	onMount(() => {
		void load();
		const offs = [live.on('outside_list', () => loadLists().catch(() => {})), live.on('resync', () => load())];
		return () => offs.forEach((off) => off());
	});

	function openForm(l?: OutsideList) {
		formError = '';
		editId = l?.id ?? null;
		name = l?.name ?? '';
		kind = l?.kind ?? 'tmdb_list';
		ref = l?.ref ?? '';
		folderId = l?.library_folder_id ?? movieFolders[0]?.id ?? 0;
		rulesId = l?.pick_rules_id ?? 0;
		keep = l?.keep_up_to_date ?? true;
		searchOnAdd = l?.search_on_add ?? false;
		enabled = l?.enabled ?? true;
		hours = l?.sync_every_hours ?? 12;
		formOpen = true;
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		saving = true;
		const body = {
			name: name.trim(),
			kind,
			ref: ref.trim(),
			library_folder_id: folderId,
			pick_rules_id: rulesId,
			keep_up_to_date: keep,
			search_on_add: searchOnAdd,
			enabled,
			sync_every_hours: Number(hours) || 12
		};
		try {
			if (editId === null) await api.post<OutsideList>('/outside-lists', body);
			else await api.put<OutsideList>(`/outside-lists/${editId}`, body);
			formOpen = false;
			toasts.push(enabled ? 'Saved. HolyArrs is reading the list now.' : 'Saved. The list is off.', 'ok');
			await loadLists();
		} catch (err) {
			formError = message(err);
		} finally {
			saving = false;
		}
	}

	async function syncNow(l: OutsideList) {
		busyId = l.id;
		try {
			await api.post(`/outside-lists/${l.id}/sync`);
			toasts.push(`Reading ${l.name} now.`, 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busyId = null;
		}
	}

	async function remove(l: OutsideList) {
		if (!confirm(`Remove the list ${l.name}? Movies it added stay in your Library.`)) return;
		busyId = l.id;
		try {
			await api.del(`/outside-lists/${l.id}`);
			lists = lists.filter((x) => x.id !== l.id);
			toasts.push('List removed.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busyId = null;
		}
	}

	async function saveTrakt(e: SubmitEvent) {
		e.preventDefault();
		traktError = '';
		if (!traktKey.trim()) {
			traktError = 'Paste the Client ID of your Trakt app.';
			return;
		}
		traktSaving = true;
		try {
			traktMasked = (await api.put<{ key: string }>('/settings/trakt-key', { key: traktKey.trim() })).key;
			traktKey = '';
			toasts.push('Trakt key saved.', 'ok');
		} catch (err) {
			traktError = err instanceof ApiError && err.status === 403 ? 'Sign in with your password to change the key.' : message(err);
		} finally {
			traktSaving = false;
		}
	}

	async function addNever(e: SubmitEvent) {
		e.preventDefault();
		naError = '';
		const id = naId.trim().replace(/^tmdb:/i, '');
		if (!/^\d+$/.test(id) || !naName.trim()) {
			naError = 'Give the TMDB number and the name.';
			return;
		}
		try {
			const n = await api.post<NeverAddEntry>('/never-add', { tmdb_id: id, name: naName.trim(), reason: naReason.trim() });
			never = [n, ...never.filter((x) => x.id !== n.id)];
			naId = naName = naReason = '';
		} catch (err) {
			naError = message(err);
		}
	}

	async function removeNever(n: NeverAddEntry) {
		try {
			await api.del(`/never-add/${n.id}`);
			never = never.filter((x) => x.id !== n.id);
			toasts.push(`${n.name} may be added again.`, 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}
</script>

<svelte:head><title>Outside lists · HolyArrs</title></svelte:head>

<PageHeader title="Outside lists">
	{#snippet actions()}
		{#if !formOpen && loaded && hasData && lists.length}<Button icon="plus" variant="primary" type="button" onclick={() => openForm()}>Add a list</Button>{/if}
	{/snippet}
</PageHeader>

{#if formOpen}
	<form class="sc-form" onsubmit={save} aria-label={editId === null ? 'Add a list' : `Change ${name}`}>
		<SettingsCard title={editId === null ? 'Add a list' : `Change ${name}`}>
			<Field id="ol-name" label="Name">
				<input id="ol-name" class="input" bind:value={name} maxlength="80" required placeholder="Family favourites" />
			</Field>
			<Field id="ol-kind" label="Kind">
				<select id="ol-kind" class="input" bind:value={kind}>
					{#each listKinds as [k, label] (k)}<option value={k}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="ol-ref" label="Which list">
				<input id="ol-ref" class="input mono" bind:value={ref} required autocomplete="off" spellcheck="false" />
				<span class="hint">{listKinds.find(([k]) => k === kind)?.[2]}</span>
			</Field>
			<Field id="ol-folder" label="Library folder">
				<select id="ol-folder" class="input" bind:value={folderId}>
					{#each movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
			</Field>
			<Field id="ol-rules" label="Pick rules">
				<select id="ol-rules" class="input" bind:value={rulesId}>
					<option value={0}>The Library folder's own</option>
					{#each movieRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</Field>
			<Field id="ol-hours" label="Read every (hours)" hint="Between 6 hours and a week. Saving reads the list once straight away.">
				<input id="ol-hours" class="input" type="number" min="6" max="168" bind:value={hours} />
			</Field>
			<label class="checkline">
				<input type="checkbox" bind:checked={keep} />
				<span><b>Keep up to date</b><br /><span class="hint">Added movies are looked for, and a better copy later.</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={searchOnAdd} />
				<span><b>Search when added</b><br /><span class="hint">Look for a Release as soon as a movie comes in from the list.</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={enabled} />
				<span><b>Read this list</b><br /><span class="hint">Off keeps the list but stops reading it.</span></span>
			</label>
			{#if kind === 'trakt_list' && !traktMasked}<p class="notice warn">Add your Trakt key below first, or this list cannot be read.</p>{/if}
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={saving || movieFolders.length === 0} aria-busy={saving}>Save</Button>
				<Button variant="plain" type="button" onclick={() => (formOpen = false)}>Cancel</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={2} />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#if lists.length === 0 && !formOpen}
		<EmptyState icon="inbox" title="No Outside lists yet" text="Add a TMDB or Trakt list and its movies come into your Library by themselves." action={{ label: 'Add a list', onclick: () => openForm(), icon: 'plus' }} />
	{:else if lists.length}
		<ul class="card rows" aria-label="Outside lists">
			{#each lists as l (l.id)}
				<li class="list" data-testid="outside-list">
					<div class="grow">
						<div class="row" style="gap: var(--s-2)">
							<b>{l.name}</b>
							<span class="chip">{listKinds.find(([k]) => k === l.kind)?.[1]}</span>
							{#if !l.enabled}<span class="chip">Off</span>{/if}
							{#if l.last_error}<span class="chip fail">Problem</span>{/if}
						</div>
						<div class="xs muted">
							{#if listLink(l.kind, l.ref)}<a class="ul" href={listLink(l.kind, l.ref)} target="_blank" rel="noreferrer noopener">{l.ref}</a>{:else}{l.ref}{/if}
							· read {every(l.sync_every_hours)}
						</div>
						{#if l.last_synced_at}
							<p class="small" data-testid="list-result">Last read {dateTime(l.last_synced_at)}: {l.last_result}</p>
						{:else}
							<p class="small muted">Not read yet.</p>
						{/if}
						{#if l.last_error}<p class="small bad-text" role="status">{l.last_error}</p>{/if}
					</div>
					<div class="row acts">
						<Button icon="refresh" type="button" onclick={() => syncNow(l)} disabled={busyId === l.id || !l.enabled} aria-busy={busyId === l.id}>Read now</Button>
						<Button variant="plain" type="button" onclick={() => openForm(l)}>Change</Button>
						<Button variant="plain" type="button" onclick={() => remove(l)} disabled={busyId === l.id}>Remove</Button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<section class="card" id="trakt-card" aria-labelledby="h-trakt">
		<div class="card-head">
			<h2 id="h-trakt" class="h2">Trakt key</h2>
			<span class="chip {traktMasked ? 'ok' : ''}">{traktMasked ? 'Saved' : 'Not set'}</span>
		</div>
		<p class="small muted">
			Trakt lists need the Client ID of a Trakt app you make on trakt.tv. It is stored encrypted and never shown again in full.{hasTrakt && !traktMasked ? ' Your Trakt lists cannot be read until it is set.' : ''}
		</p>
		<form onsubmit={saveTrakt} style="margin-top: var(--s-4)">
			<Field id="trakt" label={traktMasked ? 'Replace the Trakt key' : 'Paste the Trakt key'}>
				<input id="trakt" class="input mono" type="password" bind:value={traktKey} autocomplete="off" spellcheck="false" />
			</Field>
			{#if traktError}<p class="form-error" role="alert">{traktError}</p>{/if}
			<Button type="submit" disabled={traktSaving} aria-busy={traktSaving}>Save</Button>
		</form>
	</section>

	<SettingsCard id="never-card" title="Never-add list" description="Outside lists skip these movies, and they cannot be added by hand until taken off.">
		<form class="never-form" onsubmit={addNever}>
			<Field id="na-id" label="TMDB number">
				<input id="na-id" class="input mono" bind:value={naId} inputmode="numeric" autocomplete="off" />
			</Field>
			<Field id="na-name" label="Name">
				<input id="na-name" class="input" bind:value={naName} maxlength="200" />
			</Field>
			<div class="field">
				<label for="na-reason">Why <span class="muted">(optional)</span></label>
				<input id="na-reason" class="input" bind:value={naReason} maxlength="200" />
			</div>
			<div class="na-act"><Button type="submit">Add to the Never-add list</Button></div>
		</form>
		{#if naError}<p class="form-error" role="alert">{naError}</p>{/if}
		{#if never.length}
			<ul class="never" aria-label="Never-add list">
				{#each never as n (n.id)}
					<li data-testid="never-add">
						<span class="grow"><b>{n.name}</b> <span class="xs muted mono">tmdb:{n.ids.tmdb}</span>{#if n.reason}<span class="xs muted"> · {n.reason}</span>{/if}</span>
						<Button variant="plain" type="button" onclick={() => removeNever(n)}>Take off {n.name}</Button>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="small muted" style="margin-top: var(--s-3)">Nothing on it.</p>
		{/if}
	</SettingsCard>
{/if}

<style>
	.list {
		display: flex;
		gap: var(--s-4);
		align-items: flex-start;
		padding: var(--s-4);
	}
	.list + .list {
		border-top: 1px solid var(--line);
	}
	.acts {
		flex: none;
		gap: var(--s-2);
	}
	.ul {
		text-decoration: underline;
	}
	.bad-text {
		color: var(--fail-text);
	}
	.never-form {
		margin-top: var(--s-4);
	}
	.na-act {
		margin-top: var(--s-4);
	}
	.never li {
		display: flex;
		gap: var(--s-3);
		align-items: center;
		padding: var(--s-2) 0;
	}
	.never li + li {
		border-top: 1px solid var(--line);
	}
	@media (max-width: 700px) {
		.list {
			flex-direction: column;
		}
		.acts {
			flex-wrap: wrap;
		}
	}
</style>
