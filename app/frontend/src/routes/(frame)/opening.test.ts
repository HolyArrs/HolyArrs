import { afterEach, describe, expect, it, vi } from 'vitest';
import { session } from '#lib/core/session.svelte.ts';
import { api } from '#lib/core/api.ts';
import { openedAtRoot, openingAfterSignIn, openingHere, openingPath, type OpeningInput } from './opening.ts';

/** Signs someone in as GET /auth/me describes them. */
async function signIn(me: Record<string, unknown>) {
	vi.spyOn(api, 'get').mockResolvedValue({ id: 2, name: 'ana', csrf: 'x', role: 'member', views: { 'welcome.seen': '1' }, ...me });
	await session.load();
}

describe('where the app opens', () => {
	const who: OpeningInput = { opensOn: 'explore', role: 'member', setupOpen: false, setupSkipped: false, mustTwoFactor: false, seen: '1', exploreOff: false };
	it.each<[string, OpeningInput, string]>([
		['someone who picked Explore', who, '/explore'],
		['someone who kept Home', { ...who, opensOn: 'home' }, '/'],
		['the owner who picked Explore', { ...who, role: 'owner', seen: undefined }, '/explore'],
		['a member with the welcome still to see', { ...who, seen: undefined }, '/'],
		['an admin with the welcome still to see', { ...who, role: 'admin', seen: undefined }, '/'],
		['the owner still in first-run setup', { ...who, role: 'owner', setupOpen: true }, '/'],
		['the owner with the Finish setup card', { ...who, role: 'owner', setupSkipped: true }, '/'],
		['someone who must turn on two-factor first', { ...who, mustTwoFactor: true }, '/'],
		['someone who may not see Explore', { ...who, exploreOff: true }, '/']
	])('%s', (_n, input, want) => {
		expect(openingPath(input)).toBe(want);
	});
});

describe('opening the app', () => {
	it('counts only the bare root, never a link to a page', () => {
		expect(openedAtRoot(new URL('http://h/'))).toBe(true);
		expect(openedAtRoot(new URL('http://h/explore'))).toBe(false);
		expect(openedAtRoot(new URL('http://h/title/movie/603'))).toBe(false);
		expect(openedAtRoot(new URL('http://h/?tab=system'))).toBe(false);
		expect(openedAtRoot(new URL('http://h/#downloads'))).toBe(false);
	});
});

describe('the opening page for whoever is signed in', () => {
	afterEach(() => vi.restoreAllMocks());
	it('is Home until Explore is picked', async () => {
		await signIn({});
		expect(openingHere()).toBe('/');
		await signIn({ opens_on: 'explore' });
		expect(openingHere()).toBe('/explore');
		await signIn({ opens_on: 'something else' });
		expect(openingHere()).toBe('/');
	});
	it('stays Home while the welcome waits there', async () => {
		await signIn({ opens_on: 'explore', views: {} });
		expect(openingHere()).toBe('/');
	});
	it('stays Home while the owner has setup to finish', async () => {
		await signIn({ opens_on: 'explore', role: 'owner', setup_skipped: true });
		expect(openingHere()).toBe('/');
		await signIn({ opens_on: 'explore', role: 'owner', setup_open: true });
		expect(openingHere()).toBe('/');
		await signIn({ opens_on: 'explore', role: 'owner' });
		expect(openingHere()).toBe('/explore');
	});
	it('after sign-in, goes to the opening page only when no page was asked for', async () => {
		await signIn({ opens_on: 'explore' });
		expect(openingAfterSignIn('/')).toBe('/explore');
		expect(openingAfterSignIn('/library?kind=movie')).toBe('/library?kind=movie');
		expect(openingAfterSignIn('/?tab=system')).toBe('/?tab=system');
		await signIn({});
		expect(openingAfterSignIn('/')).toBe('/');
	});
});
