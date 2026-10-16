<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { copyEdition, inspectedAudio, languageList, versionCriteria } from './copy-controls.ts';
	import { session } from './session.svelte.ts';
	import { toasts } from './toasts.svelte.ts';
	import type { CopyFile, LibraryFolder, PickRulesView, TitleView, VersionView } from './types.ts';

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
	let editionInputs: Record<number, HTMLInputElement> = {};
	let checkedEdition = $state<Record<number, boolean>>({});
	let acknowledge = $state<Record<number, boolean>>({});

	let addDialog: HTMLDialogElement | undefined = $state();
	let addFile = $state<CopyFile | null>(null);
	let addLabel = $state('4K');
	let addRules = $state(0);
	let addFolder = $state(0);
	let addEdition = $state('');
	let addAudio = $state('');
	let addKeep = $state(true);

	let adoptDialog: HTMLDialogElement | undefined = $state();
	let adoptVersion = $state<VersionView | null>(null);
	let adoptPath = $state('');
	let adoptEdition = $state('');
	let adoptChecked = $state(false);

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

	async function setProtection(file: CopyFile, protectedValue: boolean) {
		busy = `protect-${file.ID}`;
		try {
			await api.put(`/titles/${titleId}/files/${file.ID}/choices`, {
				protected: protectedValue,
				acknowledge_protection: !protectedValue && acknowledge[file.ID] === true
			});
			await changed(protectedValue ? 'This copy is protected.' : 'Protection is removed from this copy.');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	async function saveEdition(file: CopyFile) {
		busy = `edition-${file.ID}`;
		try {
			await api.put(`/titles/${titleId}/files/${file.ID}/choices`, { edition: (editionInputs[file.ID]?.value ?? '').trim() });
			checkedEdition[file.ID] = false;
			await changed('The checked edition is saved for this copy.');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

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
			addDialog?.showModal();
		} catch (e) {
			toasts.push(message(e), 'fail');
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
			addDialog?.close();
			await changed(`The existing copy is protected and the ${addLabel.trim()} Version is added.`);
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
		adoptEdition = version.edition ?? '';
		adoptChecked = false;
		adoptDialog?.showModal();
	}

	async function adopt(e: SubmitEvent) {
		e.preventDefault();
		if (!adoptVersion) return;
		busy = 'adopt';
		try {
			await api.post(`/titles/${titleId}/versions/${adoptVersion.id}/adopt`, {
				relative_path: adoptPath.trim(),
				...(adoptChecked && adoptEdition.trim() ? { confirmed_edition: adoptEdition.trim() } : {})
			});
			adoptDialog?.close();
			await changed(`The existing file now belongs to the ${adoptVersion.label} Version.`);
		} catch (e) {
			formError = message(e);
		} finally {
			busy = '';
		}
	}

	onMount(() => void load());
</script>

<section class="card" aria-labelledby="copy-controls-title" data-testid="copy-controls">
	<div class="card-head">
		<div>
			<h2 id="copy-controls-title" class="h2">Copies and Version criteria</h2>
			<p class="small muted">Edition and wanted audio describe a Version. Inspected audio below comes from the video file itself.</p>
		</div>
	</div>
	{#if !loaded}
		<div class="sk" style="height: 100px"></div>
	{:else if error}
		<p class="notice bad" role="alert">{error}</p>
	{:else}
		<ul class="copy-list">
			{#each versions as version (version.id)}
				{@const file = fileFor(version.id)}
				{@const criteria = versionCriteria(version)}
				<li class="copy" data-testid="copy-version">
					<div class="copy-head">
						<div><b>{version.label}{version.main ? ' (main)' : ''}</b><div class="xs muted">{criteria.join(' · ')}</div></div>
						{#if file?.Protected}<span class="chip warn">Protected</span>{/if}
					</div>
					{#if file}
						<p class="mono xs path">{file.RelativePath}</p>
						<p class="xs"><b>Inspected audio:</b> {inspectedAudio(file).join(', ') || 'Unknown'} · <b>File edition:</b> {copyEdition(file) || 'Unknown'}{file.State === 'missing' ? ' · Missing' : ''}</p>
						{#if staff}
							<div class="choice-grid">
								<div class="field compact">
									<label for="copy-edition-{file.ID}">Checked edition</label>
									<input id="copy-edition-{file.ID}" class="input" value={copyEdition(file)} bind:this={editionInputs[file.ID]} maxlength="80" />
									<label class="checkline small"><input type="checkbox" bind:checked={checkedEdition[file.ID]} /> I checked this file's edition</label>
									<button class="btn btn-quiet" type="button" onclick={() => saveEdition(file)} disabled={!checkedEdition[file.ID] || busy !== ''}>Save edition</button>
								</div>
								<div class="protect">
									{#if file.Protected}
										<label class="checkline small"><input type="checkbox" bind:checked={acknowledge[file.ID]} /> I understand this removes the copy's protection</label>
										<button class="btn btn-quiet" type="button" onclick={() => setProtection(file, false)} disabled={!acknowledge[file.ID] || busy !== ''}>Remove protection</button>
									{:else}
										<button class="btn btn-quiet" type="button" onclick={() => setProtection(file, true)} disabled={busy !== ''}>Protect this copy</button>
									{/if}
									<button class="btn btn-ghost" type="button" onclick={() => openAdd(file)} disabled={busy !== ''}>Keep this copy and add another Version</button>
								</div>
							</div>
						{/if}
					{:else}
						<p class="small muted">No copy is recorded for this Version.</p>
						{#if staff && kind === 'movie'}<button class="btn btn-ghost" type="button" onclick={() => openAdopt(version)}>Use an existing video in this Version folder</button>{/if}
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<dialog bind:this={addDialog} aria-labelledby="keep-add-title" class="modal">
	<form onsubmit={keepAndAdd}>
		<h2 id="keep-add-title" class="h2">Keep this copy and add another Version</h2>
		<p class="small muted">The current copy becomes protected. The new Version has its own edition and audio criteria.</p>
		<div class="field"><label for="copy-new-label">Version name</label><input id="copy-new-label" class="input" bind:value={addLabel} maxlength="40" required /></div>
		<div class="field"><label for="copy-new-rules">Pick rules</label><select id="copy-new-rules" class="input" bind:value={addRules}>{#each rules as rule (rule.id)}<option value={rule.id}>{rule.name}</option>{/each}</select></div>
		<div class="field"><label for="copy-new-folder">Library folder</label><select id="copy-new-folder" class="input" bind:value={addFolder}>{#each folders as folder (folder.id)}<option value={folder.id}>{folder.label || folder.path} ({folder.path})</option>{/each}</select></div>
		<div class="field"><label for="copy-new-edition">Edition wanted <span class="muted">(optional)</span></label><input id="copy-new-edition" class="input" bind:value={addEdition} maxlength="80" placeholder="Theatrical" /></div>
		<div class="field"><label for="copy-new-audio">Audio languages wanted <span class="muted">(optional, comma-separated codes)</span></label><input id="copy-new-audio" class="input" bind:value={addAudio} placeholder="en, fr" /></div>
		<label class="checkline"><input type="checkbox" bind:checked={addKeep} /> <span><b>Find this Version and better copies</b><br /><span class="hint">Look for this Version and better copies later.</span></span></label>
		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
		<div class="row modal-actions"><button class="btn btn-quiet" type="button" onclick={() => addDialog?.close()}>Cancel</button><button class="btn btn-red" type="submit" disabled={busy !== '' || !addLabel.trim() || !addRules || !addFolder}>Protect and add Version</button></div>
	</form>
</dialog>

<dialog bind:this={adoptDialog} aria-labelledby="adopt-copy-title" class="modal">
	<form onsubmit={adopt}>
		<h2 id="adopt-copy-title" class="h2">Use an existing video for {adoptVersion?.label}</h2>
		<p class="small muted">The file must already be inside <span class="mono">{adoptVersion?.folder_path}</span>. HolyArrs checks its actual audio and edition, then records it without moving or rewriting it.</p>
		<div class="field"><label for="adopt-path">Path relative to the Library folder</label><input id="adopt-path" class="input" bind:value={adoptPath} required /></div>
		<div class="field"><label for="adopt-edition">Checked edition <span class="muted">(only when the filename needs correction)</span></label><input id="adopt-edition" class="input" bind:value={adoptEdition} maxlength="80" /></div>
		<label class="checkline small"><input type="checkbox" bind:checked={adoptChecked} /> I checked the file and want to use this edition correction</label>
		{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
		<div class="row modal-actions"><button class="btn btn-quiet" type="button" onclick={() => adoptDialog?.close()}>Cancel</button><button class="btn btn-red" type="submit" disabled={busy !== '' || !adoptPath.trim()}>Check and use file</button></div>
	</form>
</dialog>

<style>
	.copy-list { list-style: none; padding: 0; margin: 0; display: grid; }
	.copy { padding: var(--s-4) 0; min-width: 0; }
	.copy + .copy { border-top: 1px solid var(--line); }
	.copy-head { display: flex; align-items: start; justify-content: space-between; gap: var(--s-3); }
	.path { overflow-wrap: anywhere; }
	.choice-grid { display: grid; grid-template-columns: minmax(14rem, 1fr) minmax(14rem, 1fr); gap: var(--s-4); margin-top: var(--s-3); }
	.compact, .protect { align-content: start; display: grid; gap: var(--s-2); }
	@media (max-width: 700px) { .choice-grid { grid-template-columns: 1fr; } }
</style>
