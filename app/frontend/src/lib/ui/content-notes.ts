// Content notes: what a title shows that some people want to know before they ask for it.
// The server sends note keys read from the title's TMDB keywords; this turns them into words.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export const CONTENT_NOTES = {
	nudity: () => t('ui.contentNotes.nudity'),
	sex: () => t('ui.contentNotes.sex'),
	sexual_violence: () => t('ui.contentNotes.sexualViolence'),
	gore: () => t('ui.contentNotes.gore'),
	animal_death: () => t('ui.contentNotes.animalDeath'),
	suicide: () => t('ui.contentNotes.suicide'),
	drugs: () => t('ui.contentNotes.drugs'),
	child_abuse: () => t('ui.contentNotes.childAbuse'),
	domestic_abuse: () => t('ui.contentNotes.domesticAbuse')
} as const;

type NoteKey = keyof typeof CONTENT_NOTES;
const isNote = (v: unknown): v is NoteKey => typeof v === 'string' && Object.hasOwn(CONTENT_NOTES, v);

/** The words for the known notes in v, each once, in the order sent; unknown entries are dropped. */
export function contentNotes(v: unknown): string[] {
	if (!Array.isArray(v)) return [];
	return [...new Set(v.filter(isNote))].map((k) => CONTENT_NOTES[k]());
}
