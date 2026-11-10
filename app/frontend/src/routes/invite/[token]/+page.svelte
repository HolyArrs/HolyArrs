<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { theme, alreadyLinked, inviteTiles, type Way, SigninBackdrop, SigninPortals } from '#lib/accounts/index.ts';
	import { onDestroy, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { to } from '#lib/core/paths.ts';
	import { api, ApiError, message } from '#lib/core/api.ts';
	import { session, type SignInResult } from '#lib/core/session.svelte.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);
	const linkPart = $derived(page.params.token ?? '');
	// The outside ways the owner turned on, as tiles above the name and password form.
	const tiles = $derived(inviteTiles(session.s.ways as Way[]));
	const wayWord = { jellyfin: 'Jellyfin', emby: 'Emby' } as const;

	let invite = $state<{ role: string; max_rating?: string } | null>(null);
	let gone = $state('');
	let name = $state('');
	let pw = $state('');
	let pwAgain = $state('');
	let error = $state('');
	// The outside account already has a user: the invite stays unused.
	let linked = $state(false);
	let busy = $state(false);
	// Opened while signed in: say who, and let them go home or sign out to join.
	let leavingAccount = $state(false);
	const signedInAs = $derived(session.s.state === 'signed_in' && !leavingAccount ? session.s.name : '');
	async function signOutAndJoin() {
		leavingAccount = true;
		try {
			await session.logout();
		} catch (e) {
			leavingAccount = false;
			error = message(e);
		}
	}
	// 'password' is a new name and password; Jellyfin or Emby open their own form.
	let via = $state<'password' | 'jellyfin' | 'emby'>('password');
	let plexWaiting = $state(false);
	let plexTimer: ReturnType<typeof setInterval> | undefined;

	onMount(async () => {
		// /auth/state gives both the shortest password and the ways that are on.
		void session.loadWays();
		try {
			invite = await api.get<{ role: string; max_rating?: string }>(`/auth/invites/${encodeURIComponent(linkPart)}`);
		} catch (e) {
			// A gone link (404 or 410) asks for a new one; anything else is worth a retry.
			gone = e instanceof ApiError && (e.status === 404 || e.status === 410) ? 'gone' : message(e) || t('accounts.invite.openFailed');
		}
	});
	onDestroy(() => clearInterval(plexTimer));

	const invitedAs = $derived(invite?.role === 'admin' ? t('accounts.invite.asAdmin') : invite?.max_rating ? t('accounts.invite.asKid', { max: invite.max_rating }) : t('accounts.invite.asMember'));

	// A failed join: an account that is already linked gets its own line with a sign-in link.
	function failed(err: unknown) {
		linked = alreadyLinked(err);
		error = linked ? '' : message(err);
	}

	async function done(r: SignInResult) {
		if (r.done) return void (await goto(to('/'), { replace: true }));
		// The account has two-factor on: finish with the code on the sign-in page.
		try {
			sessionStorage.setItem('holyarrs-2fa-ticket', r.ticket);
		} catch {
			// No storage: the sign-in page asks for the name and password again.
		}
		await goto(to('/login'), { replace: true });
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		linked = false;
		if (via !== 'password') return joinServer(via);
		if (!name.trim()) return void (error = t('accounts.invite.pickName'));
		if (pw.length < MIN_LENGTH) return void (error = t('accounts.invite.pwMin', { n: MIN_LENGTH }));
		if (pw !== pwAgain) return void (error = t('accounts.login.noMatch'));
		busy = true;
		try {
			await session.acceptInvite(linkPart, name.trim(), pw);
			await goto(to('/'), { replace: true });
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	async function joinServer(kind: 'jellyfin' | 'emby') {
		if (!name.trim()) return void (error = t('accounts.login.enterName', { way: wayWord[kind] }));
		busy = true;
		try {
			await done(await session.inviteMediaServer(linkPart, kind, name.trim(), pw));
		} catch (err) {
			failed(err);
			pw = '';
		} finally {
			busy = false;
		}
	}

	function press(w: Way) {
		error = '';
		linked = false;
		if (w === 'plex') return void plex();
		if (w === 'jellyfin' || w === 'emby') {
			via = w;
			name = '';
			pw = '';
		}
	}

	function back() {
		via = 'password';
		error = '';
		linked = false;
		name = '';
		pw = '';
	}

	let win: Window | null = null;
	function cancelPlex() {
		clearInterval(plexTimer);
		win?.close();
		plexWaiting = false;
	}

	// The sign-in page's PIN steps, bound to this invite.
	async function plex() {
		// Open the window while the click still counts, then point it at Plex.
		win = window.open('', 'plex-sign-in', 'width=600,height=720');
		try {
			const p = await session.invitePlexStart(linkPart);
			if (win) {
				win.opener = null; // The Plex page cannot reach back into HolyArrs.
				win.location.href = p.auth_url;
			} else window.open(p.auth_url, '_blank', 'noopener');
			plexWaiting = true;
			const started = Date.now();
			clearInterval(plexTimer);
			plexTimer = setInterval(async () => {
				if (Date.now() - started > 9 * 60 * 1000) {
					cancelPlex();
					error = t('accounts.invite.plexTimeout');
					return;
				}
				try {
					const r = await session.invitePlexFinish(linkPart, p.id);
					if (r === 'waiting') return;
					cancelPlex();
					win?.close();
					await done(r);
				} catch (err) {
					cancelPlex();
					win?.close();
					failed(err);
				}
			}, 2000);
		} catch (err) {
			win?.close();
			failed(err);
		}
	}
	let showPw = $state(false);
</script>

<svelte:head><title>{t('accounts.invite.join')}</title></svelte:head>

{#snippet problem()}
	{#if linked}
		<p class="form-error" role="alert" data-testid="invite-linked"><Words key="accounts.invite.linked" links={{ a: to('/login') }} /></p>
	{:else if error}
		<p class="form-error" role="alert">{error}</p>
	{/if}
{/snippet}

<!-- The same frame as the sign-in page: the wallpaper, the edge frame and the glass card (dark ink on light glass in light). -->
<main class="wrap" data-theme="dark">
	<SigninBackdrop />
	<div class="panel" class:light={theme.value === 'light'} data-theme={theme.value} data-signin-panel>
		<div class="brand">
			<!-- Both marks load up front, so switching the appearance never shows a blank logo. -->
			<img class="mark-dark" src={to('/brand/mark-color-on-dark.svg')} alt="" width="36" height="36" />
			<img class="mark-light" src={to('/brand/mark-color.svg')} alt="" width="36" height="36" />
			<span>HolyArrs</span>
		</div>
		{#if plexWaiting}
			<div class="wait" role="status" aria-live="polite" data-testid="plex-wait">
				<div class="ring" aria-hidden="true">
					<span class="orbit"></span>
					<svg viewBox="0 0 24 24"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
				</div>
				<h1 class="title">{t('accounts.signin.waitTitle')}</h1>
				<p class="lead">{t('accounts.signin.waitLead')}</p>
				<button class="link" type="button" onclick={cancelPlex}>{t('accounts.signin.cancel')}</button>
			</div>
		{:else}
			<h1 class="title">{t('accounts.invite.join')}</h1>
			{#if gone}
				<p class="form-error" role="alert">{gone === 'gone' ? t('accounts.invite.gone') : gone}</p>
				<p class="back"><a href={to(`/login`)}>{t('accounts.invite.goSignIn')}</a></p>
			{:else if signedInAs}
				<p class="lead" data-testid="invite-signed-in">{t('accounts.invite.signedInAs', { name: signedInAs })}</p>
				{#if error}<p class="form-error" role="alert">{error}</p>{/if}
				<Button variant="primary" size="lg" full href={to('/')}>{t('accounts.invite.goHome')}</Button>
				<button class="link" type="button" onclick={signOutAndJoin}>{t('accounts.invite.signOutJoin')}</button>
			{:else if !invite}
				<div class="sk"></div>
			{:else if via !== 'password'}
				<p class="lead">{t('accounts.invite.yourNamePw', { way: wayWord[via] })}</p>
				<form onsubmit={submit} novalidate>
					<div class="field">
						<label for="name">{t('accounts.invite.wayName', { way: wayWord[via] })}</label>
						<input id="name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="128" required />
					</div>
					<div class="field">
						<label for="password">{t('accounts.signin.passwordLabel')}</label>
						<div class="pw">
							<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" />
							<IconButton icon={showPw ? 'eyeOff' : 'eye'} label={showPw ? t('accounts.invite.hidePasswordLabel') : t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
						</div>
					</div>
					{@render problem()}
					<div class="submit"><Button variant="primary" size="lg" full type="submit" {busy}>{t('accounts.invite.joinWith', { way: wayWord[via] })}</Button></div>
				</form>
				<button class="link" type="button" onclick={back}>{t('accounts.invite.otherWays')}</button>
			{:else}
				<p class="lead">{invitedAs}</p>
				{#if tiles.length}
					<div class="provs" data-n={tiles.length} data-testid="invite-ways">
						{#each tiles as tl (tl.way)}
							<button class="prov" type="button" data-way={tl.way} disabled={busy} onclick={() => press(tl.way)}>
								{#if tl.way === 'plex'}
									<svg viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
								{:else if tl.way === 'jellyfin'}
									<svg viewBox="0 0 24 24" aria-hidden="true"><defs><linearGradient id="jg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg)" /></svg>
								{:else}
									<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>
								{/if}
								<span>{tl.label}</span>
							</button>
						{/each}
					</div>
					<div class="or">{t('accounts.login.or')}</div>
				{/if}
				<form onsubmit={submit} novalidate>
					<div class="field">
						<label for="name">{t('accounts.signin.name')}</label>
						<input id="name" class="input" bind:value={name} autocomplete="username" maxlength="64" required />
					</div>
					<div class="field">
						<label for="password">{t('accounts.signin.passwordLabel')}</label>
						<div class="pw">
							<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="new-password" placeholder={t('accounts.signin.atLeast', { n: MIN_LENGTH })} aria-describedby="pw-min" required />
							<IconButton icon={showPw ? 'eyeOff' : 'eye'} label={showPw ? t('accounts.invite.hidePasswordLabel') : t('accounts.signin.showPasswordLabel')} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
						</div>
						<span id="pw-min" class="sr-only">{t('accounts.signin.atLeast', { n: MIN_LENGTH })}</span>
					</div>
					<div class="field">
						<label for="pwAgain">{t('accounts.invite.confirm')}</label>
						<input id="pwAgain" class="input" type={showPw ? 'text' : 'password'} bind:value={pwAgain} autocomplete="new-password" required />
					</div>
					{@render problem()}
					<div class="submit"><Button variant="primary" size="lg" full type="submit" {busy}>{t('accounts.invite.joinSignIn')}</Button></div>
				</form>
			{/if}
		{/if}
	</div>
	<SigninPortals />
</main>

<style>
	/* The sign-in page's frame (routes/login): one full-bleed wallpaper, an inset edge frame, and the card on hairline glass. The page stays dark in light mode too; the card turns to light glass. */
	.wrap {
		position: relative;
		min-height: 100dvh;
		display: grid;
		justify-items: center;
		align-items: start;
		--fr: 14px;
		padding: calc(var(--fr) + max(clamp(16px, 5vh, 48px), (100dvh - 600px) / 2)) calc(clamp(16px, 5vw, 96px) + var(--fr)) calc(clamp(16px, 5vh, 48px) + var(--fr));
		background: #050507;
		color: var(--text);
		isolation: isolate;
	}
	:global(html.sa-seeded) .wrap { background: transparent; }
	.panel {
		position: relative;
		z-index: 1;
		--z: 1;
		zoom: var(--z);
		width: min(calc(clamp(360px, 24vw, 440px) / var(--z)), calc(100% / var(--z)));
		padding: 34px 34px 32px;
		border-radius: 28px;
		background: rgb(14 14 18 / 0.46);
		-webkit-backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		border: 1px solid rgb(255 255 255 / 0.07);
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.35), 0 30px 90px rgb(0 0 0 / 0.55), 0 6px 18px rgb(0 0 0 / 0.35);
	}
	.panel.light {
		background: rgb(255 253 250 / 0.66);
		-webkit-backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		border-color: rgb(255 255 255 / 0.55);
		box-shadow: 0 0 0 1px rgb(255 255 255 / 0.25), 0 24px 70px rgb(0 0 0 / 0.2), 0 4px 14px rgb(0 0 0 / 0.08);
		--focus-ring: 0 0 0 2px rgb(255 255 255 / 0.9), 0 0 0 4px #1d1d1f;
	}
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.panel { background: rgb(18 18 22 / 0.92); }
		.panel.light { background: rgb(248 246 242 / 0.94); }
	}
	.mark-light, .light .mark-dark { display: none; }
	.light .mark-light { display: block; }
	.brand { display: flex; align-items: center; justify-content: center; gap: 14px; margin-bottom: 28px; font: 650 24px/1 var(--font-ui); letter-spacing: -0.015em; color: #fff; }
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; text-wrap: balance; }
	.lead { margin: 8px 0 20px; color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; text-wrap: pretty; }
	.light .brand, .light .title { color: #111; }
	.light .lead, .light .back a { color: rgb(0 0 0 / 0.64); }
	.panel :global(.field + .field) { margin-top: 12px; }
	.panel :global(.field) { text-align: center; }
	.panel :global(.field .input) { text-align: left; }
	.light :global(.input) { background: rgb(255 255 255 / 0.6); border-color: rgb(255 255 255 / 0.75); color: #111; }
	.light :global(.input:hover) { border-color: rgb(0 0 0 / 0.16); }
	.light :global(.input:focus-visible) { border-color: var(--accent); box-shadow: 0 0 0 3px rgb(229 53 43 / 0.22); }
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 52px; }
	.pw .input::-ms-reveal { display: none; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	.submit { margin-top: 20px; }
	.submit :global(.ui-btn[aria-busy='true']:disabled) { opacity: 1; }
	.sk { height: 300px; margin-top: 20px; border-radius: var(--r-card); background: rgb(255 255 255 / 0.06); }
	.light .sk { background: rgb(0 0 0 / 0.05); }
	.form-error { margin-top: 14px; padding: 10px 14px; border-radius: 12px; background: rgb(255 69 58 / 0.12); border: 1px solid rgb(255 69 58 / 0.26); color: #ffb3ad; font: 500 13px/1.45 var(--font-ui); text-align: center; }
	.light .form-error { background: rgb(255 241 240 / 0.88); border-color: rgb(215 0 21 / 0.35); color: #9b0010; }
	.form-error :global(a) { color: inherit; font-weight: 600; text-decoration: underline; text-underline-offset: 3px; }
	.back { margin: 16px 0 0; text-align: center; font: var(--t-subhead); }
	.back a { color: rgb(255 255 255 / 0.78); text-decoration: underline; text-underline-offset: 3px; }
	/* Provider tiles, "or", the text link and the Plex wait share the sign-in page's looks (routes/login). */
	.provs { display: grid; gap: 10px; grid-template-columns: repeat(auto-fit, minmax(0, 1fr)); }
	.prov {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: flex-start;
		gap: 6px;
		min-height: 76px;
		padding: 15px 6px 10px;
		text-align: center;
		border: 1px solid rgb(255 255 255 / 0.08);
		border-radius: 16px;
		background: rgb(118 118 128 / 0.22);
		color: rgb(255 255 255 / 0.78);
		font: 500 12px/1.2 var(--font-ui);
		cursor: pointer;
		transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	.prov svg { width: 26px; height: 26px; flex: none; }
	.provs[data-n='1'] .prov { flex-direction: row; justify-content: center; padding: 0 6px; min-height: 50px; gap: 10px; font-size: 16px; color: #fff; }
	.prov:active:not(:disabled) { transform: scale(0.97); }
	.prov:disabled { opacity: 0.5; cursor: default; }
	.prov:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (hover: hover) { .prov:not(:disabled):hover { background: rgb(118 118 128 / 0.34); color: #fff; } }
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		.prov:not(:disabled):hover { transform: translateY(-1px); box-shadow: 0 6px 22px rgb(0 0 0 / 0.35), 0 0 18px rgb(255 255 255 / 0.08); }
		.light .prov:not(:disabled):hover { box-shadow: 0 4px 14px rgb(0 0 0 / 0.1), 0 1px 3px rgb(0 0 0 / 0.06); }
	}
	.light .prov { background: rgb(255 255 255 / 0.45); border-color: rgb(255 255 255 / 0.6); color: #1d1d1f; }
	.light .provs[data-n='1'] .prov { color: #111; }
	@media (hover: hover) { .light .prov:not(:disabled):hover { background: rgb(255 255 255 / 0.65); color: #111; } }
	.or { display: flex; align-items: center; gap: 10px; margin: 18px 0; color: rgb(255 255 255 / 0.45); font: var(--t-footnote); }
	.or::before, .or::after { content: ''; flex: 1; height: 1px; background: rgb(255 255 255 / 0.12); }
	.light .or { color: rgb(0 0 0 / 0.62); }
	.light .or::before, .light .or::after { background: rgb(0 0 0 / 0.14); }
	.link { display: block; margin: 12px auto 0; padding: 0 10px; min-height: 36px; border: 0; border-radius: var(--r-sm); background: none; color: var(--accent-text); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.light .link { color: #1d1d1f; font-weight: 600; text-decoration: underline; text-underline-offset: 3px; text-decoration-color: rgb(0 0 0 / 0.25); }
	@media (hover: hover) {
		.link:hover { background: rgb(255 255 255 / 0.08); }
		.light .link:hover { background: rgb(0 0 0 / 0.05); }
	}
	.wait { text-align: center; }
	.ring { position: relative; width: 64px; height: 64px; margin: 4px auto 16px; display: grid; place-items: center; }
	.ring svg { width: 34px; height: 34px; }
	.orbit { position: absolute; inset: 0; border-radius: 50%; border: 2px solid rgb(255 255 255 / 0.14); border-top-color: #e5a00d; animation: orbit 0.9s linear infinite; }
	.light .orbit { border-color: rgb(0 0 0 / 0.14); border-top-color: #e5a00d; }
	@keyframes orbit { to { transform: rotate(360deg); } }
	@media (prefers-reduced-motion: reduce) {
		.orbit { animation-duration: 2.4s; }
		.prov:active:not(:disabled) { transform: none; filter: brightness(0.86); }
	}
	@media (min-width: 601px) { .wrap { --fr: 18px; } }
	@media (min-width: 1280px) { .wrap { --fr: 22px; } }
	@media (min-width: 1440px) { .wrap { --fr: 24px; } }
	@media (min-width: 1920px) { .wrap { --fr: 32px; } }
	@media (min-width: 1600px) and (min-height: 760px) { .panel { --z: 1.08; } }
	@media (min-width: 2200px) and (min-height: 1000px) { .panel { --z: 1.2; } }
	/* Phones: a floating card at the bottom, as on the sign-in page. */
	@media (max-width: 600px) {
		.wrap { --fr: 0px; place-items: end center; padding: max(24px, env(safe-area-inset-top)) max(24px, env(safe-area-inset-right)) max(24px, calc(env(safe-area-inset-bottom) + 12px)) max(24px, env(safe-area-inset-left)); }
		.panel { width: 100%; max-width: 440px; padding: 14px 16px 16px; border-radius: 24px; }
		.brand { gap: 12px; margin-bottom: 16px; font-size: 18px; }
		.brand img { width: 26px; height: 26px; }
		.title { font-size: 20px; line-height: 1.15; }
		.lead { margin: 4px 0 10px; }
		.panel :global(.field) { gap: 2px; }
		.panel :global(.field + .field) { margin-top: 4px; }
		.panel :global(.input), .submit :global(.ui-btn) { min-height: 48px; }
		.submit { margin-top: 8px; }
		.prov { min-height: 64px; padding-top: 11px; }
		.provs[data-n='1'] .prov { min-height: 48px; }
		.or { margin: 10px 0; }
		.link { margin-top: 4px; }
	}
	/* Touch screens: a 44 px tap target for the text link. */
	@media (pointer: coarse) { .link { min-height: 44px; } }
</style>
