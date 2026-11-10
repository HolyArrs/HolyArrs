<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { savedDefaults, changedParts, foldersFor, gateChoices, rulesFor } from '#lib/mediafiles/index.ts';
	import { detailsSources, episodeOrders, seasonChoices } from '#lib/shows/index.ts';
	import type { GateDefaults, TitleDefaults } from '#lib/mediafiles/index.ts';
	import type { ShowSettings } from '#lib/shows/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { to } from '#lib/core/paths.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import type { LibraryFolder, PickRulesView } from '#lib/core/types.ts';

	// Settings > Movies and TV defaults: what a new movie or show gets (folder, Pick rules, Keep up to date, Search when added), with the Release gate and the show choices behind Advanced. Three server routes, one form.

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

	// Saves only the routes whose part changed, one after the other; a part that saved stays saved when a later one fails.
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
			savedDefaults.forget();
			toasts.push(t('settings.titleDefaults.saved'), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t('settings.titleDefaults.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.titleDefaults.title')} />

{#if loading && !form}
	<SkSettings rows={4} />
{:else if !form}
	<LoadError hasData={false} message={loadError} onretry={load} busy={loading} />
{:else}
	{#if !mayEdit}<p class="hint" data-testid="read-only">{t('settings.titleDefaults.readOnly')}</p>{/if}
	<form class="sc-form" id="title-defaults-form" onsubmit={save} aria-label={t('settings.titleDefaults.title')}>
	<fieldset class="all" disabled={!mayEdit}>
	<legend class="sr-only">{mayEdit ? t('settings.titleDefaults.title') : t('settings.titleDefaults.titleReadOnly')}</legend>
		<SettingsCard id="movie-defaults" icon="film" title={t('settings.titleDefaults.movies')}>
			<Field id="td-movie-folder" label={t('settings.titleDefaults.folder')}>
				<select id="td-movie-folder" class="input" bind:value={form.defaults.movie.library_folder_id}>
					<option value={0}>{t('settings.titleDefaults.firstMovie')}</option>
					{#each movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
			</Field>
			<Field id="td-movie-rules" label={t('settings.titleDefaults.quality')}>
				<select id="td-movie-rules" class="input" bind:value={form.defaults.movie.pick_rules_id}>
					<option value={0}>{t('settings.titleDefaults.folderQuality')}</option>
					{#each movieRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</Field>
			<label class="checkline">
				<input id="td-movie-keep" type="checkbox" bind:checked={form.defaults.movie.keep_up_to_date} />
				<span><b>{t('settings.titleDefaults.keep')}</b><br /><span class="hint">{t('settings.titleDefaults.keepHint')}</span></span>
			</label>
			<label class="checkline">
				<input id="td-movie-search" type="checkbox" bind:checked={form.defaults.movie.search_on_add} />
				<span><b>{t('settings.titleDefaults.search')}</b></span>
			</label>
			{#snippet advanced()}
				<Field id="td-gate" label={t('settings.titleDefaults.gate')} hint={gateChoices.find(([g]) => g === form?.gate.release_gate)?.[2]}>
					<select id="td-gate" class="input" bind:value={form!.gate.release_gate}>
						{#each gateChoices as [g, l] (g)}<option value={g}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="td-gate-days" label={t('settings.titleDefaults.gateDays')} hint={t('settings.titleDefaults.gateDaysHint')}>
					<input id="td-gate-days" class="input narrow" type="number" min="0" max="365" step="1" bind:value={form!.gate.gate_delay_days} />
				</Field>
			{/snippet}
		</SettingsCard>

		<SettingsCard id="show-defaults" icon="tv" title={t('settings.titleDefaults.shows')}>
			<Field id="td-show-folder" label={t('settings.titleDefaults.folder')}>
				<select id="td-show-folder" class="input" bind:value={form.defaults.show.library_folder_id}>
					<option value={0}>{t('settings.titleDefaults.firstShow')}</option>
					{#each showFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
			</Field>
			<Field id="td-show-rules" label={t('settings.titleDefaults.quality')}>
				<select id="td-show-rules" class="input" bind:value={form.defaults.show.pick_rules_id}>
					<option value={0}>{t('settings.titleDefaults.folderQuality')}</option>
					{#each showRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</Field>
			<label class="checkline">
				<input id="td-show-keep" type="checkbox" bind:checked={form.defaults.show.keep_up_to_date} />
				<span><b>{t('settings.titleDefaults.keep')}</b></span>
			</label>
			<label class="checkline">
				<input id="td-show-search" type="checkbox" bind:checked={form.defaults.show.search_on_add} />
				<span><b>{t('settings.titleDefaults.search')}</b><br /><span class="hint">{t('settings.titleDefaults.searchHint')}</span></span>
			</label>
			{#snippet advanced()}
				<Field id="td-seasons" label={t('settings.titleDefaults.seasons')} hint={seasonChoices.find(([v]) => v === form?.shows.season_choice)?.[2]}>
					<select id="td-seasons" class="input" bind:value={form!.shows.season_choice}>
						{#each seasonChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="td-new-seasons" label={t('settings.titleDefaults.newSeasons')}>
					<select id="td-new-seasons" class="input" bind:value={form!.shows.new_seasons}>
						<option value="all">{t('settings.titleDefaults.newAll')}</option>
						<option value="none">{t('settings.titleDefaults.newNone')}</option>
					</select>
				</Field>
				<Field id="td-order" label={t('settings.titleDefaults.order')} hint={episodeOrders.find(([v]) => v === form?.shows.episode_order)?.[2]}>
					<select id="td-order" class="input" bind:value={form!.shows.episode_order}>
						{#each episodeOrders as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
				<Field id="td-source" label={t('settings.titleDefaults.details')}>
					<select id="td-source" class="input" bind:value={form!.shows.details_source}>
						{#each detailsSources as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</Field>
			{/snippet}
		</SettingsCard>

		<p class="hint where">
			<Words key="settings.titleDefaults.where" links={{ a: to('/settings/library-folders'), a2: to('/settings/requests'), a3: to('/settings/outside-lists') }} />
		</p>
	</fieldset>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	{#if mayEdit}<SaveBar {dirty} form="title-defaults-form" saving={busy} ondiscard={() => (form = JSON.parse(saved))} />{/if}
{/if}

<style>
	/* The fieldset only turns the form read only; it adds no box of its own. */
	.all { display: contents; border: 0; padding: 0; margin: 0; }
	.narrow { max-width: 160px; }
	.where { margin-top: 4px; }
	.where :global(a) { color: var(--accent-text); font-weight: 600; }
</style>
