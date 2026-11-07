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
		vi.useFakeTimers();
		src.onerror?.(new Event('error'));
		expect(live.status.state).toBe('live');
		vi.advanceTimersByTime(0);
		expect(factory).toHaveBeenCalledTimes(2);
		src.onerror?.(new Event('error'));
		expect(live.status.state).toBe('reconnecting');
		vi.useRealTimers();
		src.onmessage?.(new MessageEvent('message', { data: JSON.stringify({ topic: 'x' }) }));
		expect(live.status.last).toBeGreaterThan(0);
		live.stop();
		expect(src.close).toHaveBeenCalled();
		expect(live.status.state).toBe('off');
	});
});

describe('toasts', () => {
	afterEach(() => vi.useRealTimers());

	it('shows the newest two at once, counts the rest and clears itself', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		for (let i = 0; i < 4; i++) t.push(`m${i}`);
		expect(t.list.map((x) => x.text)).toEqual(['m2', 'm3']);
		expect(t.counts.waiting).toBe(2);
		vi.advanceTimersByTime(1001);
		expect(t.list).toHaveLength(0);
		expect(t.counts.waiting).toBe(0);
	});

	it('waits only behind two that both offer Undo', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		const undo = { label: 'Undo', run: () => {} };
		t.push('a', 'ok', { action: undo, ms: 5000 });
		t.push('b', 'ok', { action: undo, ms: 5000 });
		t.push('c');
		expect(t.list.map((x) => x.text)).toEqual(['a', 'b']);
		expect(t.counts.waiting).toBe(1);
		vi.advanceTimersByTime(5001);
		expect(t.list.map((x) => x.text)).toEqual(['c']);
	});

	it('refreshes the same message instead of stacking it', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		const a = t.push('Copied', 'ok');
		vi.advanceTimersByTime(800);
		const b = t.push('Copied', 'ok');
		expect(b).toBe(a);
		expect(t.list).toHaveLength(1);
		expect(t.list[0].seen).toBe(1);
		vi.advanceTimersByTime(800);
		expect(t.list).toHaveLength(1);
		vi.advanceTimersByTime(300);
		expect(t.list).toHaveLength(0);
	});

	it('keeps failures longer, longer for more text, and can be closed by hand', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		const id = t.push('bad', 'fail');
		const long = t.push('x'.repeat(200), 'fail');
		vi.advanceTimersByTime(8001);
		expect(t.list.map((x) => x.id)).toEqual([long]);
		vi.advanceTimersByTime(7000);
		expect(t.list).toHaveLength(0);
		const again = t.push('bad', 'fail');
		t.dismiss(again);
		expect(t.list).toHaveLength(0);
		expect(id).not.toBe(again);
	});

	it('holds while hovered and gives back the time left', () => {
		vi.useFakeTimers();
		const t = createToasts(4000);
		const id = t.push('hi');
		vi.advanceTimersByTime(1000);
		t.pause(id);
		vi.advanceTimersByTime(60000);
		expect(t.list).toHaveLength(1);
		t.resume(id);
		vi.advanceTimersByTime(2999);
		expect(t.list).toHaveLength(1);
		vi.advanceTimersByTime(2);
		expect(t.list).toHaveLength(0);
	});

	it('keeps every alert in Recent alerts, even ones a burst drops', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		for (let i = 0; i < 12; i++) t.push(`n${i}`);
		expect(t.list.map((x) => x.text)).toEqual(['n10', 'n11']);
		expect(t.counts.waiting).toBe(10);
		expect(t.past.map((p) => p.text)).toHaveLength(12);
		expect(t.past[0].text).toBe('n11');
		expect(t.unseen()).toBe(12);
		t.seeAll();
		expect(t.counts.waiting).toBe(0);
		expect(t.unseen()).toBe(0);
		t.forget(t.past[0].id);
		expect(t.past).toHaveLength(11);
		t.forgetAll();
		expect(t.past).toHaveLength(0);
	});

	it('runs Undo from the list once, while it still works', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		const run = vi.fn();
		const id = t.push('Severance deleted', 'ok', { action: { label: 'Undo', run }, ms: 2000 });
		t.runPast(id);
		t.runPast(id);
		expect(run).toHaveBeenCalledTimes(1);
		expect(t.list).toHaveLength(0);
		const late = vi.fn();
		const id2 = t.push('Andor deleted', 'ok', { action: { label: 'Undo', run: late }, ms: 2000 });
		vi.advanceTimersByTime(10_000);
		t.runPast(id2);
		expect(late).not.toHaveBeenCalled();
	});

	it('never drops an Undo from the queue in a burst', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		t.push('a');
		t.push('b');
		t.push('Deleted', 'ok', { action: { label: 'Undo', run: () => {} }, ms: 5000 });
		for (let i = 0; i < 10; i++) t.push(`n${i}`);
		vi.advanceTimersByTime(1001);
		expect(t.list.map((x) => x.text)).toContain('Deleted');
	});

	it('carries an action and its own time', () => {
		vi.useFakeTimers();
		const t = createToasts(1000);
		const run = vi.fn();
		t.push('done', 'ok', { action: { label: 'Undo', run }, ms: 3000 });
		vi.advanceTimersByTime(1001);
		expect(t.list[0].action?.label).toBe('Undo');
		t.list[0].action?.run();
		expect(run).toHaveBeenCalled();
		vi.advanceTimersByTime(2000);
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
		expect(s.s.setupWays).toEqual(['password']);
	});

	it('reads the first-run ways and makes the owner from a media server', async () => {
		let sent: unknown;
		let meOK = false;
		const c = createClient(async (url: string, init?: RequestInit) => {
			if (url.endsWith('/auth/state')) {
				return new Response(JSON.stringify({ needs_setup: true, ways: ['password'], setup_ways: ['password', 'plex', 'jellyfin', 'emby'] }));
			}
			if (url.endsWith('/auth/setup/discover')) {
				return new Response(JSON.stringify({ servers: [{ kind: 'jellyfin', name: 'Den', address: 'http://192.168.1.20:8096' }] }));
			}
			if (url.endsWith('/auth/setup/media-server/jellyfin')) {
				sent = JSON.parse(String(init?.body));
				meOK = true;
				return new Response(JSON.stringify({ name: 'Boss', csrf: 'temporary' }), { status: 201 });
			}
			if (url.endsWith('/auth/me') && meOK) {
				return new Response(JSON.stringify({ name: 'Boss', via: 'session', csrf: 'c3', role: 'owner', perms: [], auth_method: 'jellyfin' }));
			}
			return new Response(JSON.stringify({ error: 'sign in first' }), { status: 401 });
		});
		const s = createSession(c);
		expect(await s.load()).toBe('needs_setup');
		expect(s.s.setupWays).toEqual(['password', 'plex', 'jellyfin', 'emby']);
		expect(await s.setupFindServers()).toEqual([{ kind: 'jellyfin', name: 'Den', address: 'http://192.168.1.20:8096' }]);
		await s.setupMediaServer('jellyfin', 'http://192.168.1.20:8096', 'boss', 'pw');
		expect(sent).toEqual({ url: 'http://192.168.1.20:8096', name: 'boss', password: 'pw' });
		expect(s.s.state).toBe('signed_in');
		expect(s.s.role).toBe('owner');
	});

	it('finds no servers when the search fails', async () => {
		const c = createClient(async () => new Response(JSON.stringify({ error: 'not found' }), { status: 404 }));
		expect(await createSession(c).setupFindServers()).toEqual([]);
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
		await s.setup('luis', pw);
		expect(sent).toEqual({ name: 'luis', password: pw });
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

	it('joins through an invite with Plex or a media server', async () => {
		let confirmed = false;
		const c = server({
			'POST /auth/invites/t%2F1/plex/start': json(200, { id: 9, auth_url: 'https://plex.example/auth' }),
			'POST /auth/invites/t%2F1/plex/finish': () => new Response(JSON.stringify(confirmed ? { name: 'kid', csrf: 'p' } : { waiting: true }), { status: confirmed ? 200 : 202 }),
			'POST /auth/invites/t%2F1/media-server/emby': json(409, { error: 'This account already has a HolyArrs account. Sign in instead.', code: 'linked' }),
			'GET /auth/me': json(200, { name: 'kid', via: 'session', csrf: 'p', role: 'member', perms: ['ask'] })
		});
		const s = createSession(c);
		expect((await s.invitePlexStart('t/1')).id).toBe(9);
		expect(await s.invitePlexFinish('t/1', 9)).toBe('waiting');
		confirmed = true;
		expect(await s.invitePlexFinish('t/1', 9)).toEqual({ done: true });
		expect(s.s.name).toBe('kid');
		await expect(s.inviteMediaServer('t/1', 'emby', 'mum', 'right')).rejects.toMatchObject({ status: 409, body: { code: 'linked' } });
	});

	it('reads the sign-in ways when signed out', async () => {
		const c = server({ 'GET /auth/me': json(401, {}), 'GET /auth/state': json(200, { needs_setup: false, ways: ['password', 'jellyfin'] }) });
		const s = createSession(c);
		await s.load();
		expect(s.s.ways).toEqual(['password', 'jellyfin']);
	});

	it('reads SSO ways and assurance while signed in', async () => {
		const c = server({
			'GET /auth/me': json(200, {
				name: 'ana',
				via: 'session',
				csrf: 'c4',
				auth_method: 'oidc',
				local_recent: false
			}),
			'GET /auth/state': json(200, { needs_setup: false, ways: ['password', 'oidc', 'forward_auth'] })
		});
		const s = createSession(c);
		await s.load();
		await s.loadWays();
		expect(s.s.authMethod).toBe('oidc');
		expect(s.s.localRecent).toBe(false);
		expect(s.s.ways).toEqual(['password', 'oidc', 'forward_auth']);
	});

	it('starts and finishes OIDC sign-in through the canonical session read', async () => {
		const c = server({
			'POST /auth/oidc/start': json(200, { auth_url: 'https://id.example.test/authorize' }),
			'POST /auth/oidc/finish': json(200, { name: 'uncanonical', csrf: 'temporary' }),
			'GET /auth/me': json(200, { name: 'ana', via: 'session', csrf: 'oidc-csrf', auth_method: 'oidc', role: 'member', perms: ['ask'] })
		});
		const s = createSession(c);
		expect(await s.oidcStart()).toEqual({ auth_url: 'https://id.example.test/authorize' });
		expect(await s.oidcFinish()).toEqual({ done: true });
		expect(s.s.name).toBe('ana');
		expect(s.s.authMethod).toBe('oidc');
		expect(c.csrf()).toBe('oidc-csrf');
	});

	it('does not let a stale initial load overwrite a finished OIDC sign-in', async () => {
		let answerInitial!: (response: Response) => void;
		const initialMe = new Promise<Response>((resolve) => (answerInitial = resolve));
		let meCalls = 0;
		let stateCalls = 0;
		const c = createClient(async (url: string) => {
			if (url.endsWith('/auth/oidc/finish')) return new Response(JSON.stringify({ name: 'uncanonical', csrf: 'temporary' }));
			if (url.endsWith('/auth/me')) {
				meCalls++;
				if (meCalls === 1) return initialMe;
				return new Response(JSON.stringify({ name: 'ana', via: 'session', csrf: 'oidc-csrf', auth_method: 'oidc', role: 'member' }));
			}
			if (url.endsWith('/auth/state')) {
				stateCalls++;
				return new Response(JSON.stringify({ needs_setup: false, ways: ['password', 'oidc'] }));
			}
			return new Response(JSON.stringify({ error: 'no route' }), { status: 404 });
		});
		const s = createSession(c);
		const initialLoad = s.load();
		await Promise.resolve();
		expect(meCalls).toBe(1);
		expect(await s.oidcFinish()).toEqual({ done: true });
		answerInitial(new Response(JSON.stringify({ error: 'sign in first' }), { status: 401 }));
		await initialLoad;
		expect(s.s.state).toBe('signed_in');
		expect(s.s.name).toBe('ana');
		expect(c.csrf()).toBe('oidc-csrf');
		expect(stateCalls).toBe(0);
	});

	it('settles signed out when OIDC finishes but its canonical identity read fails', async () => {
		let answerInitial!: (response: Response) => void;
		const initialMe = new Promise<Response>((resolve) => (answerInitial = resolve));
		let meCalls = 0;
		let stateCalls = 0;
		const c = createClient(async (url: string) => {
			if (url.endsWith('/auth/oidc/finish')) return new Response(JSON.stringify({ name: 'uncanonical', csrf: 'temporary' }));
			if (url.endsWith('/auth/me')) {
				meCalls++;
				if (meCalls === 1) return initialMe;
				return new Response(JSON.stringify({ error: 'canonical identity unavailable' }), { status: 500 });
			}
			if (url.endsWith('/auth/state')) {
				stateCalls++;
				return new Response(JSON.stringify({ needs_setup: false, ways: ['password', 'oidc'] }));
			}
			return new Response(JSON.stringify({ error: 'no route' }), { status: 404 });
		});
		const s = createSession(c);
		const initialLoad = s.load();
		await expect(s.oidcFinish()).rejects.toMatchObject({ status: 500 });
		expect(s.s.state).toBe('signed_out');
		expect(c.csrf()).toBe('');
		expect(stateCalls).toBe(1);
		answerInitial(new Response(JSON.stringify({ error: 'sign in first' }), { status: 401 }));
		await initialLoad;
		expect(s.s.state).toBe('signed_out');
	});

	it('uses an explicit forward-auth action and still canonicalizes the session', async () => {
		const c = server({
			'POST /auth/forward/login': json(200, { name: 'uncanonical', csrf: 'temporary' }),
			'GET /auth/me': json(200, { name: 'proxy-user', via: 'session', csrf: 'forward-csrf', auth_method: 'forward_auth', role: 'member' })
		});
		const s = createSession(c);
		expect(await s.forwardLogin()).toEqual({ done: true });
		expect(s.s.name).toBe('proxy-user');
		expect(s.s.authMethod).toBe('forward_auth');
	});

	it('reauthenticates with local credentials and replaces the current session assurance', async () => {
		let sent: unknown;
		const c = createClient(async (url: string, init?: RequestInit) => {
			if (url.endsWith('/auth/reauth')) {
				sent = JSON.parse(String(init?.body));
				return new Response(JSON.stringify({ name: 'ana', csrf: 'fresh' }));
			}
			return new Response(JSON.stringify({
				name: 'ana', via: 'session', csrf: 'fresh', auth_method: 'password', local_recent: true, role: 'member'
			}));
		});
		const s = createSession(c);
		await s.reauth('<local-password>', '123456');
		expect(sent).toEqual({ password: '<local-password>', code: '123456' });
		expect(s.s.localRecent).toBe(true);
		expect(s.s.authMethod).toBe('password');
		expect(c.csrf()).toBe('fresh');
	});

	it('starts and finishes OIDC linking and links forward auth explicitly', async () => {
		const c = server({
			'POST /auth/oidc/link': json(200, { auth_url: 'https://id.example.test/link' }),
			'POST /auth/oidc/link-finish': json(200, { linked: true }),
			'POST /auth/forward/link': json(200, { linked: true })
		});
		const s = createSession(c);
		expect(await s.oidcLink()).toEqual({ auth_url: 'https://id.example.test/link' });
		expect(await s.oidcLinkFinish()).toEqual({ linked: true });
		expect(await s.forwardLink()).toEqual({ linked: true });
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
