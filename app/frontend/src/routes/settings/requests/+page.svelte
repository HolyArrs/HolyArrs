<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { PlexFriends, qualityName, type AskSettings, type QualityLine } from '#lib/requests/index.ts';
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
		four_k_library_folder_id: 0,
		four_k_pick_rules_id: 0,
		show_four_k_library_folder_id: 0,
		show_four_k_pick_rules_id: 0,
		plex_watchlists: false,
		plex_watchlists_everyone: false,
		plex_friends: false
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
	const kinds = $derived([
		{ key: 'movies_per_week', id: 'per-week-movies', label: t('settings.requests.movieReq'), hint: '' },
		{ key: 'shows_per_week', id: 'per-week-shows', label: t('settings.requests.showReq'), hint: t('settings.requests.showReqHint') },
		{ key: 'music_per_week', id: 'per-week-music', label: t('settings.requests.musicReq'), hint: t('settings.requests.musicReqHint') }
	] as const);

	// What people can ask for: each line's name and whether people see it. Its Pick rules come from the settings above.
	const lineKinds = $derived([
		{ kind: 'movie', title: t('settings.requests.movieReq'), plain: settings.pick_rules_id, fourK: settings.four_k_pick_rules_id },
		{ kind: 'show', title: t('settings.requests.showReq'), plain: settings.show_pick_rules_id, fourK: settings.show_four_k_pick_rules_id }
	] as const);
	function lineHint(l: QualityLine, plain: number, fourK: number): string {
		const id = l.extra ? fourK : plain;
		const name = id ? (rules.find((r) => r.id === id)?.name ?? '') : t(l.extra ? 'settings.requests.fourKNamedRules' : 'settings.requests.folderQuality');
		const line = t(l.extra ? 'settings.requests.choiceOwn' : 'settings.requests.choiceMain', { rules: name });
		return l.ready ? line : `${line}. ${t('settings.requests.choiceNotReady')}`;
	}

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
			toasts.push(t('settings.requests.saved'), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t('settings.requests.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.requests.title')} />

