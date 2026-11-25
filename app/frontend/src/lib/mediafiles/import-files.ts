// Types and rules for importing files by hand into the Library.
import { episodeCode as code } from '../core/episodes.ts';
import { t } from '../core/i18n/i18n.svelte.ts';
import { serverWords } from '../core/i18n/server-words.ts';

export interface ByHandGuess {
	title_id?: number;
	title_name?: string;
	title_kind?: string;
	year?: number;
	version_id?: number;
	season?: number;
	episode?: number;
	slot_id?: number;
	has_file?: boolean;
	confidence: number;
	detail?: string;
	album_id?: number;
	album_name?: string;
	album_year?: number;
	disc?: number;
	track?: number;
	track_name?: string;
	/** Why a music file has no match: no_artist, no_album, two_album, no_track or no_facts. */
	reason?: string;
}

export interface ByHandFile {
	path: string;
	name: string;
	size: number;
	kind: 'video' | 'audio';
	quality?: string;
	guess: ByHandGuess;
	note?: string;
	in_library?: boolean;
}

export interface ByHandScan {
	path: string;
	files: ByHandFile[];
	more: boolean;
}

export interface ByHandResult {
	path: string;
	outcome: 'placed' | 'refused' | 'failed';
	placed_by?: string;
	target?: string;
	title_id?: number;
	slot_id?: number;
	file_id?: number;
	error?: string;
}

export interface Pick {
	title_id: number;
	title_name: string;
	title_kind: string;
	year?: number;
	version_id?: number;
	season?: number;
	episode?: number;
	has_file?: boolean;
	// Music: the album and track (its Slot) the file fills.
	album_id?: number;
	album_name?: string;
	album_year?: number;
	disc?: number;
	track?: number;
	track_name?: string;
	slot_id?: number;
	// Put this file in place of the one it has; the old one goes to Recently deleted.
	replace?: boolean;
}

// Starts from the scanner guess; no guess means no pick.
export function startPick(f: ByHandFile): Pick | null {
	const g = f.guess;
	if (!g.title_id || !g.title_name) return null;
	return {
		title_id: g.title_id,
		title_name: g.title_name,
		title_kind: g.title_kind ?? 'movie',
		year: g.year || undefined,
		version_id: g.version_id || undefined,
		season: g.season,
		episode: g.episode,
		has_file: g.has_file,
		...(g.title_kind === 'artist'
			? { version_id: undefined, album_id: g.album_id, album_name: g.album_name, album_year: g.album_year, disc: g.disc, track: g.track, track_name: g.track_name, slot_id: g.slot_id }
			: {})
	};
}

function trackLine(p: Pick): string {
	const n = String(p.track ?? 0).padStart(2, '0');
	return `${p.disc && p.disc > 1 ? `${p.disc}-` : ''}${n}${p.track_name ? ` ${p.track_name}` : ''}`;
}

export function episodeCode(season?: number, episode?: number): string {
	if (season === undefined || episode === undefined) return '';
	return code(season, episode);
}

export function pickLine(p: Pick | null): string {
	if (!p) return t('mediafiles.byHand.noMatch');
	const name = `${p.title_name}${p.year ? ` (${p.year})` : ''}`;
	if (p.title_kind === 'artist') {
		if (!p.album_id) return `${name} · ${t('mediafiles.byHand.pickAlbum')}`;
		const album = `${p.album_name ?? ''}${p.album_year ? ` (${p.album_year})` : ''}`;
		return `${name} · ${album} · ${p.slot_id ? trackLine(p) : t('mediafiles.byHand.pickTrack')}`;
	}
	if (p.title_kind !== 'show') return name;
	return `${name} · ${episodeCode(p.season, p.episode) || t('mediafiles.byHand.pickEpisode')}`;
}

// Returns why a file cannot be imported, or empty when it can.
export function blocker(f: ByHandFile, p: Pick | null): string {
	if (f.in_library) return t('mediafiles.byHand.already');
	if (f.kind === 'audio' && (!p || p.title_kind === 'artist')) {
		if (!p || !p.slot_id) return musicWhy(f, p);
		if (p.has_file && !p.replace) return t('mediafiles.byHand.hasFileReplace');
		return '';
	}
	if (!p) return t('mediafiles.byHand.pickTitle');
	if (p.title_kind === 'show' && (p.season === undefined || p.episode === undefined)) return t('mediafiles.byHand.pickSeasonEpisode');
	if (p.has_file && !p.replace) return t('mediafiles.byHand.hasFileReplace');
	return '';
}

