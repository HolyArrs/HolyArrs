<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { loaderReady, playSignIn } from '#lib/loader.ts';
	import { page } from '$app/state';
	import { ApiError, message } from '#lib/api.ts';
	import { session, type SignInResult } from '#lib/session.svelte.ts';
	import { safeNext } from '#lib/next.ts';
	import SigninBackdrop from '#lib/SigninBackdrop.svelte';
	import { providerButtons, type Way } from '#lib/signinArt.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);

	let name = $state('');
	let pw = $state('');
	let pwAgain = $state('');
	let code = $state('');
	let error = $state('');
	let busy = $state(false);
	let waitUntil = $state(0);
	let now = $state(Date.now());

	// Which way: the list of buttons, the HolyArrs password, or a Jellyfin
	// or Emby account. With only the password on, the form shows at once.
	let chosen = $state<'choose' | 'password' | 'jellyfin' | 'emby'>('choose');
	// Set once the first step passed and a two-factor code is needed.
	let ticket = $state('');
	let otp = $state('');
	let plexWaiting = $state(false);
	let ssoFinishing = $state(false);
	let plexTimer: ReturnType<typeof setInterval> | undefined;

	const mode = $derived(session.s.state === 'needs_setup' ? 'setup' : ticket ? 'code' : 'login');
	const waiting = $derived(Math.max(0, Math.ceil((waitUntil - now) / 1000)));
	const wayWord = { jellyfin: 'Jellyfin', emby: 'Emby' } as const;
	const outside = $derived(session.s.ways.filter((w) => w !== 'password'));
	const buttons = $derived(providerButtons(session.s.ways as Way[]));
	const via = $derived(chosen === 'choose' && !outside.length ? 'password' : chosen);

	function next(): string {
		const n = page.url.searchParams.get('next') ?? '/';
		// Only paths inside this app; never another site.
		return safeNext(n);
	}

	onMount(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		if (page.url.searchParams.get('sso') === 'finish') void finishOIDC();
		return () => {
			clearInterval(t);
			clearInterval(plexTimer);
		};
	});

	async function finishOIDC() {
		error = '';
		ssoFinishing = true;
		try {
			took(await session.oidcFinish());
		} catch (err) {
			failed(err);
		} finally {
			ssoFinishing = false;
		}
	}

	// Right after the owner is made, the first-run step asks which parts to use.
	let firstRun = false;
	$effect(() => {
		if (session.s.state !== 'signed_in') return;
		// Signing in plays the loading screen once while the app opens.
		playSignIn(session.s.name);
		void goto(to(firstRun ? '/welcome?first=1' : next()), { replace: true }).finally(loaderReady);
	});

	function took(r: SignInResult) {
		if (!r.done) {
			ticket = r.ticket;
			otp = '';
		}
	}

	function failed(err: unknown) {
		if (err instanceof ApiError && err.status === 429) {
			waitUntil = Date.now() + (err.retryAfter ?? 30) * 1000;
			now = Date.now();
			error = 'Too many failed sign-ins. Wait a little and try again.';
		} else {
			error = message(err);
		}
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (mode === 'setup') {
			if (pw.length < MIN_LENGTH) {
				error = `The password needs at least ${MIN_LENGTH} characters.`;
				return;
			}
			if (pw !== pwAgain) {
				error = 'The two passwords are not the same.';
				return;
			}
		}
		busy = true;
		try {
			if (mode === 'setup') {
				firstRun = true;
				await session.setup(code.trim(), name.trim(), pw);
			}
			else if (via === 'password') took(await session.login(name.trim(), pw));
			else if (via !== 'choose') took(await session.mediaServer(via, name.trim(), pw));
		} catch (err) {
			if (err instanceof ApiError && err.status === 409 && mode === 'setup') {
				error = 'The owner is already set up. Sign in instead.';
				await session.load();
			} else {
				failed(err);
			}
			pw = '';
			pwAgain = '';
		} finally {
			busy = false;
		}
	}

	async function submitCode(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			await session.loginCode(ticket, otp.trim());
		} catch (err) {
			if (err instanceof ApiError && err.status === 401 && err.message.includes('ended')) {
				ticket = '';
				pw = '';
			}
			failed(err);
			otp = '';
		} finally {
			busy = false;
		}
	}

	function restart() {
		ticket = '';
		error = '';
		pw = '';
	}

	function pick(w: 'choose' | 'password' | 'jellyfin' | 'emby') {
		chosen = w;
		error = '';
		pw = '';
	}

	function press(w: Way) {
		if (w === 'plex') void plex();
		else if (w === 'oidc') void oidc();
		else if (w === 'forward_auth') void forwardAuth();
		else pick(w);
	}

	function cancelPlex() {
		clearInterval(plexTimer);
		plexWaiting = false;
	}

	async function plex() {
		error = '';
		// Open the window while the click still counts, then point it at Plex.
		const win = window.open('', 'plex-sign-in', 'width=600,height=720');
		try {
			const p = await session.plexStart();
			if (win) {
				win.opener = null; // the Plex page cannot reach back into HolyArrs
				win.location.href = p.auth_url;
			}
			else window.open(p.auth_url, '_blank', 'noopener');
			plexWaiting = true;
			const started = Date.now();
			clearInterval(plexTimer);
			plexTimer = setInterval(async () => {
				if (Date.now() - started > 9 * 60 * 1000) {
					clearInterval(plexTimer);
					plexWaiting = false;
					error = 'The Plex sign-in took too long. Try again.';
					return;
				}
				try {
					const r = await session.plexFinish(p.id);
					if (r === 'waiting') return;
					clearInterval(plexTimer);
					plexWaiting = false;
					win?.close();
					took(r);
				} catch (err) {
					clearInterval(plexTimer);
					plexWaiting = false;
					win?.close();
					failed(err);
				}
			}, 2000);
		} catch (err) {
			win?.close();
			failed(err);
		}
	}

	async function oidc() {
		error = '';
		busy = true;
		try {
			const { auth_url } = await session.oidcStart();
			window.location.assign(auth_url);
		} catch (err) {
			failed(err);
			busy = false;
		}
	}

	async function forwardAuth() {
		error = '';
		busy = true;
		try {
			took(await session.forwardLogin());
		} catch (err) {
			failed(err);
		} finally {
			busy = false;
		}
	}
	let showPw = $state(false);
