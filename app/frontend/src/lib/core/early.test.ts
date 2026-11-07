import { readFileSync } from 'node:fs';
import { describe, expect, it, vi } from 'vitest';
import { createClient, EARLY_MAX, type EarlyAsks } from './api.ts';
import { headPath } from '../discover/explore-server.ts';
import { readServices, serviceRegion } from '../discover/explore-services.ts';
import { fetchMusicExplore } from '../music/music-trending.ts';
import { musicPrefetchURLs } from '../music/music-prefetch.ts';
import { firstPageURL } from '../discover/person-prefetch.ts';
import { loadDetails } from '../metadata/title-details.ts';

const html = readFileSync(new URL('../../app.html', import.meta.url), 'utf8');
const script = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map((m) => m[1]).find((s) => s.includes('holyarrs-early'));
if (!script) throw new Error('app.html has no holyarrs-early script');

interface Page {
	url: string;
	icon?: string;
	stored?: Record<string, string>;
	langs?: string[];
	tz?: string;
}

function boot({ url, icon = '/brand/favicon.svg', stored = {}, langs = ['en-US'], tz = 'Europe/Berlin' }: Page) {
	const u = new URL(url, 'http://h');
	const win: { __holyarrsEarly?: EarlyAsks } = {};
	const fetch = vi.fn((_u: string, _o?: RequestInit) => Promise.resolve(new Response('{}')));
	const document = { querySelector: (s: string) => (s === 'link[rel=icon]' ? { getAttribute: () => icon } : null) };
	const localStorage = { getItem: (k: string) => stored[k] ?? null };
	const Intl = { DateTimeFormat: () => ({ resolvedOptions: () => ({ timeZone: tz }) }) };
	new Function('window', 'location', 'document', 'localStorage', 'navigator', 'fetch', 'Intl', script!)(win, { pathname: u.pathname, search: u.search }, document, localStorage, { languages: langs }, fetch, Intl);
	return { asked: fetch.mock.calls.map((c) => c[0]), early: win.__holyarrsEarly, init: fetch.mock.calls[0]?.[1] };
}

const A = '9fff2f8a-21e6-47de-a2b8-7f449929d43f';
const B = 'f9133869-f87c-459c-9aa7-2f176cda7e06';

describe('app.html boot requests', () => {
	it('asks who is signed in and Home on /', () => {
		const { asked, init } = boot({ url: '/' });
		expect(asked).toEqual(['/api/v1/auth/me', '/api/v1/home']);
		expect(init).toEqual({ headers: { Accept: 'application/json' }, credentials: 'same-origin' });
	});

	it('asks an artist page exactly as the page does', () => {
		expect(boot({ url: `/music/artist/${A.toUpperCase()}` }).asked).toEqual(['/api/v1/auth/me', ...musicPrefetchURLs(`/music/artist/${A}`).map((p) => '/api/v1' + p)]);
	});

	it('asks an album page exactly as the page does', () => {
		expect(boot({ url: `/music/album/${B}?artist=${A.toUpperCase()}` }).asked).toEqual([
			'/api/v1/auth/me',
			...musicPrefetchURLs(`/music/album/${B}?artist=${A}`).map((p) => '/api/v1' + p)
		]);
		expect(boot({ url: `/music/album/${B}` }).asked).toEqual(['/api/v1/auth/me', `/api/v1/music/albums/${B}?artist=&similar=0`]);
	});

	it("asks Explore's head with the saved services, as headPath does", () => {
		const stored = { 'holyarrs.explore.services': JSON.stringify({ DE: [8, 9, -1, 'x'] }) };
		const region = serviceRegion('DE');
		const want = headPath({ type: 'shows', services: readServices(region, stored['holyarrs.explore.services']) ?? [], region, tz: 'Europe/Berlin' });
		expect(boot({ url: '/explore?type=shows', stored, langs: ['de-DE'] }).asked).toEqual(['/api/v1/auth/me', '/api/v1' + want]);
		expect(boot({ url: '/explore', langs: ['en'] }).asked[1]).toBe('/api/v1' + headPath({ type: 'all', services: [], region: 'US', tz: 'Europe/Berlin' }));
	});

	it('asks the music rows on Explore Music, from the address or the saved type', async () => {
		let want = '';
		await fetchMusicExplore('', 'GB', { get: async (p: string) => ((want = p), {}) as never });
		expect(boot({ url: '/explore?type=music', langs: ['en-GB'] }).asked).toEqual(['/api/v1/auth/me', '/api/v1' + want, '/api/v1/explore/music/rising']);
		const saved = boot({ url: '/explore', stored: { 'holyarrs.explore.type': 'music', 'holyarrs.music-trending': 'GB' } }).asked;
		expect(saved[1]).toBe('/api/v1' + want);
		expect(boot({ url: '/explore?type=movies', stored: { 'holyarrs.explore.type': 'music' } }).asked[1]).toMatch(/^\/api\/v1\/explore\/head\?type=movies&/);
	});

	it('keeps the URL base the server put before /brand/', () => {
		expect(boot({ url: '/holyarrs/music/artist/' + A, icon: '/holyarrs/brand/favicon.svg' }).asked).toEqual([
			'/holyarrs/api/v1/auth/me',
			`/holyarrs/api/v1/music/artists/${A}?similar=0`
		]);
	});

	it('asks nothing on the sign-in pages and only who is signed in elsewhere', () => {
		expect(boot({ url: '/login' }).asked).toEqual([]);
		expect(boot({ url: '/invite/abc' }).asked).toEqual([]);
		expect(boot({ url: '/settings/schedules' }).asked).toEqual(['/api/v1/auth/me']);
		expect(boot({ url: '/music/artist/not-an-id' }).asked).toEqual(['/api/v1/auth/me']);
	});

	it('survives storage that throws', () => {
		const u = new URL('/explore', 'http://h');
		const win: { __holyarrsEarly?: EarlyAsks } = {};
		const fetch = vi.fn(() => Promise.resolve(new Response('{}')));
		const localStorage = {
			getItem: () => {
				throw new Error('blocked');
			}
		};
		new Function('window', 'location', 'document', 'localStorage', 'navigator', 'fetch', 'Intl', script!)(win, u, { querySelector: () => null }, localStorage, { languages: [] }, fetch, globalThis.Intl);
		expect(fetch).toHaveBeenCalledTimes(2);
	});

	it('asks a title page exactly as loadDetails does', async () => {
		const asked: string[] = [];
		const get = <T,>(p: string) => (asked.push(p), Promise.resolve({ movies: [], shows: [] } as T));
		await loadDetails(get, 'movie', '693134').catch(() => {});
		await loadDetails(get, 'show', '1396').catch(() => {});
		expect(boot({ url: '/title/movie/693134' }).asked).toEqual(['/api/v1/auth/me', ...asked.slice(0, 2).map((p) => '/api/v1' + p)]);
		expect(boot({ url: '/title/show/1396' }).asked).toEqual(['/api/v1/auth/me', ...asked.slice(2, 4).map((p) => '/api/v1' + p)]);
		expect(boot({ url: '/title/tv/1396' }).asked).toEqual(['/api/v1/auth/me']);
	});

	it('asks a person page as person-prefetch does, in the saved order', () => {
		expect(boot({ url: '/people/287' }).asked).toEqual(['/api/v1/auth/me', '/api/v1' + firstPageURL('287', 'oldest')]);
		expect(boot({ url: '/people/287', stored: { 'holyarrs.people.sort': 'popular' } }).asked).toEqual(['/api/v1/auth/me', '/api/v1' + firstPageURL('287', 'popular')]);
	});

	it('asks Library, a Library title, Requests and Downloads', () => {
		expect(boot({ url: '/library' }).asked).toEqual(['/api/v1/auth/me', '/api/v1/titles', '/api/v1/collections']);
		expect(boot({ url: '/library/294' }).asked).toEqual(['/api/v1/auth/me', '/api/v1/titles/294']);
		expect(boot({ url: '/library/held' }).asked).toEqual(['/api/v1/auth/me']);
		expect(boot({ url: '/requests' }).asked).toEqual(['/api/v1/auth/me', '/api/v1/asks']);
		expect(boot({ url: '/downloads' }).asked).toEqual(['/api/v1/auth/me', '/api/v1/downloads/view?limit=100']);
	});
});

