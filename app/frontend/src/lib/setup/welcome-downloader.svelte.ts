// The welcome step for the download client: the form, the test and the saved list.
import { emptyForm, formBody, formFrom, reachWords, type DownloaderForm, type DownloaderView } from '#lib/downloads/index.ts';
import { tick } from 'svelte';
import { api, message, ApiError } from '#lib/core/api.ts';
import { t as say } from '#lib/core/i18n/i18n.svelte.ts';

export function createWelcomeDownloader(ctx: any) {
	const dlKinds: DownloaderForm['kind'][] = ['qbittorrent', 'sabnzbd', 'transmission', 'nzbget', 'deluge', 'rtorrent'];
	const dlPorts: Record<DownloaderForm['kind'], string> = { qbittorrent: '8080', sabnzbd: '8080', transmission: '9091', nzbget: '6789', deluge: '8112', rtorrent: '8000' };
	let dl = $state<DownloaderForm>(emptyForm());
	// Every connected downloader, each with its last test answer; the form shows when there is none yet or the owner adds another.
	let dlList = $state<DownloaderView[]>([]);
	let dlTests = $state<Record<number, { ok: boolean; text: string }>>({});
	let dlAdding = $state(false);
	// The downloader being edited (its form is open), or null when adding a new one.
	let dlEditing = $state<number | null>(null);
	// Connect's answer when the downloader did not answer, shown under Address.
	let dlError = $state('');
	// Shown under User name when an app that signs in turned down a try without one.
	let dlAuth = $state('');
	const dlForm = $derived(dlList.length === 0 || dlAdding);
	const dlUser = $derived(dl.kind === 'qbittorrent' || dl.kind === 'transmission' || dl.kind === 'nzbget' || dl.kind === 'rtorrent');
	function pickDl(k: DownloaderForm['kind']) {
		ctx.error = '';
		dlError = '';
		dlAuth = '';
		const url = dl.url;
		dl = { ...emptyForm(k), url: url !== 'http://' ? url : 'http://' };
	}
	async function listDownloaders(): Promise<DownloaderView[]> {
		try {
			return (await api.get<{ downloaders: DownloaderView[] }>('/downloaders')).downloaders ?? [];
		} catch {
			return [];
		}
	}
	// The list did not load: shown like the Source step, with Try again.
	let dlLoadError = $state('');
	async function loadDownloader() {
		try {
			dlList = (await api.get<{ downloaders: DownloaderView[] }>('/downloaders')).downloaders ?? [];
			dlLoadError = '';
		} catch (err) {
			dlLoadError = message(err);
			return;
		}
		// Each one is tested again, so the row shows its version or that it is down.
		await Promise.all(dlList.map((d) => testDl(d.id)));
	}
	async function testDl(id: number) {
		try {
			const t = await api.post<{ ok: boolean; text: string }>(`/downloaders/${id}/test`);
			dlTests = { ...dlTests, [id]: { ok: t.ok, text: t.ok ? t.text : reachWords(t.text) } };
		} catch (err) {
			dlTests = { ...dlTests, [id]: { ok: false, text: reachWords(message(err)) } };
		}
	}
	function editDl(d: DownloaderView) {
		ctx.error = '';
		dlError = '';
		dl = formFrom(d);
		dlEditing = d.id;
		dlAdding = true;
	}
	async function removeDl(d: DownloaderView) {
		ctx.error = '';
		ctx.busy = true;
		try {
			await api.del(`/downloaders/${d.id}`);
			dlList = dlList.filter((x) => x.id !== d.id);
		} catch (err) {
			ctx.error = message(err);
		} finally {
			ctx.busy = false;
		}
	}
	async function saveDownloader(e: SubmitEvent) {
		e.preventDefault();
		if (ctx.busy) return;
		if (!dlForm) return afterDownloader();
		ctx.error = '';
		ctx.busy = true;
		dlError = '';
		dlAuth = '';
		// A bare host:port gets http:// in front.
		const u = dl.url.trim();
		if (u && !/^[a-z][a-z0-9+.-]*:\/\//i.test(u)) dl.url = `http://${u}`;
		try {
			const editing = dlEditing;
			const v = editing != null ? await api.put<DownloaderView>(`/downloaders/${editing}`, formBody(dl)) : await api.post<DownloaderView>('/downloaders', formBody(dl));
			if (editing != null) dlList = dlList.map((x) => (x.id === v.id ? v : x));
			// Only a downloader that answers becomes a row; a new one that does not is taken back and the form stays.
			const t = await api.post<{ ok: boolean; text: string }>(`/downloaders/${v.id}/test`).catch((err) => ({ ok: false, text: message(err) }));
			if (!t.ok) {
				if (editing == null) await api.del(`/downloaders/${v.id}`).catch(() => {});
				else dlEditing = v.id;
				const words = reachWords(t.text);
				if (!askSignIn(words)) dlError = words;
				return;
			}
			dlTests = { ...dlTests, [v.id]: { ok: t.ok, text: t.text } };
			if (editing == null) dlList = [...dlList, v];
			dlEditing = null;
			dlAdding = false;
			dl = emptyForm();
		} catch (err) {
			// HolyArrs itself did not answer: the Source step's wording.
			if (err instanceof ApiError && (err.status === 0 || err.status >= 500)) ctx.error = say('setup.notAnswering');
			else if (!askSignIn(reachWords(message(err)))) ctx.error = message(err);
		} finally {
			ctx.busy = false;
			// A failed Connect: focus the field the answer is about (the key, or else the address).
			if (dlError || (ctx.error && dlForm)) void tick().then(() => document.getElementById(!dlError && /key|password/i.test(ctx.error) ? 'dl-secret' : 'dl-url')?.focus());
		}
	}
	// An app that signs in, turned down with no user name typed: ask for both, under User name (the server's answer names an API key field the wizard does not have).
	function askSignIn(words: string): boolean {
		if (!dlUser || dl.username.trim() || words.startsWith("Can't reach")) return false;
		dlAuth = say('setup.addUserPasswordHint');
		void tick().then(() => document.getElementById('dl-user')?.focus());
		return true;
	}
	// Fresh setup always asks for a Source next; after an import only when none came over.
	async function afterDownloader() {
		if (ctx.busy) return;
		ctx.busy = true;
		try {
			if (ctx.fromApps && (await ctx.sourcesStep.sourceCount()) > 0) return ctx.go('done');
			ctx.go('source');
		} finally {
			ctx.busy = false;
		}
	}

	// Source.

	return {
		listDownloaders,
		get dl() { return dl; },
		set dl(v) { dl = v; },
		loadDownloader,
		saveDownloader,
		get dlLoadError() { return dlLoadError; },
		set dlLoadError(v) { dlLoadError = v; },
		get dlList() { return dlList; },
		set dlList(v) { dlList = v; },
		get dlTests() { return dlTests; },
		set dlTests(v) { dlTests = v; },
		get dlForm() { return dlForm; },
		editDl,
		removeDl,
		get dlAdding() { return dlAdding; },
		set dlAdding(v) { dlAdding = v; },
		get dlKinds() { return dlKinds; },
		pickDl,
		get dlError() { return dlError; },
		set dlError(v) { dlError = v; },
		get dlPorts() { return dlPorts; },
		get dlUser() { return dlUser; },
		get dlAuth() { return dlAuth; },
		set dlAuth(v) { dlAuth = v; },
		get dlEditing() { return dlEditing; },
		set dlEditing(v) { dlEditing = v; },
		afterDownloader,
	};
}
