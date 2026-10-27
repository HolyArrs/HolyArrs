// My account, for the layouts Luis is comparing (?view=1…5). One object holds
// the page's state and the same handlers and endpoints as Account.svelte, so
// every layout changes the real account the same way. Account.svelte (today's
// page) keeps its own copy until a layout is picked.

import { personName } from '../plain.ts';
import { api, message, ApiError } from '../api.ts';
import { session } from '../session.svelte.ts';
import { toasts } from '../toasts.svelte.ts';
import { goto } from '$app/navigation';
import { page } from '$app/state';
import { to } from '../paths.ts';
import { setFollow } from '../follow.ts';
import { autoplayPref, setAutoplayPref, type AutoplayPref } from '../trailer.ts';
import { endedWords, order, others, type SessionRow } from '../sessions.ts';
import type { Ask } from '../types.ts';

export type Followed = { title_id: number; kind: string; name: string; year?: number };

export const VIA: Record<string, string> = {
	session: 'with your password or a linked account',
	api_key: 'with an API key',
	local_network: 'from your local network, without a password'
};
/** Short words for the same, for one-line rows. */
export const VIA_SHORT: Record<string, string> = {
	session: 'Password or linked account',
	api_key: 'API key',
	local_network: 'Local network'
};
export const ROLE: Record<string, string> = { owner: 'Owner', admin: 'Admin', member: 'Member' };

export function authMethodWord(method: string): string {
	if (method === 'local_password' || method === 'local_setup') return 'HolyArrs password';
	if (method === 'oidc' || method.startsWith('oidc:')) return 'single sign-on';
	if (method === 'forward_auth' || method.startsWith('forward_auth:')) return 'trusted proxy';
	if (method === 'legacy') return 'an older session';
	return method;
}

export const TRAILER_OPTIONS = [
	{ value: 'auto', label: 'Auto' },
	{ value: 'always', label: 'Always' },
	{ value: 'never', label: 'Never' }
] as { value: AutoplayPref; label: string }[];
export const TRAILER_HELP = 'Auto follows your system (reduced motion, data saver). Always plays them anyway; Never turns them off.';
export const PICTURE_HELP = 'Shown in the sidebar and on the Users screen. Pick a JPEG, PNG or WebP picture up to 5 MB. It is cropped to a square around the middle.';
/** How many devices show before Show all. */
export const DEVICES_FIRST = 6;
const AVATAR_MAX = 5 * 1024 * 1024;

export class AccountModel {
	// Alerts
	targets = $state<{ email: string; discord_id: string; new_episodes?: boolean }>({ email: '', discord_id: '', new_episodes: true });
	tgBusy = $state(false);
	tgError = $state('');
	// Asked by
	showMyName = $state(false);
	// Follow
	follows = $state<Followed[]>([]);
	// Recommendations
	resetBusy = $state(false);
	// Password
	current = $state('');
	next = $state('');
	again = $state('');
	pwError = $state('');
	pwBusy = $state(false);
	// Two-factor
	setup = $state<{ secret: string; uri: string } | null>(null);
	code = $state('');
	offCode = $state('');
	offMode = $state<'off' | 'codes'>('codes');
	tfError = $state('');
	tfBusy = $state(false);
	codes = $state<string[]>([]);
	// Confirm local sign-in and linked sign-in
	reauthPassword = $state('');
	reauthCode = $state('');
	reauthError = $state('');
	reauthBusy = $state(false);
	linkError = $state('');
	linkBusy = $state(false);
	// Trailers
	pref = $state<AutoplayPref>(autoplayPref());
	// Picture
	avatarInput: HTMLInputElement | undefined = $state();
	picked = $state<File | null>(null);
	preview = $state('');
	avatarBusy = $state(false);
	// Signed-in devices (/auth/sessions, as Sessions.svelte)
	rows = $state<SessionRow[] | null>(null);
	rowsError = $state('');
	rowsBusy = $state(false);
	/** Long lists show the first few devices until Show all. */
	allDevices = $state(false);
	// My asks, for the profile layout
	asks = $state<Ask[] | null>(null);

