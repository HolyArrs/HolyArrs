// Who is signed in, what they may do, and the sign-in, first-run, invite,
// two-factor, Plex, Jellyfin, Emby and sign-out steps.

import { api as defaultClient, type Client } from './api';

export type SessionState = 'unknown' | 'signed_out' | 'needs_setup' | 'signed_in';
export type Role = 'owner' | 'admin' | 'member';
export type Perm = 'ask' | 'auto_approve' | 'approve' | 'manage_library' | 'settings';
export type Way = 'password' | 'plex' | 'jellyfin' | 'emby' | 'oidc' | 'forward_auth';
export type AuthMethod = string;

export interface Me {
	id?: number;
	name: string;
	via: string; // session, api_key, local_network
	csrf: string;
	role?: Role;
	perms?: Perm[];
	two_factor?: boolean;
	must_two_factor?: boolean;
	safe_mode?: boolean;
	auth_method?: AuthMethod;
	local_recent?: boolean;
	/** A Plex, Jellyfin or Emby sign-in under ten minutes old. */
	outside_recent?: boolean;
	/** False for someone who joined through a media server and set no password yet. */
	has_password?: boolean;
	/** Address of my profile picture, or null. */
	avatar?: string | null;
	/** Age-rating limit ('' = none; set means a Kid account). */
	max_rating?: string;
	rating_country?: string;
}

/** A sign-in step answers with a session, or asks for a two-factor code. */
export type SignInResult = { done: true } | { done: false; ticket: string };

interface SignInAnswer {
	name?: string;
	csrf?: string;
	two_factor?: boolean;
	ticket?: string;
}

export function createSession(client: Client = defaultClient) {
	// Async reads may finish out of order. Authentication completions, logout,
	// and lost-session notices advance this value so an older load cannot
	// replace a newer identity.
	let generation = 0;
	const s = $state<{
		state: SessionState;
		/** My account number (from /auth/me); undefined until signed in. */
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
		/** Address of my profile picture; null shows the initials. */
		avatar: string | null;
		/** Age-rating limit; '' means none. A Kid has one. */
		maxRating: string;
		ways: Way[];
		/** The server started in safe mode: only the core runs. */
		safeMode: boolean;
		/** /config is not mapped to the host (first-run note). */
		configUnmapped: boolean;
		/** The shortest password the owner allows (a setting; 4 at least). */
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
		avatar: null,
		maxRating: '',
		ways: ['password'],
		safeMode: false,
		configUnmapped: false,
		minPassword: 8
	});

	interface AuthState {
		needs_setup: boolean;
		ways?: Way[];
		safe_mode?: boolean;
		min_password_length?: number;
		config_unmapped?: boolean;
	}
	function tookState(st: AuthState) {
		s.minPassword = Math.max(4, st.min_password_length ?? 8);
		if (st.ways) s.ways = st.ways;
		if (st.safe_mode !== undefined) s.safeMode = !!st.safe_mode;
		s.configUnmapped = !!st.config_unmapped;
	}

	/** loadMinPassword reads the owner's shortest-password setting again. */
	async function loadMinPassword(): Promise<number> {
		try {
			tookState(await client.get<AuthState>('/auth/state'));
		} catch {
			// keep what we had; the server checks again anyway
		}
		return s.minPassword;
	}

	/** loadWays refreshes which sign-in providers are currently offered. */
	async function loadWays(): Promise<Way[]> {
		try {
			tookState(await client.get<AuthState>('/auth/state'));
		} catch {
			// Keep the last known choices; individual actions still fail closed.
		}
		return s.ways;
	}

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
		s.avatar = me.avatar ?? null;
		s.maxRating = me.max_rating ?? '';
		s.safeMode = !!me.safe_mode;
		s.state = 'signed_in';
	}

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

	/** load asks the server who this is. */
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

	/** finish takes a sign-in answer: a session, or a two-factor ticket. */
	async function finish(r: SignInAnswer): Promise<SignInResult> {
		if (r.two_factor && r.ticket) return { done: false, ticket: r.ticket };
		const expected = ++generation;
		// The sign-in answer carries only the name; ask for the role and
		// permissions before showing anything. If that canonical read fails,
		// fail closed instead of making an identity from the login response.
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

	/** plexFinish is asked again and again until Plex says yes. */
	async function plexFinish(id: number): Promise<SignInResult | 'waiting'> {
		const r = await client.post<SignInAnswer & { waiting?: boolean }>('/auth/plex/finish', { id });
		if (r?.waiting) return 'waiting';
		return finish(r);
	}

	async function acceptInvite(token: string, name: string, password: string): Promise<SignInResult> {
		return finish(await client.post<SignInAnswer>(`/auth/invites/${encodeURIComponent(token)}`, { name, password }));
	}

	// First run has no setup code (decisions/0009-no-setup-code.md, intentional).
	async function setup(name: string, password: string) {
		await finish(await client.post<SignInAnswer>('/auth/setup', { name, password }));
	}

	async function logout() {
		const expected = ++generation;
		try {
			await client.post('/auth/logout');
		} finally {
			await signedOutState(expected);
		}
	}

	/** lost is called when any call answers 401. */
	function lost() {
		if (s.state === 'signed_in') void signedOutState(++generation);
	}

	/** can says whether the signed-in person may do p. */
	function can(p: Perm | 'users'): boolean {
		if (s.role === 'owner') return true;
		if (p === 'users') return s.role === 'admin';
		return s.perms.includes(p);
	}

	client.onSignedOut(lost);

	return {
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
		setup,
		logout,
		lost,
		can,
		/** True for a Kid account (an age-rating limit is set). */
		get kid() {
			return !!s.maxRating;
		},
		loadMinPassword,
		loadWays,
		/** setAvatar remembers my picture's address after an upload or a removal. */
		setAvatar(url: string | null) {
			s.avatar = url;
		}
	};
}

export const session = createSession();
