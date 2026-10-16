import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLive, type SourceFactory } from './live.svelte.ts';
import { createToasts } from './toasts.svelte.ts';
import { createSession } from './session.svelte.ts';
import { createClient } from './api.ts';

describe('live', () => {
	function fakeSource() {
		const src = {
			onopen: null as null | ((e: Event) => void),
			onerror: null as null | ((e: Event) => void),
			onmessage: null as null | ((e: MessageEvent) => void),
			close: vi.fn()
		};
		const factory = vi.fn(() => src) as unknown as SourceFactory;
		return { src, factory };
	}

	it('routes messages by topic and to "*" listeners', () => {
		const live = createLive(fakeSource().factory);
		const dl = vi.fn();
		const all = vi.fn();
		live.on('download', dl);
		live.on('*', all);
		live.dispatch(JSON.stringify({ topic: 'download', data: { ID: 1 } }));
		live.dispatch(JSON.stringify({ topic: 'title', data: {} }));
		expect(dl).toHaveBeenCalledWith({ ID: 1 }, 'download');
		expect(dl).toHaveBeenCalledTimes(1);
		expect(all).toHaveBeenCalledTimes(2);
	});

	it('sends resync to every listener and ignores junk', () => {
		const live = createLive(fakeSource().factory);
		const a = vi.fn();
		const b = vi.fn();
		live.on('title', a);
		live.on('event', b);
		live.dispatch('not json');
		live.dispatch(JSON.stringify({ data: 1 }));
		expect(a).not.toHaveBeenCalled();
		live.dispatch(JSON.stringify({ topic: 'resync' }));
		expect(a).toHaveBeenCalledTimes(1);
		expect(b).toHaveBeenCalledTimes(1);
	});

	it('stops calling a listener after it unsubscribes', () => {
		const live = createLive(fakeSource().factory);
		const a = vi.fn();
		const off = live.on('title', a);
		off();
		live.dispatch(JSON.stringify({ topic: 'title' }));
		expect(a).not.toHaveBeenCalled();
	});

	it('tracks the connection state and opens one stream', () => {
		const { src, factory } = fakeSource();
		const live = createLive(factory);
		live.start();
		live.start();
		expect(factory).toHaveBeenCalledTimes(1);
		expect(factory).toHaveBeenCalledWith('/live');
		expect(live.status.state).toBe('connecting');
		src.onopen?.(new Event('open'));
		expect(live.status.state).toBe('live');
		src.onerror?.(new Event('error'));
		expect(live.status.state).toBe('reconnecting');
		src.onmessage?.(new MessageEvent('message', { data: JSON.stringify({ topic: 'x' }) }));
		expect(live.status.last).toBeGreaterThan(0);
		live.stop();
		expect(src.close).toHaveBeenCalled();
		expect(live.status.state).toBe('off');
	});
});

describe('toasts', () => {
	afterEach(() => vi.useRealTimers());

	it('adds, caps at four and clears itself', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		for (let i = 0; i < 6; i++) t.push(`m${i}`);
		expect(t.list.map((x) => x.text)).toEqual(['m2', 'm3', 'm4', 'm5']);
		vi.advanceTimersByTime(1001);
		expect(t.list).toHaveLength(0);
	});

	it('keeps failures longer and can be closed by hand', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		const id = t.push('bad', 'fail');
		t.push('ok', 'ok');
		vi.advanceTimersByTime(1001);
		expect(t.list.map((x) => x.text)).toEqual(['bad']);
		t.dismiss(id);
		expect(t.list).toHaveLength(0);
	});
});