// Why a music file has no full match yet, from the scan's reason while the pick is still the guess.
function musicWhy(f: ByHandFile, p: Pick | null): string {
	const g = f.guess;
	const asGuessed = !p || (p.title_id === g.title_id && p.album_id === g.album_id);
	const name = g.detail ?? '';
	if (asGuessed && g.reason === 'no_facts') return t('mediafiles.byHand.music.noFacts');
	if (!p) return name && g.reason === 'no_artist' ? t('mediafiles.byHand.music.noArtist', { name }) : t('mediafiles.byHand.music.pickAll');
	if (!p.album_id) {
		if (asGuessed && name && g.reason === 'no_album') return t('mediafiles.byHand.music.noAlbum', { artist: p.title_name, name });
		if (asGuessed && name && g.reason === 'two_album') return t('mediafiles.byHand.music.twoAlbums', { artist: p.title_name, name });
		return t('mediafiles.byHand.music.pickAlbumTrack');
	}
	if (asGuessed && g.reason === 'no_track') return t('mediafiles.byHand.music.noTrack', { album: p.album_name ?? '' });
	return t('mediafiles.byHand.music.pickTrack');
}

// Only confident guesses start ticked.
export function startTicked(f: ByHandFile, p: Pick | null): boolean {
	return !blocker(f, p) && !f.note && f.guess.confidence >= 0.7;
}

// When several ticked files go to the same track, keeps only the surest one ticked
// (then the bigger file) and returns the paths it unticked.
export function oneTickPerTrack(files: ByHandFile[], picks: Record<string, Pick | null>, ticked: Record<string, boolean>): Record<string, boolean> {
	const best = new Map<string, ByHandFile>();
	const held: Record<string, boolean> = {};
	for (const f of files) {
		const p = picks[f.path];
		if (!ticked[f.path] || p?.title_kind !== 'artist' || !p.slot_id) continue;
		const key = `${p.title_id}:${p.slot_id}`;
		const was = best.get(key);
		if (!was) {
			best.set(key, f);
			continue;
		}
		const better = f.guess.confidence > was.guess.confidence || (f.guess.confidence === was.guess.confidence && f.size > was.size);
		const lose = better ? was : f;
		if (better) best.set(key, f);
		ticked[lose.path] = false;
		held[lose.path] = true;
	}
	return held;
}

// Builds the import request from ticked files that have a valid pick.
// No mode is sent: files are placed like automatic imports (a link when possible, else a real copy).
export function itemsFor(files: ByHandFile[], picks: Record<string, Pick | null>, ticked: Record<string, boolean>, mode: ImportMode = 'copy') {
	const out: { path: string; title_id: number; slot_id?: number; version_id?: number; season?: number; episode?: number; replace?: boolean; mode: ImportMode }[] = [];
	for (const f of files) {
		const p = picks[f.path] ?? null;
		if (!ticked[f.path] || !p || blocker(f, p)) continue;
		const item: (typeof out)[number] = { path: f.path, title_id: p.title_id, mode };
		if (p.title_kind === 'artist') item.slot_id = p.slot_id;
		else if (p.version_id) item.version_id = p.version_id;
		if (p.has_file && p.replace) item.replace = true;
		if (p.title_kind === 'show') {
			item.season = p.season;
			item.episode = p.episode;
		}
		out.push(item);
	}
	return out;
}

// Plain words for what happened to one file.
export function resultLine(r: ByHandResult): string {
	if (r.outcome === 'placed') {
		const how = r.placed_by === 'move' ? t('mediafiles.byHand.moved') : r.placed_by === 'copy' ? t('mediafiles.byHand.copied') : r.placed_by === 'hardlink' ? t('mediafiles.byHand.linked') : t('mediafiles.byHand.recorded');
		return r.error ? `${how}. ${serverWords(r.error)}` : how;
	}
	return t('mediafiles.byHand.outcome', { outcome: r.outcome === 'failed' ? t('mediafiles.byHand.failed') : t('mediafiles.byHand.notImported'), why: r.error ? serverWords(r.error) : t('mediafiles.byHand.noReason') });
}

// How Import places the files: copy keeps the original (a link when the
// drive allows it), move removes it once the Library file is checked.
export type ImportMode = 'copy' | 'move';

// One step of the folder picker.
export type FolderList = { path: string; parent: string; folders: { name: string; path: string }[]; files: number; more: boolean };
