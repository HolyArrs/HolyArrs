<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { albumChoices, extraKinds, mainKinds, newAlbumChoices, pressingRules, pressingStatuses, toggle } from '#lib/music/index.ts';
	import type { MusicSettings } from '#lib/music/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	// Settings > Music: what a new artist gets by default, which Pressing is used for an album, and where MusicBrainz answers come from.

	let settings = $state<MusicSettings>({
		album_choice: 'all',
		new_albums: 'all',
		album_types: ['album', 'ep'],
		secondary_types: ['studio'],
		release_statuses: ['official'],
		pressing_rule: 'common',
		musicbrainz_url: '',
		cache_hours: 24
	});
	let loaded = $state(false);
	let busy = $state(false);
	let error = $state('');
	let saved = $state('');
	let loadError = $state('');
	const dirty = $derived(loaded && !!saved && JSON.stringify(settings) !== saved);
	// Save waits until each group of kinds has at least one ticked.
	const emptyKinds = $derived(
		!settings.album_types.length
			? t('settings.music.needAlbum')
			: !settings.secondary_types.length
				? t('settings.music.needExtra')
				: !settings.release_statuses.length
					? t('settings.music.needPressing')
					: ''
	);

	async function load() {
		loaded = false;
		loadError = '';
		try {
			settings = await api.get<MusicSettings>('/settings/music');
			saved = JSON.stringify(settings);
		} catch (e) {
			loadError = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			settings = await api.put<MusicSettings>('/settings/music', { ...settings, musicbrainz_url: settings.musicbrainz_url.trim(), cache_hours: Number(settings.cache_hours) });
			saved = JSON.stringify(settings);
			toasts.push(t('settings.music.saved'), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t('settings.music.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.music.title')} />

{#if !loaded}
	<SkSettings rows={3} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	<form class="sc-form" id="music-form" onsubmit={save} aria-label={t('settings.music.title')}>
		<SettingsCard id="new-artist" title={t('settings.music.artist')} description={t('settings.music.artistDesc')}>
			<Field id="m-albums" label={t('settings.music.albums')}>
				<select id="m-albums" class="input" bind:value={settings.album_choice}>
					{#each albumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
				</select>
				<span class="hint">{albumChoices.find(([v]) => v === settings.album_choice)?.[2]}</span>
			</Field>
			<Field id="m-new" label={t('settings.music.newAlbums')} hint={t('settings.music.newAlbumsHint')}>
				<select id="m-new" class="input" bind:value={settings.new_albums}>
					{#each newAlbumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
				</select>
			</Field>
			<fieldset class="kinds" id="album-kinds" tabindex="-1">
				<legend class="small"><b>{t('settings.music.albumKinds')}</b> <span class="hint">{t('settings.music.albumKindsHint')}</span></legend>
				<div class="boxes">
					{#each mainKinds as [v, l] (v)}
						<label class="box"><input type="checkbox" checked={settings.album_types.includes(v)} onchange={(e) => (settings.album_types = toggle(settings.album_types, v, e.currentTarget.checked, mainKinds))} /> {l}</label>
					{/each}
				</div>
			</fieldset>
			{#snippet advanced()}
				<fieldset class="kinds" id="extra-kinds" tabindex="-1">
					<legend class="small"><b>{t('settings.music.extraKinds')}</b> <span class="hint">{t('settings.music.extraKindsHint')}</span></legend>
					<div class="boxes">
						{#each extraKinds as [v, l] (v)}
							<label class="box"><input type="checkbox" checked={settings.secondary_types.includes(v)} onchange={(e) => (settings.secondary_types = toggle(settings.secondary_types, v, e.currentTarget.checked, extraKinds))} /> {l}</label>
						{/each}
					</div>
				</fieldset>
				<fieldset class="kinds" id="pressing-statuses" tabindex="-1">
					<legend class="small"><b>{t('settings.music.pressings')}</b></legend>
					<div class="boxes">
						{#each pressingStatuses as [v, l] (v)}
							<label class="box"><input type="checkbox" checked={settings.release_statuses.includes(v)} onchange={(e) => (settings.release_statuses = toggle(settings.release_statuses, v, e.currentTarget.checked, pressingStatuses))} /> {l}</label>
						{/each}
					</div>
				</fieldset>
				<Field id="m-rule" label={t('settings.music.rule')}>
					<select id="m-rule" class="input" bind:value={settings.pressing_rule}>
						{#each pressingRules as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint">{pressingRules.find(([v]) => v === settings.pressing_rule)?.[2]} {t('settings.music.ruleHint')}</span>
				</Field>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="musicbrainz" title="MusicBrainz" description={t('settings.music.mbDesc')}>
			{#snippet advanced()}
				<div class="field">
					<label for="m-url">{t('settings.music.mbAddress')} <span class="muted">{t('settings.music.optional')}</span></label>
					<input id="m-url" class="input mono" bind:value={settings.musicbrainz_url} placeholder="https://musicbrainz.org" autocomplete="off" spellcheck="false" />
					<span class="hint">{t('settings.music.mbHint')}</span>
				</div>
				<Field id="m-hours" label={t('settings.music.cache')}>
					<input id="m-hours" class="input narrow" type="number" min="1" max="720" bind:value={settings.cache_hours} />
				</Field>
			{/snippet}
		</SettingsCard>
		{#if emptyKinds}<p class="form-error" role="status">{emptyKinds}</p>{/if}
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	<SaveBar {dirty} form="music-form" saving={busy} disabled={!!emptyKinds} ondiscard={() => (settings = JSON.parse(saved))} />
{/if}

<style>
	.narrow {
		max-width: 160px;
	}
	.kinds {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.kinds legend {
		margin-bottom: var(--s-2);
	}
	.boxes {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: var(--s-2);
	}
	.box {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-height: var(--tap);
		font-size: var(--text-sm);
	}
</style>
