<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { codecLabel, type CopySearchPreview } from './copies.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	type Props = {
		deleteDialog: HTMLDialogElement | null;
		copyDialog: HTMLDialogElement | null;
		missingDialog: HTMLDialogElement | null;
		saveDialog: HTMLDialogElement | null;
		copyPreview: CopySearchPreview | null;
		saveName: string;
		deleteIds: number[];
		deleteWord: string;
		busy: boolean;
		missingCount: number;
		searchingAll: boolean;
		confirmDelete: () => void;
		findCopies: () => void;
		searchAllMissing: () => void;
		saveCurrent: () => void;
	};
	let { deleteDialog = $bindable(), copyDialog = $bindable(), missingDialog = $bindable(), saveDialog = $bindable(), copyPreview = $bindable(), saveName = $bindable(), deleteIds, deleteWord, busy, missingCount, searchingAll, confirmDelete, findCopies, searchAllMissing, saveCurrent }: Props = $props();
</script>

<dialog bind:this={deleteDialog} aria-labelledby="bulk-delete-title" class="modal" data-testid="bulk-delete">
	<h2 id="bulk-delete-title" class="h2">{t('mediafiles.dialogs.remove', { word: deleteWord })}</h2>
	<p class="small muted">{deleteIds.length === 1 ? t('mediafiles.dialogs.leavesOne') : t('mediafiles.dialogs.leavesMany', { n: deleteIds.length })}</p>
	<div class="modal-actions">
		<Button onclick={() => deleteDialog?.close()} disabled={busy}>{t('mediafiles.dialogs.cancel')}</Button>
		<Button variant="destructive" icon="trash" disabled={busy} onclick={confirmDelete} data-testid="bulk-delete-confirm">{t(deleteIds.length === 1 ? 'mediafiles.dialogs.removeOne' : 'mediafiles.dialogs.removeMany', { n: deleteIds.length })}</Button>
	</div>
</dialog>

<dialog bind:this={copyDialog} aria-labelledby="find-copy-title" class="modal" data-testid="find-copy" onclose={() => (copyPreview = null)}>
	{#if copyPreview}
		<h2 id="find-copy-title" class="h2">{t(copyPreview.copies.length === 1 ? 'mediafiles.dialogs.findOne' : 'mediafiles.dialogs.findMany', { n: copyPreview.copies.length })}</h2>
		<p class="small muted">{t(copyPreview.slot_count === 1 ? 'mediafiles.dialogs.searchesOne' : 'mediafiles.dialogs.searchesMany', { n: copyPreview.slot_count })}</p>
		<ul class="copy-list">{#each copyPreview.copies as c (`${c.file_id}:${c.version_id}`)}<li><b>{c.title}</b><span>{c.version} · {codecLabel(c.codec)}</span></li>{/each}</ul>
	{/if}
	<div class="modal-actions">
		<Button onclick={() => copyDialog?.close()} disabled={busy}>{t('mediafiles.dialogs.cancel')}</Button>
		<Button variant="primary" icon="search" {busy} onclick={findCopies} data-testid="find-copy-confirm">{t('mediafiles.dialogs.search')}</Button>
	</div>
</dialog>

<dialog bind:this={missingDialog} aria-labelledby="search-missing-title" class="modal" data-testid="search-missing">
	<h2 id="search-missing-title" class="h2">{t(missingCount === 1 ? 'mediafiles.dialogs.missingOne' : 'mediafiles.dialogs.missingMany', { n: missingCount })}</h2>
	<p class="small muted">{t('mediafiles.dialogs.missingNote')}</p>
	<div class="modal-actions">
		<Button onclick={() => missingDialog?.close()} disabled={searchingAll}>{t('mediafiles.dialogs.cancel')}</Button>
		<Button variant="primary" icon="search" busy={searchingAll} onclick={searchAllMissing} data-testid="search-missing-confirm">{t('mediafiles.dialogs.search')}</Button>
	</div>
</dialog>

<dialog bind:this={saveDialog} aria-labelledby="save-filter-title" class="modal">
	<form method="dialog" onsubmit={(e) => { e.preventDefault(); saveCurrent(); }}>
		<h2 id="save-filter-title" class="h2">{t('mediafiles.dialogs.saveFilter')}</h2>
		<label class="small muted" for="save-filter-name">{t('mediafiles.dialogs.name')}</label>
		<input id="save-filter-name" class="input" bind:value={saveName} maxlength="60" />
		<div class="modal-actions">
			<Button onclick={() => saveDialog?.close()}>{t('mediafiles.dialogs.cancel')}</Button>
			<Button variant="primary" type="submit" disabled={!saveName.trim()}>{t('mediafiles.dialogs.save')}</Button>
		</div>
	</form>
</dialog>

<style>
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: var(--s-6); }
	.modal .input { width: 100%; margin-top: 6px; }
	.copy-list { display: grid; gap: 6px; max-height: 240px; overflow: auto; margin: 0 0 14px; padding: 0; list-style: none; font: var(--t-callout); }
	.copy-list li { display: flex; justify-content: space-between; gap: 12px; }
	.copy-list span { color: var(--text-muted); white-space: nowrap; }
</style>