</script>

<svelte:head>
	<title>{mode === 'setup' ? 'Set up HolyArrs' : 'Sign in'} · HolyArrs</title>
</svelte:head>

<main class="wrap" data-theme="dark">
	<SigninBackdrop />
	<div class="panel rise">
		<div class="brand">
			<img src={to('/brand/mark-color-on-dark.svg')} alt="" width="40" height="40" />
			<span>HolyArrs</span>
		</div>

		{#if session.s.state === 'unknown' || ssoFinishing}
			<div class="sk" style="height: 260px; border-radius: var(--r-card)"></div>
			{#if ssoFinishing}<p class="small muted" role="status">Finishing your sign-in…</p>{/if}
		{:else}
			<h1 class="title">{mode === 'setup' ? 'Set up HolyArrs' : mode === 'code' ? 'Two-factor sign-in' : 'Sign in'}</h1>
			<p class="muted small lead">
				{#if mode === 'setup'}
					First start. Make the owner account: the one person who runs this HolyArrs.
				{:else if mode === 'code'}
					Type the six-digit code from your authenticator app, or one of your backup codes.
				{:else if via === 'jellyfin' || via === 'emby'}
					Sign in with your {wayWord[via]} name and password.
				{:else if via === 'choose'}
					Welcome back. Choose how to sign in.
				{:else}
					Welcome back.
				{/if}
			</p>

			{#if mode === 'code'}
				<form onsubmit={submitCode} novalidate>
					<div class="field">
						<label for="otp">Code</label>
						<input id="otp" class="input mono otp" inputmode="numeric" bind:value={otp} autocomplete="one-time-code" maxlength="11" required />
					</div>
					{#if error}
						<p class="form-error" role="alert">
							{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}
						</p>
					{/if}
					<div class="submit"><Button variant="primary" size="lg" full type="submit" disabled={waiting > 0 || !otp.trim()} {busy}>Sign in</Button></div>
					<div class="again"><Button variant="plain" full type="button" onclick={restart}>Start again</Button></div>
				</form>
			{:else if mode === 'login' && via === 'choose'}
				<div class="ways" data-testid="signin-ways">
					{#each buttons as b (b.way)}
						{#if b.way === 'plex' && plexWaiting}
							<div class="plex-wait" role="status" aria-live="polite">
								<span class="orbit" aria-hidden="true"></span>
								<span><b>Waiting for Plex…</b><br /><span class="xs">Finish signing in on the Plex window that opened, then come back here.</span></span>
								<Button variant="plain" size="sm" type="button" onclick={cancelPlex}>Cancel</Button>
							</div>
						{:else}
							<Button
								full
								size="lg"
								variant={b.way === 'password' ? 'plain' : 'secondary'}
								class="way way-{b.way}"
								type="button"
								data-way={b.way}
								disabled={busy}
								onclick={() => press(b.way)}
							>
								{b.label}
							</Button>
						{/if}
					{/each}
					{#if error}
						<p class="form-error" role="alert">
							{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}
						</p>
					{/if}
				</div>
			{:else}
				<form onsubmit={submit} novalidate>
					{#if mode === 'setup'}
						<div class="field">
							<label for="code">Setup code</label>
							<input id="code" class="input mono" bind:value={code} autocomplete="one-time-code" spellcheck="false" required />
							<span class="hint"
								>Printed in the HolyArrs log at start, and saved in <code>setup-code.txt</code> in the config folder.</span
							>
						</div>
					{/if}
					<div class="field">
						<label for="name">Name</label>
						<input id="name" class="input" bind:value={name} autocomplete="username" maxlength={via === 'password' ? 64 : 128} required />
					</div>
					<div class="field">
						<label for="password">Password</label>
						<div class="pw">
							{#if mode === 'setup'}
								<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="new-password" required />
							{:else}
								<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" required />
							{/if}
							<IconButton icon={showPw ? 'eyeOff' : 'eye'} label={showPw ? 'Hide password' : 'Show password'} aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
						</div>
						{#if mode === 'setup'}<span class="hint">At least {MIN_LENGTH} characters.</span>{/if}
					</div>
					{#if mode === 'setup'}
						<div class="field">
							<label for="pwAgain">Password again</label>
							<input id="pwAgain" class="input" type={showPw ? 'text' : 'password'} bind:value={pwAgain} autocomplete="new-password" required />
						</div>
					{/if}

					{#if error}
						<p class="form-error" role="alert">
							{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}
						</p>
					{/if}

					<div class="submit">
						<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>
							{mode === 'setup' ? 'Make the owner and sign in' : via === 'jellyfin' || via === 'emby' ? `Sign in with ${wayWord[via]}` : 'Sign in'}
						</Button>
					</div>
					{#if mode === 'login' && outside.length}
						<div class="again"><Button variant="plain" full type="button" onclick={() => pick('choose')}>Other ways to sign in</Button></div>
					{/if}
				</form>
			{/if}

			{#if mode === 'login' && via === 'password'}
					<details class="help small muted">
						<summary>Forgot the password?</summary>
						<p>
							Ask the owner. The owner runs <code>holyarrs reset-owner</code> on the machine that runs HolyArrs. It asks for a new name and
							password, turns off two-factor sign-in, and signs out every device and API key.
						</p>
						<p data-testid="reset-owner-help">
							In Docker: <code>docker exec -i holyarrs /holyarrs reset-owner</code>, then type the name on one line and the new
							password on the next, and press Ctrl+D. It works while HolyArrs runs; nothing else changes.
						</p>
					</details>
				{/if}
		{/if}
	</div>
</main>

<style>
	/* Direction B: one cinematic backdrop, the card on frosted glass. The
	   page stays dark in light mode too (data-theme on the wrapper). */
	.wrap {
		position: relative;
		min-height: 100dvh;
		display: grid;
		place-items: center;
		padding: clamp(16px, 5vh, 48px) clamp(16px, 5vw, 96px);
		background: #050507;
		color: var(--text);
		isolation: isolate;
	}
	.panel {
		position: relative;
		z-index: 1;
		/* Visual width clamp(360px, 26vw, 480px); type and inputs scale with
		   --z on big screens (zoom multiplies the width, so divide it out). */
		--z: 1;
		zoom: var(--z);
		width: min(calc(clamp(360px, 26vw, 480px) / var(--z)), calc(100% / var(--z)));
		padding: clamp(24px, 4vw, 36px);
		border-radius: var(--r-modal);
		background: rgb(18 18 22 / 0.62);
		-webkit-backdrop-filter: blur(28px) saturate(1.4);
		backdrop-filter: blur(28px) saturate(1.4);
		box-shadow: 0 30px 80px rgb(0 0 0 / 0.5), inset 0 0 0 1px rgb(255 255 255 / 0.09);
	}
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.panel { background: rgb(18 18 22 / 0.92); }
	}
	.brand {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 10px;
		margin-bottom: 18px;
		font: var(--t-headline);
		color: rgb(255 255 255 / 0.78);
	}
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; }
	.lead { margin: 8px 0 24px; color: rgb(255 255 255 / 0.72); font: var(--t-callout); text-align: center; }
	.panel :global(.field + .field) { margin-top: 14px; }
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 52px; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	.otp { text-align: center; font-size: 22px; letter-spacing: 0.3em; }
	.submit { margin-top: 24px; }
	.again { margin-top: 8px; }
	code { font-family: var(--font-mono); color: var(--text); overflow-wrap: anywhere; }
	.help { margin-top: 20px; color: var(--text-muted); font: var(--t-callout); }
	.help summary { cursor: pointer; min-height: 44px; display: flex; align-items: center; justify-content: center; border-radius: var(--r-sm); color: var(--accent-text); font-weight: 500; }
	.help summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.help p { margin-top: 8px; }
	.ways { display: grid; gap: 10px; }
	.ways :global(.way) { min-height: 48px; }
	.ways :global(.way:not(.way-password)) { background: rgb(255 255 255 / 0.1); border-color: rgb(255 255 255 / 0.14); color: #fff; }
	.ways :global(.way:not(.way-password):hover) { background: rgb(255 255 255 / 0.16); }
	/* Plex's own amber, used only to label its button. */
	.ways :global(.way.way-plex) { background: #e5a00d; border-color: #e5a00d; color: #1b1300; }
	.ways :global(.way.way-plex:hover) { background: #f0ad1f; }
	.ways :global(.way-password) { margin-top: 4px; }
	.plex-wait {
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: center;
		gap: 12px;
		min-height: 48px;
		padding: 10px 12px;
		border-radius: var(--r-btn);
		background: rgb(229 160 13 / 0.14);
		box-shadow: inset 0 0 0 1px rgb(229 160 13 / 0.45);
		color: #fff;
		font: var(--t-footnote);
	}
	/* The 22 px ring the Orbit uses inside buttons. */
	.orbit {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		border: 2px solid rgb(255 255 255 / 0.25);
		border-top-color: #e5a00d;
		animation: orbit 0.9s linear infinite;
	}
	@keyframes orbit { to { transform: rotate(360deg); } }
	@media (prefers-reduced-motion: reduce) { .orbit { animation-duration: 2.4s; } }
	/* Large screens: type, inputs and spacing grow with the card. */
	@media (min-width: 1600px) and (min-height: 760px) { .panel { --z: 1.08; } }
	@media (min-width: 2200px) and (min-height: 1000px) { .panel { --z: 1.2; } }
	@media (min-width: 3000px) and (min-height: 1300px) { .panel { --z: 1.32; } }
	/* Phones: a full-width bottom sheet over the art, clear of the home bar. */
	@media (max-width: 600px) {
		.wrap { place-items: end stretch; padding: 0; }
		.panel {
			--z: 1;
			width: 100%;
			margin-top: 30dvh;
			border-radius: 22px 22px 0 0;
			padding: 22px max(20px, env(safe-area-inset-right)) max(24px, calc(env(safe-area-inset-bottom) + 16px)) max(20px, env(safe-area-inset-left));
			box-shadow: 0 -20px 60px rgb(0 0 0 / 0.45), inset 0 1px 0 rgb(255 255 255 / 0.1);
		}
		.brand { margin-bottom: 10px; }
		.lead { margin-bottom: 18px; }
	}
</style>
