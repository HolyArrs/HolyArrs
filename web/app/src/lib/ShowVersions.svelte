<script lang="ts">
	// The Versions card of a show (4.8.11): every wanted copy, such as HD and
	// 4K, with its Pick rules and folder; staff add, change and remove them.
	import { api, message } from './api.ts';
	import Icon from './Icon.svelte';
	import { secondVersion } from './library.ts';
	import { toasts } from './toasts.svelte.ts';
	import type { LibraryFolder, PickRulesView, TitleView, VersionView } from './types.ts';

	let { title, staff, onchange }: { title: TitleView; staff: boolean; onchange: () => void | Promise<void> } = $props();

	let dialog: HTMLDialogElement | undefined = $state();
	let rules = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	/** The Version being changed; null adds a new one. */
	let editing = $state<VersionView | null>(null);
	let vLabel = $state('4K');
	let vAudio = $state('');
	let vRules = $state(0);
	let vFolder = $state(0);
	let vKeep = $state(true);
	let vError = $state('');
	let vBusy = $state(false);
	const showFolders = $derived(folders.filter((f) => f.kind === 'show'));

	async function choices() {
		if (!rules.length) {
			rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('show'));
		}
		if (!folders.length) folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
	}

	async function openAdd() {
		editing = null;
		vError = '';
		try {
			await choices();
			const used = new Set(title.versions.map((v) => v.label.toLowerCase()));
			const sv = secondVersion(rules, showFolders, title.library_folder_id ?? 0);
			vLabel = used.has(sv.label.toLowerCase()) ? '' : sv.label;
			vRules = sv.pick_rules_id;
			vFolder = sv.library_folder_id || showFolders[0]?.id || 0;
			vKeep = true;
			vAudio = '';
		} catch (e) {
			vError = message(e);
		}
		dialog?.showModal();
	}

	async function openEdit(v: VersionView) {
		editing = v;
		vError = '';
		vLabel = v.label;
		vRules = v.pick_rules_id;
		vKeep = v.keep_up_to_date;
		vAudio = v.audio_languages?.join(', ') ?? '';
		try {
			await choices();
		} catch (e) {
			vError = message(e);
		}
		dialog?.showModal();
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		vBusy = true;
		vError = '';
		const label = vLabel.trim();
		try {
			if (editing) {
				await api.put(`/titles/${title.id}/versions/${editing.id}`, { label, pick_rules_id: vRules, keep_up_to_date: vKeep, audio_languages: vAudio.split(/[,\s]+/).filter(Boolean) });
				toasts.push(`The ${label} Version is saved.`, 'ok');
			} else {
				await api.post(`/titles/${title.id}/versions`, { label, pick_rules_id: vRules, library_folder_id: vFolder, keep_up_to_date: vKeep, audio_languages: vAudio.split(/[,\s]+/).filter(Boolean) });
				toasts.push(`The ${label} Version is added. Each episode gets its own Slot for it.`, 'ok');
			}
			dialog?.close();
			await onchange();
		} catch (err) {
			vError = message(err);
		} finally {
			vBusy = false;
		}
	}

	async function remove(v: VersionView) {
		if (!confirm(`Stop keeping the ${v.label} Version of this show? Files on disk are not touched.`)) return;
		try {
			await api.del(`/titles/${title.id}/versions/${v.id}`);
			toasts.push(`The ${v.label} Version is removed.`, 'ok');
			await onchange();
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}
</script>

<section class="card" aria-labelledby="h-show-versions" data-testid="show-versions">
	<div class="card-head">
		<h2 id="h-show-versions" class="h2">Versions</h2>
		{#if staff}
			<button class="btn btn-ghost" type="button" onclick={openAdd}><Icon name="plus" size={16} /> Add a Version</button>
		{/if}
	</div>
	<ul class="versions">
		{#each title.versions as v (v.id)}
			<li class="version" data-testid="version">
				<b>{v.label}{v.main ? ' (main)' : ''}</b>
				<div class="xs muted">Audio required: {v.audio_languages?.join(', ') || 'Any'}</div>
				<div class="xs muted">Pick rules: {v.pick_rules_name}{v.keep_up_to_date ? '' : ' · not kept up to date'}</div>
				{#if staff}
					<div class="xs muted path">In {v.folder_path}</div>
					<div class="row">
						<button class="btn btn-quiet" type="button" onclick={() => openEdit(v)} aria-label="Change the {v.label} Version">Change</button>
						{#if !v.main}
							<button class="btn btn-quiet" type="button" onclick={() => remove(v)}>Remove {v.label}</button>
						{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ul>
	{#if title.versions.length > 1}
		<p class="xs muted">Each episode is looked for, downloaded and placed once per Version. Edit and Move change the main Version only.</p>
	{/if}
</section>

<dialog bind:this={dialog} aria-labelledby="show-version-title" class="modal">
	<form onsubmit={save}>
		<h2 id="show-version-title" class="h2">{editing ? `Change the ${editing.label} Version` : `Add a Version of ${title.name}`}</h2>
		{#if !editing}<p class="small muted">Another copy of every episode to keep next to the others, for example 4K, with its own Pick rules and folder.</p>{/if}
		<div class="field" style="margin-top: var(--s-5)">
			<label for="sv-label">Version name</label>
			<input id="sv-label" class="input" bind:value={vLabel} maxlength="40" required />
		</div>
		<div class="field">
            <label for="sv-audio">Required audio languages</label>
            <input id="sv-audio" class="input" bind:value={vAudio} placeholder="For example, en, ja" />
            <span class="hint">Every language must be present in inspected episode audio. Subtitles are separate.</span>
        </div>
        <div class="field">
            <label for="sv-rules">Pick rules for this Version</label>
			<select id="sv-rules" class="input" bind:value={vRules}>
				{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</div>
		{#if !editing}
			<div class="field">
				<label for="sv-folder">Library folder for this Version</label>
				<select id="sv-folder" class="input" bind:value={vFolder}>
					{#each showFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
				<span class="hint">In the main Version's folder, the show folder gets the Version name in brackets.</span>
			</div>
		{/if}
		<label class="checkline">
			<input type="checkbox" bind:checked={vKeep} />
			<span><b>Keep up to date</b><br /><span class="hint">HolyArrs looks for this Version's episodes and for better copies.</span></span>
		</label>
		{#if vError}<p class="form-error" role="alert">{vError}</p>{/if}
		<div class="row modal-actions">
			<button class="btn btn-quiet" type="button" onclick={() => dialog?.close()}>Cancel</button>
			<button class="btn btn-red" type="submit" disabled={vBusy || !vLabel.trim()} aria-busy={vBusy}>{editing ? 'Save' : 'Add Version'}</button>
		</div>
	</form>
</dialog>

<style>
	.versions .version {
		padding: var(--s-3);
		border-radius: 5px;
		background: var(--surface-2);
	}
	.versions .version + .version {
		margin-top: var(--s-2);
	}
	.path {
		overflow-wrap: anywhere;
	}
</style>
