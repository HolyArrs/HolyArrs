<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import {
		airDateChoices,
		colonChoices,
		episodeExampleRows,
		multiEpisodeChoices,
		illegalChoices,
		musicExampleRows,
		musicWords,
		showWords,
		type EpisodeExamples,
		type FileRules,
		type MusicExamples,
		type NamingSettings,
		type NamingView
	} from '#lib/organise.ts';

	let ns = $state<NamingSettings>({ file_pattern: '', folder_pattern: '', colon: 'smart', illegal: 'replace', rename_on_import: true, adopt_scan_limit: 5000 });
	let fr = $state<FileRules>({ holding_days: 7, min_free_mb: 100, skip_free_space_check: false });
	let words = $state<Record<string, string>>({});
	let example = $state('');
	let exampleFolder = $state('');
	let episodes = $state<EpisodeExamples | undefined>(undefined);
	let music = $state<MusicExamples | undefined>(undefined);
	let exampleError = $state('');
	let loaded = $state(false);
	let busy = $state(false);
	let cleaning = $state(false);
	let error = $state('');
	let ask = 0;
	// What the server holds now, to tell changes and to put them back.
	let saved = $state('');
	const snapshot = () => JSON.stringify({ ns, fr });
	const dirty = $derived(loaded && !!saved && snapshot() !== saved);
	function discard() {
		const v = JSON.parse(saved);
		ns = v.ns;
		fr = v.fr;
	}

	onMount(async () => {
		try {
			const [n, f] = await Promise.all([api.get<NamingView>('/settings/naming'), api.get<FileRules>('/settings/files')]);
			ns = n.naming;
			words = n.words;
			example = n.example;
			exampleFolder = n.example_folder;
			episodes = n.example_episodes;
			music = n.example_music;
			fr = f;
			saved = snapshot();
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	});

	// The live example: what the patterns on screen would name a sample
	// movie. Nothing is saved.
	$effect(() => {
		if (!loaded) return;
		const body = {
			file_pattern: ns.file_pattern,
			folder_pattern: ns.folder_pattern,
			colon: ns.colon,
			illegal: ns.illegal,
			rename_on_import: ns.rename_on_import,
			show_folder_pattern: ns.show_folder_pattern,
			season_folders: ns.season_folders,
			season_folder_pattern: ns.season_folder_pattern,
			specials_folder: ns.specials_folder,
			episode_pattern: ns.episode_pattern,
			daily_pattern: ns.daily_pattern,
			anime_pattern: ns.anime_pattern,
			multi_episode_style: ns.multi_episode_style,
			air_date_style: ns.air_date_style,
			absolute_digits: Number(ns.absolute_digits) || undefined,
			artist_folder_pattern: ns.artist_folder_pattern,
			album_folder_pattern: ns.album_folder_pattern,
			track_pattern: ns.track_pattern,
			multi_disc_track_pattern: ns.multi_disc_track_pattern,
			disc_folders: ns.disc_folders,
			disc_folder_pattern: ns.disc_folder_pattern
		};
		const mine = ++ask;
		const t = setTimeout(async () => {
			try {
				const r = await api.post<{ example: string; example_folder: string; example_episodes?: EpisodeExamples; example_music?: MusicExamples }>(
					'/settings/naming/example',
					body
				);
				if (mine !== ask) return;
				example = r.example;
				exampleFolder = r.example_folder;
				episodes = r.example_episodes;
				music = r.example_music;
				exampleError = '';
			} catch (e) {
				if (mine === ask) exampleError = message(e);
			}
		}, 250);
		return () => clearTimeout(t);
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		const days = Number(fr.holding_days);
		const floor = Number(fr.min_free_mb);
		if (!Number.isInteger(days) || days < 0) {
			error = 'Holding days must be 0 or more.';
			return;
		}
		if (!Number.isFinite(floor) || floor < 100) {
			error = 'Keep at least 100 MB free.';
			return;
		}
		const scanLimit = Number(ns.adopt_scan_limit);
		if (!Number.isInteger(scanLimit) || scanLimit < 10 || scanLimit > 100000) {
			error = 'Folders read per scan must be from 10 to 100000.';
			return;
		}
		const musicLimit = Number(ns.music_adopt_scan_limit ?? 200);
		if (!Number.isInteger(musicLimit) || musicLimit < 10 || musicLimit > 100000) {
			error = 'Artist folders read per music scan must be from 10 to 100000.';
			return;
		}
		busy = true;
		try {
			const n = await api.put<NamingView>('/settings/naming', { ...ns, adopt_scan_limit: scanLimit, music_adopt_scan_limit: musicLimit });
			ns = n.naming;
			fr = await api.put<FileRules>('/settings/files', { holding_days: days, min_free_mb: floor, skip_free_space_check: fr.skip_free_space_check });
			saved = snapshot();
			toasts.push('Naming and file settings saved.', 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function cleanNow() {
		cleaning = true;
		try {
			await api.post('/holding/clean');
			toasts.push(
				fr.holding_days > 0 ? `Emptying holding days older than ${fr.holding_days} days.` : 'Holding days is 0, so held files are kept; nothing is removed.',
				'ok'
			);
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			cleaning = false;
		}
	}

	function addWord(target: 'file_pattern' | 'folder_pattern' | 'episode_pattern' | 'track_pattern', w: string) {
		const cur = ns[target] ?? '';
		ns[target] = cur + (cur && !cur.endsWith(' ') ? ' ' : '') + `{${w}}`;
	}
</script>

<svelte:head><title>Naming and files · HolyArrs</title></svelte:head>

<PageHeader title="Naming and files" />

{#if !loaded}
	<SkSettings rows={4} />
{:else}
	<form class="sc-form stack" id="naming-form" onsubmit={save} aria-label="Naming and files">
		<SettingsCard id="movie-names" title="Movie names" description="How a movie's file and folder are named.">
			<Field id="file-pattern" label="File name pattern" hint="Words in braces are filled in for each movie. A word with nothing to fill in is dropped with its brackets.">
				<input id="file-pattern" class="input mono" bind:value={ns.file_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<Field id="folder-pattern" label="Folder name pattern" hint="For movies added from now on. Folders of movies you already have keep their names.">
				<input id="folder-pattern" class="input mono" bind:value={ns.folder_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<div class="example" aria-live="polite" data-testid="naming-example">
				<p class="xs muted">Example</p>
				{#if exampleError}
					<p class="form-error" role="alert">{exampleError}</p>
				{:else}
					<p class="mono small path">{exampleFolder}/{example}</p>
				{/if}
			</div>
			<details class="words">
				<summary class="small">Words you can use</summary>
				<dl>
					{#each Object.entries(words)
						.filter(([w]) => !showWords.includes(w) && !musicWords.includes(w))
						.sort() as [w, about] (w)}
						<dt>
							<Button variant="plain" class="mono" type="button" onclick={() => addWord('file_pattern', w)} aria-label="Add {'{'}{w}{'}'} to the file name pattern">{'{'}{w}{'}'}</Button>
						</dt>
						<dd class="small muted">{about}</dd>
					{/each}
				</dl>
			</details>
			{#snippet advanced()}
			<Field id="colon" label="A colon in a name becomes" hint="Windows and many shares cannot hold a colon in a name.">
				<select id="colon" class="input" bind:value={ns.colon}>
					{#each colonChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="illegal" label="Other characters a disk cannot hold" hint="What happens to characters such as ? and * that a disk cannot store.">
				<select id="illegal" class="input" bind:value={ns.illegal}>
					{#each illegalChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			{/snippet}
		</SettingsCard>

		<SettingsCard id="show-names" title="Show names" description="How a show, its seasons and its episode files are named.">
			<Field id="show-folder-pattern" label="Show folder pattern" hint="For shows added from now on. Folders of shows you already have keep their names.">
				<input id="show-folder-pattern" class="input mono" bind:value={ns.show_folder_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<label class="checkline">
				<input id="season-folders" type="checkbox" bind:checked={ns.season_folders} />
				<span><b>A folder for each season</b><br /><span class="hint">Off puts every episode straight in the show folder.</span></span>
			</label>
			<Field id="season-folder-pattern" label="Season folder pattern">
				<input id="season-folder-pattern" class="input mono" bind:value={ns.season_folder_pattern} autocomplete="off" spellcheck="false" disabled={!ns.season_folders} />
				<span class="hint">Must hold {'{'}season{'}'} (Season 1) or {'{'}season_padded{'}'} (Season 01).</span>
			</Field>
			<Field id="episode-pattern" label="Episode file name pattern">
				<input id="episode-pattern" class="input mono" bind:value={ns.episode_pattern} autocomplete="off" spellcheck="false" />
				<span class="hint">{'{'}episode{'}'} gives S01E02, or S01E02-E03 for a file holding two episodes.</span>
			</Field>
			<div class="example" aria-live="polite" data-testid="episode-examples">
				<p class="xs muted">Examples</p>
				{#if exampleError}
					<p class="form-error" role="alert">{exampleError}</p>
				{:else}
					<dl class="exlist">
						{#each episodeExampleRows(episodes) as [label, path] (label)}
							<dt class="xs muted">{label}</dt>
							<dd class="mono small path">{path}</dd>
						{/each}
					</dl>
				{/if}
			</div>
			<details class="words">
				<summary class="small">Words for episodes</summary>
				<dl>
					{#each Object.entries(words)
						.filter(([w]) => showWords.includes(w) || ['title', 'year', 'quality', 'group', 'release'].includes(w))
						.sort() as [w, about] (w)}
						<dt>
							<Button variant="plain" class="mono" type="button" onclick={() => addWord('episode_pattern', w)} aria-label="Add {'{'}{w}{'}'} to the episode file name pattern"
								>{'{'}{w}{'}'}</Button>
						</dt>
						<dd class="small muted">{about}</dd>
					{/each}
				</dl>
			</details>
			{#snippet advanced()}
			<Field id="specials-folder" label="Folder for specials (season 0)" hint="A plain name, such as Specials or Season 00.">
				<input id="specials-folder" class="input mono" bind:value={ns.specials_folder} autocomplete="off" spellcheck="false" disabled={!ns.season_folders} />
			</Field>
			<Field id="daily-pattern" label="For shows ordered by air day" hint="Used for talk shows and news, whose episodes are known by date.">
				<input id="daily-pattern" class="input mono" bind:value={ns.daily_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<Field id="anime-pattern" label="For shows ordered by running number" hint="Used for shows whose episodes are counted from 1 across every season.">
				<input id="anime-pattern" class="input mono" bind:value={ns.anime_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<Field id="multi-style" label="A file holding several episodes" hint="How the episode numbers of a two-episode file are written.">
				<select id="multi-style" class="input" bind:value={ns.multi_episode_style}>
					{#each multiEpisodeChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="air-date-style" label="Air day" hint="How the date is written in names of shows ordered by air day.">
				<select id="air-date-style" class="input" bind:value={ns.air_date_style}>
					{#each airDateChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="absolute-digits" label="Digits in a running number" hint="Short numbers get leading zeros up to this many digits.">
				<select id="absolute-digits" class="input" bind:value={ns.absolute_digits}>
					<option value={2}>2: 07</option>
					<option value={3}>3: 007</option>
					<option value={4}>4: 0007</option>
				</select>
			</Field>
			{/snippet}
		</SettingsCard>

		<div data-testid="music-naming">
			<SettingsCard id="music-names" title="Music names" description="How an artist, an album and its track files are named.">
				<Field id="artist-folder-pattern" label="Artist folder pattern">
					<input id="artist-folder-pattern" class="input mono" bind:value={ns.artist_folder_pattern} autocomplete="off" spellcheck="false" />
					<span class="hint"
						>For artists added from now on. {'{'}artist_sort{'}'} moves a leading The to the end (Beatles, The). Folders of artists you already have keep their
						names.</span
					>
				</Field>
				<Field id="album-folder-pattern" label="Album folder pattern">
					<input id="album-folder-pattern" class="input mono" bind:value={ns.album_folder_pattern} autocomplete="off" spellcheck="false" />
					<span class="hint">Must hold {'{'}album{'}'}. {'{'}edition{'}'} gives the Pressing's note, such as deluxe.</span>
				</Field>
				<Field id="track-pattern" label="Track file name pattern">
					<input id="track-pattern" class="input mono" bind:value={ns.track_pattern} autocomplete="off" spellcheck="false" />
					<span class="hint">Must hold {'{'}track{'}'} (03).</span>
				</Field>
				<div class="example" aria-live="polite" data-testid="music-examples">
					<p class="xs muted">Examples</p>
					{#if exampleError}
						<p class="form-error" role="alert">{exampleError}</p>
					{:else}
						<dl class="exlist">
							{#each musicExampleRows(music) as [label, path] (label)}
								<dt class="xs muted">{label}</dt>
								<dd class="mono small path">{path}</dd>
							{/each}
						</dl>
					{/if}
				</div>
				<details class="words">
					<summary class="small">Words for music</summary>
					<dl>
						{#each Object.entries(words)
							.filter(([w]) => musicWords.includes(w) || ['year', 'edition'].includes(w))
							.sort() as [w, about] (w)}
							<dt>
								<Button variant="plain" class="mono" type="button" onclick={() => addWord('track_pattern', w)} aria-label="Add {'{'}{w}{'}'} to the track file name pattern"
									>{'{'}{w}{'}'}</Button>
							</dt>
							<dd class="small muted">{about}</dd>
						{/each}
					</dl>
				</details>
				{#snippet advanced()}
				<Field id="multi-disc-pattern" label="Track file name on an album of more than one disc">
					<input id="multi-disc-pattern" class="input mono" bind:value={ns.multi_disc_track_pattern} autocomplete="off" spellcheck="false" disabled={ns.disc_folders} />
					<span class="hint">Must hold {'{'}disc{'}'} and {'{'}track{'}'} (2-03). With a folder for each disc the track pattern is used inside it.</span>
				</Field>
				<label class="checkline">
					<input id="disc-folders" type="checkbox" bind:checked={ns.disc_folders} />
					<span><b>A folder for each disc</b><br /><span class="hint">Only on albums of more than one disc. Off keeps every track in the album folder.</span></span>
				</label>
				<Field id="disc-folder-pattern" label="Disc folder pattern">
					<input id="disc-folder-pattern" class="input mono" bind:value={ns.disc_folder_pattern} autocomplete="off" spellcheck="false" disabled={!ns.disc_folders} />
					<span class="hint">Must hold {'{'}disc{'}'} (Disc 2).</span>
				</Field>
				<label class="checkline">
					<input id="read-tags" type="checkbox" bind:checked={ns.music_read_tags} />
					<span
						><b>Read a track's own tags when its name is not enough</b><br /><span class="hint"
							>FLAC and MP3 only, and only to tell which track a file is (title, track and disc numbers) when its name leaves it open. Tags are never
							changed.</span
						></span
					>
				</label>
				{/snippet}
			</SettingsCard>
		</div>

		<SettingsCard id="importing" title="Importing" description="What happens to a file's name when it comes in.">
			<label class="checkline">
				<input id="rename-on-import" type="checkbox" bind:checked={ns.rename_on_import} />
				<span
					><b>Rename files when they are imported</b><br /><span class="hint"
						>Off keeps the name the Release came with. Files you already have are only renamed when you ask, from Rename files.</span
					></span
				>
			</label>
		</SettingsCard>

		<SettingsCard id="where-files-go" title="Where files go" description="Deleted files, free space and how big a library scan is.">
			<Field id="holding-days" label="Keep deleted files for (days)">
				<input id="holding-days" class="input narrow" type="number" min="0" max="3650" bind:value={fr.holding_days} />
				<span class="hint"
					>A deleted or replaced file first goes to the holding folder in its Library folder (.holyarrs-replaced). After this many days it is removed for good. 0
					keeps them until you empty it yourself.</span
				>
			</Field>
			{#snippet advanced()}
			<Field id="scan-limit" label="Folders read per Import a library scan" hint="A bigger library is scanned in parts: the scan offers Scan the rest for the next part.">
				<input id="scan-limit" class="input narrow" type="number" min="10" max="100000" bind:value={ns.adopt_scan_limit} />
			</Field>
			<Field id="music-scan-limit" label="Artist folders read per music scan" hint="Each artist folder takes a few MusicBrainz lookups, about one a second, so music is scanned in smaller parts.">
				<input id="music-scan-limit" class="input narrow" type="number" min="10" max="100000" bind:value={ns.music_adopt_scan_limit} />
			</Field>
			<Field id="min-free" label="Always leave this much free space (MB)" hint="An import that would leave less than this free is refused before anything is written. 100 MB is the lowest.">
				<input id="min-free" class="input narrow" type="number" min="100" bind:value={fr.min_free_mb} />
			</Field>
			<label class="checkline">
				<input id="skip-free-check" type="checkbox" bind:checked={fr.skip_free_space_check} />
				<span><b>Skip the free-space check</b><br /><span class="hint">Only for disks that report their free space wrongly.</span></span>
			</label>
			{/snippet}
		</SettingsCard>

		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>

	<SettingsCard id="holding-card" title="Holding folder" description="HolyArrs empties old holding days once a day. Run it now to free the space straight away.">
		<div class="row" style="margin-top: var(--s-4)">
			<Button type="button" onclick={cleanNow} disabled={cleaning} aria-busy={cleaning}>Empty old holding days now</Button>
		</div>
	</SettingsCard>
	<SaveBar {dirty} form="naming-form" saving={busy} disabled={!!exampleError} ondiscard={discard} />
{/if}

<style>
	.stack {
		display: flex;
		flex-direction: column;
	}
	.narrow {
		max-width: 160px;
	}
	.example {
		margin: var(--s-2) 0 var(--s-4);
		padding: var(--s-3);
		border-left: 3px solid var(--ok);
		background: var(--surface-2);
		border-radius: var(--r-sm);
	}
	.path {
		overflow-wrap: anywhere;
	}
	.words {
		margin-bottom: var(--s-4);
	}
	.words dl {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: var(--s-1) var(--s-3);
		align-items: center;
		margin-top: var(--s-2);
	}
	.words dd {
		margin: 0;
	}
	.checkline {
		margin-top: var(--s-4);
	}
	.exlist {
		display: grid;
		gap: var(--s-1);
		margin: var(--s-1) 0 0;
	}
	.exlist dd {
		margin: 0 0 var(--s-2);
	}
</style>
