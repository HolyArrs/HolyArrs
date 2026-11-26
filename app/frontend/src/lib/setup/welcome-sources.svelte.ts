// The welcome steps for sources and the import: the sources found, what is left to do.
import { kindWords, type DownloaderView } from '#lib/downloads/index.ts';
import { setContext } from 'svelte';
import { api } from '#lib/core/api.ts';
import type { AlertView, LibraryFolder } from '#lib/core/types.ts';
import { alertMissing } from '#lib/alerts/index.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type Todo = { label: string; path: string; end?: string };

export function createWelcomeSources(ctx: { busy: boolean; listDownloaders: () => Promise<DownloaderView[]>; reload: () => Promise<void> }) {
	let sources = $state(0);
	let viaProwlarr = $state(0);
	let prowlarrAddr = $state('');
	let prowlarrNames = $state<string[]>([]);
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
		// Indexers that are Prowlarr per-site feeds (http://prowlarr:9696/5/api): Radarr or Sonarr only had them from Prowlarr.
		const list = await api.get<{ sources: { url: string; name: string }[] }>('/sources').then((r) => r.sources ?? [], () => []);
		const viaFeeds = list.filter((x) => { try { return /^\/\d+\/api\/?$/.test(new URL(x.url).pathname); } catch { return false; } });
		const feeds = viaFeeds.map((x) => new URL(x.url));
		prowlarrNames = viaFeeds.map((x) => x.name);
		viaProwlarr = feeds.length;
		// Prowlarr's own address, from its per-site feeds: Manage opens with it filled in.
		prowlarrAddr = feeds[0]?.origin ?? '';
		return s + b;
	}
	async function loadSources() {
		sources = await sourceCount();
	}

	// Import: the Settings move flow, right here in the panel.
	const IMPORT_QUERY = 'first=1&step=apps';
	// Import offers every app, except a downloader or Prowlarr that is already connected here. The list is filled before the Read sheet first opens.
	const hideApps: string[] = [];
	setContext('move-hide-apps', hideApps);
	setContext('move-hide-count', true);
	let moveStep = $state('read');
	// After the move: the downloaders and sources it brought show in their sections below, on the same step.
	async function afterImport() {
		ctx.busy = true;
		try {
			await ctx.reload();
		} finally {
			ctx.busy = false;
		}
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
		const [fl, dl, br, srcs, al] = await Promise.all([
			list<LibraryFolder>('/library-folders', 'library_folders'),
			list<DownloaderView>('/downloaders', 'downloaders'),
			list<{ kind: string }>('/bridges', 'bridges'),
			sourceCount(),
			list<AlertView>('/alerts', 'alerts')
		]);
		// Imported Alerts come over off until their token is typed.
		const keyless = (al ?? []).filter((a) => !a.enabled && alertMissing(a)).length;
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
			...(srcs ? [] : [{ label: t('setup.source'), path: '/settings/sources' }]),
			...(keyless ? [{ label: keyless === 1 ? t('setup.alertsNeedKeyOne') : t('setup.alertsNeedKey', { n: keyless }), path: '/settings/alerts', end: t('welcome.change') }] : [])
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
		get viaProwlarr() { return viaProwlarr; },
		get prowlarrAddr() { return prowlarrAddr; },
		get prowlarrNames() { return prowlarrNames; },
		get IMPORT_QUERY() { return IMPORT_QUERY; },
		afterImport,
		get haveSet() { return haveSet; },
		set haveSet(v) { haveSet = v; },
	};
}
