// The line a player shows when a trailer will not play, in plain words:
// why, when the server said (the reader's reason from its 404), else the
// plain "can't play here". Every player that shows such a line uses this.

/** YouTube refusing HolyArrs as a whole, not this one video. */
const DOOR_REASONS = new Set(['bot_check', 'no_address', 'refused', 'network']);

/**
 * missLine is the line for a trailer that will not play. why is the
 * server's reason ("" or undefined when it gave none); word is what plays
 * ("trailer", "teaser", "music video").
 */
export function missLine(why: string | undefined, word = 'trailer'): string {
	switch (why) {
		case 'age_restricted':
			return `YouTube plays this ${word} only for signed-in adults, so it can't play here.`;
		case 'unavailable':
			return `This ${word} was removed from YouTube.`;
		case 'sign_in':
			return `This ${word} is private or for members only on YouTube.`;
		case 'unplayable':
			return `YouTube won't play this ${word} here.`;
		case 'content_check':
			return `YouTube shows this ${word} only after a content warning.`;
	}
	if (why && DOOR_REASONS.has(why)) return "Trailers can't be reached right now.";
	return `This ${word} can't play here.`;
}
