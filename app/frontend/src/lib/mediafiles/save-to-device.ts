import { session } from '#lib/core/session.svelte.ts';
import { to } from '#lib/core/paths.ts';

/** The link that saves a Library file to this device as it is, or '' when there is no file or the person may not save files (a Kid never may). */
export function saveHref(titleId: number | null | undefined, fileId: number | null | undefined): string {
	if (!titleId || !fileId || !session.can('save_files') || session.s.maxRating) return '';
	return to(`/api/v1/titles/${titleId}/files/${fileId}/save`);
}
