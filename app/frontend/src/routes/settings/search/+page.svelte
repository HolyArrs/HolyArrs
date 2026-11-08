<script lang="ts">
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
			error = 'Use 10 to 120, or 0 for off';
			return;
		}
		busy = true;
		try {
			settings = await api.put<SearchSettings>('/settings/search', body);
			saved = JSON.stringify(settings);
			toasts.push('Search settings saved', 'ok');
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
			toasts.push("Couldn't start search", 'fail', { body: message(e) });
		} finally {
			now = '';
		}
	}
</script>

<svelte:head><title>Search · HolyArrs</title></svelte:head>

<PageHeader title="Search">
	{#snippet actions()}
		{#if loaded && !loadError}
			<span class="now" id="search-now">
				<Button type="button" disabled={!!now} aria-busy={now === '/search/feed-check'} onclick={() => run('/search/feed-check', 'Checking every feed')}>Check feeds now</Button>
				<Button type="button" disabled={!!now} aria-busy={now === '/search/better'} onclick={() => run('/search/better', 'Looking for better copies')}>Find a better copy for all</Button>
			</span>
		{/if}
	{/snippet}
</PageHeader>

{#if !loaded}
	<SkSettings rows={3} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	<form class="sc-form" id="search-form" onsubmit={save} aria-label="Search settings">
		<SettingsCard id="nightly" title="Nightly search">
				<Field id="missing-count" label="Titles to search each night" hint="Oldest searched first. 0 turns it off.">
					<input id="missing-count" class="input narrow" type="number" min="0" max="500" bind:value={settings.missing_search_count} />
				</Field>
				<label class="checkline">
					<input id="better-nightly" type="checkbox" bind:checked={settings.missing_search_better} />
					<span><b>Also find better copies each night</b><br /><span class="hint">For titles below Good enough</span></span>
				</label>
				<label class="checkline">
					<input id="missing-episodes" type="checkbox" bind:checked={settings.missing_search_shows} />
					<span
						><b>Search for missing episodes too</b><br /><span class="hint"
							>Aired episodes; a whole season when several are missing</span
						></span
					>
				</label>
			{#snippet advanced()}
						<Field id="feed-minutes" label="Check Source feeds every (minutes)" hint="10–120; 0 turns them off">
							<input id="feed-minutes" class="input narrow" type="number" min="0" max="120" bind:value={settings.feed_check_minutes} />
						</Field>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="early-fake" title="Early and fake Releases" description="Turned down before they download">
				<label class="checkline">
					<input id="refuse-early" type="checkbox" bind:checked={settings.refuse_early_releases} data-testid="refuse-early" />
					<span
						><b>Turn down Releases posted before release</b><br /><span class="hint"
							>Likely fakes. Choose a release still shows them, with a warning.</span
						></span
					>
				</label>
				<label class="checkline">
					<input id="refuse-tiny" type="checkbox" bind:checked={settings.refuse_tiny_releases} data-testid="refuse-tiny" />
					<span
						><b>Turn down fakes by size and file type</b><br /><span class="hint"
							>Too-small Releases, empty files and .wmv or .asf under HD names</span
						></span
					>
				</label>
			{#snippet advanced()}
						<Field id="early-grace" label="Hours a Release may be early">
							<input id="early-grace" class="input narrow" type="number" min="0" max="720" bind:value={settings.early_release_grace_hours} />
							<p class="hint">For time zones. Default 24.</p>
						</Field>
						<label class="checkline">
							<input id="import-floors" type="checkbox" bind:checked={settings.import_size_floors} data-testid="import-floors" />
							<span
								><b>Check file sizes at import</b><br /><span class="hint"
									>Refuses files far too small for their length</span
								></span
							>
						</label>
						<Field id="min-video" label="Smallest video file to place (MB)">
							<input id="min-video" class="input narrow" type="number" min="0" max="10000" bind:value={settings.import_min_video_mb} />
							<p class="hint">0 uses 1 MB a minute of the video's length, or 20 MB when the length is not known.</p>
						</Field>
						<Field id="min-track" label="Smallest track file to place (KB)">
							<input id="min-track" class="input narrow" type="number" min="0" max="100000" bind:value={settings.import_min_track_kb} />
							<p class="hint">0 uses 512 KB.</p>
						</Field>
						<Field id="episode-minutes" label="Episode length when not known (minutes)">
							<input id="episode-minutes" class="input narrow" type="number" min="1" max="240" bind:value={settings.episode_default_minutes} />
							<p class="hint">For season pack size checks. Default 20.</p>
						</Field>
						<label class="checkline">
							<input id="hold-unknown" type="checkbox" bind:checked={settings.hold_unknown_releases} data-testid="hold-unknown" />
							<span
								><b>Hold Releases of unknown size or date for you</b><br /><span class="hint"
									>The checks above can't run without size or date</span
								></span
							>
						</label>
						<p class="hint">Program and fake files are handled in Downloaders</p>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="season-packs" title="Season packs">
				<Field id="pack-upgrade" label="A season pack is taken when">
					<select id="pack-upgrade" class="input" bind:value={settings.season_pack_upgrade}>
						<option value="all">Every episode in it would be new or better</option>
						<option value="any">At least one episode in it would be new or better</option>
					</select>
				</Field>
				<Field id="pack-prefer" label="Season pack or single episodes" hint="Which wins when a season pack and single episodes are both on offer.">
					<select id="pack-prefer" class="input" bind:value={settings.season_pack_prefer}>
						<option value="rank">Best copy first; a season pack wins when they are equal</option>
						<option value="always">A season pack whenever one passes the quality set</option>
					</select>
				</Field>
			{#snippet advanced()}
						<label class="checkline">
							<input id="multi-season" type="checkbox" bind:checked={settings.multi_season_packs} />
							<span
								><b>Take Releases with several seasons</b><br /><span class="hint"
									>Fills every missing episode in it</span
								></span
							>
						</label>
			{/snippet}
		</SettingsCard>
		<SettingsCard id="music-search" title="Music">
				<label class="checkline">
					<input id="missing-albums" type="checkbox" bind:checked={settings.missing_search_music} data-testid="missing-music" />
					<span
						><b>Search for missing albums too</b><br /><span class="hint"
							>Released albums, one search each</span
						></span
					>
				</label>
			{#snippet advanced()}
						<label class="checkline">
							<input id="discographies" type="checkbox" bind:checked={settings.music_discography_packs} />
							<span
								><b>Take whole discographies</b></span
							>
						</label>
						<Field id="track-tolerance" label="Tracks an album Release may lack">
							<input id="track-tolerance" class="input" type="number" min="-1" max="50" bind:value={settings.album_track_tolerance} />
							<p class="hint">0 needs every track; -1 skips the check</p>
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
