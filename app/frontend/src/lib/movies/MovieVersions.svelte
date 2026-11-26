<script lang="ts">
	import { ask } from '#lib/ui/ask.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import Button from '#lib/ui/Button.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import { shortFolder } from '#lib/ui/TitleHero.svelte';
	import { api, ApiError, message } from '../core/api.ts';
	import { bytes, date, tone, words } from '../core/format.ts';
	import { secondVersion } from '../mediafiles/index.ts';
	import { copyActions, editionDevice, editionProblem, editionRefused, saveThenReload, showSaved } from './copy-menu.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { LibraryFolder, PickRulesView, TitleView, VersionView } from '../core/types.ts';

	type SlotFile = NonNullable<TitleView['slots'][number]['file']>;

	let { list = true, title, onchange }: { list?: boolean; title: TitleView; onchange: () => void | Promise<void> } = $props();

	let dialog: HTMLDialogElement | undefined = $state();
	let all = $state(false);
	let rules = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let vLabel = $state('4K');
	let vEdition = $state('');
	let vAudio = $state('');
	let vFolderName = $state('');
	let vRules = $state(0);
	let vFolder = $state(0);
	let vError = $state('');
	let vBusy = $state(false);
	let keepFile = $state(0);
	let editDialog: HTMLDialogElement | undefined = $state();
	let editFile = $state<SlotFile | null>(null);
	let editValue = $state('');
	let editError = $state('');
	let editBusy = $state(false);

	const lead = $derived((title.versions.find((v) => v.main) ?? title.versions[0])?.id);
	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));

	export async function openAdd(keepFileId = 0) {
		vError = '';
		keepFile = keepFileId;
		try {
			if (!rules.length) rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('movie'));
			if (!folders.length) folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
			const used = new Set(title.versions.map((v) => v.label.toLowerCase()));
			const sv = secondVersion(rules, movieFolders, title.library_folder_id ?? 0);
			vLabel = used.has(sv.label.toLowerCase()) ? '' : sv.label;
			vRules = sv.pick_rules_id;
			vEdition = '';
			vAudio = '';
			vFolderName = '';
			vFolder = sv.library_folder_id || movieFolders[0]?.id || 0;
		} catch (e) {
			vError = message(e);
		}
		// Show the dialog only after the defaults are set, so they can't overwrite typing.
		dialog?.showModal();
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		vError = editionError(vEdition);
		if (vError) return;
		vBusy = true;
		try {
			await api.post(keepFile ? `/titles/${title.id}/files/${keepFile}/keep-and-add` : `/titles/${title.id}/versions`, { label: vLabel.trim(), pick_rules_id: vRules, library_folder_id: vFolder, edition: vEdition.trim(), audio_languages: vAudio.split(/[,\s]+/).filter(Boolean), folder_name: vFolderName });
			dialog?.close();
			all = true;
			await onchange();
		} catch (err) {
			vError = err instanceof ApiError && editionRefused(err.status, err.message) ? t('movies.versions.editionInvalid') : message(err);
		} finally {
			vBusy = false;
		}
	}

	export async function remove(v: VersionView) {
		if (!(await ask(t('movies.versions.stopConfirm', { label: v.label }), { yes: t('ui.ask.stop') }))) return;
		try {
			await api.del(`/titles/${title.id}/versions/${v.id}`);
			await onchange();
		} catch (e) {
			toasts.push(t('movies.versions.removeFailed'), 'fail', { body: message(e) });
		}
	}

	type DeletePlan = { held: unknown[]; kept: { why: string }[] };

	// Deletes one copy's file: the server plans it first, then, once confirmed, moves it to Recently deleted.
	async function deleteFile(file: SlotFile) {
		if (!file.ID) return;
		const name = file.RelativePath.split('/').pop() ?? file.RelativePath;
		const body = { file_id: file.ID, delete_files: true };
		try {
			const plan = await api.post<DeletePlan>(`/titles/${title.id}/delete`, { ...body, apply: false });
			if (!plan.held.length) {
				toasts.push(t('movies.versions.deleteFileFailed'), 'fail', { body: plan.kept[0]?.why ?? '' });
				return;
			}
			if (!(await ask(t('movies.versions.deleteFileConfirm', { name }), { yes: t('movies.versions.deleteFile'), danger: true }))) return;
			await api.post(`/titles/${title.id}/delete`, { ...body, apply: true });
			await onchange();
		} catch (e) {
			toasts.push(t('movies.versions.deleteFileFailed'), 'fail', { body: message(e) });
		}
	}

	// Why an edition can't be saved, or nothing when it can.
	function editionError(edition: string): string {
		const device = editionDevice(edition);
		if (device) return t('movies.versions.editionDevice', { name: device });
		return editionProblem(edition) ? t('movies.versions.editionInvalid') : '';
	}

	async function putChoice(file: SlotFile, body: object) {
		// The card shows the answer at once; the reload brings the rest.
		await saveThenReload(async () => showSaved(title.slots, await api.put<SlotFile>(`/titles/${title.id}/files/${file.ID}/choices`, body)), onchange);
		toasts.push(t('mediafiles.title.saved', { name: title.name }), 'ok');
	}

	async function saveEdition(e: SubmitEvent) {
		e.preventDefault();
		if (!editFile || editBusy) return;
		editError = editionError(editValue);
		if (editError) return;
		editBusy = true;
		try {
			await putChoice(editFile, { edition: editValue.trim() });
			editDialog?.close();
		} catch (err) {
			editError = err instanceof ApiError && editionRefused(err.status, err.message) ? t('movies.versions.editionInvalid') : message(err);
		} finally {
			editBusy = false;
		}
	}

	async function setProtection(file: SlotFile, on: boolean, label: string) {
		if (!on && !(await ask(t('movies.versions.unprotectConfirm', { label }), { yes: t('mediafiles.copy.removeProtection'), danger: true }))) return;
		try {
			await putChoice(file, on ? { protected: true } : { protected: false, acknowledge_protection: true });
		} catch (e) {
			toasts.push(t('mediafiles.copy.protectFailed'), 'fail', { body: message(e) });
		}
	}

	// The actions on one movie copy's card.
	export function copyMenu(v: VersionView, file?: SlotFile | null): MenuItem[] {
		const items: Record<string, MenuItem> = {
			edition: { label: t('movies.versions.setEdition'), icon: 'film', onselect: () => { editFile = file ?? null; editValue = typeof file?.MediaInfo?.edition === 'string' ? file.MediaInfo.edition : ''; editError = ''; editDialog?.showModal(); } },
			protect: { label: t('mediafiles.copy.protect'), icon: 'shield', onselect: () => file && void setProtection(file, true, v.label) },
			unprotect: { label: t('mediafiles.copy.removeProtection'), icon: 'shield', onselect: () => file && void setProtection(file, false, v.main ? t('discover.titleLibrary.mainCopy') : v.label) },
			keepAdd: { label: t('mediafiles.copy.keepAdd'), icon: 'plus', onselect: () => void openAdd(file?.ID ?? 0) },
			deleteFile: { label: t('movies.versions.deleteFile'), icon: 'trash', destructive: true, onselect: () => file && void deleteFile(file) },
			remove: { label: t('movies.versions.remove', { label: v.label }), icon: 'trash', destructive: true, onselect: () => remove(v) }
		};
		return copyActions(v, file).map((a) => items[a]);
	}

	const slotOf = (v: VersionView) => title.slots.find((s) => s.version_id === v.id);
