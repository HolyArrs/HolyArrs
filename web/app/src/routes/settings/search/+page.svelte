<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { feedMinutesOk } from '#lib/releases.ts';
	import type { SearchSettings } from '#lib/types.ts';

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
	let now = $state(false);

	onMount(async () => {
		try {
			settings = await api.get<SearchSettings>('/settings/search');
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	});

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
			error = 'Check feeds every 10 to 120 minutes, or 0 to turn feed checks off.';
			return;
		}
		busy = true;
		try {
			settings = await api.put<SearchSettings>('/settings/search', body);
			toasts.push('Search settings saved.', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function run(path: string, said: string) {
		now = true;
		try {
			await api.post(path);
			toasts.push(said, 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			now = false;
		}
	}
</script>

<svelte:head><title>Search settings · HolyArrs</title></svelte:head>

<PageHeader title="Search" subtitle="How HolyArrs looks for Releases by itself." />

{#if !loaded}
	<div class="sk" style="height: 240px"></div>
{:else}
	<form class="card" onsubmit={save}>
		<div class="field">
			<label for="feed-minutes">Check Source feeds every (minutes)</label>
			<input id="feed-minutes" class="input narrow" type="number" min="0" max="120" bind:value={settings.feed_check_minutes} />
			<span class="hint">10 to 120 minutes; 0 turns feed checks off. Each check reads the newest Releases of every Source with feeds on and picks for movies and episodes that want one.</span>
		</div>
		<div class="field">
			<label for="missing-count">Movies to search for each night</label>
			<input id="missing-count" class="input narrow" type="number" min="0" max="500" bind:value={settings.missing_search_count} />
			<span class="hint">Once a day HolyArrs searches for this many missing movies and episodes, the ones searched longest ago first. 0 turns it off.</span>
		</div>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.missing_search_better} />
			<span><b>Also Find a better copy each night</b><br /><span class="hint">The nightly search also looks for better copies of movies below Good enough.</span></span>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.missing_search_shows} />
			<span
				><b>Search for missing episodes too</b><br /><span class="hint"
					>The nightly search and Search every missing one also cover episodes that have aired, a whole season at a time when several are missing.</span
				></span
			>
		</label>
		<h2 class="h2 group">Season packs</h2>
		<div class="field">
			<label for="pack-upgrade">A season pack is taken when</label>
			<select id="pack-upgrade" class="input" bind:value={settings.season_pack_upgrade}>
				<option value="all">Every episode in it would be new or better</option>
				<option value="any">At least one episode in it would be new or better</option>
			</select>
			<span class="hint">With the second choice a pack fills only the episodes it improves; the copies you have stay.</span>
		</div>
		<div class="field">
			<label for="pack-prefer">Season pack or single episodes</label>
			<select id="pack-prefer" class="input" bind:value={settings.season_pack_prefer}>
				<option value="rank">Best copy first; a season pack wins when they are equal</option>
				<option value="always">A season pack whenever one passes the Pick rules</option>
			</select>
		</div>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.multi_season_packs} />
			<span
				><b>Take Releases with several seasons</b><br /><span class="hint"
					>Off: a Release holding seasons 1 to 3, or a whole show, is turned away. On: it fills every missing episode it holds.</span
				></span
			>
		</label>
		<h2 class="h2 group">Music</h2>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.missing_search_music} data-testid="missing-music" />
			<span
				><b>Search for missing albums too</b><br /><span class="hint"
					>The nightly search and Search every missing one also cover albums that are out, one search per album.</span
				></span
			>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.music_discography_packs} />
			<span
				><b>Take whole discographies</b><br /><span class="hint">Off: a Release holding every album of an artist is turned away. On: it may fill the album searched for.</span></span
			>
		</label>
		<div class="field">
			<label for="track-tolerance">Tracks an album Release may lack</label>
			<input id="track-tolerance" class="input" type="number" min="-1" max="50" bind:value={settings.album_track_tolerance} />
			<p class="hint">When a Source says how many files a Release holds: 0 turns away any Release with fewer files than the album has tracks; -1 never checks.</p>
		</div>
		<h2 class="h2 group">Early and fake Releases</h2>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.refuse_early_releases} data-testid="refuse-early" />
			<span
				><b>Turn away Releases posted before it came out</b><br /><span class="hint"
					>A Release posted before the movie came out, the episode aired or the album was released is likely a fake. Your own searches show it with a warning instead.</span
				></span
			>
		</label>
		<div class="field">
			<label for="early-grace">Hours a Release may be early</label>
			<input id="early-grace" class="input narrow" type="number" min="0" max="720" bind:value={settings.early_release_grace_hours} />
			<p class="hint">For time zones and early air times. 24 by default.</p>
		</div>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.refuse_tiny_releases} data-testid="refuse-tiny" />
			<span
				><b>Turn away fakes by size and file kind</b><br /><span class="hint"
					>A Release far too small for what it says it holds is turned away. At import an empty file, or an old .wmv or .asf file under an HD name, is refused and the next Release is taken.</span
				></span
			>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.import_size_floors} data-testid="import-floors" />
			<span
				><b>Check file sizes at import</b><br /><span class="hint"
					>A file far too small for its length, or a download much smaller than its Release said, is refused. Off, only empty files and files under the sizes you set below are refused.</span
				></span
			>
		</label>
		<div class="field">
			<label for="min-video">Smallest video file to place (MB)</label>
			<input id="min-video" class="input narrow" type="number" min="0" max="10000" bind:value={settings.import_min_video_mb} />
			<p class="hint">0 uses 1 MB a minute of the video's length, or 20 MB when the length is not known.</p>
		</div>
		<div class="field">
			<label for="min-track">Smallest track file to place (KB)</label>
			<input id="min-track" class="input narrow" type="number" min="0" max="100000" bind:value={settings.import_min_track_kb} />
			<p class="hint">0 uses 512 KB.</p>
		</div>
		<div class="field">
			<label for="episode-minutes">Episode length when not known (minutes)</label>
			<input id="episode-minutes" class="input narrow" type="number" min="1" max="240" bind:value={settings.episode_default_minutes} />
			<p class="hint">Used to judge whether a season pack is too small. 20 by default.</p>
		</div>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.hold_unknown_releases} data-testid="hold-unknown" />
			<span
				><b>Hold Releases of unknown size or date for you</b><br /><span class="hint"
					>When a site does not say how big a Release is or when it was posted, the checks above cannot run. Off, automatic searches may still take it and show a warning; on, it waits for you to choose.</span
				></span
			>
		</label>
		<p class="hint">Program files, password notes, web shortcuts and torrents holding only archives are failed in the Downloader (Settings, Downloaders, Fail downloads that hold programs or fakes).</p>
		<h2 class="h2 group">Downloaders</h2>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.try_next_downloader} />
			<span><b>Try the next Downloader</b><br /><span class="hint">When a Downloader cannot take a Release, hand it to the next one of the same kind.</span></span>
		</label>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<div class="row" style="margin-top: var(--s-5)">
			<Button variant="primary" type="submit" disabled={busy} aria-busy={busy}>Save</Button>
		</div>
	</form>

	<SettingsCard title="Search now" description="Start a search without waiting for the timer. Home shows it running and what it found.">
		<div class="row now">
			<Button type="button" disabled={now} onclick={() => run('/search/feed-check', 'Checking every feed now.')}>Check feeds now</Button>
			<Button type="button" disabled={now} onclick={() => run('/search/missing', 'Searching for every missing movie and episode.')}>Search every missing one</Button>
			<Button type="button" disabled={now} onclick={() => run('/search/better', 'Looking for better copies of everything below Good enough.')}>Find a better copy for all</Button>
		</div>
	</SettingsCard>
{/if}

<style>
	.narrow {
		max-width: 160px;
	}
	.checkline {
		margin-top: var(--s-4);
	}
	.group {
		margin-top: var(--s-6);
		margin-bottom: var(--s-2);
	}
	.now {
		margin-top: var(--s-4);
		gap: var(--s-2);
		flex-wrap: wrap;
	}
</style>
