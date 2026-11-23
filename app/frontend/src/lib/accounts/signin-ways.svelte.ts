// The sign-in tiles that sign in by themselves: Plex (its window), single sign-on (away and back) and the trusted proxy.
import { tick } from 'svelte';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { session } from '#lib/core/session.svelte.ts';
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

	async function oidc() {
		ctx.error = '';
		ctx.busy = true;
		try {
			const { auth_url } = await session.oidcStart();
			window.location.assign(auth_url);
		} catch (err) {
			ctx.failed(err);
			ctx.busy = false;
		}
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
		finishOIDC,
		press,
		cancelPlex,
		plex,
	};
}