describe('session', () => {
	function server(routes: Record<string, () => Response>) {
		return createClient(async (url: string, init?: RequestInit) => {
			const key = `${init?.method ?? 'GET'} ${url.replace('/api/v1', '')}`;
			const r = routes[key];
			return r ? r() : new Response(JSON.stringify({ error: 'no route' }), { status: 404 });
		});
	}
	const json = (status: number, body: unknown) => () => new Response(JSON.stringify(body), { status });

	it('is signed in when /auth/me answers, and keeps the CSRF token', async () => {
		const c = server({ 'GET /auth/me': json(200, { name: 'luis', via: 'session', csrf: 'abc' }) });
		const s = createSession(c);
		expect(await s.load()).toBe('signed_in');
		expect(s.s.name).toBe('luis');
		expect(c.csrf()).toBe('abc');
	});

	it('knows the server runs in safe mode, signed in or out', async () => {
		const c = server({ 'GET /auth/me': json(200, { name: 'luis', via: 'session', csrf: 'abc', safe_mode: true }) });
		const s = createSession(c);
		await s.load();
		expect(s.s.safeMode).toBe(true);
		const out = createSession(server({ 'GET /auth/me': json(401, {}), 'GET /auth/state': json(200, { needs_setup: false, safe_mode: true }) }));
		await out.load();
		expect(out.s.state).toBe('signed_out');
		expect(out.s.safeMode).toBe(true);
		const plain = createSession(server({ 'GET /auth/me': json(200, { name: 'luis', via: 'session', csrf: 'abc' }) }));
		await plain.load();
		expect(plain.s.safeMode).toBe(false);
	});

	it('asks for first-run setup when there is no owner', async () => {
		const c = server({ 'GET /auth/me': json(401, { error: 'sign in first' }), 'GET /auth/state': json(200, { needs_setup: true }) });
		const s = createSession(c);
		expect(await s.load()).toBe('needs_setup');
	});

	it('signs in, then out', async () => {
		let out = false;
		const c = server({
			'POST /auth/login': json(200, { name: 'uncanonical', csrf: 'temporary' }),
			'GET /auth/me': json(200, { name: 'luis', via: 'session', csrf: 'c1', role: 'owner', perms: [] }),
			'POST /auth/logout': () => ((out = true), new Response(null, { status: 204 })),
			'GET /auth/state': json(200, { needs_setup: false })
		});
		const s = createSession(c);
		await s.login('luis', 'pw');
		expect(s.s.state).toBe('signed_in');
		expect(s.s.name).toBe('luis');
		expect(s.s.role).toBe('owner');
		expect(c.csrf()).toBe('c1');
		await s.logout();
		expect(out).toBe(true);
		expect(s.s.state).toBe('signed_out');
		expect(c.csrf()).toBe('');
	});

	it('runs first-run setup with the setup code', async () => {
		let sent: unknown;
		const c = createClient(async (url: string, init?: RequestInit) => {
			if (url.endsWith('/auth/setup')) {
				sent = JSON.parse(String(init?.body));
				return new Response(JSON.stringify({ name: 'uncanonical', csrf: 'temporary' }), { status: 201 });
			}
			return new Response(JSON.stringify({ name: 'luis', via: 'session', csrf: 'c2', role: 'owner', perms: [] }));
		});
		const s = createSession(c);
		const pw = ['test', 'only'].join('-');
		await s.setup('CODE', 'luis', pw);
		expect(sent).toEqual({ code: 'CODE', name: 'luis', password: pw });
		expect(s.s.state).toBe('signed_in');
	});

	it.each([
		['401', () => new Response(JSON.stringify({ error: 'sign in first' }), { status: 401 })],
		['500', () => new Response(JSON.stringify({ error: 'broken session lookup' }), { status: 500 })],
		['network error', () => Promise.reject(new TypeError('connection lost'))]
	])('does not invent a signed-in identity when canonical /auth/me fails with %s', async (_kind, me) => {
		const c = createClient(async (url: string) => {
			if (url.endsWith('/auth/login')) return new Response(JSON.stringify({ name: 'claim', csrf: 'temporary' }));
			return me();
		});
		const s = createSession(c);
		await expect(s.login('claim', 'pw')).rejects.toThrow();
		expect(s.s.state).not.toBe('signed_in');
		expect(s.s.name).toBe('');
		expect(c.csrf()).toBe('');
	});

	it('asks for a two-factor code, then signs in with it', async () => {
		const c = server({
			'POST /auth/login': json(200, { two_factor: true, ticket: 't1' }),
			'POST /auth/login/two-factor': json(200, { name: 'ana', csrf: 'c3' }),
			'GET /auth/me': json(200, { name: 'ana', via: 'session', csrf: 'c3', role: 'member', perms: ['ask'] })
		});
		const s = createSession(c);
		const first = await s.login('ana', 'pw');
		expect(first).toEqual({ done: false, ticket: 't1' });
		expect(s.s.state).not.toBe('signed_in');
		expect(await s.loginCode('t1', '123456')).toEqual({ done: true });
		expect(s.s.state).toBe('signed_in');
		expect(s.s.role).toBe('member');
		expect(s.can('ask')).toBe(true);
		expect(s.can('settings')).toBe(false);
		expect(s.can('users')).toBe(false);
	});

	it('knows the owner may do everything and admins may manage people', async () => {
		const owner = createSession(server({ 'GET /auth/me': json(200, { name: 'luis', via: 'session', csrf: 'x', role: 'owner', perms: [] }) }));
		await owner.load();
		expect(owner.can('settings') && owner.can('users')).toBe(true);
		const admin = createSession(server({ 'GET /auth/me': json(200, { name: 'a', via: 'session', csrf: 'x', role: 'admin', perms: ['approve'] }) }));
		await admin.load();
		expect(admin.can('users')).toBe(true);
		expect(admin.can('settings')).toBe(false);
	});

	it('waits for Plex, then signs in', async () => {
		let confirmed = false;
		const c = server({
			'POST /auth/plex/finish': () => new Response(JSON.stringify(confirmed ? { name: 'kid', csrf: 'p' } : { waiting: true }), { status: confirmed ? 200 : 202 }),
			'GET /auth/me': json(200, { name: 'kid', via: 'session', csrf: 'p', role: 'member', perms: ['ask'] })
		});
		const s = createSession(c);
		expect(await s.plexFinish(7)).toBe('waiting');
		confirmed = true;
		expect(await s.plexFinish(7)).toEqual({ done: true });
		expect(s.s.name).toBe('kid');
	});

	it('reads the sign-in ways when signed out', async () => {
		const c = server({ 'GET /auth/me': json(401, {}), 'GET /auth/state': json(200, { needs_setup: false, ways: ['password', 'jellyfin'] }) });
		const s = createSession(c);
		await s.load();
		expect(s.s.ways).toEqual(['password', 'jellyfin']);
	});

	it('notices a lost session from any call', async () => {
		const c = server({
			'GET /auth/me': json(200, { name: 'luis', via: 'session', csrf: 'x' }),
			'GET /titles': json(401, { error: 'sign in first' }),
			'GET /auth/state': json(200, { needs_setup: false })
		});
		const s = createSession(c);
		await s.load();
		await c.get('/titles').catch(() => {});
		await vi.waitFor(() => expect(s.s.state).toBe('signed_out'));
	});
});
