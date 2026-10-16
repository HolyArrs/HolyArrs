// Who is signed in, what they may do, and the sign-in, first-run, invite,
// two-factor, Plex, Jellyfin, Emby and sign-out steps.

import { api as defaultClient, type Client } from './api';

export type SessionState = 'unknown' | 'signed_out' | 'needs_setup' | 'signed_in';
export type Role = 'owner' | 'admin' | 'member';
export type Perm = 'ask' | 'auto_approve' | 'approve' | 'manage_library' | 'settings';
export type Way = 'password' | 'plex' | 'jellyfin' | 'emby';

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
	const s = $state<{
		state: SessionState;
		name: string;
		via: string;
		role: Role;
		perms: Perm[];
		twoFactor: boolean;
		mustTwoFactor: boolean;
		ways: Way[];
		/** The server started in safe mode: only the core runs. */
		safeMode: boolean;
		/** The shortest password the owner allows (a setting; 4 at least). */
		minPassword: number;
	}>({ state: 'unknown', name: '', via: '', role: 'member', perms: [], twoFactor: false, mustTwoFactor: false, ways: ['password'], safeMode: false, minPassword: 4 });

	interface AuthState {
		needs_setup: boolean;
		ways?: Way[];
		safe_mode?: boolean;
		min_password_length?: number;
	}
	function tookState(st: AuthState) {
		s.minPassword = Math.max(4, st.min_password_length ?? 4);
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

	function apply(me: Me) {
		client.setCsrf(me.csrf ?? '');
		s.name = me.name;
		s.via = me.via;
		s.role = me.role ?? 'member';
		s.perms = me.perms ?? [];
		s.twoFactor = !!me.two_factor;
		s.mustTwoFactor = !!me.must_two_factor;
		s.safeMode = !!me.safe_mode;
		s.state = 'signed_in';
	}

	async function signedOutState() {
		client.setCsrf('');
		s.name = '';
		s.via = '';
		s.perms = [];
		try {
			const st = await client.get<AuthState>('/auth/state');
			tookState(st);
			s.ways = st.ways ?? ['password'];
			s.safeMode = !!st.safe_mode;
			s.state = st.needs_setup ? 'needs_setup' : 'signed_out';
		} catch {
			s.state = 'signed_out';
		}
	}

	/** load asks the server who this is. */
	async function load(): Promise<SessionState> {
		try {
			apply(await client.get<Me>('/auth/me'));
		} catch {
			await signedOutState();
		}
		return s.state;
	}

	/** finish takes a sign-in answer: a session, or a two-factor ticket. */
	async function finish(r: SignInAnswer): Promise<SignInResult> {
		if (r.two_factor && r.ticket) return { done: false, ticket: r.ticket };
		// The sign-in answer carries only the name; ask for the role and
		// permissions before showing anything. If that canonical read fails,
		// fail closed instead of making an identity from the login response.
		client.setCsrf(r.csrf ?? '');
		try {
			apply(await client.get<Me>('/auth/me'));
		} catch (err) {
			client.setCsrf('');
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

	async function setup(code: string, name: string, password: string) {
		await finish(await client.post<SignInAnswer>('/auth/setup', { code, name, password }));
	}

	async function logout() {
		try {
			await client.post('/auth/logout');
		} finally {
			await signedOutState();
		}
	}

	/** lost is called when any call answers 401. */
	function lost() {
		if (s.state === 'signed_in') void signedOutState();
	}

	/** can says whether the signed-in person may do p. */
	function can(p: Perm | 'users'): boolean {
		if (s.role === 'owner') return true;
		if (p === 'users') return s.role === 'admin';
		return s.perms.includes(p);
	}

	client.onSignedOut(lost);

	return { s, load, login, loginCode, mediaServer, plexStart, plexFinish, acceptInvite, setup, logout, lost, can, loadMinPassword };
}

export const session = createSession();
