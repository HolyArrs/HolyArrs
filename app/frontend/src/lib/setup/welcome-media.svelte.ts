// The welcome step for the media folders: which parts are on and where each keeps its files.
import { type IconName } from '#lib/ui/Icon.svelte';
import { partsNow, refreshParts } from '#lib/mediafiles/index.ts';
import { applyParts } from './controls.ts';
import { tick } from 'svelte';
import { api, message, ApiError } from '#lib/core/api.ts';
import type { LibraryFolder } from '#lib/core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

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
		// Back or reload, once a folder is saved: show the parts as saved, so Continue does not switch one off. A fresh install keeps the defaults.
		if (have.length) await refreshParts().catch(() => {});
		const on = ctx.partsOn();
		if (have.length && partsNow.answer && (on.movies || on.shows || on.music)) media = { movies: on.movies, shows: on.shows, music: on.music };
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
		return text ? text[0].toUpperCase() + text.slice(1) : text;
	}
	// skip: keep the parts, add no folders now (Settings has them later).
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
			// Subtitles come on with Movies or TV shows; Requests stays as it is.
			await applyParts({ ...media, subtitles: media.movies || media.shows, wishlist: ctx.partsOn().wishlist });
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
		if (!failed) ctx.go('downloader');
		else {
			await tick();
			document.getElementById(`folder-${rows.find((r) => r.error)?.kind}`)?.focus();
		}
	}

	// Downloader.

	return {
		loadMedia,
		saveMedia,
		get rows() { return rows; },
		set rows(v) { rows = v; },
		get media() { return media; },
		set media(v) { media = v; },
		get mediaOff() { return mediaOff; },
	};
}
