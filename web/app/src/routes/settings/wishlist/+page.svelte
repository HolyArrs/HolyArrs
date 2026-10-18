<script lang="ts">
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import type { AskSettings, LibraryFolder, PickRulesView } from '#lib/types.ts';

	let settings = $state<AskSettings>({
		movies_per_week: 10,
		shows_per_week: 10,
		music_per_week: 10,
		library_folder_id: 0,
		pick_rules_id: 0,
		search_on_approve: true,
		show_library_folder_id: 0,
		show_pick_rules_id: 0,
		music_library_folder_id: 0,
		music_pick_rules_id: 0
	});
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let busy = $state(false);
	let error = $state('');
	let loaded = $state(false);

	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	const movieRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('movie')));
	const showFolders = $derived(folders.filter((f) => f.kind === 'show'));
	const showRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('show')));
	const musicFolders = $derived(folders.filter((f) => f.kind === 'music'));
	const musicRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('music')));
	const kinds = [
		{ key: 'movies_per_week', id: 'per-week-movies', label: 'Movie asks', hint: 'One movie is one ask.' },
		{ key: 'shows_per_week', id: 'per-week-shows', label: 'Show asks', hint: 'A show ask counts as one, whatever its seasons.' },
		{ key: 'music_per_week', id: 'per-week-music', label: 'Music asks', hint: 'An artist or album ask counts as one, whatever its albums.' }
	] as const;

	onMount(async () => {
		try {
			settings = await api.get<AskSettings>('/settings/asks');
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
		api
			.get<{ library_folders: LibraryFolder[] }>('/library-folders')
			.then((r) => (folders = r.library_folders))
			.catch(() => {});
		api
			.get<{ pick_rules: PickRulesView[] }>('/pick-rules')
			.then((r) => (rules = r.pick_rules))
			.catch(() => {});
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			settings = await api.put<AskSettings>('/settings/asks', {
				...settings,
				movies_per_week: Number(settings.movies_per_week),
				shows_per_week: Number(settings.shows_per_week),
				music_per_week: Number(settings.music_per_week)
			});
			toasts.push('Wishlist settings saved.', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Wishlist settings · HolyArrs</title></svelte:head>

<PageHeader title="Wishlist" subtitle="How many movies, shows and albums people may ask for, and what an approved ask gets." />

{#if !loaded}
	<div class="sk" style="height: 240px"></div>
{:else}
	<form class="card" onsubmit={save}>
		<fieldset class="kinds">
			<legend class="label">Asks each person may make in 7 days</legend>
			<div class="kind-row">
				{#each kinds as k (k.key)}
					<div class="field">
						<label for={k.id}>{k.label}</label>
						<input id={k.id} class="input narrow" type="number" min="0" max="1000" required bind:value={settings[k.key]} aria-describedby="{k.id}-hint" />
						<span id="{k.id}-hint" class="hint">{k.hint}</span>
					</div>
				{/each}
			</div>
			<span class="hint">
				These are the defaults; give one person their own limits on the
				{#if session.can('users')}<a href={to(`/settings/users`)}>Users</a>{:else}Users{/if} page. 0 means no limit. People who may approve
				have no limit. Taking an ask back or a turned down ask frees it up; joining someone else's ask costs nothing.
			</span>
		</fieldset>
		<div class="field">
			<label for="ask-folder">Library folder for approved movie asks</label>
			<select id="ask-folder" class="input" bind:value={settings.library_folder_id}>
				<option value={0}>The first movie folder</option>
				{#each movieFolders as f (f.id)}
					<option value={f.id}>{f.label || f.path} ({f.path})</option>
				{/each}
			</select>
		</div>
		<div class="field">
			<label for="ask-rules">Pick rules for approved movie asks</label>
			<select id="ask-rules" class="input" bind:value={settings.pick_rules_id}>
				<option value={0}>The folder's own Pick rules</option>
				{#each movieRules as r (r.id)}
					<option value={r.id}>{r.name}</option>
				{/each}
			</select>
		</div>
		<div class="field">
			<label for="ask-show-folder">Library folder for approved show asks</label>
			<select id="ask-show-folder" class="input" bind:value={settings.show_library_folder_id}>
				<option value={0}>The first show folder</option>
				{#each showFolders as f (f.id)}
					<option value={f.id}>{f.label || f.path} ({f.path})</option>
				{/each}
			</select>
		</div>
		<div class="field">
			<label for="ask-show-rules">Pick rules for approved show asks</label>
			<select id="ask-show-rules" class="input" bind:value={settings.show_pick_rules_id}>
				<option value={0}>The folder's own Pick rules</option>
				{#each showRules as r (r.id)}
					<option value={r.id}>{r.name}</option>
				{/each}
			</select>
		</div>
		<div class="field">
			<label for="ask-music-folder">Library folder for approved music asks</label>
			<select id="ask-music-folder" class="input" bind:value={settings.music_library_folder_id}>
				<option value={0}>The first music folder</option>
				{#each musicFolders as f (f.id)}
					<option value={f.id}>{f.label || f.path} ({f.path})</option>
				{/each}
			</select>
		</div>
		<div class="field">
			<label for="ask-music-rules">Pick rules for approved music asks</label>
			<select id="ask-music-rules" class="input" bind:value={settings.music_pick_rules_id}>
				<option value={0}>The folder's own Pick rules</option>
				{#each musicRules as r (r.id)}
					<option value={r.id}>{r.name}</option>
				{/each}
			</select>
			<span class="hint">Whoever approves can pick another folder and Pick rules for one ask. An approved music ask adds the artist with only the albums asked for kept up to date.</span>
		</div>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.search_on_approve} />
			<span><b>Search when approved</b><br /><span class="hint">Start looking for a Release as soon as a movie ask is approved. Approved shows are looked for at the next feed check and each night.</span></span>
		</label>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<div class="row" style="margin-top: var(--s-5)">
			<Button variant="primary" type="submit" disabled={busy} aria-busy={busy}>Save</Button>
		</div>
	</form>
{/if}

<style>
	.narrow {
		max-width: 160px;
	}
	.kinds {
		border: 0;
		padding: 0;
		margin: 0 0 var(--s-4);
	}
	.kind-row {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: var(--s-3);
	}
	.checkline {
		margin-top: var(--s-4);
	}
</style>
