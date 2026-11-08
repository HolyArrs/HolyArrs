<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import type { AskSettings } from '#lib/requests/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { LibraryFolder, PickRulesView } from '#lib/core/types.ts';

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
		plex_watchlists: false
	});
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let busy = $state(false);
	let error = $state('');
	let loaded = $state(false);
	let loading = $state(false);
	let loadError = $state('');
	let saved = $state('');
	const dirty = $derived(loaded && !!saved && JSON.stringify(settings) !== saved);

	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	const movieRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('movie')));
	const showFolders = $derived(folders.filter((f) => f.kind === 'show'));
	const showRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('show')));
	const musicFolders = $derived(folders.filter((f) => f.kind === 'music'));
	const musicRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('music')));
	const kinds = [
		{ key: 'movies_per_week', id: 'per-week-movies', label: 'Movie requests', hint: '' },
		{ key: 'shows_per_week', id: 'per-week-shows', label: 'Show requests', hint: 'Any number of seasons counts as one' },
		{ key: 'music_per_week', id: 'per-week-music', label: 'Music requests', hint: 'An artist or album counts as one' }
	] as const;

	async function load() {
		loading = true;
		loadError = '';
		try {
			settings = await api.get<AskSettings>('/settings/asks');
			saved = JSON.stringify(settings);
		} catch (e) {
			loadError = message(e);
		} finally {
			loaded = true;
			loading = false;
		}
	}

	onMount(async () => {
		await load();
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
			toasts.push('Request settings saved', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Request rules · HolyArrs</title></svelte:head>

<PageHeader title="Request rules" />

{#if !loaded}
	<SkSettings rows={3} />
{:else if !saved}
	<LoadError hasData={false} message={loadError} onretry={load} busy={loading} />
{:else}
	<form class="sc-form" id="wishlist-form" onsubmit={save} aria-label="Request rules">
		<SettingsCard id="weekly-limits" title="Weekly limits" description="Per person, per 7 days">
			<fieldset class="kinds">
				<div class="kind-row">
					{#each kinds as k (k.key)}
						<div class="field">
							<label for={k.id}>{k.label}</label>
							<input id={k.id} class="input narrow" type="number" min="0" max="1000" required bind:value={settings[k.key]} aria-describedby={k.hint ? `${k.id}-hint` : undefined} />
							{#if k.hint}<span id="{k.id}-hint" class="hint">{k.hint}</span>{/if}
						</div>
					{/each}
				</div>
				<span class="hint">
					Defaults for everyone; set one person's own limits on the
					{#if session.can('users')}<a href={to(`/settings/users`)}>Users and roles</a>{:else}Users and roles{/if} page. 0 means no limit. Approvers have none. Taken-back and turned-down requests don't count, nor does joining someone else's.
				</span>
			</fieldset>
		</SettingsCard>
		<SettingsCard id="approved-asks" title="Approved requests">
			<Field id="ask-folder" label="Movie folder">
				<select id="ask-folder" class="input" bind:value={settings.library_folder_id}>
					<option value={0}>First movie folder</option>
					{#each movieFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-show-folder" label="Show folder">
				<select id="ask-show-folder" class="input" bind:value={settings.show_library_folder_id}>
					<option value={0}>First show folder</option>
					{#each showFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-music-folder" label="Music folder" hint="Only requested albums are kept up to date">
				<select id="ask-music-folder" class="input" bind:value={settings.music_library_folder_id}>
					<option value={0}>First music folder</option>
					{#each musicFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<label class="checkline">
				<input id="search-approved" type="checkbox" bind:checked={settings.search_on_approve} />
				<span><b>Search when approved</b><br /><span class="hint">Movies search at once; shows at the next feed check</span></span>
			</label>
			{#snippet advanced()}
				<Field id="ask-rules" label="Movie quality" hint="Approvers can override per request">
					<select id="ask-rules" class="input" bind:value={settings.pick_rules_id}>
						<option value={0}>The folder's own quality</option>
						{#each movieRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
				<Field id="ask-show-rules" label="Show quality">
					<select id="ask-show-rules" class="input" bind:value={settings.show_pick_rules_id}>
						<option value={0}>The folder's own quality</option>
						{#each showRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
				<Field id="ask-music-rules" label="Music quality">
					<select id="ask-music-rules" class="input" bind:value={settings.music_pick_rules_id}>
						<option value={0}>The folder's own quality</option>
						{#each musicRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
			{/snippet}
		</SettingsCard>
		{#if !session.held('plex_watchlists')}<SettingsCard id="plex-watchlists" title="Plex watchlists" description="Titles on your Plex watchlist become requests">
			<label class="checkline">
				<input id="plex-watchlists-on" type="checkbox" bind:checked={settings.plex_watchlists} />
				<span><b>Request from Plex watchlists</b><br /><span class="hint">Each person turns it on in My account. Their Limits apply.</span></span>
			</label>
		</SettingsCard>{/if}
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
