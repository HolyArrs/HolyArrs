// First run on the sign-in page: the language step, Welcome, the ways to make the owner (password, Plex, Jellyfin, Emby), and the owner's ways on setup's Sign in step.
import { untrack } from 'svelte';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { ApiError, api } from '#lib/core/api.ts';
import { session, type FoundServer } from '#lib/core/session.svelte.ts';

export type SetupWay = 'password' | 'plex' | 'jellyfin' | 'emby';

// Past Welcome before the owner exists: kept in this tab's storage (?setup=account also opens it), so a reload stays on the account step with the typed names (never the passwords or the picture).
const SETUP_KEY = 'holyarrs.setup.account';
export function setupSaved(): { name?: string; display?: string; past?: boolean } | null {
	try {
		const v = sessionStorage.getItem(SETUP_KEY);
		return v ? JSON.parse(v) : null;
	} catch {
		return null;
	}
}
export function forgetSetup() {
	try {
		sessionStorage.removeItem(SETUP_KEY);
	} catch {
		// Nothing kept.
	}
}
// The account step's password: long enough, typed the same twice.
export function passwordProblem(pw: string, again: string, min: number): string {
	if (pw.length < min) return t('accounts.login.useAtLeast', { n: min });
	return pw !== again ? t('accounts.login.noMatch') : '';
}
// The owner's picture, sent once the account exists.
export async function uploadAvatar(file: File) {
	const r = await api.request<{ avatar: string | null }>('/account/avatar', { method: 'PUT', raw: file });
	session.setAvatar(r.avatar);
}
// Saves the changed owner account (back on the account step during setup), with its picture.
export async function saveAccount(a: { username: string; display: string; password: string; avatar: File | null; avatarGone: boolean }) {
	await api.put('/auth/setup/account', { username: a.username, display_name: a.display, password: a.password });
	if (a.avatar) await uploadAvatar(a.avatar);
	else if (a.avatarGone) {
		await api.del('/account/avatar');
		session.setAvatar(null);
	}
	await session.load();
}
// The language step comes first; done once per tab (kept so a reload doesn't ask again).
const LANG_KEY = 'holyarrs.setup.lang';

export function createSetupSignin(ctx: any) {
	// Setup makes the owner with a password (the account step); Plex, Jellyfin and Emby are linked on the next step.
	let setupWay = $state<'choose' | SetupWay>('password');
	// First run opens on Welcome (what HolyArrs is, who built it); Get started shows the ways to make the admin account. Back from the wizard skips it.
	// A reload lands where the person was: past Welcome only if they had gone on (Back keeps the names but not the step).
	let welcomed = $state(untrack(() => ctx.welcomed as boolean) || (setupSaved()?.past ?? setupSaved() != null));
	let langDone = $state(untrack(() => welcomed) || (() => { try { return sessionStorage.getItem(LANG_KEY) === '1'; } catch { return false; } })());
	function finishLang() {
		langDone = true;
		try { sessionStorage.setItem(LANG_KEY, '1'); } catch { /* the step then shows again after a reload */ }
		ctx.focusStep();
	}
	function backToLang() {
		langDone = false;
		try { sessionStorage.removeItem(LANG_KEY); } catch { /* fine */ }
		ctx.focusStep();
	}
	const setupName = (w: SetupWay) => (w === 'password' ? t('accounts.signin.passwordLabel') : w.charAt(0).toUpperCase() + w.slice(1));
	const setupDesc = (w: SetupWay) => (w === 'password' ? t('welcome.way.passwordWay') : t('welcome.way.server', { name: setupName(w) }));
	const setupTiles = $derived((['password', 'plex', 'jellyfin', 'emby'] as const).filter((w) => session.s.setupWays.includes(w)));
	const setupServer = $derived(setupWay === 'jellyfin' || setupWay === 'emby' ? setupWay : null);
	// The Jellyfin or Emby server's address, and the servers that answered on the network.
	let address = $state('');
	let found = $state<FoundServer[]>([]);
	let looking = $state(false);
	const foundHere = $derived(found.filter((f) => f.kind === setupServer));
	$effect(() => {
		if (ctx.mode !== 'setup' || ctx.editAcct) return;
		const v = JSON.stringify({ name: ctx.name, display: ctx.displayName, past: welcomed });
		try {
			sessionStorage.setItem(SETUP_KEY, v);
		} catch {
			// No storage: a reload starts at Welcome.
		}
	});
	// Back from that Sign in step: the Media server step, showing the ways the owner can sign in.
	let ownerWays = $state(false);
	const ownerTiles = $derived((['password', 'plex', 'jellyfin', 'emby'] as const).filter((w) => session.s.ways.includes(w)));
	function ownerPick(w: SetupWay) {
		ownerWays = false;
		if (w === 'plex') ctx.press('plex');
		else ctx.pick(w);
		ctx.focusStep();
	}

	function setupPick(w: 'choose' | SetupWay) {
		setupWay = w;
		ctx.error = '';
		ctx.pw = '';
		ctx.pwAgain = '';
		// No search on its own: the person presses refresh to look on the network.
		ctx.focusStep();
	}
	function setupPress(w: SetupWay) {
		// Every choice opens its own sign-in step; Plex's has the button that opens the Plex window.
		ctx.lastWay = w === 'plex' ? 'plex' : '';
		setupPick(w);
	}
	// Asks the network for Jellyfin and Emby servers (the refresh button next to Server).
	async function refreshServers() {
		looking = true;
		found = await session.setupFindServers();
		looking = false;
	}

	async function submitServer(e: SubmitEvent) {
		e.preventDefault();
		ctx.lastWay = '';
		ctx.error = '';
		const kind = setupServer;
		if (!kind) return;
		if (!address.trim()) {
			ctx.error = t('accounts.login.enterAddress', { way: ctx.wayWord[kind] });
			return;
		}
		if (!ctx.name.trim()) {
			ctx.error = t('accounts.login.enterName', { way: ctx.wayWord[kind] });
			return;
		}
		ctx.busy = true;
		try {
			ctx.firstRun = true;
			ctx.firstRunWay = kind;
			await session.setupMediaServer(kind, address.trim(), ctx.name.trim(), ctx.pw);
		} catch (err) {
			ctx.firstRunWay = '';
			if (err instanceof ApiError && err.status === 409) {
				ctx.error = t('accounts.login.ownerExists');
				await session.load();
			} else {
				ctx.failed(err);
			}
			ctx.pw = '';
		} finally {
			ctx.busy = false;
		}
	}

	return {
		get setupWay() { return setupWay; },
		get welcomed() { return welcomed; },
		set welcomed(v) { welcomed = v; },
		get langDone() { return langDone; },
		get setupTiles() { return setupTiles; },
		get setupServer() { return setupServer; },
		get address() { return address; },
		set address(v) { address = v; },
		get looking() { return looking; },
		get foundHere() { return foundHere; },
		get ownerWays() { return ownerWays; },
		set ownerWays(v) { ownerWays = v; },
		get ownerTiles() { return ownerTiles; },
		finishLang,
		backToLang,
		setupName,
		setupDesc,
		ownerPick,
		setupPick,
		setupPress,
		refreshServers,
		submitServer,
	};
}
