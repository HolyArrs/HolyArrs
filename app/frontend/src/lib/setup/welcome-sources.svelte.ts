// The welcome steps for sources and the import: the sources found, what is left to do.
import type { Step } from './welcome-steps.ts';
import { kindWords, type DownloaderView } from '#lib/downloads/index.ts';
import { setContext } from 'svelte';
import { api } from '#lib/core/api.ts';
import type { LibraryFolder } from '#lib/core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type Todo = { label: string; path: string };

export function createWelcomeSources(ctx: { busy: boolean; listDownloaders: () => Promise<DownloaderView[]>; extra: Step[]; go: (s: Step) => void }) {
	let sources = $state(0);
	// Sources plus linked Jackett or Prowlarr (a fresh link may not have synced its Sources yet).
	async function sourceCount(): Promise<number> {
		const n = async (path: string, key: string) => {
			try {
				return ((await api.get<Record<string, unknown[]>>(path))[key] ?? []).length;
			} catch {
				return 0;
			}
		};
		const [s, b] = await Promise.all([n('/sources', 'sources'), n('/bridges', 'bridges')]);
		return s + b;
	}
	async function loadSources() {
		sources = await sourceCount();
	}

	// Import: the Settings move flow, right here in the panel.
	const IMPORT_QUERY = 'first=1&step=import';
	// Import offers every app, except a downloader or Prowlarr that is already connected here. The list is filled before the Read sheet first opens.
	const hideApps: string[] = [];
	setContext('move-hide-apps', hideApps);
	setContext('move-hide-count', true);
	let moveStep = $state('read');
	// After the move: ask only for what Import did not bring.
	async function afterImport() {
		ctx.busy = true;
		const [dls, srcs] = await Promise.all([ctx.listDownloaders(), sourceCount()]);
		ctx.busy = false;
		ctx.extra = [...(dls.length ? [] : (['downloader'] as Step[])), ...(srcs ? [] : (['source'] as Step[]))];
		ctx.go(ctx.extra[0] ?? 'done');
	}
	async function loadHidden() {
		const [dls, bridges] = await Promise.all([
			ctx.listDownloaders(),
			api.get<{ bridges: { kind: string }[] }>('/bridges').then((r) => r.bridges ?? [], () => [])
		]);
		hideApps.length = 0;
		for (const k of ['qbittorrent', 'sabnzbd'] as const) if (dls.some((d) => d.kind === k)) hideApps.push(k);
		if (bridges.some((b) => b.kind === 'prowlarr')) hideApps.push('prowlarr');
	}

	// Done: what a skipped step left to do, each a Settings page.
	
	let todo = $state<Todo[]>([]);
	// What is set: a short line each ("Movies · /data/movies", "qBittorrent", "Prowlarr").
	let haveSet = $state<string[]>([]);
	async function loadTodo() {
		const list = <T,>(path: string, key: string) => api.get<Record<string, T[]>>(path).then((r) => r[key] ?? [], () => null);
		const [fl, dl, br, srcs] = await Promise.all([
			list<LibraryFolder>('/library-folders', 'library_folders'),
			list<DownloaderView>('/downloaders', 'downloaders'),
			list<{ kind: string }>('/bridges', 'bridges'),
			sourceCount()
		]);
		const kindName: Record<string, string> = { movie: t('setup.movies'), show: t('setup.tvShows'), music: t('setup.music') };
		const bridgeName: Record<string, string> = { prowlarr: 'Prowlarr', jackett: 'Jackett' };
		// One line each: two downloaders of one kind would repeat a line, and the list is keyed by its text.
		haveSet = [...new Set([
			...(fl ?? []).map((f) => `${kindName[f.kind] ?? f.label} · ${f.path}`),
			...(dl ?? []).map((d) => kindWords[d.kind] ?? d.name),
			...(br ?? []).map((b) => bridgeName[b.kind] ?? b.kind)
		])];
		// Unknown (the list failed to load) counts as set, so nothing is asked twice.
		const folders = fl ? fl.length : 1;
		const dls = dl ? dl.length : 1;
		todo = [
			...(folders ? [] : [{ label: t('setup.folders'), path: '/settings/library-folders' }]),
			...(dls ? [] : [{ label: t('setup.downloader'), path: '/settings/downloaders' }]),
			...(srcs ? [] : [{ label: t('setup.source'), path: '/settings/sources' }])
		];
	}
	// Invite family: turns on Requests (what the old "family" question did), then opens Users.

	return {
		loadTodo,
		sourceCount,
		loadSources,
		loadHidden,
		get todo() { return todo; },
		set todo(v) { todo = v; },
		get moveStep() { return moveStep; },
		set moveStep(v) { moveStep = v; },
		get sources() { return sources; },
		set sources(v) { sources = v; },
		get IMPORT_QUERY() { return IMPORT_QUERY; },
		afterImport,
		get haveSet() { return haveSet; },
		set haveSet(v) { haveSet = v; },
	};
}
