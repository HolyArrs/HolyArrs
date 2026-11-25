// Why a Title's newest search came back empty. Owners and admins get the Sources that were paused or failed; everyone else only "No results yet".
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { listItem, serverWords } from '#lib/core/i18n/server-words.ts';

export interface SkippedSource {
	id: number;
	name: string;
	why: string; // Pause reason: rate_limited, down, blocked, login_refused, broken_definition, paused; or bot_check, site_ban.
}

export interface SearchOutcome {
	empty: boolean;
	at?: string;
	sources?: SkippedSource[]; // Only for people who manage Settings.
	note?: string; // What the search said about the Sources it asked; the same people only.
	refused?: boolean; // Releases were found but the Pick rules turned every one down; shown to everyone.
}

export const MEMBER_LINE = 'No results yet';

// Pause reasons from the server, with the line each one gets.
const reasonKey: Record<string, string> = { rate_limited: 'slowed', down: 'down', blocked: 'blocked', login_refused: 'keyRefused', broken_definition: 'broken', bot_check: 'botCheck', site_ban: 'siteBan' };

/** Why one Source gave nothing, in lower case. */
export function whyWords(why: string): string {
	// A site's bot check or ban: HolyArrs cannot pass either.
	// Left out for its tags, not paused: the stored line says it, without the Source's name (the link before it has that).
	if (/^(its tags \(|it has no tags)/.test(why)) return listItem(`\u00a7: ${why}`).replace(/^\u00a7\s*[:\uff1a]\s*/, '');
	return t(`requests.outcome.why.${reasonKey[why] ?? 'paused'}`);
}

/** The text before the Source links, or '' when there is nothing to say. */
export function outcomeLine(o: SearchOutcome | null | undefined): string {
	if (!o?.empty) return '';
	if (o.refused) return t('requests.outcome.refused');
	return o.sources?.length ? t('requests.outcome.nothingFound') : t('requests.outcome.noResults');
}

/** What the search said about the Sources it asked, in the app's language; '' for nothing. */
export function outcomeNote(o: SearchOutcome | null | undefined): string {
	return o?.empty && o.note ? serverWords(o.note) : '';
}
