import { ApiError } from '#lib/core/api.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

// The server's English line for a Library folder path it can't find.
const ABSENT = 'the folder does not exist';

/** For a "folder does not exist" answer: how to mount the typed folder into the container; null for any other error. */
export function folderMissing(e: unknown, typed: string, kind: string): string | null {
	if (!(e instanceof ApiError) || e.body?.error !== ABSENT) return null;
	const host = typed.trim().replace(/(.)\/+$/, '$1');
	const inside = kind === 'show' ? '/shows' : kind === 'music' ? '/music' : '/movies';
	return t('settings.libraryFolders.notMounted', { path: host, inside });
}
