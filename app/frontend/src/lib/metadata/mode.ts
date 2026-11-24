// Settings > Metadata: the answer of /settings/metadata and the own-keys form.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type Mode = 'full' | 'own' | 'off';
export type KeyName = 'tmdb_token' | 'tvdb_key' | 'tvdb_pin' | 'mdblist_key' | 'fanart_key' | 'omdb_key';
export const KEY_NAMES: KeyName[] = ['tmdb_token', 'tvdb_key', 'tvdb_pin', 'mdblist_key', 'fanart_key', 'omdb_key'];

// The server's stand-in for a saved key; sending it back keeps the key.
export const MASK = '••••••••';

export interface MetadataAnswer {
	mode: Mode;
	locked: boolean;
	locked_note?: string;
	docker_value?: string;
	keys: Partial<Record<string, boolean>>;
	tmdb_key_from_docker: boolean;
	sources: { id: string; name: string; state: 'on' | 'off' | 'needs_key'; key?: string }[];
	hosts: string[];
	note?: string;
	own_keys_note: string;
}

// A saved key shows as the mask; an empty field means none.
export function keyForm(saved: Partial<Record<string, boolean>>): Record<KeyName, string> {
	return Object.fromEntries(KEY_NAMES.map((k) => [k, saved[k] ? MASK : ''])) as Record<KeyName, string>;
}

// Every key goes back: the mask keeps it, an empty value removes it, anything else replaces it.
export function keyBody(form: Record<KeyName, string>): Record<KeyName, string> {
	return Object.fromEntries(KEY_NAMES.map((k) => [k, (form[k] ?? '').trim()])) as Record<KeyName, string>;
}

// The test answer in the person's language; an unknown line keeps the server's words.
// Where each own key comes from; the page links there ("Get a key").
export const KEY_LINKS: Partial<Record<KeyName, string>> = Object.fromEntries([
	['tmdb_token', 'https://www.themoviedb.org/settings/api'],
	['tvdb_key', 'https://thetvdb.com/dashboard/account/apikey'],
	['mdblist_key', 'https://mdblist.com/preferences/'],
	['fanart_key', 'https://fanart.tv/get-an-api-key/']
]);

// A Test answer in the person's language: each code says what to fix.
// An unknown code keeps the server's words.
export function testWords(msg: string, code?: string): string {
	switch (code) {
		case 'works': return t('settings.tmdb.testWorks');
		case 'no_key': return t('settings.tmdb.testNoKey');
		case 'refused': return t('settings.tmdb.testRefused');
		case 'tvdb_refused': return t('settings.tmdb.testTvdbRefused');
		case 'tvdb_pin_refused': return t('settings.tmdb.testPinRefused');
		case 'unreachable': return t('settings.tmdb.testUnreachable');
	}
	switch (msg) {
		case 'Key works': return t('settings.tmdb.testWorks');
		case 'Key refused': return t('settings.tmdb.testRefused');
		case 'Key or PIN refused': return t('settings.tmdb.testPinRefused');
		case 'No key saved yet': case 'Enter a key first': return t('settings.tmdb.testNoKey');
		case 'Could not reach the service; try again': return t('settings.tmdb.testUnreachable');
		default: return msg;
	}
}
