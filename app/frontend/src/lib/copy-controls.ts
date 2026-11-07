import type { CopyFile, VersionView } from './core/types.ts';

export type CopyControlFile = Pick<CopyFile, 'ID' | 'VersionID' | 'RelativePath' | 'Protected' | 'MediaInfo' | 'State'>;
export type CopyControlVersion = Pick<VersionView, 'edition' | 'audio_languages'>;

/** inspectedAudio reports only audio found by media inspection. */
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

/** languageList turns the comma-separated Version criterion into stable codes. */
export function languageList(value: string): string[] {
	return [...new Set(value.split(',').map((part) => part.trim().toLowerCase()).filter(Boolean))];
}

/** versionCriteria describes what a Version asks for, separately from file facts. */
export function versionCriteria(version: CopyControlVersion): [string, string] {
	const edition = version.edition?.trim();
	const audio = version.audio_languages?.filter(Boolean) ?? [];
	return [edition ? `Edition: ${edition}` : 'Any edition', audio.length ? `Required audio: ${audio.join(', ')}` : 'Any audio language'];
}

export function editionDecisionBody(edition: string): { edition: string; confirm: true; retry: true } {
	return { edition: edition.trim(), confirm: true, retry: true };
}
