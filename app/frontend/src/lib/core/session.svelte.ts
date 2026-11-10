// Who is signed in, what they may do, and every sign-in route.

import { api as defaultClient, type Client } from './api';
import { setHeld } from './settings.ts';

export type SessionState = 'unknown' | 'signed_out' | 'needs_setup' | 'signed_in';
export type Role = 'owner' | 'admin' | 'member';
export type Perm = 'ask' | 'auto_approve' | 'approve' | 'manage_library' | 'settings';
export type Way = 'password' | 'plex' | 'jellyfin' | 'emby' | 'oidc' | 'forward_auth';
export type AuthMethod = string;

export interface Me {
	id?: number;
	name: string;
	via: string;
	csrf: string;
	role?: Role;
	perms?: Perm[];
	two_factor?: boolean;
	must_two_factor?: boolean;
	safe_mode?: boolean;
	auth_method?: AuthMethod;
	local_recent?: boolean;
	outside_recent?: boolean;
	has_password?: boolean;
	setup_open?: boolean;
	avatar?: string | null;
	max_rating?: string;
	rating_country?: string;
	held?: string[];
}

export interface FoundServer {
	kind: 'jellyfin' | 'emby';
	name: string;
	address: string;
}

export type SignInResult = { done: true } | { done: false; ticket: string };

interface SignInAnswer {
	name?: string;
	csrf?: string;
	two_factor?: boolean;
	ticket?: string;
}

