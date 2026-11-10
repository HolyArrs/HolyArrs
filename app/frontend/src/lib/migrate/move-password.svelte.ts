// The move wizard's first password and sign-in again steps.
import { goto } from '$app/navigation';
import { session } from '#lib/core/session.svelte.ts';
import { api, ApiError } from '#lib/core/api.ts';
import { plainMessage as message } from '#lib/core/settings.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { to } from '#lib/core/paths.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createMovePassword(ctx: any) {
	let mustSetPw = $state(false);
	// Set it right here: the first password, then a fresh check with it, so Move works without leaving.
	let pwA = $state('');
	let pwB = $state('');
	let pwErr = $state('');
	let pwBusy = $state(false);
	async function setFirstPw(e: SubmitEvent) {
		e.preventDefault();
		pwErr = '';
		const min = session.s.minPassword;
		if (pwA.length < min) return void (pwErr = t('migrate.useAtLeastCharacters', { n: min }));
		if (pwA !== pwB) return void (pwErr = t('migrate.passwordsDonTMatch'));
		pwBusy = true;
		try {
			await api.post('/auth/password', { new: pwA });
			session.s.hasPassword = true;
			await session.reauth(pwA);
			pwA = pwB = '';
			mustSetPw = false;
			pwSheet = false;
			if (resumeMove) {
				resumeMove = false;
				await ctx.runFlow.start();
			} else toasts.push(t('migrate.passwordSet'), 'ok');
		} catch (err) {
			// A first password needs a fresh sign-in with the media server (the server's rule).
			if (err instanceof ApiError && err.body?.code === 'reauth') pwStale = true;
			else pwErr = message(err);
		} finally {
			pwBusy = false;
		}
	}
	let pwStale = $state(false);
	let pwSheet = $state(false);
	let resumeMove = $state(false);
	async function signInAgain() {
		const back = location.pathname + location.search;
		await session.logout();
		await goto(to(`/login?next=${encodeURIComponent(back)}`));
	}

	return {
		get mustSetPw() { return mustSetPw; },
		set mustSetPw(v) { mustSetPw = v; },
		get resumeMove() { return resumeMove; },
		set resumeMove(v) { resumeMove = v; },
		get pwSheet() { return pwSheet; },
		set pwSheet(v) { pwSheet = v; },
		setFirstPw,
		get pwA() { return pwA; },
		set pwA(v) { pwA = v; },
		get pwB() { return pwB; },
		set pwB(v) { pwB = v; },
		get pwBusy() { return pwBusy; },
		set pwBusy(v) { pwBusy = v; },
		get pwStale() { return pwStale; },
		set pwStale(v) { pwStale = v; },
		signInAgain,
		get pwErr() { return pwErr; },
		set pwErr(v) { pwErr = v; },
	};
}
