<script lang="ts">
	// Settings > YouTube: the folder videos go to and the free space to keep (the owner changes them, admins read them), the programs downloads need, and the totals over everyone (counts only, never titles).
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { onMount } from 'svelte';
	import { bytes, dateTime } from '#lib/core/format.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { ytSaveSettings, ytSettings, type YtSettings } from '#lib/youtube/index.ts';

	let view = $state<YtSettings | null>(null);
	let folder = $state('');
	let minFree = $state(10);
	let loaded = $state(false);
	let busy = $state(false);
	let error = $state('');
	let loadError = $state('');
	const isOwner = $derived(session.can('owner'));
	// The folder stays put once it holds videos; the server refuses the change too.
	const folderLocked = $derived(!!view && view.totals.videos > 0);
	const dirty = $derived(!!view && isOwner && (folder.trim() !== view.folder || Number(minFree) !== view.min_free_gb));

	function take(v: YtSettings) {
		view = v;
		folder = v.folder;
		minFree = v.min_free_gb;
	}

	async function load() {
		loaded = false;
		loadError = '';
		try {
			take(await ytSettings());
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
			take(await ytSaveSettings(folder.trim(), Number(minFree)));
			toasts.push(t('youtube.settings.saved'), 'ok');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{t('youtube.settings.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('youtube.settings.title')} />

{#if !loaded}
	<SkSettings rows={3} />
{:else if loadError || !view}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	<form class="sc-form" id="youtube-form" onsubmit={save} aria-label={t('youtube.settings.title')}>
		<SettingsCard id="yt-folder-card" title={t('youtube.settings.folderCard')} description={t('youtube.settings.folderCardDesc')}>
			<fieldset class="plain" disabled={!isOwner}>
				<Field id="yt-folder" label={t('youtube.settings.folder')} hint={folderLocked ? t('youtube.settings.folderLocked') : t('youtube.settings.folderHint')}>
					<input id="yt-folder" class="input mono" dir="ltr" bind:value={folder} placeholder="/data/youtube" autocomplete="off" spellcheck="false" readonly={folderLocked} aria-describedby="yt-folder-hint" />
				</Field>
				<Field id="yt-min-free" label={t('youtube.settings.minFree')} hint={t('youtube.settings.minFreeHint')}>
					<input id="yt-min-free" class="input narrow" type="number" min="1" max="100000" bind:value={minFree} aria-describedby="yt-min-free-hint" />
				</Field>
			</fieldset>
			{#if !isOwner}<p class="small muted" data-testid="yt-owner-only">{t('youtube.settings.ownerOnly')}</p>{/if}
		</SettingsCard>

		<SettingsCard id="yt-programs" title={t('youtube.settings.programs')} description={t('youtube.settings.programsDesc')}>
			<dl class="facts">
				<dt>yt-dlp</dt>
				<dd data-testid="yt-ytdlp">
					{#if view.ytdlp_path}
						<span class="ok">{t('youtube.settings.found')}</span> <code dir="ltr">{view.ytdlp_path}</code>
						{#if view.version}<br />{t('youtube.settings.version', { version: view.version })}{/if}
					{:else}
						<span class="fail">{t('youtube.settings.notFound')}</span>
					{/if}
					<br />{view.ytdlp_updated_at ? t('youtube.settings.updated', { when: dateTime(view.ytdlp_updated_at) }) : t('youtube.settings.notUpdated')}
					{#if view.ytdlp_update_error}<br /><span class="fail">{t('youtube.settings.updateError', { error: view.ytdlp_update_error })}</span>{/if}
				</dd>
				<dt>ffmpeg</dt>
				<dd data-testid="yt-ffmpeg">
					{#if view.ffmpeg_path}
						<span class="ok">{t('youtube.settings.found')}</span> <code dir="ltr">{view.ffmpeg_path}</code>
					{:else}
						<span class="fail">{t('youtube.settings.notFound')}</span>
					{/if}
				</dd>
			</dl>
		</SettingsCard>

		<SettingsCard id="yt-totals" title={t('youtube.settings.totals')} description={t('youtube.settings.totalsDesc')}>
			<p class="small" data-testid="yt-totals-line">{t('youtube.settings.totalsLine', { videos: view.totals.videos, size: bytes(view.totals.bytes), people: view.totals.people })}</p>
		</SettingsCard>

		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	</form>
	{#if isOwner}<SaveBar {dirty} form="youtube-form" saving={busy} ondiscard={() => view && take(view)} />{/if}
{/if}

<style>
	.plain {
		border: 0;
		padding: 0;
		margin: 0;
		min-width: 0;
	}
	.narrow {
		max-width: 160px;
	}
	.facts {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: var(--s-2) var(--s-4);
		margin: 0;
		font-size: var(--text-sm);
	}
	.facts dt {
		font-weight: 600;
	}
	.facts dd {
		margin: 0;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.ok {
		color: var(--ok-text);
	}
	.fail {
		color: var(--fail-text);
	}
</style>
