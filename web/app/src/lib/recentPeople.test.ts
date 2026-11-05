import { beforeEach, describe, expect, it } from 'vitest';
const store = new Map<string, string>();
(globalThis as { localStorage?: unknown }).localStorage = {
	getItem: (k: string) => store.get(k) ?? null,
	setItem: (k: string, v: string) => void store.set(k, v),
	clear: () => store.clear()
};

import { forgetPerson, recentPeople, rememberPerson } from './recentPeople.ts';

describe('recent people', () => {
	beforeEach(() => localStorage.clear());
	it('keeps the newest first, once each, at most six', () => {
		for (const n of ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'b']) rememberPerson(n);
		expect(recentPeople().map((p) => p.name)).toEqual(['b', 'g', 'f', 'e', 'd', 'c']);
	});
	it('keeps the way each person used, and the latest way wins', () => {
		rememberPerson('ana', 'plex');
		rememberPerson('bo', 'jellyfin', 'bo-jf');
		rememberPerson('ana', 'password');
		expect(recentPeople()).toEqual([
			{ name: 'ana', way: 'password' },
			{ name: 'bo', way: 'jellyfin', login: 'bo-jf' }
		]);
	});
	it('reads older lists of plain names as password people', () => {
		localStorage.setItem('holyarrs.recent-people', JSON.stringify(['ana', '', 'bo', { name: 'cy', way: 'odd' }]));
		expect(recentPeople()).toEqual([
			{ name: 'ana', way: 'password' },
			{ name: 'bo', way: 'password' },
			{ name: 'cy', way: 'password' }
		]);
	});
	it('forgets a name and survives bad storage', () => {
		rememberPerson('ana');
		forgetPerson('ana');
		expect(recentPeople()).toEqual([]);
		localStorage.setItem('holyarrs.recent-people', '{bad');
		expect(recentPeople()).toEqual([]);
	});
});
