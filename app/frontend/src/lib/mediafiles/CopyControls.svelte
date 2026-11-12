<!-- Lists each Version's copy with protect, keep-and-add and adopt actions. -->
<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { onMount } from 'svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import { shortFolder } from '#lib/ui/TitleHero.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { copyEdition, inspectedAudio, languageList, versionCriteria } from './copy-controls.ts';
	import { api, message } from '../core/api.ts';
	import { session } from '../core/session.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { plainReason } from '../core/plain.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { CopyFile, LibraryFolder, PickRulesView, TitleView, VersionView } from '../core/types.ts';

	let { title, onchange = () => {} }: { title: TitleView; onchange?: () => void | Promise<void> } = $props();
	const titleId = $derived(title.id);
	const kind = $derived(title.kind);
	const staff = $derived(session.can('manage_library'));

	let files = $state<CopyFile[]>([]);
	let versions = $state<VersionView[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let formError = $state('');
	let busy = $state('');
	let acknowledge = $state<Record<number, boolean>>({});

	let addOpen = $state(false);
	let addFile = $state<CopyFile | null>(null);
	let addLabel = $state('4K');
	let addRules = $state(0);
	let addFolder = $state(0);
	let addEdition = $state('');
	let addAudio = $state('');
	let addKeep = $state(true);

	let adoptOpen = $state(false);
	let adoptVersion = $state<VersionView | null>(null);
	let adoptPath = $state('');

	onMount(() => void load());

	const fileFor = (versionId: number) => files.find((file) => file.VersionID === versionId);

	async function load() {
		try {
			const result = await api.get<{ files: CopyFile[]; versions: VersionView[] }>(`/titles/${titleId}/copies`);
			files = result.files;
			versions = result.versions;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	async function changed(text: string) {
		toasts.push(text, 'ok');
		await load();
		await onchange();
	}

	// Removing protection needs the box ticked first.
	async function setProtection(file: CopyFile, protectedValue: boolean) {
		busy = `protect-${file.ID}`;
		try {
			await api.put(`/titles/${titleId}/files/${file.ID}/choices`, {
				protected: protectedValue,
				acknowledge_protection: !protectedValue && acknowledge[file.ID] === true
			});
			await changed(protectedValue ? t('mediafiles.copy.protected') : t('mediafiles.copy.protectionRemoved'));
		} catch (e) {
			toasts.push(t('mediafiles.copy.protectFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	// Fetched once, on first use, and cut down to this kind.
	async function choices() {
		if (!rules.length) rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((rule) => !rule.kinds.length || rule.kinds.includes(kind));
		if (!folders.length) folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders.filter((folder) => folder.kind === kind);
	}

	async function openAdd(file: CopyFile) {
		addFile = file;
		formError = '';
		addLabel = '';
		addEdition = '';
		addAudio = '';
		addKeep = true;
		try {
			await choices();
			addRules = rules[0]?.id ?? 0;
			addFolder = folders[0]?.id ?? 0;
			addOpen = true;
		} catch (e) {
			toasts.push(t('mediafiles.copy.formFailed'), 'fail', { body: message(e) });
		}
	}

	async function keepAndAdd(e: SubmitEvent) {
		e.preventDefault();
		if (!addFile) return;
		busy = 'add';
		try {
			await api.post(`/titles/${titleId}/files/${addFile.ID}/keep-and-add`, {
				label: addLabel.trim(),
				pick_rules_id: addRules,
				library_folder_id: addFolder,
				keep_up_to_date: addKeep,
				edition: addEdition.trim(),
				audio_languages: languageList(addAudio)
			});
			addOpen = false;
			await changed(t('mediafiles.copy.versionAdded', { label: addLabel.trim() }));
		} catch (e) {
			formError = message(e);
		} finally {
			busy = '';
		}
	}

	function openAdopt(version: VersionView) {
		adoptVersion = version;
		formError = '';
		adoptPath = '';
		adoptOpen = true;
	}

	async function adopt(e: SubmitEvent) {
		e.preventDefault();
		if (!adoptVersion) return;
		busy = 'adopt';
		try {
			await api.post(`/titles/${titleId}/versions/${adoptVersion.id}/adopt`, {
				relative_path: adoptPath.trim()
			});
			adoptOpen = false;
			await changed(t('mediafiles.copy.fileMoved', { label: adoptVersion.label }));
		} catch (e) {
			formError = message(e);
		} finally {
			busy = '';
		}
	}

</script>

<section class="cc" aria-labelledby="copy-controls-title" data-testid="copy-controls">
	<h2 id="copy-controls-title" class="cc-h">{t('mediafiles.copy.title')}</h2>
	{#if !loaded}
		<Skeleton lines={3} />
	{:else if error}
		<p class="form-error" role="alert">{plainReason(error).text}</p>
	{:else}
		<ul class="cc-list">
			{#each versions as version (version.id)}
				{@const file = fileFor(version.id)}
				{@const criteria = versionCriteria(version)}
				{@const audio = file ? inspectedAudio(file) : []}
				<li class="cc-group" data-testid="copy-version">
					<div class="cc-row info">
						<span><b>{version.label}{#if file?.Protected}<Badge label={t('mediafiles.copy.protectedBadge')} />{/if}</b><small>{criteria.join(' · ')}</small></span>
					</div>
					{#if file}
						<div class="cc-row info">
							<span><small class="mono" title={file.RelativePath}>{file.RelativePath}</small><small>{t('mediafiles.copy.audio', { languages: audio.join(', ') || t('mediafiles.copy.unknown') })}{copyEdition(file) ? ` · ${copyEdition(file)}` : ''}{file.State === 'missing' ? ` · ${t('mediafiles.copy.missing')}` : ''}</small></span>
						</div>
						{#if staff}
							{#if file.Protected}
								<div class="cc-row">
									<label class="cc-check"><input type="checkbox" bind:checked={acknowledge[file.ID]} /><span>{t('mediafiles.copy.understand')}</span></label>
									<button type="button" class="cc-btn" onclick={() => setProtection(file, false)} disabled={!acknowledge[file.ID] || busy !== ''}>{t('mediafiles.copy.removeProtection')}</button>
								</div>
							{:else}
								<button type="button" class="cc-row act" onclick={() => setProtection(file, true)} disabled={busy !== ''}>
									<span><b>{t('mediafiles.copy.protect')}</b><small>{t('mediafiles.copy.protectHint')}</small></span><Icon name="shield" size={16} />
								</button>
							{/if}
							<button type="button" class="cc-row act" onclick={() => openAdd(file)} disabled={busy !== ''}>
								<span><b>{t('mediafiles.copy.keepAdd')}</b><small>{t('mediafiles.copy.keepAddHint')}</small></span><Icon name="plus" size={16} />
							</button>
						{/if}
					{:else}
						<div class="cc-row info"><span><small>{t('mediafiles.copy.noCopy')}</small></span></div>
						{#if staff && kind === 'movie'}
							<button type="button" class="cc-row act" onclick={() => openAdopt(version)}><span><b>{t('mediafiles.copy.useExisting')}</b><small>{t('mediafiles.copy.useExistingHint')}</small></span><Icon name="chevronRight" size={16} /></button>
						{/if}
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<Sheet bind:open={addOpen} title={t('mediafiles.copy.keepAdd')}>
	<form id="keep-add-form" class="sheet-form" onsubmit={keepAndAdd}>

		<div class="field"><label for="copy-new-label">{t('mediafiles.copy.versionName')}</label><input id="copy-new-label" class="input" bind:value={addLabel} maxlength="40" required /></div>
		<div class="field"><label for="copy-new-rules">{t('mediafiles.copy.pickRules')}</label><select id="copy-new-rules" class="input" bind:value={addRules}>{#each rules as rule (rule.id)}<option value={rule.id}>{rule.name}</option>{/each}</select></div>
		<div class="field"><label for="copy-new-folder">{t('mediafiles.copy.libraryFolder')}</label><select id="copy-new-folder" class="input" bind:value={addFolder}>{#each folders as folder (folder.id)}<option value={folder.id}>{folder.label || folder.path} ({folder.path})</option>{/each}</select></div>
		<div class="field"><label for="copy-new-edition">{t('mediafiles.copy.editionWish')} <span class="muted">{t('mediafiles.copy.optional')}</span></label><input id="copy-new-edition" class="input" bind:value={addEdition} maxlength="80" placeholder={t('mediafiles.copy.editionExample')} /></div>
		<div class="field"><label for="copy-new-audio">{t('mediafiles.copy.audioWish')} <span class="muted">{t('mediafiles.copy.optionalAudio')}</span></label><input id="copy-new-audio" class="input" bind:value={addAudio} placeholder={t('mediafiles.copy.audioExample')} /></div>
		<label class="checkline"><input type="checkbox" bind:checked={addKeep} /> <span><b>{t('mediafiles.copy.lookFor')}</b></span></label>
		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (addOpen = false)}>{t('mediafiles.dialogs.cancel')}</Button>
		<Button variant="primary" type="submit" form="keep-add-form" disabled={busy !== '' || !addLabel.trim() || !addRules || !addFolder}>{t('mediafiles.copy.protectAdd')}</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={adoptOpen} title={t('mediafiles.copy.useFor', { label: adoptVersion?.label ?? t('mediafiles.copy.thisVersion') })}>
	<form id="adopt-copy-form" class="sheet-form" onsubmit={adopt}>
		<p class="small muted"><Words key="mediafiles.copy.adoptNote" vars={{ folder: shortFolder(adoptVersion?.folder_path) }} /></p>
		{#if session.can('manage_library')}<details class="small muted"><summary>{t('mediafiles.copy.details')}</summary><span class="mono path">{adoptVersion?.folder_path}</span></details>{/if}
		<div class="field"><label for="adopt-path">{t('mediafiles.copy.relativePath')}</label><input id="adopt-path" class="input" bind:value={adoptPath} required /></div>

		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (adoptOpen = false)}>{t('mediafiles.dialogs.cancel')}</Button>
		<Button variant="primary" type="submit" form="adopt-copy-form" disabled={busy !== '' || !adoptPath.trim()}>{t('mediafiles.copy.checkUse')}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }

	.cc { min-width: 0; }
	.cc-h { margin: 0 0 12px; font: 700 20px/1.2 var(--font-ui); letter-spacing: -.01em; }
	.cc-list { display: grid; gap: 12px; margin: 0; padding: 0; list-style: none; }
	.cc-group { display: grid; border-radius: 16px; background: var(--surface-2); overflow: hidden; min-width: 0; }
	.cc-row { display: flex; align-items: center; justify-content: space-between; gap: 12px 16px; width: 100%; min-width: 0; padding: 14px 20px; border: 0; background: none; color: var(--text); text-align: left; font: inherit; }
	.cc-row + .cc-row { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.cc-row > span { display: grid; gap: 3px; min-width: 0; }
	.cc-row b { display: inline-flex; align-items: center; gap: 8px; font: 600 15px/1.2 var(--font-ui); }
	.cc-row small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cc-row small.mono { font: 500 12px/1.4 var(--font-mono, ui-monospace, monospace); }
	.cc-row.act { cursor: pointer; }
	.cc-row.act:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.cc-row.act:disabled { opacity: var(--disabled-opacity); cursor: default; }
	.cc-row :global(svg) { flex: none; color: var(--text-muted); }
	.cc-check { display: inline-flex; align-items: center; gap: 7px; color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); cursor: pointer; }
	.cc-check input { width: 16px; height: 16px; margin: 0; accent-color: var(--text); }
	.cc-btn { flex: none; height: 34px; padding: 0 14px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 10%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; }
	.cc-btn:disabled { opacity: var(--disabled-opacity); cursor: default; }
	/* Larger hit areas on touch screens. */
	@media (pointer: coarse) { .cc-btn { height: 44px; } .cc-check input { width: 20px; height: 20px; } }
	@media (max-width: 600px) { .cc-row { padding: 12px 16px; } }
</style>
