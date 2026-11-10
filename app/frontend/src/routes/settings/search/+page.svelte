<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { feedMinutesOk } from '#lib/mediafiles/index.ts';
	import type { SearchSettings } from '#lib/sources/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	let settings = $state<SearchSettings>({
		feed_check_minutes: 15,
		missing_search_count: 20,
		missing_search_better: false,
		try_next_downloader: true,
		missing_search_shows: true,
		season_pack_upgrade: 'all',
		season_pack_prefer: 'rank',
		multi_season_packs: false,
		missing_search_music: true,
		music_discography_packs: false,
		album_track_tolerance: 0,
		refuse_early_releases: true,
		early_release_grace_hours: 24,
		refuse_tiny_releases: true,
		import_min_video_mb: 0,
		import_min_track_kb: 0,
		import_size_floors: true,
		hold_unknown_releases: false,
		episode_default_minutes: 20
	});
	let loaded = $state(false);
	let busy = $state(false);
	let error = $state('');
	let loadError = $state('');
	// The path of the search started from Search now, while it is being sent.
	let now = $state('');
	let saved = $state('');
	const dirty = $derived(loaded && !!saved && JSON.stringify(settings) !== saved);

	async function load() {
		loaded = false;
		loadError = '';
		try {
			settings = await api.get<SearchSettings>('/settings/search');
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
		error = '';
		const body = {
			...settings,
			feed_check_minutes: Number(settings.feed_check_minutes),
			missing_search_count: Number(settings.missing_search_count),
			early_release_grace_hours: Number(settings.early_release_grace_hours),
			import_min_video_mb: Number(settings.import_min_video_mb),
			import_min_track_kb: Number(settings.import_min_track_kb),
			episode_default_minutes: Number(settings.episode_default_minutes)
		};
		if (!feedMinutesOk(body.feed_check_minutes)) {
			error = t('settings.search.feedRange');
			return;
		}
		busy = true;
		try {
			settings = await api.put<SearchSettings>('/settings/search', body);
			saved = JSON.stringify(settings);
			toasts.push(t('settings.search.saved'), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function run(path: string, said: string) {
		now = path;
		try {
			await api.post(path);
			toasts.push(said, 'ok');
		} catch (e) {
			toasts.push(t('settings.search.couldntStart'), 'fail', { body: message(e) });
		} finally {
			now = '';
		}
	}
</script>

<svelte:head><title>{t('settings.search.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.search.title')}>
	{#snippet actions()}
		{#if loaded && !loadError}
			<span class="now" id="search-now">
				<Button type="button" disabled={!!now} aria-busy={now === '/search/feed-check'} onclick={() => run('/search/feed-check', t('settings.search.checkingFeeds'))}>{t('settings.search.checkFeeds')}</Button>
				<Button type="button" disabled={!!now} aria-busy={now === '/search/better'} onclick={() => run('/search/better', t('settings.search.lookingBetter'))}>{t('settings.search.findBetter')}</Button>
			</span>
		{/if}
	{/snippet}
</PageHeader>

{#if !loaded}
	<SkSettings rows={3} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	<form class="sc-form" id="search-form" onsubmit={save} aria-label={t('settings.search.settings')}>
		<SettingsCard id="nightly" title={t('settings.search.nightly')}>
				<Field id="missing-count" label={t('settings.search.count')} hint={t('settings.search.countHint')}>
					<input id="missing-count" class="input narrow" type="number" min="0" max="500" bind:value={settings.missing_search_count} />
				</Field>
				<label class="checkline">
					<input id="better-nightly" type="checkbox" bind:checked={settings.missing_search_better} />
					<span><b>{t('settings.search.betterNightly')}</b><br /><span class="hint">{t('settings.search.betterNightlyHint')}</span></span>
				</label>
				<label class="checkline">
					<input id="missing-episodes" type="checkbox" bind:checked={settings.missing_search_shows} />
					<span
						><b>{t('settings.search.episodes')}</b><br /><span class="hint"
							>{t('settings.search.episodesHint')}</span
						></span
					>
				</label>
			{#snippet advanced()}
						<Field id="feed-minutes" label={t('settings.search.feedMinutes')} hint={t('settings.search.feedMinutesHint')}>
							<input id="feed-minutes" class="input narrow" type="number" min="0" max="120" bind:value={settings.feed_check_minutes} />
						</Field>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="early-fake" title={t('settings.search.early')} description={t('settings.search.earlyDesc')}>
				<label class="checkline">
					<input id="refuse-early" type="checkbox" bind:checked={settings.refuse_early_releases} data-testid="refuse-early" />
					<span
						><b>{t('settings.search.refuseEarly')}</b><br /><span class="hint"
							>{t('settings.search.refuseEarlyHint')}</span
						></span
					>
				</label>
				<label class="checkline">
					<input id="refuse-tiny" type="checkbox" bind:checked={settings.refuse_tiny_releases} data-testid="refuse-tiny" />
					<span
						><b>{t('settings.search.refuseTiny')}</b><br /><span class="hint"
							>{t('settings.search.refuseTinyHint')}</span
						></span
					>
				</label>
			{#snippet advanced()}
						<Field id="early-grace" label={t('settings.search.grace')}>
							<input id="early-grace" class="input narrow" type="number" min="0" max="720" bind:value={settings.early_release_grace_hours} />
							<p class="hint">{t('settings.search.graceHint')}</p>
						</Field>
						<label class="checkline">
							<input id="import-floors" type="checkbox" bind:checked={settings.import_size_floors} data-testid="import-floors" />
							<span
								><b>{t('settings.search.floors')}</b><br /><span class="hint"
									>{t('settings.search.floorsHint')}</span
								></span
							>
						</label>
						<Field id="min-video" label={t('settings.search.minVideo')}>
							<input id="min-video" class="input narrow" type="number" min="0" max="10000" bind:value={settings.import_min_video_mb} />
							<p class="hint">{t('settings.search.minVideoHint')}</p>
						</Field>
						<Field id="min-track" label={t('settings.search.minTrack')}>
							<input id="min-track" class="input narrow" type="number" min="0" max="100000" bind:value={settings.import_min_track_kb} />
							<p class="hint">{t('settings.search.minTrackHint')}</p>
						</Field>
						<Field id="episode-minutes" label={t('settings.search.episodeMinutes')}>
							<input id="episode-minutes" class="input narrow" type="number" min="1" max="240" bind:value={settings.episode_default_minutes} />
							<p class="hint">{t('settings.search.episodeMinutesHint')}</p>
						</Field>
						<label class="checkline">
							<input id="hold-unknown" type="checkbox" bind:checked={settings.hold_unknown_releases} data-testid="hold-unknown" />
							<span
								><b>{t('settings.search.hold')}</b><br /><span class="hint"
									>{t('settings.search.holdHint')}</span
								></span
							>
						</label>
						<p class="hint">{t('settings.search.programNote')}</p>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="season-packs" title={t('settings.search.packs')}>
				<Field id="pack-upgrade" label={t('settings.search.packUpgrade')}>
					<select id="pack-upgrade" class="input" bind:value={settings.season_pack_upgrade}>
						<option value="all">{t('settings.search.packAll')}</option>
						<option value="any">{t('settings.search.packAny')}</option>
					</select>
				</Field>
				<Field id="pack-prefer" label={t('settings.search.packPrefer')} hint={t('settings.search.packPreferHint')}>
					<select id="pack-prefer" class="input" bind:value={settings.season_pack_prefer}>
						<option value="rank">{t('settings.search.preferRank')}</option>
						<option value="always">{t('settings.search.preferAlways')}</option>
					</select>
				</Field>
			{#snippet advanced()}
						<label class="checkline">
							<input id="multi-season" type="checkbox" bind:checked={settings.multi_season_packs} />
							<span
								><b>{t('settings.search.multi')}</b><br /><span class="hint"
									>{t('settings.search.multiHint')}</span
								></span
							>
						</label>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="music-search" title={t('settings.search.music')}>
				<label class="checkline">
					<input id="missing-albums" type="checkbox" bind:checked={settings.missing_search_music} data-testid="missing-music" />
					<span
						><b>{t('settings.search.albums')}</b><br /><span class="hint"
							>{t('settings.search.albumsHint')}</span
						></span
					>
				</label>
			{#snippet advanced()}
						<label class="checkline">
							<input id="discographies" type="checkbox" bind:checked={settings.music_discography_packs} />
							<span
								><b>{t('settings.search.discographies')}</b></span
							>
						</label>
						<Field id="track-tolerance" label={t('settings.search.tolerance')}>
							<input id="track-tolerance" class="input" type="number" min="-1" max="50" bind:value={settings.album_track_tolerance} />
							<p class="hint">{t('settings.search.toleranceHint')}</p>
						</Field>
			{/snippet}
		</SettingsCard>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	<SaveBar {dirty} form="search-form" saving={busy} ondiscard={() => (settings = JSON.parse(saved))} />

{/if}

<style>
	.narrow {
		max-width: 160px;
	}
	.checkline {
		margin-top: var(--s-4);
	}
	.now {
		display: inline-flex;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
</style>
