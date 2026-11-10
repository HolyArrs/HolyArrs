// The movie page's versions: adding and removing a version.
import { secondVersion } from './library.ts';
import { api, message } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { LibraryFolder, PickRulesView, TitleView, VersionView } from '#lib/core/types.ts';

export function createTitleVersions(ctx: { title: TitleView | null; rules: PickRulesView[]; id: number; allVersions: boolean; load: (which?: 'all' | 'title' | 'releases') => Promise<void> }) {
	let versionDialog: HTMLDialogElement | undefined = $state();
	let folders = $state<LibraryFolder[]>([]);
	let vLabel = $state('4K');
	let vEdition = $state('');
	let vAudio = $state('');
	let vFolderName = $state('');
	let vRules = $state(0);
	let vFolder = $state(0);
	let vError = $state('');
	let vBusy = $state(false);
	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	const slotOf = (v: VersionView) => ctx.title?.slots.find((s) => s.version_id === v.id);

	async function movieRules() {
		if (!ctx.rules.length) {
			ctx.rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('movie'));
		}
	}

	async function openVersion() {
		if (!ctx.title) return;
		vError = '';
		try {
			await movieRules();
			if (!folders.length) folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
			const used = new Set(ctx.title.versions.map((v) => v.label.toLowerCase()));
			const sv = secondVersion(ctx.rules, movieFolders, ctx.title.library_folder_id ?? 0);
			vLabel = used.has(sv.label.toLowerCase()) ? '' : sv.label;
			vRules = sv.pick_rules_id;
			vEdition = ''; vAudio = ''; vFolderName = '';
			vFolder = sv.library_folder_id || movieFolders[0]?.id || 0;
		} catch (e) {
			vError = message(e);
		}
		// Open once the choices are filled in, so typing is never overwritten.
		versionDialog?.showModal();
	}

	async function addVersion(e: SubmitEvent) {
		e.preventDefault();
		vBusy = true;
		vError = '';
		try {
			await api.post(`/titles/${ctx.id}/versions`, { label: vLabel.trim(), pick_rules_id: vRules, library_folder_id: vFolder, edition: vEdition.trim(), audio_languages: vAudio.split(/[,\s]+/).filter(Boolean), folder_name: vFolderName });
			versionDialog?.close();
			ctx.allVersions = true;
			await ctx.load('title');
		} catch (err) {
			vError = message(err);
		} finally {
			vBusy = false;
		}
	}

	async function removeVersion(v: VersionView) {
		if (!confirm(t('movies.versions.stopConfirm', { label: v.label }))) return;
		try {
			await api.del(`/titles/${ctx.id}/versions/${v.id}`);
			await ctx.load('title');
		} catch (e) {
			toasts.push(t('movies.versions.removeFailed'), 'fail', { body: message(e) });
		}
	}

	// Choose a release: the confirm dialog and the turn-down dialog.

	return {
		openVersion,
		slotOf,
		removeVersion,
		get versionDialog() { return versionDialog; },
		set versionDialog(v) { versionDialog = v; },
		addVersion,
		get vLabel() { return vLabel; },
		set vLabel(v) { vLabel = v; },
		get vEdition() { return vEdition; },
		set vEdition(v) { vEdition = v; },
		get vAudio() { return vAudio; },
		set vAudio(v) { vAudio = v; },
		get vFolderName() { return vFolderName; },
		set vFolderName(v) { vFolderName = v; },
		get vRules() { return vRules; },
		set vRules(v) { vRules = v; },
		get vFolder() { return vFolder; },
		set vFolder(v) { vFolder = v; },
		get movieFolders() { return movieFolders; },
		get vError() { return vError; },
		set vError(v) { vError = v; },
		get vBusy() { return vBusy; },
		set vBusy(v) { vBusy = v; },
	};
}