interface AuthState {
	needs_setup: boolean;
	ways?: Way[];
	setup_ways?: Way[];
	safe_mode?: boolean;
	min_password_length?: number;
	config_unmapped?: boolean;
	host_refused?: string;
}
// One session store per client; the default one is exported below.
export function createSession(client: Client = defaultClient) {
	// Bumped on every sign-in or sign-out so a slow older response cannot overwrite newer state.
	let generation = 0;
	const s = $state<{
		state: SessionState;
		id: number | undefined;
		name: string;
		via: string;
		role: Role;
		perms: Perm[];
		twoFactor: boolean;
		mustTwoFactor: boolean;
		authMethod: AuthMethod;
		localRecent: boolean;
		outsideRecent: boolean;
		hasPassword: boolean;
		setupOpen: boolean;
		avatar: string | null;
		maxRating: string;
		ways: Way[];
		setupWays: Way[];
		safeMode: boolean;
		held: string[];
		configUnmapped: boolean;
		hostRefused: string;
		minPassword: number;
	}>({
		state: 'unknown',
		id: undefined,
		name: '',
		via: '',
		role: 'member',
		perms: [],
		twoFactor: false,
		mustTwoFactor: false,
		authMethod: '',
		localRecent: false,
		outsideRecent: false,
		hasPassword: true,
		setupOpen: false,
		avatar: null,
		maxRating: '',
		ways: ['password'],
		setupWays: ['password'],
		safeMode: false,
		held: [],
		configUnmapped: false,
		hostRefused: '',
		minPassword: 8
	});

	// Applies the public auth settings the server shares before and after sign-in.
	function tookState(st: AuthState) {
		// Floor of 4 guards against a bad server value.
		s.minPassword = Math.max(4, st.min_password_length ?? 8);
		if (st.ways) s.ways = st.ways;
		s.setupWays = st.setup_ways?.length ? st.setup_ways : ['password'];
		if (st.safe_mode !== undefined) s.safeMode = !!st.safe_mode;
		s.configUnmapped = !!st.config_unmapped;
		s.hostRefused = st.host_refused ?? '';
	}

	// Failure keeps the last value; the server checks the length again.
	async function loadMinPassword(): Promise<number> {
		try {
			tookState(await client.get<AuthState>('/auth/state'));
		} catch {
		}
		return s.minPassword;
	}

	// Failure keeps the last known choices; each action still fails closed.
	async function loadWays(): Promise<Way[]> {
		try {
			tookState(await client.get<AuthState>('/auth/state'));
		} catch {
		}
		return s.ways;
	}

	// Copies the profile into state and sets the token used on writes.
	function apply(me: Me) {
		client.setCsrf(me.csrf ?? '');
		s.id = me.id;
		s.name = me.name;
		s.via = me.via;
		s.role = me.role ?? 'member';
		s.perms = me.perms ?? [];
		s.twoFactor = !!me.two_factor;
		s.mustTwoFactor = !!me.must_two_factor;
		s.authMethod = me.auth_method ?? '';
		s.localRecent = !!me.local_recent;
		s.outsideRecent = !!me.outside_recent;
		s.hasPassword = me.has_password ?? true;
		s.setupOpen = !!me.setup_open;
		s.avatar = me.avatar ?? null;
		s.maxRating = me.max_rating ?? '';
		s.safeMode = !!me.safe_mode;
		s.held = me.held ?? [];
		setHeld((f) => s.held.includes(f));
		s.state = 'signed_in';
	}

	// Clears the person; a stale call (older generation) does nothing.
	async function signedOutState(expected: number) {
		if (expected !== generation) return;
		client.setCsrf('');
		s.id = undefined;
		s.name = '';
		s.via = '';
		s.perms = [];
		s.authMethod = '';
		s.localRecent = false;
		s.outsideRecent = false;
		s.hasPassword = true;
		s.setupOpen = false;
		s.avatar = null;
		s.maxRating = '';
		try {
			const st = await client.get<AuthState>('/auth/state');
			if (expected !== generation) return;
			tookState(st);
			s.ways = st.ways ?? ['password'];
			s.safeMode = !!st.safe_mode;
			s.state = st.needs_setup ? 'needs_setup' : 'signed_out';
		} catch {
			if (expected === generation) s.state = 'signed_out';
		}
	}

	async function load(): Promise<SessionState> {
		const expected = generation;
		try {
			const me = await client.get<Me>('/auth/me');
			if (expected === generation) apply(me);
		} catch {
			await signedOutState(expected);
		}
		return s.state;
	}

	// Shared end of every sign-in: a two-factor ticket pauses it, otherwise load the profile.
	async function finish(r: SignInAnswer): Promise<SignInResult> {
		if (r.two_factor && r.ticket) return { done: false, ticket: r.ticket };
		const expected = ++generation;
		client.setCsrf(r.csrf ?? '');
		try {
			const me = await client.get<Me>('/auth/me');
			if (expected === generation) apply(me);
		} catch (err) {
			await signedOutState(expected);
			throw err;
		}
		return { done: true };
	}

	async function login(name: string, password: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>('/auth/login', { name, password }));
	}

	async function loginCode(ticket: string, code: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>('/auth/login/two-factor', { ticket, code }));
	}

	async function mediaServer(kind: 'jellyfin' | 'emby', name: string, password: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>(`/auth/media-server/${kind}`, { name, password }));
	}

	async function oidcStart(): Promise<{ auth_url: string }> {
		return client.post('/auth/oidc/start');
	}

	async function oidcFinish(): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>('/auth/oidc/finish'));
	}

	async function forwardLogin(): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>('/auth/forward/login'));
	}

	async function reauth(password: string, code?: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>('/auth/reauth', { password, ...(code ? { code } : {}) }));
	}

	async function oidcLink(): Promise<{ auth_url: string }> {
		return client.post('/auth/oidc/link');
	}

	async function oidcLinkFinish(): Promise<{ linked: boolean }> {
		return client.post('/auth/oidc/link-finish');
	}

	async function forwardLink(): Promise<{ linked: boolean }> {
		return client.post('/auth/forward/link');
	}

	async function plexStart(): Promise<{ id: number; auth_url: string }> {
		return client.post('/auth/plex/start');
	}

	// Plex returns waiting until the person approves in the other window.
	async function plexFinish(id: number): Promise<SignInResult | 'waiting'> {
		const r = await client.post<SignInAnswer & { waiting?: boolean }>('/auth/plex/finish', { id });
		if (r?.waiting) return 'waiting';
		return finish(r);
	}

	async function acceptInvite(token: string, name: string, password: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>(`/auth/invites/${encodeURIComponent(token)}`, { name, password }));
	}

	async function invitePlexStart(token: string): Promise<{ id: number; auth_url: string }> {
		return client.post(`/auth/invites/${encodeURIComponent(token)}/plex/start`);
	}

	async function invitePlexFinish(token: string, id: number): Promise<SignInResult | 'waiting'> {
		const r = await client.post<SignInAnswer & { waiting?: boolean }>(`/auth/invites/${encodeURIComponent(token)}/plex/finish`, { id });
		if (r?.waiting) return 'waiting';
		return finish(r);
	}

	async function inviteMediaServer(token: string, kind: 'jellyfin' | 'emby', name: string, password: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>(`/auth/invites/${encodeURIComponent(token)}/media-server/${kind}`, { name, password }));
	}

	async function setup(name: string, password: string) {
		await finish(await client.post<SignInAnswer>('/auth/setup', { name, password }));
	}

	async function setupPlexStart(): Promise<{ id: number; auth_url: string }> {
		return client.post('/auth/setup/plex/start');
	}

	async function setupPlexFinish(id: number): Promise<SignInResult | 'waiting'> {
		const r = await client.post<SignInAnswer & { waiting?: boolean }>('/auth/setup/plex/finish', { id });
		if (r?.waiting) return 'waiting';
		return finish(r);
	}

	async function setupMediaServer(kind: 'jellyfin' | 'emby', url: string, name: string, password: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>(`/auth/setup/media-server/${kind}`, { url, name, password }));
	}

	async function setupFindServers(): Promise<FoundServer[]> {
		try {
			return (await client.get<{ servers?: FoundServer[] }>('/auth/setup/discover')).servers ?? [];
		} catch {
			return [];
		}
	}

	async function logout() {
		const expected = ++generation;
		try {
			await client.post('/auth/logout');
		} finally {
			await signedOutState(expected);
		}
	}

	// Called when the server says the session ended.
	function lost() {
		if (s.state === 'signed_in') void signedOutState(++generation);
	}

	// Owners can do anything; admins also manage users.
	function can(p: Perm | 'users'): boolean {
		if (s.role === 'owner') return true;
		if (p === 'users') return s.role === 'admin';
		return s.perms.includes(p);
	}

	function held(name: string): boolean {
		return s.held.includes(name);
	}

	client.onSignedOut(lost);

	return {
		held,
		s,
		load,
		login,
		loginCode,
		mediaServer,
		oidcStart,
		oidcFinish,
		forwardLogin,
		reauth,
		oidcLink,
		oidcLinkFinish,
		forwardLink,
		plexStart,
		plexFinish,
		acceptInvite,
		invitePlexStart,
		invitePlexFinish,
		inviteMediaServer,
		setup,
		setupPlexStart,
		setupPlexFinish,
		setupMediaServer,
		setupFindServers,
		logout,
		lost,
		can,
		get kid() {
			return !!s.maxRating;
		},
		loadMinPassword,
		loadWays,
		setAvatar(url: string | null) {
			s.avatar = url;
		}
	};
}

export const session = createSession();
