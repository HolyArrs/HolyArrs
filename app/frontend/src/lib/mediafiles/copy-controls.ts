// Helpers for reading copy details and building edition and audio criteria text.

import type { CopyFile, VersionView } from '../core/types.ts';
import { t } from '../core/i18n/i18n.svelte.ts';

export type CopyControlFile = Pick<CopyFile, 'ID' | 'VersionID' | 'RelativePath' | 'Protected' | 'MediaInfo' | 'State'>;
export type CopyControlVersion = Pick<VersionView, 'edition' | 'audio_languages'>;

// Media info is untyped JSON, so keep only clean, unique lower case codes.
export function inspectedAudio(file: CopyControlFile): string[] {
	const raw = file.MediaInfo?.audio_languages;
	if (!Array.isArray(raw)) return [];
	const seen = new Set<string>();
	for (const value of raw) {
		if (typeof value === 'string' && value.trim()) seen.add(value.trim().toLowerCase());
	}
	return [...seen];
}

export function copyEdition(file: CopyControlFile): string {
	const value = file.MediaInfo?.edition;
	return typeof value === 'string' ? value : '';
}

export function languageList(value: string): string[] {
	return [...new Set(value.split(',').map((part) => part.trim().toLowerCase()).filter(Boolean))];
}

export function versionCriteria(version: CopyControlVersion): [string, string] {
	const edition = version.edition?.trim();
	const audio = version.audio_languages?.filter(Boolean) ?? [];
	return [edition ? t('mediafiles.criteria.edition', { edition }) : t('mediafiles.criteria.anyEdition'), audio.length ? t('mediafiles.criteria.audio', { languages: audio.join(', ') }) : t('mediafiles.criteria.anyAudio')];
}

// Confirm and retry tell the server to apply the edition and search again.
export function editionDecisionBody(edition: string): { edition: string; confirm: true; retry: true } {
	return { edition: edition.trim(), confirm: true, retry: true };
}
