// Include Plex friends: the people Request rules lists under the switch, in plain words.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { ago } from '#lib/core/format.ts';

export type PlexFriend = {
	user_id: number;
	name: string;
	plex_name: string;
	kid: boolean;
	checked_at?: string;
	error?: string;
	asked: number;
};

export type PlexFriendsView = { on: boolean; owner_linked: boolean; people: PlexFriend[] };

/** The note under the switch: link Plex first, or nobody imported yet. Empty when the list says enough. */
export function friendsNote(v: PlexFriendsView | null, on: boolean): string {
	if (!v || !on) return '';
	if (!v.owner_linked) return t('settings.requests.plexFriendsNoLink');
	return v.people.length ? '' : t('settings.requests.plexFriendsNone');
}

/** One friend's second line: Plex name, last read and how many titles it asked for. */
export function friendLine(f: PlexFriend, now = Date.now()): string {
	const read = f.checked_at ? t('settings.requests.plexFriendsRead', { when: ago(f.checked_at, now) }) : t('settings.requests.plexFriendsNotYet');
	return [f.plex_name ? t('accounts.account.plexName', { name: f.plex_name }) : '', read, t('settings.requests.plexFriendsAsked', { n: f.asked })].filter(Boolean).join(' · ');
}
