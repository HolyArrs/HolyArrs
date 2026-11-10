// Types, filters and display helpers for the library copies list.
import { lang, t } from '../core/i18n/i18n.svelte.ts';

export const copyCodecs = ['all', 'unknown', 'h264', 'hevc', 'av1', 'vp9', 'mpeg2', 'xvid', 'vc1'] as const;
export const copySorts = ['title', 'codec', 'added'] as const;
export type CopyCodec = (typeof copyCodecs)[number];
export type CopySort = (typeof copySorts)[number];

export interface CopySelection {
	file_id: number;
	version_id: number;
}

export interface LibraryCopy extends CopySelection {
	title_id: number;
	title: string;
	year?: number;
	version: string;
	edition: string;
	version_audio_languages: string[];
	relative_path: string;
	protected: boolean;
	codec: Exclude<CopyCodec, 'all'>;
	inspection_status: string;
	container: string;
	audio_languages: string[];
	audio_tracks: number;
	audio_unknown: boolean;
	added_at: string;
	checked_at?: string;
}

export interface LibraryCopyPage {
	copies: LibraryCopy[];
	total: number;
	limit: number;
	offset: number;
	codecs: Record<string, number>;
}

export interface CopySearchPreview {
	copies: LibraryCopy[];
	slot_count: number;
	review_token: string;
}

export interface CopySearchResultItem extends CopySelection {
	slot_ids: number[];
	job_ids: number[];
}

export interface CopySearchResult {
	items: CopySearchResultItem[];
	job_ids: number[];
}

export interface CopyFilters {
	q: string;
	codec: CopyCodec;
	sort: CopySort;
	limit: number;
	offset: number;
}

// Server page size is capped at 100.
const maxPageSize = 100;

// Language names follow the app language; one formatter is kept per language.
const displayNames = new Map<string, Intl.DisplayNames | null>();
function languageNames(): Intl.DisplayNames | null {
	const code = lang.value;
	if (!displayNames.has(code)) displayNames.set(code, typeof Intl !== 'undefined' && 'DisplayNames' in Intl ? new Intl.DisplayNames([code], { type: 'language' }) : null);
	return displayNames.get(code) ?? null;
}

// A copy is one file in one version, so both ids make the key.
export function copyKey(copy: CopySelection): string {
	return `${copy.file_id}:${copy.version_id}`;
}

export function toggleCopy(selected: CopySelection[], copy: CopySelection): CopySelection[] {
	const key = copyKey(copy);
	return selected.some((item) => copyKey(item) === key) ? selected.filter((item) => copyKey(item) !== key) : [...selected, copy];
}

export function copiesQuery(filters: CopyFilters): string {
	const params = new URLSearchParams();
	if (filters.q.trim()) params.set('q', filters.q.trim());
	if (filters.codec !== 'all') params.set('codec', filters.codec);
	params.set('sort', filters.sort);
	params.set('limit', String(Math.min(maxPageSize, Math.max(1, filters.limit))));
	params.set('offset', String(Math.max(0, filters.offset)));
	return `/library/copies?${params.toString()}`;
}

// Falls back to the raw code when the code is not a valid language tag.
export function languageName(code: string): string {
	try {
		return languageNames()?.of(code) ?? code;
	} catch {
		return code;
	}
}

// Unlisted codecs show in upper case.
export function codecLabel(codec: string): string {
	const labels: Record<string, string> = { unknown: t('mediafiles.copies.unknown'), h264: 'H.264', hevc: 'HEVC', av1: 'AV1', vp9: 'VP9', mpeg2: 'MPEG-2', xvid: 'Xvid', vc1: 'VC-1' };
	return labels[codec] ?? codec.toUpperCase();
}
