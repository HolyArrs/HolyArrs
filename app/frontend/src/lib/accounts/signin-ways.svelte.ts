// The sign-in tiles that sign in by themselves: Plex (its window), single sign-on (away and back) and the trusted proxy.
import { tick } from 'svelte';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { session } from '#lib/core/session.svelte.ts';
import { safeNext } from './next.ts';
import { plexSignIn } from './signin-plex.ts';
import type { Way } from './signin-art.ts';

export function createSigninWays(ctx: any) {
	let plexWaiting = $state(false);
	let ssoFinishing = $state(false);
	let plexRun: { stop: () => void } | undefined;

	async function finishOIDC() {
		ctx.lastWay = 'oidc';
		ctx.error = '';
		ssoFinishing = true;
		try {
			ctx.took(await session.oidcFinish());
		} catch (err) {
			ctx.failed(err);
		} finally {
			ssoFinishing = false;
		}
	}

	// What single sign-on sent this page here for (?sso=).
	function arrive(sso: string | null) {
		if (sso === 'finish') return void finishOIDC();
		if (sso === 'start') {
			ctx.lastWay = 'oidc';
			continueOIDC();
		} else if (sso === 'failed' || sso === 'elsewhere') {
			// 'elsewhere': the browser that came back is not the one that started (a home-screen app, an in-app browser), or it took too long.
			ctx.lastWay = 'oidc';
			ctx.error = t(sso === 'elsewhere' ? 'accounts.login.ssoElsewhere' : 'accounts.login.ssoFailed');
		}
	}

	function press(w: Way) {
		ctx.lastWay = w;
		if (w === 'plex') void plex();
		else if (w === 'oidc') void oidc();
		else if (w === 'forward_auth') void forwardAuth();
		else ctx.pick(w);
	}

	function cancelPlex() {
		plexRun?.stop();
		plexWaiting = false;
		// Focus goes back to the Plex button (or the first choice), not the page.
		const panelEl: HTMLElement | undefined = ctx.panelEl;
		void tick().then(() => (panelEl?.querySelector<HTMLElement>('[data-way="plex"]') ?? panelEl?.querySelector<HTMLElement>('button:not([disabled])'))?.focus());
	}

	function plex() {
		ctx.error = '';
		// First run: the Plex account becomes the owner.
		const setup = ctx.mode === 'setup';
		if (setup) ctx.firstRun = true;
		plexRun = plexSignIn({ setup, waiting: (on) => (plexWaiting = on), done: ctx.took, failed: ctx.failed, timedOut: () => (ctx.error = t('accounts.security.plexTimeout')) });
	}

	// sent: this page was opened by the address the sign-in was pressed at (?sso=start), with fresh when it is a proof.
	async function oidc(sent = false, fresh = false) {
		ctx.error = '';
		ctx.busy = true;
		try {
			const { auth_url, continue_at } = await session.oidcStart(fresh);
			if (auth_url) return window.location.assign(auth_url);
			// The provider answers at another address of this server, and only a browser there can finish: go on there. Sent here already and still not there: stop, no loop.
			if (!continue_at || sent) throw new Error(t('accounts.login.ssoFailed'));
			window.location.assign(continue_at);
		} catch (err) {
			ctx.failed(err);
			ctx.busy = false;
		}
	}

	// Goes on with a single sign-on pressed at another address of this server, once.
	function continueOIDC() {
		const here = new URL(window.location.href);
		const fresh = here.searchParams.get('fresh') === '1';
		const next = here.searchParams.get('next');
		try {
			if (next) sessionStorage.setItem('holyarrs-sso-next', safeNext(next));
		} catch {
			// No storage: Home instead.
		}
		// Out of the address first, so Back from the provider or a reload does not start again.
		for (const k of ['sso', 'fresh']) here.searchParams.delete(k);
		history.replaceState(history.state, '', here.pathname + here.search);
		void oidc(true, fresh);
	}

	async function forwardAuth() {
		ctx.error = '';
		ctx.busy = true;
		try {
			ctx.took(await session.forwardLogin());
		} catch (err) {
			ctx.failed(err);
		} finally {
			ctx.busy = false;
		}
	}

	return {
		get plexWaiting() { return plexWaiting; },
		get ssoFinishing() { return ssoFinishing; },
		stop: () => plexRun?.stop(),
		arrive,
		press,
		cancelPlex,
		plex,
	};
}
