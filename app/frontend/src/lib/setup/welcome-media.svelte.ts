// The welcome step for the media folders: which parts are on and where each keeps its files.
import { type IconName } from '#lib/ui/Icon.svelte';
import { partsNow, refreshParts } from '#lib/mediafiles/index.ts';
import { applyParts } from './controls.ts';
import { tick } from 'svelte';
import { api, message, ApiError } from '#lib/core/api.ts';
import type { LibraryFolder } from '#lib/core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Library } from './welcome-steps.ts';

export type MediaPart = 'movies' | 'shows' | 'music';

export type Kind = 'movie' | 'show' | 'music';

export type Row = { part: MediaPart; kind: Kind; label: string; icon: IconName; hint: string; path: string; saved: string; id: number | null; editing: boolean; error: string };

export function createWelcomeMedia(ctx: any) {
	
	
	// saved/id: the folder already made; editing reopens it to type a new path.
	
	const kindRows: Pick<Row, 'part' | 'kind' | 'label' | 'icon' | 'hint'>[] = [
		{ part: 'movies', kind: 'movie', get label() { return t('setup.movies'); }, icon: 'film', hint: '/movies' },
		{ part: 'shows', kind: 'show', get label() { return t('setup.tvShows'); }, icon: 'tv', hint: '/tv' },
		{ part: 'music', kind: 'music', get label() { return t('setup.music'); }, icon: 'music', hint: '/music' }
	];
	let rows = $state<Row[]>([]);
	let media = $state<Record<MediaPart, boolean>>({ movies: true, shows: true, music: false });
	const mediaOff = $derived(!media.movies && !media.shows && !media.music);
	async function loadMedia() {
		let have: LibraryFolder[] = [];
		try {
			have = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders ?? [];
		} catch {
			// Show empty rows.
		}
		// The parts picked on the choice screen: one folder row each.
		await refreshParts().catch(() => {});
		const on = ctx.partsOn();
		if (partsNow.answer && (on.movies || on.shows || on.music)) media = { movies: on.movies, shows: on.shows, music: on.music };
		// Keep anything typed while the list was loading.
		rows = kindRows.map((k) => {
			const f = have.find((x) => x.kind === k.kind);
			const cur = rows.find((r) => r.kind === k.kind);
			return { ...k, path: cur?.path ?? '', saved: f?.path ?? '', id: f?.id ?? null, editing: cur?.editing ?? false, error: '' };
		});
	}
	// The server's folder answers, in plain words.
	function folderWords(text: string, kind: Kind = 'movie'): string {
		if (/absolute/i.test(text)) return t('setup.fullPath', { path: kind === 'show' ? '/data/tv' : kind === 'music' ? '/data/music' : '/data/movies' });
		if (/already a library folder/i.test(text)) return t('setup.folderUsed');
		if (/does not exist|not a folder|no such file/i.test(text)) return t('setup.folderMissing');
		if (/permission|read-only|access is denied|not allowed/i.test(text)) return t('setup.folderNoAccess');
		return t('setup.folderCantUse');
	}
	// skip: keep the parts, add no folders now (Settings has them later).
	// Change on one saved folder: Save (or Enter) keeps the new path for that part alone and shows it saved again.
	async function saveRow(r: Row) {
		const path = r.path.trim();
		if (!path || ctx.busy) return;
		if (path === r.saved) return void ((r.editing = false), (r.error = ''));
		ctx.busy = true;
		r.error = '';
		const old = r.id != null ? r.saved : '';
		try {
			if (old) await api.del(`/library-folders/${r.id}`);
			const f = await api.post<LibraryFolder>('/library-folders', { path, kind: r.kind, label: r.label });
			r.saved = f.path;
			r.id = f.id;
			r.editing = false;
		} catch (err) {
			r.error = folderWords(message(err), r.kind);
			if (old) {
				const back = await api.post<LibraryFolder>('/library-folders', { path: old, kind: r.kind, label: r.label }).catch(() => null);
				r.saved = back?.path ?? '';
				r.id = back?.id ?? null;
			}
		} finally {
			ctx.busy = false;
		}
	}
	async function saveMedia(skip: boolean) {
		if (mediaOff || ctx.busy) return;
		ctx.error = '';
		// Continue needs a folder for every part that is on (Skip adds none).
		if (!skip) {
			const empty = rows.filter((r) => media[r.part] && !(r.saved && !r.editing) && !r.path.trim());
			for (const r of empty) r.error = t('setup.addFolderOrOff');
			if (empty.length) return void tick().then(() => document.getElementById(`folder-${empty[0].kind}`)?.focus());
		}
		ctx.busy = true;
		try {
			await refreshParts();
			// Subtitles and Requests stay as picked on the choice screen (Subtitles needs Movies or TV shows).
			await applyParts({ ...media, subtitles: ctx.partsOn().subtitles && (media.movies || media.shows), wishlist: ctx.partsOn().wishlist });
			await refreshParts();
		} catch (err) {
			ctx.busy = false;
			return void (ctx.error = message(err));
		}
		let failed = false;
		for (const r of rows) {
			if (skip || !media[r.part]) continue;
			if (r.editing && r.path.trim() === r.saved) r.editing = false;
			if ((r.saved && !r.editing) || !r.path.trim()) continue;
			r.error = '';
			// A changed folder is replaced: on first run it holds nothing yet. If the new path is refused, the old one is put back.
			const old = r.editing && r.id != null ? r.saved : '';
			try {
				if (old) {
					await api.del(`/library-folders/${r.id}`);
					r.saved = '';
					r.id = null;
				}
				// Named after its part ("Movies"), not the last bit of the path.
				const f = await api.post<LibraryFolder>('/library-folders', { path: r.path.trim(), kind: r.kind, label: r.label });
				r.saved = f.path;
				r.id = f.id;
				r.editing = false;
			} catch (err) {
				// Saved meanwhile (another tab, same path): take that one and carry on.
				if (err instanceof ApiError && err.status === 409) {
					try {
						const have = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders ?? [];
						const norm = (s: string) => s.trim().replace(/\\/g, '/').replace(/\/+$/, '');
						const same = have.find((x) => x.kind === r.kind && norm(x.path) === norm(r.path));
						if (same) {
							r.saved = same.path;
							r.id = same.id;
							r.editing = false;
							continue;
						}
					} catch {
						// Fall through to the error.
					}
				}
				r.error = folderWords(message(err), r.kind);
				failed = true;
				if (old && r.id == null) {
					try {
						const back = await api.post<LibraryFolder>('/library-folders', { path: old, kind: r.kind, label: r.label });
						r.saved = back.path;
						r.id = back.id;
					} catch {
						// The old folder could not come back; the row stays empty to fill again.
					}
				}
			}
		}
		ctx.busy = false;
		if (!failed) {
			// Skip counts as finished only once its save went through, so a reload never passes Folders on a failed skip.
			if (skip) {
				try {
					sessionStorage.setItem(SKIP_KEY, '1');
				} catch {
					// No storage: a reload asks for folders again.
				}
			}
			ctx.next();
		}
		else {
			await tick();
			document.getElementById(`folder-${rows.find((r) => r.error)?.kind}`)?.focus();
		}
	}

	// Downloader.

	// Folders > Skip for now counts as finished for this tab, so a reload past Folders stays where it was.
	const SKIP_KEY = 'holyarrs.welcome.foldersSkipped';
	function foldersSkipped(): boolean {
		try {
			return sessionStorage.getItem(SKIP_KEY) === '1';
		} catch {
			return false;
		}
	}
	function skipFolders() {
		void saveMedia(true);
	}

	// The server's paths of the libraries picked on Connect server, to fill Folders.
	let serverLibs: Library[] = [];
	// Folders: rows still empty take the path of a library of the same kind on the media server.
	// The media server's own path (C:\Data\Plex\Movies on a Windows Plex) often means nothing here: the box gets
	// the folder HolyArrs itself can see, the same one or the one with the same name (/media/Movies), else stays empty.
	async function foldersHere(): Promise<string[]> {
		type Ans = { folders: { path: string }[] };
		try {
			const roots = (await api.post<Ans>('/migrate/folders', { path: '', names: [] })).folders.map((f) => f.path);
			const kids = await Promise.all(roots.map((p) => api.post<Ans>('/migrate/folders', { path: p, names: [] }).then((a) => a.folders.map((f) => f.path), () => [])));
			return [...roots, ...kids.flat()];
		} catch {
			return [];
		}
	}
	function hereFor(p: string, here: string[]): string {
		const clean = p.replace(/\\/g, '/').replace(/\/+$/, '');
		if (here.includes(clean)) return clean;
		const leaf = (x: string) => (x.split('/').filter(Boolean).pop() ?? '').toLowerCase();
		const hits = here.filter((h) => leaf(h) === leaf(clean));
		return hits.length === 1 ? hits[0] : '';
	}
	async function prefillFolders() {
		let libs = [...serverLibs];
		if (!libs.length && ctx.withServer) {
			try {
				const ms = (await api.get<{ media_servers: { id: number; kind: string }[] }>('/media-servers')).media_servers ?? [];
				// Newest server first: the one just linked on Connect server.
				for (const m of [...ms].reverse()) libs.push(...((await api.get<{ libraries: Library[] }>(`/media-servers/${m.id}/libraries`)).libraries ?? []).filter((l) => l.on));
			} catch {
				// Typed by hand then.
			}
		}
		if (!serverLibs.length && libs.length) serverLibs = libs;
		const here = libs.length ? await foldersHere() : [];
		for (const r of rows) {
			const l = libs.find((x) => x.kind === r.kind && x.paths.length);
			if (l && !r.saved && !r.path.trim()) r.path = hereFor(l.paths[0], here);
			// A part the owner already picked (or turned off) stays as picked.
			if (l && !r.saved && !partsNow.answer) media[r.part] = true;
		}
	}

	return {
		loadMedia,
		saveMedia,
		saveRow,
		foldersSkipped,
		skipFolders,
		prefillFolders,
		get serverLibs() { return serverLibs; },
		set serverLibs(v) { serverLibs = v; },
		get rows() { return rows; },
		set rows(v) { rows = v; },
		get media() { return media; },
		set media(v) { media = v; },
		get mediaOff() { return mediaOff; },
	};
}
