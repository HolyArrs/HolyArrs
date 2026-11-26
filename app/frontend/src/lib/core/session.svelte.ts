// Who is signed in, what they may do, and every sign-in route.

import { api as defaultClient, type Client } from './api';
import { setHeld } from './settings.ts';
import { views as accountViews } from './views.svelte.ts';

// Things outside core that hold one person's data (Watched marks) clear themselves here when that person signs out.
const signOutHooks = new Set<() => void>();
export function onSignedOut(fn: () => void): () => void {
	signOutHooks.add(fn);
	return () => signOutHooks.delete(fn);
}

export type SessionState = 'unknown' | 'signed_out' | 'needs_setup' | 'signed_in';
export type Role = 'owner' | 'admin' | 'member';
export type Perm = 'ask' | 'auto_approve' | 'auto_approve_four_k' | 'approve' | 'manage_library' | 'settings';
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
	// Whether the server takes this session as a recent proof for risky changes right now.
	recent_proof?: boolean;
	has_password?: boolean;
	setup_open?: boolean;
	setup_skipped?: boolean;
	avatar?: string | null;
	max_rating?: string;
	rating_country?: string;
	held?: string[];
	views?: Record<string, unknown>;
	// Hands-off mode: HolyArrs only sends to the Downloader and needs no Library folder.
	hands_off?: boolean;
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
	// The owner exists but first-run setup is not finished: sign-in stays a setup step.
	setup_skipped?: boolean;
	setup_open?: boolean;
	config_unmapped?: boolean;
	host_refused?: string;
	// "Forgot password?" is shown; _env: HOLYARRS_RESET_OWNER printed a code at start-up.
	owner_reset?: boolean;
	owner_reset_env?: boolean;
}
// One session store per client; the default one is exported below. The person's saved page views ride along with the session.
export function createSession(client: Client = defaultClient, views: Pick<typeof accountViews, 'take' | 'forget'> = accountViews) {
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
		/** The owner skipped the rest of first-run setup; a Finish setup card offers it again. */
		setupSkipped: boolean;
		avatar: string | null;
		maxRating: string;
		ways: Way[];
		setupWays: Way[];
		setupPending: boolean;
		safeMode: boolean;
		handsOff: boolean;
		held: string[];
		configUnmapped: boolean;
		hostRefused: string;
		minPassword: number;
		ownerReset: boolean;
		ownerResetEnv: boolean;
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
		setupSkipped: false,
		avatar: null,
		maxRating: '',
		ways: ['password'],
		setupWays: ['password'],
		setupPending: false,
		safeMode: false,
		handsOff: false,
		held: [],
		configUnmapped: false,
		hostRefused: '',
		minPassword: 8,
		ownerReset: false,
		ownerResetEnv: false
	});

	// Applies the public auth settings the server shares before and after sign-in.
	function tookState(st: AuthState) {
		// Floor of 4 guards against a bad server value.
		s.minPassword = Math.max(4, st.min_password_length ?? 8);
		if (st.ways) s.ways = st.ways;
		s.setupWays = st.setup_ways?.length ? st.setup_ways : ['password'];
		s.setupPending = !!st.setup_open;
		if (st.safe_mode !== undefined) s.safeMode = !!st.safe_mode;
		s.configUnmapped = !!st.config_unmapped;
		s.hostRefused = st.host_refused ?? '';
		s.ownerReset = !!st.owner_reset;
		s.ownerResetEnv = !!st.owner_reset_env;
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
		// The server's own answer: a recent password or setup sign-in, or a fresh Plex, Jellyfin or Emby one when there is no password, and no two-factor still to turn on.
		s.localRecent = me.recent_proof ?? !!me.local_recent;
		s.outsideRecent = !!me.outside_recent;
		s.hasPassword = me.has_password ?? true;
		s.setupOpen = !!me.setup_open;
		s.setupSkipped = !!me.setup_skipped;
		s.avatar = me.avatar ?? null;
		s.maxRating = me.max_rating ?? '';
		s.safeMode = !!me.safe_mode;
		s.handsOff = !!me.hands_off;
		s.held = me.held ?? [];
		setHeld((f) => s.held.includes(f));
		views.take(me.views);
		s.state = 'signed_in';
	}

	// Clears the person; a stale call (older generation) does nothing.
	async function signedOutState(expected: number) {
		if (expected !== generation) return;
		client.setCsrf('');
		views.forget();
		// The next person never sees the last one's data (Watched marks among it).
		for (const fn of signOutHooks) fn();
		s.id = undefined;
		s.name = '';
		s.via = '';
		s.perms = [];
		s.authMethod = '';
		s.localRecent = false;
		s.outsideRecent = false;
		s.hasPassword = true;
		s.setupOpen = false;
		s.setupSkipped = false;
		s.avatar = null;
		s.maxRating = '';
		s.handsOff = false;
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

	// continue_at: this is not the address the provider answers at, so the sign-in goes on there (its /login?sso=start).
	async function oidcStart(fresh = false): Promise<{ auth_url?: string; continue_at?: string }> {
		return client.post('/auth/oidc/start', fresh ? { fresh: true } : undefined);
	}

	// Proof for someone with no password: single sign-on goes straight to the provider, which must ask again, and comes back to `back`; Plex, Jellyfin and Emby sign out and sign in again on the sign-in page.
	async function signInAgain(back: string, goto: (url: string) => unknown, to: (path: string) => string): Promise<void> {
		if (s.authMethod === 'oidc') {
			const { auth_url, continue_at } = await oidcStart(true);
			// Another address's sign-in is no proof here: ask there, and go on there with the page it was asked on.
			if (!auth_url) return window.location.assign(`${continue_at}&next=${encodeURIComponent(back)}`);
			try {
				sessionStorage.setItem('holyarrs-sso-next', back);
			} catch {
				// No storage: the sign-in page opens Home instead.
			}
			window.location.assign(auth_url);
			return;
		}
		await logout();
		await goto(to(`/login?next=${encodeURIComponent(back)}`));
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

	// link_at: linking only works at that address of this server (where the provider answers), not this one.
	async function oidcLink(): Promise<{ auth_url?: string; link_at?: string }> {
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

	async function setup(name: string, password: string, displayName = '') {
		await finish(await client.post<SignInAnswer>('/auth/setup', { name, password, display_name: displayName }));
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
	function can(p: Perm | 'users' | 'owner'): boolean {
		if (s.role === 'owner') return true;
		if (p === 'owner') return false;
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
		signInAgain,
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
		// The Settings switch saved a new hands-off mode.
		setHandsOff(on: boolean) {
			s.handsOff = on;
		},
		setAvatar(url: string | null) {
			s.avatar = url;
		}
	};
}

export const session = createSession();