{#if !loaded}
	<SkSettings rows={3} />
{:else if !saved}
	<LoadError hasData={false} message={loadError} onretry={load} busy={loading} />
{:else}
	<form class="sc-form" id="wishlist-form" onsubmit={save} aria-label={t('settings.requests.title')}>
		<SettingsCard id="weekly-limits" title={t('settings.requests.weekly')} description={t('settings.requests.weeklyDesc')}>
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
					{t('settings.requests.defaultsA')}
					{#if session.can('users')}<a href={to(`/settings/users`)}>{t('settings.requests.usersAndRoles')}</a>{:else}{t('settings.requests.usersAndRoles')}{/if} {t('settings.requests.defaultsB')}
				</span>
			</fieldset>
		</SettingsCard>
		<SettingsCard id="approved-asks" title={t('settings.requests.approved')}>
			<Field id="ask-folder" label={t('settings.requests.movieFolder')}>
				<select id="ask-folder" class="input" bind:value={settings.library_folder_id}>
					<option value={0}>{t('settings.requests.firstMovie')}</option>
					{#each movieFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-show-folder" label={t('settings.requests.showFolder')}>
				<select id="ask-show-folder" class="input" bind:value={settings.show_library_folder_id}>
					<option value={0}>{t('settings.requests.firstShow')}</option>
					{#each showFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-music-folder" label={t('settings.requests.musicFolder')} hint={t('settings.requests.musicFolderHint')}>
				<select id="ask-music-folder" class="input" bind:value={settings.music_library_folder_id}>
					<option value={0}>{t('settings.requests.firstMusic')}</option>
					{#each musicFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<label class="checkline">
				<input id="search-approved" type="checkbox" bind:checked={settings.search_on_approve} />
				<span><b>{t('settings.requests.search')}</b><br /><span class="hint">{t('settings.requests.searchHint')}</span></span>
			</label>
			{#snippet advanced()}
				<Field id="ask-rules" label={t('settings.requests.movieQuality')} hint={t('settings.requests.movieQualityHint')}>
					<select id="ask-rules" class="input" bind:value={settings.pick_rules_id}>
						<option value={0}>{t('settings.requests.folderQuality')}</option>
						{#each movieRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
				<Field id="ask-show-rules" label={t('settings.requests.showQuality')}>
					<select id="ask-show-rules" class="input" bind:value={settings.show_pick_rules_id}>
						<option value={0}>{t('settings.requests.folderQuality')}</option>
						{#each showRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
				<Field id="ask-music-rules" label={t('settings.requests.musicQuality')}>
					<select id="ask-music-rules" class="input" bind:value={settings.music_pick_rules_id}>
						<option value={0}>{t('settings.requests.folderQuality')}</option>
						{#each musicRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</Field>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="four-k" title={t('settings.requests.fourK')} description={t('settings.requests.fourKDesc')}>
			<Field id="ask-4k-folder" label={t('settings.requests.movieFolder')}>
				<select id="ask-4k-folder" class="input" bind:value={settings.four_k_library_folder_id}>
					<option value={0}>{t('settings.requests.fourKSameFolder')}</option>
					{#each movieFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-4k-rules" label={t('settings.requests.movieQuality')}>
				<select id="ask-4k-rules" class="input" bind:value={settings.four_k_pick_rules_id}>
					<option value={0}>{t('settings.requests.fourKNamedRules')}</option>
					{#each movieRules as r (r.id)}
						<option value={r.id}>{r.name}</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-show-4k-folder" label={t('settings.requests.showFolder')}>
				<select id="ask-show-4k-folder" class="input" bind:value={settings.show_four_k_library_folder_id}>
					<option value={0}>{t('settings.requests.fourKSameFolder')}</option>
					{#each showFolders as f (f.id)}
						<option value={f.id}>{f.label || f.path} ({f.path})</option>
					{/each}
				</select>
			</Field>
			<Field id="ask-show-4k-rules" label={t('settings.requests.showQuality')}>
				<select id="ask-show-4k-rules" class="input" bind:value={settings.show_four_k_pick_rules_id}>
					<option value={0}>{t('settings.requests.fourKNamedRules')}</option>
					{#each showRules as r (r.id)}
						<option value={r.id}>{r.name}</option>
					{/each}
				</select>
			</Field>
		</SettingsCard>
		{#if settings.qualities}
			<SettingsCard id="ask-qualities" title={t('settings.requests.choices')} description={t('settings.requests.choicesDesc')}>
				{#each lineKinds as k (k.kind)}
					<h3 class="q-kind">{k.title}</h3>
					{#each settings.qualities[k.kind] as l, i (l.id)}
						<Field id="q-{k.kind}-{l.id}" label={qualityName(l.id)} hint={lineHint(l, k.plain, k.fourK)}>
							<div class="q-row">
								<input id="q-{k.kind}-{l.id}" class="input" maxlength="30" placeholder={qualityName(l.id)} aria-describedby="q-{k.kind}-{l.id}-hint" bind:value={settings.qualities[k.kind][i].name} />
								<label class="q-shown"><input type="checkbox" checked={!l.hidden} onchange={(e) => (settings.qualities![k.kind][i].hidden = !e.currentTarget.checked)} data-testid="q-shown-{k.kind}-{l.id}" />{t('settings.requests.choiceShown')}</label>
							</div>
						</Field>
					{/each}
				{/each}
			</SettingsCard>
		{/if}
		<SettingsCard id="plex-watchlists" title={t('settings.requests.plex')} description={t('settings.requests.plexDesc')}>
			<label class="checkline">
				<input id="plex-watchlists-on" type="checkbox" bind:checked={settings.plex_watchlists} />
				<span><b>{t('settings.requests.plexOn')}</b><br /><span class="hint">{t('settings.requests.plexOnHint')}</span></span>
			</label>
			<label class="checkline">
				<input id="plex-watchlists-everyone" type="checkbox" bind:checked={settings.plex_watchlists_everyone} disabled={!settings.plex_watchlists} />
				<span><b>{t('settings.requests.plexEveryone')}</b><br /><span class="hint">{t('settings.requests.plexEveryoneHint')}</span></span>
			</label>
			<label class="checkline">
				<input id="plex-friends" type="checkbox" bind:checked={settings.plex_friends} disabled={!settings.plex_watchlists} data-testid="plex-friends" />
				<span><b>{t('settings.requests.plexFriends')}</b><br /><span class="hint">{t('settings.requests.plexFriendsHint')}</span></span>
			</label>
			<PlexFriends on={!!settings.plex_watchlists && !!settings.plex_friends} />
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
	.q-kind {
		margin: var(--s-3) 0 0;
		font: var(--t-subhead);
		font-weight: 600;
		color: var(--text-muted);
	}
	.q-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-3);
	}
	.q-row .input {
		flex: 1 1 120px;
		min-width: 0;
	}
	.q-shown {
		display: inline-flex;
		align-items: center;
		gap: var(--s-2);
		min-height: 44px;
		font: var(--t-callout);
		white-space: nowrap;
	}
	.kind-row {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: var(--s-3);
	}
</style>
