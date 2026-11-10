// Why a Title's newest search came back empty. Owners and admins get the Sources that were paused or failed; everyone else only "No results yet".
import { t } from '#lib/core/i18n/i18n.svelte.ts';

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

// Pause reasons from the server, with the line each one gets.
const reasonKey: Record<string, string> = { rate_limited: 'slowed', down: 'down', blocked: 'blocked', login_refused: 'keyRefused', broken_definition: 'broken', bot_check: 'botCheck', site_ban: 'siteBan' };

/** Why one Source gave nothing, in lower case. */
export function whyWords(why: string): string {
	// A site's bot check or ban: HolyArrs cannot pass either.
	return t(`requests.outcome.why.${reasonKey[why] ?? 'paused'}`);
}

/** The text before the Source links, or '' when there is nothing to say. */
export function outcomeLine(o: SearchOutcome | null | undefined): string {
	if (!o?.empty) return '';
	return o.sources?.length ? t('requests.outcome.nothingFound') : t('requests.outcome.noResults');
}
