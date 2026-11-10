<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { airDateChoices, colonChoices, episodeExampleRows, multiEpisodeChoices, illegalChoices, musicExampleRows, musicWords, showWords, type EpisodeExamples, type FileRules, type MusicExamples, type NamingSettings, type NamingView } from '#lib/mediafiles/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

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
	let loadError = $state('');
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

	async function load() {
		loaded = false;
		loadError = '';
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
			loadError = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	// The live example: what the patterns on screen would name a sample movie. Nothing is saved.
	$effect(() => {
		if (!loaded || loadError) return;
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
		const timer = setTimeout(async () => {
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
		return () => clearTimeout(timer);
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		const days = Number(fr.holding_days);
		const floor = Number(fr.min_free_mb);
		if (!Number.isInteger(days) || days < 0) {
			error = t('settings.naming.useDays');
			return;
		}
		if (!Number.isFinite(floor) || floor < 100) {
			error = t('settings.naming.keepFree');
			return;
		}
		const scanLimit = Number(ns.adopt_scan_limit);
		if (!Number.isInteger(scanLimit) || scanLimit < 10 || scanLimit > 100000) {
			error = t('settings.naming.useRange');
			return;
		}
		const musicLimit = Number(ns.music_adopt_scan_limit ?? 200);
		if (!Number.isInteger(musicLimit) || musicLimit < 10 || musicLimit > 100000) {
			error = t('settings.naming.useRange');
			return;
		}
		busy = true;
		try {
			const n = await api.put<NamingView>('/settings/naming', { ...ns, adopt_scan_limit: scanLimit, music_adopt_scan_limit: musicLimit });
			ns = n.naming;
			fr = await api.put<FileRules>('/settings/files', { holding_days: days, min_free_mb: floor, skip_free_space_check: fr.skip_free_space_check });
			saved = snapshot();
			toasts.push(t('settings.naming.saved'), 'ok');
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
				fr.holding_days > 0 ? t('settings.naming.removingHeld') : t('settings.naming.nothingRemoved'),
				'info', { body: fr.holding_days > 0 ? t('settings.naming.olderThan', { n: fr.holding_days }) : t('settings.naming.keptUntil') }
			);
		} catch (e) {
			toasts.push(t('settings.naming.cleanFailed'), 'fail', { body: message(e) });
		} finally {
			cleaning = false;
		}
	}

	function addWord(target: 'file_pattern' | 'folder_pattern' | 'episode_pattern' | 'track_pattern', w: string) {
		const cur = ns[target] ?? '';
		ns[target] = cur + (cur && !cur.endsWith(' ') ? ' ' : '') + `{${w}}`;
	}
</script>

<svelte:head><title>{t('settings.naming.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.naming.title')} />

{#if !loaded}
	<SkSettings rows={4} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	<form class="sc-form stack" id="naming-form" onsubmit={save} aria-label={t('settings.naming.title')}>
		<SettingsCard id="where-files-go" title={t('settings.naming.where')}>
			<Field id="holding-days" label={t('settings.naming.holdingDays')}>
				<input id="holding-days" class="input narrow" type="number" min="0" max="3650" bind:value={fr.holding_days} />
				<span class="hint" title={t('settings.naming.holdingTitle')}>{t('settings.naming.holdingHint')}</span>
			</Field>
			{#snippet advanced()}
			<Field id="scan-limit" label={t('settings.naming.scanLimit')} hint={t('settings.naming.scanLimitHint')}>
				<input id="scan-limit" class="input narrow" type="number" min="10" max="100000" bind:value={ns.adopt_scan_limit} />
			</Field>
			<Field id="music-scan-limit" label={t('settings.naming.musicScanLimit')} hint={t('settings.naming.musicScanLimitHint')}>
				<input id="music-scan-limit" class="input narrow" type="number" min="10" max="100000" bind:value={ns.music_adopt_scan_limit} />
			</Field>
			<Field id="min-free" label={t('settings.naming.minFree')} hint={t('settings.naming.minFreeHint')}>
				<input id="min-free" class="input narrow" type="number" min="100" bind:value={fr.min_free_mb} />
			</Field>
			<label class="checkline">
				<input id="skip-free-check" type="checkbox" bind:checked={fr.skip_free_space_check} />
				<span><b>{t('settings.naming.skipCheck')}</b><br /><span class="hint">{t('settings.naming.skipCheckHint')}</span></span>
			</label>
			{/snippet}
		</SettingsCard>

		<SettingsCard id="movie-names" title={t('settings.naming.movieNames')}>
			<Field id="file-pattern" label={t('settings.naming.filePattern')} hint={t('settings.naming.filePatternHint')}>
				<input id="file-pattern" class="input mono" bind:value={ns.file_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<Field id="folder-pattern" label={t('settings.naming.folderPattern')} hint={t('settings.naming.folderPatternHint')}>
				<input id="folder-pattern" class="input mono" bind:value={ns.folder_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<div class="example" aria-live="polite" data-testid="naming-example">
				<p class="xs muted">{t('settings.naming.example')}</p>
				{#if exampleError}
					<p class="form-error" role="alert">{exampleError}</p>
				{:else}
					<p class="mono small path">{exampleFolder}/{example}</p>
				{/if}
			</div>
			<details class="words">
				<summary class="small">{t('settings.naming.words')}</summary>
				<dl>
					{#each Object.entries(words)
						.filter(([w]) => !showWords.includes(w) && !musicWords.includes(w))
						.sort() as [w, about] (w)}
						<dt>
							<Button variant="plain" class="mono" type="button" onclick={() => addWord('file_pattern', w)} aria-label={t('settings.naming.addToFile', { word: '{' + w + '}' })}>{'{'}{w}{'}'}</Button>
						</dt>
						<dd class="small muted">{about}</dd>
					{/each}
				</dl>
			</details>
			{#snippet advanced()}
			<Field id="colon" label={t('settings.naming.colon')} hint={t('settings.naming.colonHint')}>
				<select id="colon" class="input" bind:value={ns.colon}>
					{#each colonChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="illegal" label={t('settings.naming.illegal')} hint={t('settings.naming.illegalHint')}>
				<select id="illegal" class="input" bind:value={ns.illegal}>
					{#each illegalChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			{/snippet}
		</SettingsCard>

		<SettingsCard id="show-names" title={t('settings.naming.showNames')}>
			<Field id="show-folder-pattern" label={t('settings.naming.showFolder')} hint={t('settings.naming.showFolderHint')}>
				<input id="show-folder-pattern" class="input mono" bind:value={ns.show_folder_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<label class="checkline">
				<input id="season-folders" type="checkbox" bind:checked={ns.season_folders} />
				<span><b>{t('settings.naming.seasonFolders')}</b></span>
			</label>
			<Field id="season-folder-pattern" label={t('settings.naming.seasonPattern')}>
				<input id="season-folder-pattern" class="input mono" bind:value={ns.season_folder_pattern} autocomplete="off" spellcheck="false" disabled={!ns.season_folders} />
				<span class="hint">{t('settings.naming.seasonPatternHint')}</span>
			</Field>
			<Field id="episode-pattern" label={t('settings.naming.episodePattern')}>
				<input id="episode-pattern" class="input mono" bind:value={ns.episode_pattern} autocomplete="off" spellcheck="false" />
				<span class="hint">{t('settings.naming.episodePatternHint')}</span>
			</Field>
			<div class="example" aria-live="polite" data-testid="episode-examples">
				<p class="xs muted">{t('settings.naming.examples')}</p>
				{#if exampleError}
					<p class="small muted">{t('settings.naming.seeError')}</p>
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
				<summary class="small">{t('settings.naming.wordsEpisodes')}</summary>
				<dl>
					{#each Object.entries(words)
						.filter(([w]) => showWords.includes(w) || ['title', 'year', 'quality', 'group', 'release'].includes(w))
						.sort() as [w, about] (w)}
						<dt>
							<Button variant="plain" class="mono" type="button" onclick={() => addWord('episode_pattern', w)} aria-label={t('settings.naming.addToEpisode', { word: '{' + w + '}' })}
								>{'{'}{w}{'}'}</Button>
						</dt>
						<dd class="small muted">{about}</dd>
					{/each}
				</dl>
			</details>
			{#snippet advanced()}
			<Field id="specials-folder" label={t('settings.naming.specials')} hint={t('settings.naming.specialsHint')}>
				<input id="specials-folder" class="input mono" bind:value={ns.specials_folder} autocomplete="off" spellcheck="false" disabled={!ns.season_folders} />
			</Field>
			<Field id="daily-pattern" label={t('settings.naming.daily')} hint={t('settings.naming.dailyHint')}>
				<input id="daily-pattern" class="input mono" bind:value={ns.daily_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<Field id="anime-pattern" label={t('settings.naming.anime')} hint={t('settings.naming.animeHint')}>
				<input id="anime-pattern" class="input mono" bind:value={ns.anime_pattern} autocomplete="off" spellcheck="false" />
			</Field>
			<Field id="multi-style" label={t('settings.naming.multi')} hint={t('settings.naming.multiHint')}>
				<select id="multi-style" class="input" bind:value={ns.multi_episode_style}>
					{#each multiEpisodeChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="air-date-style" label={t('settings.naming.airDay')} hint={t('settings.naming.airDayHint')}>
				<select id="air-date-style" class="input" bind:value={ns.air_date_style}>
					{#each airDateChoices as [v, label] (v)}<option value={v}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="absolute-digits" label={t('settings.naming.digits')} hint={t('settings.naming.digitsHint')}>
				<select id="absolute-digits" class="input" bind:value={ns.absolute_digits}>
					<option value={2}>2: 07</option>
					<option value={3}>3: 007</option>
					<option value={4}>4: 0007</option>
				</select>
			</Field>
			{/snippet}
		</SettingsCard>

		<div data-testid="music-naming">
			<SettingsCard id="music-names" title={t('settings.naming.musicNames')}>
				<Field id="artist-folder-pattern" label={t('settings.naming.artistFolder')}>
					<input id="artist-folder-pattern" class="input mono" bind:value={ns.artist_folder_pattern} autocomplete="off" spellcheck="false" />
					<span class="hint" title={t('settings.naming.artistFolderTitle')}>{t('settings.naming.artistFolderHint')}</span>
				</Field>
				<Field id="album-folder-pattern" label={t('settings.naming.albumFolder')}>
					<input id="album-folder-pattern" class="input mono" bind:value={ns.album_folder_pattern} autocomplete="off" spellcheck="false" />
					<span class="hint">{t('settings.naming.albumFolderHint')}</span>
				</Field>
				<Field id="track-pattern" label={t('settings.naming.trackPattern')}>
					<input id="track-pattern" class="input mono" bind:value={ns.track_pattern} autocomplete="off" spellcheck="false" />
					<span class="hint">{t('settings.naming.trackPatternHint')}</span>
				</Field>
				<div class="example" aria-live="polite" data-testid="music-examples">
					<p class="xs muted">{t('settings.naming.examples')}</p>
					{#if exampleError}
						<p class="small muted">{t('settings.naming.seeError')}</p>
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
					<summary class="small">{t('settings.naming.wordsMusic')}</summary>
					<dl>
						{#each Object.entries(words)
							.filter(([w]) => musicWords.includes(w) || ['year', 'edition'].includes(w))
							.sort() as [w, about] (w)}
							<dt>
								<Button variant="plain" class="mono" type="button" onclick={() => addWord('track_pattern', w)} aria-label={t('settings.naming.addToTrack', { word: '{' + w + '}' })}
									>{'{'}{w}{'}'}</Button>
							</dt>
							<dd class="small muted">{about}</dd>
						{/each}
					</dl>
				</details>
				{#snippet advanced()}
				<Field id="multi-disc-pattern" label={t('settings.naming.multiDisc')}>
					<input id="multi-disc-pattern" class="input mono" bind:value={ns.multi_disc_track_pattern} autocomplete="off" spellcheck="false" disabled={ns.disc_folders} />
					<span class="hint">{t('settings.naming.multiDiscHint')}</span>
				</Field>
				<label class="checkline">
					<input id="disc-folders" type="checkbox" bind:checked={ns.disc_folders} />
					<span><b>{t('settings.naming.discFolders')}</b><br /><span class="hint">{t('settings.naming.discFoldersHint')}</span></span>
				</label>
				<Field id="disc-folder-pattern" label={t('settings.naming.discPattern')}>
					<input id="disc-folder-pattern" class="input mono" bind:value={ns.disc_folder_pattern} autocomplete="off" spellcheck="false" disabled={!ns.disc_folders} />
					<span class="hint">{t('settings.naming.discPatternHint')}</span>
				</Field>
				<label class="checkline">
					<input id="read-tags" type="checkbox" bind:checked={ns.music_read_tags} />
					<span
						><b>{t('settings.naming.readTags')}</b><br /><span class="hint"
							>{t('settings.naming.readTagsHint')}</span
						></span
					>
				</label>
				{/snippet}
			</SettingsCard>
		</div>

		<SettingsCard id="importing" title={t('settings.naming.importing')}>
			<label class="checkline">
				<input id="rename-on-import" type="checkbox" bind:checked={ns.rename_on_import} />
				<span
					><b>{t('settings.naming.rename')}</b><br /><span class="hint"
						>{t('settings.naming.renameHint')}</span
					></span
				>
			</label>
		</SettingsCard>

		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>

	<SettingsCard id="holding-card" title={t('settings.naming.holding')}>
		<div class="srow">
			<div class="txt"><span class="label">{t('settings.naming.clearOld')}</span><span class="hint">{t('settings.naming.clearOldHint')}</span></div>
			<div class="ctl"><Button type="button" size="sm" onclick={cleanNow} disabled={cleaning} aria-busy={cleaning}>{t('settings.naming.clearNow')}</Button></div>
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
		margin: 12px 0 14px;
		padding: 12px 14px;
		background: color-mix(in srgb, var(--text) 4%, transparent);
		border-radius: 12px;
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
