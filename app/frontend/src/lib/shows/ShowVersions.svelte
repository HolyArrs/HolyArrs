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
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
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
				toasts.push(t('shows.versions.saved', { label }), 'ok');
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
		if (!confirm(t('shows.versions.stopConfirm', { label: v.label }))) return;
		try {
			await api.del(`/titles/${title.id}/versions/${v.id}`);
			await onchange();
		} catch (e) {
			toasts.push(t('shows.versions.removeFailed'), 'fail', { body: message(e) });
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
<TitleSection id="h-show-versions" title={t('shows.versions.title')} testid="show-versions" primary>
	{#snippet head()}
		<div class="tp-head-end">
			{#if title.versions.length > 1}
				<Button variant="plain" aria-expanded={all} aria-controls="show-versions-list" onclick={() => (all = !all)}>{all ? t('shows.versions.showLess') : t('shows.versions.seeAll', { count: title.versions.length })}</Button>
			{/if}
			{#if staff}
				<Button icon="plus" onclick={openAdd}>{t('shows.versions.add')}</Button>
			{/if}
		</div>
	{/snippet}
	<ul class="versions tp-vrow" id="show-versions-list" aria-label={t('shows.versions.title')}>
		{#each title.versions as v (v.id)}
			<li class="version" data-testid="version" hidden={!all && v.id !== lead}>
				<b>{v.label}{v.main ? ` (${t('shows.versions.mainShort')})` : ''}</b>
				<div class="xs muted">{t('shows.versions.audioRequired', { languages: v.audio_languages?.join(', ') || t('shows.versions.any') })}</div>
				<div class="xs muted">{t('shows.versions.pickRules', { name: v.pick_rules_name })}{v.keep_up_to_date ? '' : ` · ${t('shows.versions.notKeptUp')}`}</div>
				{#if staff}
					<div class="xs muted path">{t('shows.versions.inFolder', { path: shortFolder(v.folder_path) })}</div>
					<details class="xs muted"><summary>{t('shows.versions.details')}</summary><span class="mono path">{v.folder_path}</span></details>
					<div class="row">
						<Button variant="plain" onclick={() => openEdit(v)} aria-label={t('shows.versions.changeThe', { label: v.label })}>{t('shows.versions.change')}</Button>
						{#if !v.main}
							<Menu label={t('shows.versions.actionsFor', { label: v.label })} items={[{ label: t('shows.versions.remove', { label: v.label }), icon: 'trash', destructive: true, onselect: () => remove(v) }]} width={220} align="start" />
						{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ul>
	{#if title.versions.length > 1}
		<p class="xs muted">{t('shows.versions.mainOnly')}</p>
	{/if}
</TitleSection>
{/if}

<dialog bind:this={dialog} aria-labelledby="show-version-title" class="modal">
	<form onsubmit={save}>
		<h2 id="show-version-title" class="h2">{editing ? t('shows.versions.changeTitle', { label: editing.label }) : t('shows.versions.addOf', { name: title.name })}</h2>
		{#if !editing}<p class="small muted">{t('shows.versions.addIntro')}</p>{/if}
		<div class="field" style="margin-top: var(--s-5)">
			<label for="sv-label">{t('shows.versions.name')}</label>
			<input id="sv-label" class="input" bind:value={vLabel} maxlength="40" required />
		</div>
		<div class="field">
            <label for="sv-audio">{t('shows.versions.audioLabel')}</label>
            <input id="sv-audio" class="input" bind:value={vAudio} placeholder={t('shows.versions.audioExample')} />
            <span class="hint">{t('shows.versions.audioHint')}</span>
        </div>
        <div class="field">
            <label for="sv-rules">{t('shows.versions.rulesLabel')}</label>
			<select id="sv-rules" class="input" bind:value={vRules}>
				{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</div>
		{#if !editing}
			<div class="field">
				<label for="sv-folder">{t('shows.versions.folderLabel')}</label>
				<select id="sv-folder" class="input" bind:value={vFolder}>
					{#each showFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
				<span class="hint">{t('shows.versions.folderHint')}</span>
			</div>
		{/if}
		<label class="checkline">
			<input type="checkbox" bind:checked={vKeep} />
			<span><b>{t('shows.versions.keep')}</b><br /><span class="hint">{t('shows.versions.keepHint')}</span></span>
		</label>
		{#if vError}<p class="form-error" role="alert">{vError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => dialog?.close()}>{t('shows.versions.cancel')}</Button>
			<Button variant="primary" type="submit" disabled={vBusy || !vLabel.trim()} busy={vBusy}>{editing ? t('shows.versions.save') : t('shows.versions.addButton')}</Button>
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
