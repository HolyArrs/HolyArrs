<script lang="ts">
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { WORDS as ASKED } from '#lib/askedby.ts';
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
		music_pick_rules_id: 0,
		show_who_asked: 'approvers'
	});
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let busy = $state(false);
	let error = $state('');
	let loaded = $state(false);
	let saved = $state('');
	const dirty = $derived(loaded && !!saved && JSON.stringify(settings) !== saved);

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
			saved = JSON.stringify(settings);
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
			saved = JSON.stringify(settings);
			toasts.push('Wishlist settings saved.', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Wishlist rules · HolyArrs</title></svelte:head>

<PageHeader title="Wishlist rules" subtitle="How many movies, shows and albums people may ask for, and what an approved ask gets." />

{#if !loaded}
	<div class="sk" style="height: 240px"></div>
{:else}
	<form class="sc-form" id="wishlist-form" onsubmit={save} aria-label="Wishlist rules">
		<SettingsCard id="weekly-limits" title="Weekly limits" description="How many asks each person may make in 7 days.">
			<fieldset class="kinds">
				<legend class="sr-only">Asks each person may make in 7 days</legend>
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
					{#if session.can('users')}<a href={to(`/settings/users`)}>Users and roles</a>{:else}Users and roles{/if} page. 0 means no limit. People who may approve
					have no limit. Taking an ask back or a turned down ask frees it up; joining someone else's ask costs nothing.
				</span>
			</fieldset>
		</SettingsCard>
		<SettingsCard id="show-who-asked" title={ASKED.householdLabel} description={ASKED.householdHelp}>
			<Field id="show-who-asked-select" label={ASKED.householdLabel}>
				<select id="show-who-asked-select" class="input" bind:value={settings.show_who_asked} data-testid="show-who-asked">
					{#each ASKED.householdChoices as c (c.value)}<option value={c.value}>{c.label}</option>{/each}
				</select>
			</Field>
		</SettingsCard>
		<SettingsCard id="approved-asks" title="Approved asks" description="Where an approved ask goes, and whether the search starts straight away.">
			<Field id="ask-folder" label="Library folder for approved movie asks" hint="An approved movie ask is added to this folder.">
				<select id="ask-folder" class="input" bind:value={settings.library_folder_id}>
					<option value={0}>The first movie folder</option>
					{#each movieFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-show-folder" label="Library folder for approved show asks" hint="An approved show ask is added to this folder.">
				<select id="ask-show-folder" class="input" bind:value={settings.show_library_folder_id}>
					<option value={0}>The first show folder</option>
					{#each showFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-music-folder" label="Library folder for approved music asks" hint="An approved artist or album ask is added to this folder, with only the albums asked for kept up to date.">
				<select id="ask-music-folder" class="input" bind:value={settings.music_library_folder_id}>
					<option value={0}>The first music folder</option>
					{#each musicFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<label class="checkline">
				<input id="search-approved" type="checkbox" bind:checked={settings.search_on_approve} />
				<span><b>Search when approved</b><br /><span class="hint">Start looking for a Release as soon as a movie ask is approved. Approved shows are looked for at the next feed check and each night.</span></span>
			</label>
			{#snippet advanced()}
				<Field id="ask-rules" label="Pick rules for approved movie asks" hint="Whoever approves can pick another folder and Pick rules for one ask.">
					<select id="ask-rules" class="input" bind:value={settings.pick_rules_id}>
						<option value={0}>The folder's own Pick rules</option>
						{#each movieRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
				<Field id="ask-show-rules" label="Pick rules for approved show asks" hint="Which Releases an approved show takes.">
					<select id="ask-show-rules" class="input" bind:value={settings.show_pick_rules_id}>
						<option value={0}>The folder's own Pick rules</option>
						{#each showRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
				<Field id="ask-music-rules" label="Pick rules for approved music asks" hint="Which Releases an approved artist or album takes.">
					<select id="ask-music-rules" class="input" bind:value={settings.music_pick_rules_id}>
						<option value={0}>The folder's own Pick rules</option>
						{#each musicRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
			{/snippet}
		</SettingsCard>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	<SaveBar {dirty} form="wishlist-form" saving={busy} ondiscard={() => (settings = JSON.parse(saved))} />
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
</style>