	readonly name = $derived(personName(session.s.name, 'You'));
	readonly role = $derived(ROLE[session.s.role] ?? session.s.role);
	readonly via = $derived(VIA[session.s.via] ?? session.s.via);
	readonly viaShort = $derived(VIA_SHORT[session.s.via] ?? session.s.via);
	readonly method = $derived(session.s.authMethod ? authMethodWord(session.s.authMethod) : '');
	readonly minLength = $derived(session.s.minPassword);
	readonly hasLinkWay = $derived(session.s.ways.includes('oidc') || session.s.ways.includes('forward_auth'));
	readonly otherDevices = $derived(this.rows ? others(this.rows) : 0);
	readonly shownRows = $derived(this.rows ? (this.allDevices ? this.rows : this.rows.slice(0, DEVICES_FIRST)) : null);
	readonly hiddenDevices = $derived(this.rows ? Math.max(0, this.rows.length - (this.shownRows?.length ?? 0)) : 0);
	readonly myAsks = $derived((this.asks ?? []).filter((a) => a.mine).sort((a, b) => b.asked_at.localeCompare(a.asked_at)));
	readonly trailerWord = $derived(TRAILER_OPTIONS.find((o) => o.value === this.pref)?.label ?? 'Auto');

	/** start loads what the page shows; call once when the page opens. */
	start(opts: { asks?: boolean } = {}) {
		void session.loadWays().then(() => {
			if (page.url.searchParams.get('sso') === 'finish') void this.finishOIDCLink();
		});
		api.get<{ email: string; discord_id: string; new_episodes?: boolean }>('/my/alerts').then((r) => (this.targets = r)).catch(() => {});
		api.get<{ follows: Followed[] }>('/my/follows').then((r) => (this.follows = r.follows)).catch(() => {});
		api.get<{ show_my_name: boolean }>('/my/ask-privacy').then((r) => (this.showMyName = r.show_my_name)).catch(() => {});
		void this.loadSessions();
		if (opts.asks) void this.loadAsks();
	}

	async loadAsks() {
		try {
			this.asks = (await api.get<{ asks: Ask[] }>('/asks')).asks;
		} catch {
			this.asks = [];
		}
	}

	strong(e: unknown): string {
		return e instanceof ApiError && e.status === 403 && session.s.via === 'local_network' ? 'Sign in with your password for this.' : message(e);
	}

	setPref = (v: AutoplayPref) => {
		this.pref = v;
		setAutoplayPref(v);
		location.reload();
	};

	setShowMyName = async (on: boolean) => {
		try {
			this.showMyName = (await api.put<{ show_my_name: boolean }>('/my/ask-privacy', { show_my_name: on })).show_my_name;
		} catch (err) {
			this.showMyName = !on;
			toasts.push(message(err), 'fail');
		}
	};

