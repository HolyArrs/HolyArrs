// Why a Title's newest search came back empty. Owners and admins get the Sources that were paused or failed; everyone else only "No results yet".
import { reasonWords } from '../sources/index.ts';

export interface SkippedSource {
	id: number;
	name: string;
	why: string; // Pause reason: rate_limited, down, blocked, login_refused, broken_definition, paused; or bot_check, site_ban.
}

export interface SearchOutcome {
	empty: boolean;
	at?: string;
	sources?: SkippedSource[]; // Only for people who manage Settings.
}

export const MEMBER_LINE = 'No results yet';

// Pause reasons from the server, in the reason words of source-health.ts.
const reasonOf: Record<string, string> = { rate_limited: 'slowed', down: 'down', blocked: 'blocked', login_refused: 'key' };

const fallback: Record<string, string> = { slowed: 'Slowed down', down: 'Down', blocked: 'Blocked', key: 'Key refused' };

/** Why one Source gave nothing, in lower case. */
export function whyWords(why: string): string {
	const r = reasonOf[why];
	if (r === 'slowed') return 'was paused (slowed down)';
	if (r) return 'was ' + ((reasonWords as Record<string, string>)?.[r] ?? fallback[r]).toLowerCase();
	if (why === 'broken_definition') return 'is broken';
	// A site's bot check or ban: HolyArrs cannot pass either.
	if (why === 'bot_check') return "was blocked by the site's bot check";
	if (why === 'site_ban') return "refuses HolyArrs's address";
	return 'was paused';
}

/** The text before the Source links, or '' when there is nothing to say. */
export function outcomeLine(o: SearchOutcome | null | undefined): string {
	if (!o?.empty) return '';
	return o.sources?.length ? 'Nothing found:' : MEMBER_LINE;
}
