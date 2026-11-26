// The welcome step for the download client: the form, the test and the saved list.
import { emptyForm, formBody, formFrom, reachWords, type DownloaderForm, type DownloaderView } from '#lib/downloads/index.ts';
import { tick } from 'svelte';
import { api, message, ApiError } from '#lib/core/api.ts';
import { t as say } from '#lib/core/i18n/i18n.svelte.ts';

export function createWelcomeDownloader(ctx: any) {
	const dlKinds: DownloaderForm['kind'][] = ['qbittorrent', 'sabnzbd', 'transmission', 'nzbget', 'deluge', 'rtorrent', 'aria2'];
	const dlPorts: Record<DownloaderForm['kind'], string> = { qbittorrent: '8080', sabnzbd: '8080', transmission: '9091', nzbget: '6789', deluge: '8112', rtorrent: '8000', aria2: '6800' };
	let dl = $state<DownloaderForm>(emptyForm());
	// Every connected downloader, each with its last test answer; the form shows when there is none yet or the owner adds another.
	let dlList = $state<DownloaderView[]>([]);
	let dlTests = $state<Record<number, { ok: boolean; text: string }>>({});
	let dlAdding = $state(false);
	// The old app each Downloader came from ("sonarr"), by id, when an import brought it.
	// Kept for this tab, so a reload of the step still says where each came from.
	const FOUND_KEY = 'holyarrs.welcome.found';
	let found = $state<Record<number, string>>(readKept(FOUND_KEY));
	// The ones whose key or password the old app hides, by id: the client kind and what to paste.
	const HIDDEN_KEY = 'holyarrs.welcome.hidden';
	let hidden = $state<Record<number, { kind: string; needs: string }>>(readKept(HIDDEN_KEY));
	function readKept<T>(key: string): Record<number, T> {
		try {
			return JSON.parse(sessionStorage.getItem(key) ?? '{}') ?? {};
		} catch {
			return {};
		}
	}
	function keep(key: string, v: unknown) {
		try {
			sessionStorage.setItem(key, JSON.stringify(v));
		} catch {
			// No storage: this visit still shows it.
		}
	}
	// An import brought these: say where each came from, and which wait for a key.
	function addBrought(list: { id: number; app: string; kind?: string; needs?: string }[]) {
		found = { ...found, ...Object.fromEntries(list.map((b) => [b.id, b.app])) };
		hidden = { ...hidden, ...Object.fromEntries(list.filter((b) => b.needs).map((b) => [b.id, { kind: b.kind ?? '', needs: b.needs ?? '' }])) };
		keep(FOUND_KEY, found);
		keep(HIDDEN_KEY, hidden);
	}
	// A key pasted on a brought row: the row shows the new test answer.
	function keySaved(id: number, r: { ok: boolean; text: string }) {
		dlTests = { ...dlTests, [id]: r };
	}
	// The downloader being edited (its form is open), or null when adding a new one.
	let dlEditing = $state<number | null>(null);
	// Connect's answer when the downloader did not answer, shown under Address.
	let dlError = $state('');
	// Shown under User name when an app that signs in turned down a try without one.
	let dlAuth = $state('');
	const dlForm = $derived(dlList.length === 0 || dlAdding);
	// qBittorrent signs in with an API key (5.2 or newer, asked first, like Settings) or a user name and password.
	let qbitWay = $state<'key' | 'login'>('key');
	const dlUser = $derived((dl.kind === 'qbittorrent' && qbitWay === 'login') || dl.kind === 'transmission' || dl.kind === 'nzbget' || dl.kind === 'rtorrent');
	function pickWay(w: 'key' | 'login') {
		qbitWay = w;
		dl.secret = '';
		dlAuth = '';
		if (w === 'key') dl.username = '';
	}
	function pickDl(k: DownloaderForm['kind']) {
		ctx.error = '';
		dlError = '';
		dlAuth = '';
		const url = dl.url;
		dl = { ...emptyForm(k), url: url !== 'http://' ? url : 'http://' };
		qbitWay = 'key';
	}
	// Cancel and a finished save: an empty form, asking for the key again.
	function resetForm() {
		dlError = '';
		dlAuth = '';
		dlAdding = false;
		dlEditing = null;
		dl = emptyForm();
		qbitWay = 'key';
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
			const t = await api.post<{ ok: boolean; text: string; health?: { state: string } }>(`/downloaders/${id}/test`);
			dlTests = { ...dlTests, [id]: { ok: t.ok, text: t.ok ? t.text : reachWords(t.text) } };
			// One the old apps used comes over paused; it answers, so it is connected.
			if (t.ok && found[id] && t.health?.state === 'paused') await api.post(`/downloaders/${id}/resume`).catch(() => {});
		} catch (err) {
			dlTests = { ...dlTests, [id]: { ok: false, text: reachWords(message(err)) } };
		}
	}
	function editDl(d: DownloaderView) {
		ctx.error = '';
		dlError = '';
		dl = formFrom(d);
		qbitWay = d.username ? 'login' : 'key';
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
		// Enter with no form open stays on the step; Continue moves on.
		if (!dlForm) return;
		ctx.error = '';
		ctx.busy = true;
		dlError = '';
		dlAuth = '';
		// A bare host:port gets http:// in front.
		const u = dl.url.trim();
		if (u && !/^[a-z][a-z0-9+.-]*:\/\//i.test(u)) dl.url = `http://${u}`;
		try {
			const editing = dlEditing;
			// Key mode never sends a user name left over from the other way.
			const body = { ...formBody(dl), ...(dl.kind === 'qbittorrent' && qbitWay === 'key' ? { username: '' } : {}) };
			const v = editing != null ? await api.put<DownloaderView>(`/downloaders/${editing}`, body) : await api.post<DownloaderView>('/downloaders', body);
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
			resetForm();
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
		if (!dlUser || dl.username.trim() || words === say('core.settings.cantReach') || words === say('core.settings.tryHttp')) return false;
		dlAuth = say('setup.addUserPasswordHint');
		void tick().then(() => document.getElementById('dl-user')?.focus());
		return true;
	}

	// Check connection before keeping it: a new one is added, tested and taken back, so nothing is saved yet.
	let dlChecked = $state(false);
	let dlChecking = $state(false);
	async function checkDl() {
		if (ctx.busy || dlChecking || dlEditing != null) return;
		ctx.error = '';
		dlError = '';
		dlAuth = '';
		dlChecked = false;
		const u = dl.url.trim();
		if (u && !/^[a-z][a-z0-9+.-]*:\/\//i.test(u)) dl.url = `http://${u}`;
		dlChecking = true;
		try {
			const body = { ...formBody(dl), ...(dl.kind === 'qbittorrent' && qbitWay === 'key' ? { username: '' } : {}) };
			const v = await api.post<DownloaderView>('/downloaders', body);
			const t = await api.post<{ ok: boolean; text: string }>(`/downloaders/${v.id}/test`).catch((err) => ({ ok: false, text: message(err) }));
			await api.del(`/downloaders/${v.id}`).catch(() => {});
			if (t.ok) dlChecked = true;
			else {
				const words = reachWords(t.text);
				if (!askSignIn(words)) dlError = words;
			}
		} catch (err) {
			if (err instanceof ApiError && (err.status === 0 || err.status >= 500)) ctx.error = say('setup.notAnswering');
			else if (!askSignIn(reachWords(message(err)))) ctx.error = message(err);
		} finally {
			dlChecking = false;
		}
	}

	// Continue with an address typed but Connect not pressed: connect it first, and move on only once it answers.
	const typed = $derived(dlForm && dl.url.trim() !== '' && dl.url.trim() !== 'http://');
	async function connectTyped(): Promise<boolean> {
		if (!typed) return true;
		await saveDownloader(new SubmitEvent('submit'));
		return !dlForm && !dlError && !ctx.error;
	}


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
		get typed() { return typed; },
		connectTyped,
		editDl,
		removeDl,
		get found() { return found; },
		get hidden() { return hidden; },
		addBrought,
		keySaved,
		get dlAdding() { return dlAdding; },
		set dlAdding(v) { dlAdding = v; },
		get dlKinds() { return dlKinds; },
		pickDl,
		get dlError() { return dlError; },
		set dlError(v) { dlError = v; },
		get dlPorts() { return dlPorts; },
		get dlUser() { return dlUser; },
		get qbitWay() { return qbitWay; },
		pickWay,
		resetForm,
		checkDl,
		get dlChecked() { return dlChecked; },
		set dlChecked(v) { dlChecked = v; },
		get dlChecking() { return dlChecking; },
		get dlAuth() { return dlAuth; },
		set dlAuth(v) { dlAuth = v; },
		get dlEditing() { return dlEditing; },
		set dlEditing(v) { dlEditing = v; },
	};
}