	unfollow = async (f: Followed) => {
		try {
			await setFollow(api, f.title_id, false);
			this.follows = this.follows.filter((x) => x.title_id !== f.title_id);
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	};

	resetRecommendations = async () => {
		this.resetBusy = true;
		try {
			await api.del('/explore/strip/counts');
			toasts.push('Your recommendations start fresh.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			this.resetBusy = false;
		}
	};

	saveTargets = async (e: SubmitEvent): Promise<boolean> => {
		e.preventDefault();
		this.tgBusy = true;
		this.tgError = '';
		try {
			this.targets = await api.put('/my/alerts', this.targets);
			toasts.push('Saved where Alerts reach you.', 'ok');
			return true;
		} catch (err) {
			this.tgError = message(err);
			return false;
		} finally {
			this.tgBusy = false;
		}
	};

	changePassword = async (e: SubmitEvent): Promise<boolean> => {
		e.preventDefault();
		this.pwError = '';
		if (this.next.length < this.minLength) {
			this.pwError = `The new password needs at least ${this.minLength} characters.`;
			return false;
		}
		if (this.next !== this.again) {
			this.pwError = 'The two new passwords are not the same.';
			return false;
		}
		this.pwBusy = true;
		try {
			await api.post('/auth/password', { current: this.current, new: this.next });
			this.current = this.next = this.again = '';
			toasts.push('Password changed. Your other devices are signed out.', 'ok');
			void this.loadSessions();
			return true;
		} catch (err) {
			this.pwError = this.strong(err);
			return false;
		} finally {
			this.pwBusy = false;
		}
	};

	startTwoFactor = async () => {
		this.tfError = '';
		this.tfBusy = true;
		try {
			this.setup = await api.post<{ secret: string; uri: string }>('/auth/two-factor/setup');
		} catch (err) {
			this.tfError = this.strong(err);
		} finally {
			this.tfBusy = false;
		}
	};

	enable = async (e: SubmitEvent) => {
		e.preventDefault();
		this.tfError = '';
		this.tfBusy = true;
		try {
			const r = await api.post<{ backup_codes: string[] }>('/auth/two-factor/enable', { code: this.code.trim() });
			this.codes = r.backup_codes;
			this.setup = null;
			this.code = '';
			await session.load();
			toasts.push('Two-factor sign-in is on.', 'ok');
		} catch (err) {
			this.tfError = this.strong(err);
		} finally {
			this.tfBusy = false;
		}
	};

	withCode = async (e: SubmitEvent) => {
		e.preventDefault();
		const path = this.offMode === 'off' ? '/auth/two-factor/disable' : '/auth/two-factor/backup-codes';
		this.tfError = '';
		this.tfBusy = true;
		try {
			const r = await api.post<{ backup_codes?: string[] } | undefined>(path, { code: this.offCode.trim() });
			this.offCode = '';
			if (r?.backup_codes) {
				this.codes = r.backup_codes;
				toasts.push('New backup codes made. The old ones no longer work.', 'ok');
			} else {
				this.codes = [];
				await session.load();
				toasts.push('Two-factor sign-in is off.', 'ok');
			}
		} catch (err) {
			this.tfError = this.strong(err);
		} finally {
			this.tfBusy = false;
		}
	};

	copyCodes = async () => {
		try {
			await navigator.clipboard.writeText(this.codes.join('\n'));
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copy did not work here; write the codes down by hand.', 'fail');
		}
	};

	reauthenticate = async (e: SubmitEvent): Promise<boolean> => {
		e.preventDefault();
		this.reauthError = '';
		this.reauthBusy = true;
		try {
			await session.reauth(this.reauthPassword, this.reauthCode.trim() || undefined);
			this.reauthPassword = '';
			this.reauthCode = '';
			toasts.push('Your local sign-in was confirmed.', 'ok');
			return true;
		} catch (err) {
			this.reauthError = message(err);
			this.reauthPassword = '';
			this.reauthCode = '';
			return false;
		} finally {
			this.reauthBusy = false;
		}
	};

	startOIDCLink = async () => {
		this.linkError = '';
		this.linkBusy = true;
		try {
			const { auth_url } = await session.oidcLink();
			window.location.assign(auth_url);
		} catch (err) {
			this.linkError = message(err);
			this.linkBusy = false;
		}
	};

	finishOIDCLink = async () => {
		this.linkError = '';
		this.linkBusy = true;
		try {
			await session.oidcLinkFinish();
			toasts.push('Single sign-on linked.', 'ok');
			await goto(to('/account'), { replace: true });
		} catch (err) {
			this.linkError = message(err);
		} finally {
			this.linkBusy = false;
		}
	};

	linkForwardAuth = async () => {
		this.linkError = '';
		this.linkBusy = true;
		try {
			await session.forwardLink();
			toasts.push('Trusted proxy sign-in linked.', 'ok');
		} catch (err) {
			this.linkError = message(err);
		} finally {
			this.linkBusy = false;
		}
	};

	clearPick = () => {
		if (this.preview) URL.revokeObjectURL(this.preview);
		this.picked = null;
		this.preview = '';
		if (this.avatarInput) this.avatarInput.value = '';
	};

	pickAvatar = (e: Event) => {
		const f = (e.currentTarget as HTMLInputElement).files?.[0];
		if (!f) return;
		this.clearPick();
		if (!/^image\/(jpeg|png|webp)$/.test(f.type)) return void toasts.push('Choose a JPEG, PNG or WebP picture.', 'fail');
		if (f.size > AVATAR_MAX) return void toasts.push('That picture is over 5 MB.', 'fail');
		this.picked = f;
		this.preview = URL.createObjectURL(f);
	};

	choosePicture = () => this.avatarInput?.click();

	saveAvatar = async () => {
		if (!this.picked) return;
		this.avatarBusy = true;
		try {
			const r = await api.request<{ avatar: string | null }>('/account/avatar', { method: 'PUT', raw: this.picked });
			session.setAvatar(r.avatar);
			this.clearPick();
			toasts.push('Profile picture saved.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			this.avatarBusy = false;
		}
	};

	removeAvatar = async () => {
		this.avatarBusy = true;
		try {
			await api.del('/account/avatar');
			session.setAvatar(null);
			this.clearPick();
			toasts.push('Profile picture removed.', 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			this.avatarBusy = false;
		}
	};

	loadSessions = async () => {
		try {
			this.rows = order((await api.get<{ sessions: SessionRow[] }>('/auth/sessions')).sessions);
			this.rowsError = '';
		} catch (e) {
			this.rowsError = message(e);
		}
	};

	endSession = async (s: SessionRow) => {
		this.rowsBusy = true;
		try {
			await api.del(`/auth/sessions/${encodeURIComponent(s.id)}`);
			toasts.push(`Signed out ${s.device}.`, 'ok');
			await this.loadSessions();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			this.rowsBusy = false;
		}
	};

	endOtherSessions = async () => {
		this.rowsBusy = true;
		try {
			const r = await api.del<{ ended: number }>('/auth/sessions');
			toasts.push(endedWords(r.ended), 'ok');
			await this.loadSessions();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			this.rowsBusy = false;
		}
	};
}
