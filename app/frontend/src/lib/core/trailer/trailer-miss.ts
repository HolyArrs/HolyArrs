// Plain-words line shown by every player when a trailer will not play.

// Reasons where the whole service is blocked, not this one video.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
const DOOR_REASONS = new Set(['bot_check', 'no_address', 'refused', 'network']);

// why is the server's reason, if any; word names what was meant to play.
export function missLine(why: string | undefined, word = t('core.trailerMiss.trailer')): string {
	switch (why) {
		case 'age_restricted':
			return t('core.trailerMiss.ageRestricted', { word });
		case 'unavailable':
			return t('core.trailerMiss.unavailable', { word });
		case 'sign_in':
			return t('core.trailerMiss.signIn', { word });
		case 'unplayable':
			return t('core.trailerMiss.unplayable', { word });
		case 'content_check':
			return t('core.trailerMiss.contentCheck', { word });
	}
	if (why && DOOR_REASONS.has(why)) return t('core.trailerMiss.unreachable');
	return t('core.trailerMiss.cantPlay', { word });
}