function reply(status: number, body: unknown = {}) {
	return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function withEarly(asks: Record<string, unknown>, at = Date.now()) {
	const early: EarlyAsks = { at, asks: Object.fromEntries(Object.entries(asks).map(([k, v]) => [k, Promise.resolve(v as Response)])) };
	const f = vi.fn(async (_u: string, _o?: RequestInit) => reply(200, { from: 'network' }));
	return { early, f, c: createClient(f, () => early) };
}

describe('api client takes boot requests', () => {
	it('uses a boot answer once, for the same address', async () => {
		const { f, c, early } = withEarly({ '/api/v1/home': reply(200, { from: 'boot' }) });
		expect(await c.get('/home')).toEqual({ from: 'boot' });
		expect(f).not.toHaveBeenCalled();
		expect(early.asks).toEqual({});
		c.forget();
		expect(await c.get('/home')).toEqual({ from: 'network' });
		expect(await c.get('/home?x=1')).toEqual({ from: 'network' });
	});

	it('never uses an old boot answer', async () => {
		const { f, c } = withEarly({ '/api/v1/home': reply(200, { from: 'boot' }) }, Date.now() - EARLY_MAX - 1);
		expect(await c.get('/home')).toEqual({ from: 'network' });
		expect(f).toHaveBeenCalledTimes(1);
	});

	it('drops every boot answer on a change, such as signing in', async () => {
		const { c } = withEarly({ '/api/v1/home': reply(200, { from: 'boot' }) });
		await c.post('/auth/login', {});
		expect(await c.get('/home')).toEqual({ from: 'network' });
	});

	it('drops every boot answer on a 401 (signed out), even a quiet one', async () => {
		const { c } = withEarly({ '/api/v1/auth/me': reply(401, { error: 'sign in first' }), '/api/v1/home': reply(200, { from: 'boot' }) });
		await expect(c.get('/auth/me')).rejects.toMatchObject({ status: 401 });
		expect(await c.get('/home')).toEqual({ from: 'network' });
	});

	it('drops them for another session, not for the first one', async () => {
		const { c } = withEarly({ '/api/v1/a': reply(200, { from: 'boot' }), '/api/v1/b': reply(200, { from: 'boot' }) });
		c.setCsrf('t1');
		expect(await c.get('/a')).toEqual({ from: 'boot' });
		c.setCsrf('t2');
		expect(await c.get('/b')).toEqual({ from: 'network' });
	});

	it('asks again when the boot request failed on the network', async () => {
		const early: EarlyAsks = { at: Date.now(), asks: { '/api/v1/home': Promise.reject(new TypeError('offline')) } };
		const f = vi.fn(async () => reply(200, { from: 'network' }));
		expect(await createClient(f, () => early).get('/home')).toEqual({ from: 'network' });
		expect(f).toHaveBeenCalledTimes(1);
	});

	it('never answers a change from a boot request', async () => {
		const { f, c } = withEarly({ '/api/v1/home': reply(200, { from: 'boot' }) });
		expect(await c.put('/home', {})).toEqual({ from: 'network' });
		expect(f).toHaveBeenCalledTimes(1);
	});
});
