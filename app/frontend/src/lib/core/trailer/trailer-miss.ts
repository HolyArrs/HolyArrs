// Plain-words line shown by every player when a trailer will not play.

// Reasons where the whole service is blocked, not this one video.
const DOOR_REASONS = new Set(['bot_check', 'no_address', 'refused', 'network']);

// why is the server's reason, if any; word names what was meant to play.
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
