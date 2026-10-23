<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	// Settings > Music: what a new artist gets by default, which Pressing is
	// used for an album, and where MusicBrainz answers come from.
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { albumChoices, extraKinds, mainKinds, newAlbumChoices, pressingRules, pressingStatuses, toggle } from '#lib/music/music.ts';
	import type { MusicSettings } from '#lib/types.ts';

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
	const dirty = $derived(loaded && !!saved && JSON.stringify(settings) !== saved);

	onMount(async () => {
		try {
			settings = await api.get<MusicSettings>('/settings/music');
			saved = JSON.stringify(settings);
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			settings = await api.put<MusicSettings>('/settings/music', { ...settings, musicbrainz_url: settings.musicbrainz_url.trim(), cache_hours: Number(settings.cache_hours) });
			saved = JSON.stringify(settings);
			toasts.push('Music settings saved.', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Music defaults · HolyArrs</title></svelte:head>

<PageHeader title="Music defaults" subtitle="What a new artist gets, which Pressing an album follows, and where artist and album details come from." />

{#if !loaded}
	<SkSettings rows={3} />
{:else}
	<form class="sc-form" id="music-form" onsubmit={save} aria-label="Music defaults">
		<SettingsCard id="new-artist" title="A new artist" description="What every artist you add gets, unless you choose otherwise when adding.">
			<Field id="m-albums" label="Albums to keep up to date">
				<select id="m-albums" class="input" bind:value={settings.album_choice}>
					{#each albumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
				</select>
				<span class="hint">{albumChoices.find(([v]) => v === settings.album_choice)?.[2]}</span>
			</Field>
			<Field id="m-new" label="New albums" hint="What happens when an artist you have puts out a new album.">
				<select id="m-new" class="input" bind:value={settings.new_albums}>
					{#each newAlbumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
				</select>
			</Field>
			<fieldset class="kinds" id="album-kinds" tabindex="-1">
				<legend class="small"><b>Album kinds</b> <span class="hint">Albums of other kinds are listed but left out.</span></legend>
				<div class="boxes">
					{#each mainKinds as [v, l] (v)}
						<label class="box"><input type="checkbox" checked={settings.album_types.includes(v)} onchange={(e) => (settings.album_types = toggle(settings.album_types, v, e.currentTarget.checked, mainKinds))} /> {l}</label>
					{/each}
				</div>
			</fieldset>
			{#snippet advanced()}
				<fieldset class="kinds" id="extra-kinds" tabindex="-1">
					<legend class="small"><b>Extra kinds</b> <span class="hint">An album with an extra kind, such as Live, needs it ticked.</span></legend>
					<div class="boxes">
						{#each extraKinds as [v, l] (v)}
							<label class="box"><input type="checkbox" checked={settings.secondary_types.includes(v)} onchange={(e) => (settings.secondary_types = toggle(settings.secondary_types, v, e.currentTarget.checked, extraKinds))} /> {l}</label>
						{/each}
					</div>
				</fieldset>
				<fieldset class="kinds" id="pressing-statuses" tabindex="-1">
					<legend class="small"><b>Pressings that count</b> <span class="hint">Only Pressings of these kinds are chosen.</span></legend>
					<div class="boxes">
						{#each pressingStatuses as [v, l] (v)}
							<label class="box"><input type="checkbox" checked={settings.release_statuses.includes(v)} onchange={(e) => (settings.release_statuses = toggle(settings.release_statuses, v, e.currentTarget.checked, pressingStatuses))} /> {l}</label>
						{/each}
					</div>
				</fieldset>
				<Field id="m-rule" label="Which Pressing an album follows">
					<select id="m-rule" class="input" bind:value={settings.pressing_rule}>
						{#each pressingRules as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint">{pressingRules.find(([v]) => v === settings.pressing_rule)?.[2]} A Pressing picked by hand on the album page stays.</span>
				</Field>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="musicbrainz" title="MusicBrainz" description="Where artist and album details come from.">
			<p class="hint">HolyArrs asks musicbrainz.org, at most once a second.</p>
			{#snippet advanced()}
				<div class="field">
					<label for="m-url">MusicBrainz address <span class="muted">(optional)</span></label>
					<input id="m-url" class="input mono" bind:value={settings.musicbrainz_url} placeholder="https://musicbrainz.org" autocomplete="off" spellcheck="false" />
					<span class="hint">Leave empty for musicbrainz.org. A mirror of your own works too. HolyArrs asks at most once a second.</span>
				</div>
				<Field id="m-hours" label="Keep answers for (hours)" hint="Answers are reused for this long before HolyArrs asks again.">
					<input id="m-hours" class="input narrow" type="number" min="1" max="720" bind:value={settings.cache_hours} />
				</Field>
			{/snippet}
		</SettingsCard>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	<SaveBar {dirty} form="music-form" saving={busy} disabled={!settings.album_types.length || !settings.secondary_types.length || !settings.release_statuses.length} ondiscard={() => (settings = JSON.parse(saved))} />
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
