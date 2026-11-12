import { describe, expect, it } from 'vitest';
import { friendsNote, friendLine, type PlexFriendsView } from './plex-friends.ts';

const base: PlexFriendsView = { on: true, owner_linked: true, people: [] };

describe('friendsNote', () => {
	it('asks the owner to link Plex first when no link is kept', () => {
		expect(friendsNote({ ...base, owner_linked: false }, true)).toBe('Link Plex in Settings › Sign-in first.');
	});
	it('says nobody is imported yet when the list is empty', () => {
		expect(friendsNote(base, true)).toBe('No one imported from Plex yet.');
	});
	it('says nothing while the switch is off or people are listed', () => {
		expect(friendsNote(base, false)).toBe('');
		expect(friendsNote({ ...base, people: [{ user_id: 1, name: 'A', plex_name: 'a', kid: false, asked: 0 }] }, true)).toBe('');
	});
});

describe('friendLine', () => {
	it('names the Plex account, the last read and the asked count', () => {
		const at = new Date(Date.now() - 2 * 3600 * 1000).toISOString();
		expect(friendLine({ user_id: 1, name: 'A', plex_name: 'ann', kid: false, asked: 3, checked_at: at })).toBe('Plex: ann · Watchlist checked 2 h ago · Requests: 3');
	});
	it('says not read yet before the first pass and skips an empty Plex name', () => {
		expect(friendLine({ user_id: 1, name: 'A', plex_name: '', kid: false, asked: 0 })).toBe('Watchlist not checked yet · Requests: 0');
	});
});
