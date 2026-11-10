// The movie page's rename and delete dialogs.
import { type RemovePlan } from './organise.ts';
import { crumbFor } from './title-page.ts';
import { goto } from '$app/navigation';
import { to } from '#lib/core/paths.ts';
import { api, message, ApiError } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createTitleRemove(ctx: any) {
	let renameDialog: HTMLDialogElement | undefined = $state();
	let renameOpen = $state(false);
	let deleteDialog: HTMLDialogElement | undefined = $state();
	let delFiles = $state(false);
	let delNever = $state(false);
	let delPlan = $state<RemovePlan | null>(null);
	let delError = $state('');
	let delBusy = $state(false);
	let delAsk = 0;

	function openRename() {
		renameOpen = true;
		renameDialog?.showModal();
	}

	async function planDelete() {
		const ask = ++delAsk;
		delError = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${ctx.id}/delete`, { delete_files: delFiles, never_add: delNever, apply: false });
			if (ask === delAsk) delPlan = p;
		} catch (e) {
			if (ask === delAsk) delError = message(e);
		}
	}

	function openDelete() {
		delFiles = false;
		delNever = false;
		delPlan = null;
		deleteDialog?.showModal();
		void planDelete();
	}

	async function doDelete() {
		delBusy = true;
		delError = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${ctx.id}/delete`, { delete_files: delFiles, never_add: delNever, apply: true });
			deleteDialog?.close();
			toasts.push(t('mediafiles.delete.deleted', { name: p.name }), 'ok', { body: p.held.length ? (p.holding_days ? t('mediafiles.delete.keptDays', { n: p.holding_days }) : t('mediafiles.delete.keptFolder')) : t('mediafiles.delete.stay') });
			await goto(to(crumbFor(ctx.title?.kind ?? 'movie').path));
		} catch (e) {
			delError = message(e);
			if (e instanceof ApiError && e.body.plan) delPlan = e.body.plan as RemovePlan;
		} finally {
			delBusy = false;
		}
	}

	return {
		openRename,
		openDelete,
		get renameDialog() { return renameDialog; },
		set renameDialog(v) { renameDialog = v; },
		get renameOpen() { return renameOpen; },
		set renameOpen(v) { renameOpen = v; },
		get deleteDialog() { return deleteDialog; },
		set deleteDialog(v) { deleteDialog = v; },
		get delFiles() { return delFiles; },
		set delFiles(v) { delFiles = v; },
		planDelete,
		get delNever() { return delNever; },
		set delNever(v) { delNever = v; },
		get delPlan() { return delPlan; },
		set delPlan(v) { delPlan = v; },
		get delError() { return delError; },
		set delError(v) { delError = v; },
		doDelete,
		get delBusy() { return delBusy; },
		set delBusy(v) { delBusy = v; },
	};
}
