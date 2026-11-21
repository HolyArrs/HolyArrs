import { session } from '#lib/core/session.svelte.ts';
import { to } from '#lib/core/paths.ts';

// saveHref is the link that saves a movie or episode file to the person's
// device, or '' when there is no file, they may not save files, or the
// feature is held back.
export function saveHref(fileId: number | null | undefined): string {
	if (!fileId || !session.can('save_files') || session.held('youtube') || session.s.maxRating) return '';
	return to(`/api/v1/files/${fileId}/download`);
}