</script>

{#if list}
<TitleSection id="h-versions" title={t('movies.versions.title')} testid="movie-versions" primary>
	{#snippet head()}
		<div class="tp-head-end">
			{#if title.versions.length > 1}
				<Button variant="plain" aria-expanded={all} aria-controls="movie-versions-list" onclick={() => (all = !all)}>{all ? t('movies.versions.showLess') : t('movies.versions.seeAll', { count: title.versions.length })}</Button>
			{/if}
			<Button icon="plus" onclick={() => openAdd()}>{t('movies.versions.add')}</Button>
		</div>
	{/snippet}
	<ul class="tp-list tp-vrow" id="movie-versions-list" aria-label={t('movies.versions.title')}>
		{#each title.versions as v (v.id)}
			{@const s = slotOf(v)}
			{@const f = s?.file}
			<li class="tp-row" data-testid="version" hidden={!all && v.id !== lead}>
				<div class="tp-row-main">
					<b class="v-name">{v.label}{#if v.main && v.label.trim().toLowerCase() !== 'main'}<span class="v-main" data-testid="version-main">{t('movies.versions.main')}</span>{:else if v.main}<span class="sr-only" data-testid="version-main">{t('movies.versions.mainSr')}</span>{/if}</b>
					<span class="xs muted">{v.edition || t('movies.versions.anyEdition')} · {v.audio_languages?.length ? t('movies.versions.audio', { languages: v.audio_languages.join(', ') }) : t('movies.versions.anyAudio')} · {v.pick_rules_name}{v.keep_up_to_date ? '' : ` · ${t('movies.versions.notKeptUp')}`}</span>
					{#if f}
						<span class="xs muted">{bytes(f.Size)}{#if f.Quality?.resolution} · {f.Quality.resolution}p{f.Quality.source ? ` ${words(String(f.Quality.source))}` : ''}{/if} · {t('movies.versions.added', { date: date(f.AddedAt) })}</span>
					{:else}
						<span class="xs muted">{t('movies.versions.noFile')}</span>
					{/if}
					{#if f || v.folder_path}
						<details class="tp-details">
							<summary>{t('movies.versions.details')}</summary>
							<ul>
								{#if f}<li class="path">{t('movies.versions.file', { path: shortFolder(f.RelativePath) })}</li>{/if}
								{#if f?.ReleaseName}<li class="path">{t('movies.versions.fromRelease', { name: f.ReleaseName })}</li>{/if}
								{#if v.folder_path}<li class="path">{t('movies.versions.folder', { path: shortFolder(v.folder_path) })}</li>{/if}
							</ul>
						</details>
					{/if}
				</div>
				<div class="tp-row-end">
					{#if s}<span class="chip {tone(s.state)}">{serverWords(s.state_label)}</span>{/if}
					{#if !v.main || f}
						<Menu label={t('movies.versions.actionsFor', { label: v.label })} items={copyMenu(v, f)} width={360} />
					{/if}
				</div>
			</li>
		{/each}
	</ul>
</TitleSection>
{/if}

<dialog bind:this={dialog} aria-labelledby="movie-version-title" class="modal">
	<form onsubmit={add}>
		<h2 id="movie-version-title" class="h2">{keepFile ? t('mediafiles.copy.keepAdd') : t('movies.versions.addOf', { name: title.name })}</h2>
		<p class="small muted">{t('movies.versions.addIntro')}</p>
		<div class="field" style="margin-top: var(--s-5)">
			<label for="mv-label">{t('movies.versions.name')}</label>
			<input id="mv-label" class="input" bind:value={vLabel} maxlength="40" required />
		</div>
		<div class="field">
			<label for="mv-edition">{t('movies.versions.edition')}</label>
			<input id="mv-edition" class="input" bind:value={vEdition} maxlength="40" placeholder={t('movies.versions.editionExample')} />
			<span class="hint">{t('movies.versions.editionHint')} {t('movies.versions.editionChars')} <bdi dir="ltr">/ \ : * ? " &lt; &gt; | [ ] &#123; &#125;</bdi></span>
		</div>
		<div class="field">
			<label for="mv-audio">{t('movies.versions.audioLabel')}</label>
			<input id="mv-audio" class="input" bind:value={vAudio} placeholder={t('movies.versions.audioExample')} />
			<span class="hint">{t('movies.versions.audioHint')}</span>
		</div>
		<div class="field">
			<label for="mv-existing-folder">{t('movies.versions.existingFolder')}</label>
			<input id="mv-existing-folder" class="input" bind:value={vFolderName} maxlength="200" />
			<span class="hint">{t('movies.versions.existingFolderHint')}</span>
		</div>
		<div class="field">
			<label for="mv-rules">{t('movies.versions.rulesLabel')}</label>
			<select id="mv-rules" class="input" bind:value={vRules}>
				{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</div>
		<div class="field">
			<label for="mv-folder">{t('movies.versions.folderLabel')}</label>
			<select id="mv-folder" class="input" bind:value={vFolder}>
				{#each movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
			</select>
		</div>
		{#if vError}<p class="form-error" role="alert">{vError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => dialog?.close()}>{t('movies.versions.cancel')}</Button>
			<Button variant="primary" type="submit" disabled={vBusy || !vLabel.trim()} busy={vBusy}>{keepFile ? t('mediafiles.copy.protectAdd') : t('movies.versions.addButton')}</Button>
		</div>
	</form>
</dialog>

<dialog bind:this={editDialog} aria-labelledby="movie-edition-title" class="modal">
	<form onsubmit={saveEdition}>
		<h2 id="movie-edition-title" class="h2">{t('movies.versions.setEdition')}</h2>
		<div class="field" style="margin-top: var(--s-5)">
			<label for="mv-file-edition">{t('movies.versions.edition')}</label>
			<input id="mv-file-edition" class="input" bind:value={editValue} maxlength="40" placeholder={t('movies.versions.editionExample')} />
			<span class="hint">{t('movies.versions.editionFileHint')} {t('movies.versions.editionChars')} <bdi dir="ltr">/ \ : * ? " &lt; &gt; | [ ] &#123; &#125;</bdi></span>
		</div>
		{#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => editDialog?.close()}>{t('movies.versions.cancel')}</Button>
			<Button variant="primary" type="submit" busy={editBusy}>{t('mediafiles.dialogs.save')}</Button>
		</div>
	</form>
</dialog>

<style>
	.path { overflow-wrap: anywhere; }
	.tp-row[hidden] { display: none; }
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	.v-name { display: inline-flex; align-items: center; gap: 8px; }
	.v-main { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 5px; border: 1px solid color-mix(in srgb, var(--text) 30%, transparent); color: var(--text-muted); font: 700 11px/1 var(--font-ui); }
</style>
