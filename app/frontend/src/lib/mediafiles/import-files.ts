// Types and rules for importing files by hand into the Library.
import { episodeCode as code } from '../core/episodes.ts';
import { t } from '../core/i18n/i18n.svelte.ts';

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
}

export type Mode = 'copy' | 'move';

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
		has_file: g.has_file
	};
}

export function episodeCode(season?: number, episode?: number): string {
	if (season === undefined || episode === undefined) return '';
	return code(season, episode);
}

export function pickLine(p: Pick | null): string {
	if (!p) return t('mediafiles.byHand.noMatch');
	const name = `${p.title_name}${p.year ? ` (${p.year})` : ''}`;
	if (p.title_kind !== 'show') return name;
	return `${name} · ${episodeCode(p.season, p.episode) || t('mediafiles.byHand.pickEpisode')}`;
}

// Returns why a file cannot be imported, or empty when it can.
export function blocker(f: ByHandFile, p: Pick | null): string {
	if (f.kind !== 'video') return f.note || t('mediafiles.byHand.noMusic');
	if (f.in_library) return t('mediafiles.byHand.already');
	if (!p) return t('mediafiles.byHand.pickTitle');
	if (p.title_kind === 'show' && (p.season === undefined || p.episode === undefined)) return t('mediafiles.byHand.pickSeasonEpisode');
	if (p.has_file) return t('mediafiles.byHand.hasFile');
	return '';
}

// Only confident guesses start ticked.
export function startTicked(f: ByHandFile, p: Pick | null): boolean {
	return !blocker(f, p) && !f.note && f.guess.confidence >= 0.7;
}

// Builds the import request from ticked files that have a valid pick.
export function itemsFor(files: ByHandFile[], picks: Record<string, Pick | null>, ticked: Record<string, boolean>, mode: Mode) {
	const out: { path: string; title_id: number; version_id?: number; season?: number; episode?: number; mode: Mode }[] = [];
	for (const f of files) {
		const p = picks[f.path] ?? null;
		if (!ticked[f.path] || !p || blocker(f, p)) continue;
		const item: (typeof out)[number] = { path: f.path, title_id: p.title_id, mode };
		if (p.version_id) item.version_id = p.version_id;
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
		return r.error ? `${how}. ${r.error}` : how;
	}
	return t('mediafiles.byHand.outcome', { outcome: r.outcome === 'failed' ? t('mediafiles.byHand.failed') : t('mediafiles.byHand.notImported'), why: r.error || t('mediafiles.byHand.noReason') });
}
