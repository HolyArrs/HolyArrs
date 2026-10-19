<script lang="ts">
	// Settings > Movies and TV defaults: what a new movie or show gets (folder,
	// Pick rules, Keep up to date, Search when added), with the Release gate
	// and the show choices behind Advanced. Three server routes, one form.
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { changedParts, foldersFor, gateChoices, rulesFor } from '#lib/title-defaults.ts';
	import { detailsSources, episodeOrders, seasonChoices } from '#lib/shows.ts';
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import type { GateDefaults, LibraryFolder, PickRulesView, ShowSettings, TitleDefaults } from '#lib/types.ts';

	interface Parts {
		defaults: TitleDefaults;
		gate: GateDefaults;
		shows: ShowSettings;
	}

	// People without the settings permission see the defaults but cannot change them.
	const mayEdit = $derived(session.can('settings'));
	let form = $state<Parts | null>(null);
	let saved = $state('');
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let loading = $state(true);
	let loadError = $state('');
	let busy = $state(false);
	let error = $state('');
	const dirty = $derived(!!form && !!saved && JSON.stringify(form) !== saved);

	const movieFolders = $derived(foldersFor(folders, 'movie'));
	const showFolders = $derived(foldersFor(folders, 'show'));
	const movieRules = $derived(rulesFor(rules, 'movie'));
	const showRules = $derived(rulesFor(rules, 'show'));

	async function load() {
		loading = true;
		loadError = '';
		try {
			// Folders and Pick rules come with the defaults, so the choices are there on the first draw.
			const [defaults, gate, shows, f, r] = await Promise.all([
				api.get<TitleDefaults>('/settings/title-defaults'),
				api.get<GateDefaults>('/settings/release-gate'),
				api.get<ShowSettings>('/settings/shows'),
				api.get<{ library_folders: LibraryFolder[] }>('/library-folders'),
				api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')
			]);
			folders = f.library_folders;
			rules = r.pick_rules;
			form = { defaults, gate, shows };
			saved = JSON.stringify(form);
		} catch (e) {
			loadError = message(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);

	// Saves only the routes whose part changed, one after the other; a part
	// that saved stays saved when a later one fails.
	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!form) return;
		busy = true;
		error = '';
		const done: Parts = JSON.parse(saved);
		try {
			for (const part of changedParts(done, form)) {
				if (part === 'defaults') form.defaults = await api.put<TitleDefaults>('/settings/title-defaults', form.defaults);
				else if (part === 'gate') form.gate = await api.put<GateDefaults>('/settings/release-gate', { ...form.gate, gate_delay_days: Number(form.gate.gate_delay_days) || 0 });
				else form.shows = await api.put<ShowSettings>('/settings/shows', form.shows);
				Object.assign(done, { [part]: $state.snapshot(form[part]) });
				saved = JSON.stringify(done);
			}
			toasts.push('Movies and TV defaults saved.', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Movies and TV defaults · HolyArrs</title></svelte:head>

<PageHeader title="Movies and TV defaults" subtitle="What a new movie or show gets. The add dialog starts from these, and you can still change them there for one title." />

{#if loading && !form && !loadError}
	<div class="loading" aria-busy="true"><Skeleton shape="block" height="220px" /><Skeleton shape="block" height="220px" /></div>
{:else if !form}
	<LoadError hasData={false} message={loadError} onretry={load} busy={loading} />
{:else}
	{#if !mayEdit}<p class="hint" data-testid="read-only">You can see these defaults. Changing them takes the settings permission.</p>{/if}
	<form class="sc-form" id="title-defaults-form" onsubmit={save} aria-label="Movies and TV defaults">
	<fieldset class="all" disabled={!mayEdit}>
	<legend class="sr-only">{mayEdit ? 'Movies and TV defaults' : 'Movies and TV defaults, read only'}</legend>
		<SettingsCard id="movie-defaults" icon="film" title="Movies" description="What every movie you add gets.">
			<Field id="td-movie-folder" label="Library folder" hint="Where a new movie goes.">
				<select id="td-movie-folder" class="input" bind:value={form.defaults.movie.library_folder_id}>
					<option value={0}>The first working movie folder</option>
					{#each movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
			</Field>
			<Field id="td-movie-rules" label="Pick rules" hint="Which Releases count, and which one is best.">
				<select id="td-movie-rules" class="input" bind:value={form.defaults.movie.pick_rules_id}>
					<option value={0}>The folder's own Pick rules</option>
					{#each movieRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</Field>
			<label class="checkline">
				<input id="td-movie-keep" type="checkbox" bind:checked={form.defaults.movie.keep_up_to_date} />
				<span><b>Keep up to date</b><br /><span class="hint">HolyArrs looks for it, and for a better copy later.</span></span>
			</label>
			<label class="checkline">
				<input id="td-movie-search" type="checkbox" bind:checked={form.defaults.movie.search_on_add} />
				<span><b>Search when added</b><br /><span class="hint">Start looking for a Release straight away.</span></span>
			</label>
			{#snippet advanced()}
				<Field id="td-gate" label="Release gate" hint={gateChoices.find(([g]) => g === form?.gate.release_gate)?.[2]}>
					<select id="td-gate" class="input" bind:value={form!.gate.release_gate}>
						{#each gateChoices as [g, l] (g)}<option value={g}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="td-gate-days" label="Extra days to wait" hint="Days to wait after the gate opens before HolyArrs looks.">
					<input id="td-gate-days" class="input narrow" type="number" min="0" max="365" step="1" bind:value={form!.gate.gate_delay_days} />
				</Field>
			{/snippet}
		</SettingsCard>

		<SettingsCard id="show-defaults" icon="tv" title="TV shows" description="What every show you add gets.">
			<Field id="td-show-folder" label="Library folder" hint="Where a new show goes.">
				<select id="td-show-folder" class="input" bind:value={form.defaults.show.library_folder_id}>
					<option value={0}>The first working TV show folder</option>
					{#each showFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
			</Field>
			<Field id="td-show-rules" label="Pick rules" hint="Which Releases count, and which one is best.">
				<select id="td-show-rules" class="input" bind:value={form.defaults.show.pick_rules_id}>
					<option value={0}>The folder's own Pick rules</option>
					{#each showRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</Field>
			<label class="checkline">
				<input id="td-show-keep" type="checkbox" bind:checked={form.defaults.show.keep_up_to_date} />
				<span><b>Keep up to date</b><br /><span class="hint">HolyArrs looks for the episodes of the seasons it keeps up to date.</span></span>
			</label>
			<label class="checkline">
				<input id="td-show-search" type="checkbox" bind:checked={form.defaults.show.search_on_add} />
				<span><b>Search when added</b><br /><span class="hint">Look for missing episodes as soon as the show is added. Off: they are looked for at the next feed check and each night.</span></span>
			</label>
			{#snippet advanced()}
				<Field id="td-seasons" label="Seasons to keep up to date" hint={seasonChoices.find(([v]) => v === form?.shows.season_choice)?.[2]}>
					<select id="td-seasons" class="input" bind:value={form!.shows.season_choice}>
						{#each seasonChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="td-new-seasons" label="New seasons" hint="What happens to a season that comes out later.">
					<select id="td-new-seasons" class="input" bind:value={form!.shows.new_seasons}>
						<option value="all">Keep them up to date</option>
						<option value="none">Leave them off</option>
					</select>
				</Field>
				<Field id="td-order" label="Episode order" hint={episodeOrders.find(([v]) => v === form?.shows.episode_order)?.[2]}>
					<select id="td-order" class="input" bind:value={form!.shows.episode_order}>
						{#each episodeOrders as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="td-source" label="Details from" hint="Where names, seasons and air dates come from.">
					<select id="td-source" class="input" bind:value={form!.shows.details_source}>
						{#each detailsSources as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
			{/snippet}
		</SettingsCard>

		<p class="hint where">
			Each <a href={to('/settings/library-folders')}>Library folder</a> has Pick rules of its own. <a href={to('/settings/wishlist')}>Wishlist rules</a> and
			<a href={to('/settings/outside-lists')}>Outside lists</a> keep their own choices.
		</p>
	</fieldset>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	{#if mayEdit}<SaveBar {dirty} form="title-defaults-form" saving={busy} ondiscard={() => (form = JSON.parse(saved))} />{/if}
{/if}

<style>
	/* The fieldset only turns the form read only; it adds no box of its own. */
	.all { display: contents; border: 0; padding: 0; margin: 0; }
	.loading { display: flex; flex-direction: column; gap: 16px; }
	.narrow { max-width: 160px; }
	.where { margin-top: 4px; }
	.where a { color: var(--accent-text); font-weight: 600; }
</style>
