<script lang="ts">
	// Card and dialog for a show's Versions: list, add, change and remove.
	import Button from '#lib/ui/Button.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import { shortFolder } from '#lib/ui/TitleHero.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { secondVersion } from '../mediafiles/index.ts';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import type { LibraryFolder, PickRulesView, TitleView, VersionView } from '../core/types.ts';

	let { title, staff, onchange }: { title: TitleView; staff: boolean; onchange: () => void | Promise<void> } = $props();

	let dialog: HTMLDialogElement | undefined = $state();
	// Shows every Version instead of only the lead one.
	let all = $state(false);
	let rules = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	// null means a new Version is being added.
	let editing = $state<VersionView | null>(null);
	let vLabel = $state('4K');
	let vAudio = $state('');
	let vRules = $state(0);
	let vFolder = $state(0);
	let vKeep = $state(true);
	let vError = $state('');
	let vBusy = $state(false);

	const showFolders = $derived(folders.filter((f) => f.kind === 'show'));
	// Main Version, or the first when none is flagged.
	const lead = $derived((title.versions.find((v) => v.main) ?? title.versions[0])?.id);

	// Suggests a second Version that does not clash with an existing label.
	export async function openAdd() {
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
				toasts.push(`${label} saved`, 'ok');
			} else {
				await api.post(`/titles/${title.id}/versions`, { label, pick_rules_id: vRules, library_folder_id: vFolder, keep_up_to_date: vKeep, audio_languages: vAudio.split(/[,\s]+/).filter(Boolean) });
				all = true;
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
			await onchange();
		} catch (e) {
			toasts.push(`Couldn't remove Version`, 'fail', { body: message(e) });
		}
	}

	// Loads pick rules and folders once, on the first dialog open.
	async function choices() {
		if (!rules.length) {
			rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('show'));
		}
		if (!folders.length) folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
	}
</script>

<!-- Hidden with one Version, since the card would only repeat Settings. -->
{#if title.versions.length > 1}
<TitleSection id="h-show-versions" title="Versions" testid="show-versions" primary>
	{#snippet head()}
		<div class="tp-head-end">
			{#if title.versions.length > 1}
				<Button variant="plain" aria-expanded={all} aria-controls="show-versions-list" onclick={() => (all = !all)}>{all ? 'Show less' : `See all ${title.versions.length}`}</Button>
			{/if}
			{#if staff}
				<Button icon="plus" onclick={openAdd}>Add a Version</Button>
			{/if}
		</div>
	{/snippet}
	<ul class="versions tp-vrow" id="show-versions-list" aria-label="Versions">
		{#each title.versions as v (v.id)}
			<li class="version" data-testid="version" hidden={!all && v.id !== lead}>
				<b>{v.label}{v.main ? ' (main)' : ''}</b>
				<div class="xs muted">Audio required: {v.audio_languages?.join(', ') || 'Any'}</div>
				<div class="xs muted">Pick rules: {v.pick_rules_name}{v.keep_up_to_date ? '' : ' · not kept up to date'}</div>
				{#if staff}
					<div class="xs muted path">In {shortFolder(v.folder_path)}</div>
					<details class="xs muted"><summary>Details</summary><span class="mono path">{v.folder_path}</span></details>
					<div class="row">
						<Button variant="plain" onclick={() => openEdit(v)} aria-label="Change the {v.label} Version">Change</Button>
						{#if !v.main}
							<Menu label="Version actions for {v.label}" items={[{ label: `Remove ${v.label}`, icon: 'trash', destructive: true, onselect: () => remove(v) }]} width={220} align="start" />
						{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ul>
	{#if title.versions.length > 1}
		<p class="xs muted">Edit and Move change the main Version only.</p>
	{/if}
</TitleSection>
{/if}

<dialog bind:this={dialog} aria-labelledby="show-version-title" class="modal">
	<form onsubmit={save}>
		<h2 id="show-version-title" class="h2">{editing ? `Change the ${editing.label} Version` : `Add a Version of ${title.name}`}</h2>
		{#if !editing}<p class="small muted">Another copy of every episode, like 4K, with its own Pick rules and folder.</p>{/if}
		<div class="field" style="margin-top: var(--s-5)">
			<label for="sv-label">Version name</label>
			<input id="sv-label" class="input" bind:value={vLabel} maxlength="40" required />
		</div>
		<div class="field">
            <label for="sv-audio">Required audio languages</label>
            <input id="sv-audio" class="input" bind:value={vAudio} placeholder="For example, en, ja" />
            <span class="hint">Audio must have every language. Subtitles don’t count.</span>
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
			<Button variant="plain" onclick={() => dialog?.close()}>Cancel</Button>
			<Button variant="primary" type="submit" disabled={vBusy || !vLabel.trim()} busy={vBusy}>{editing ? 'Save' : 'Add Version'}</Button>
		</div>
	</form>
</dialog>

<style>
	details > summary {
		display: inline-flex;
		align-items: center;
		min-height: 24px;
		min-width: 24px;
		padding: 2px 4px;
		cursor: pointer;
	}
	@media (pointer: coarse) {
		details > summary { min-height: 44px; }
	}
	.versions .version[hidden] {
		display: none;
	}
	.versions .version {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.path {
		overflow-wrap: anywhere;
	}
	/* Native dialog styled as a modal card. */
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
</style>
