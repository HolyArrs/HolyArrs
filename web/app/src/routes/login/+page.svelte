<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, message } from '#lib/api.ts';
	import { session, type SignInResult } from '#lib/session.svelte.ts';
	import { theme } from '#lib/theme.svelte.ts';
	import { safeNext } from '#lib/next.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);

	let name = $state('');
	let pw = $state('');
	let pwAgain = $state('');
	let code = $state('');
	let error = $state('');
	let busy = $state(false);
	let waitUntil = $state(0);
	let now = $state(Date.now());

	// Which way: the HolyArrs password, or a Jellyfin or Emby account.
	let via = $state<'password' | 'jellyfin' | 'emby'>('password');
	// Set once the first step passed and a two-factor code is needed.
	let ticket = $state('');
	let otp = $state('');
	let plexWaiting = $state(false);
	let ssoFinishing = $state(false);
	let plexTimer: ReturnType<typeof setInterval> | undefined;

	const mode = $derived(session.s.state === 'needs_setup' ? 'setup' : ticket ? 'code' : 'login');
	const waiting = $derived(Math.max(0, Math.ceil((waitUntil - now) / 1000)));
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');
	const wayWord = { jellyfin: 'Jellyfin', emby: 'Emby' } as const;
	const outside = $derived(session.s.ways.filter((w) => w !== 'password'));

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
		if (session.s.state === 'signed_in') void goto(to(firstRun ? '/welcome?first=1' : next()), { replace: true });
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
			else took(await session.mediaServer(via, name.trim(), pw));
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

	function pick(w: 'password' | 'jellyfin' | 'emby') {
		via = w;
		error = '';
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
</script>

<svelte:head>
	<title>{mode === 'setup' ? 'Set up HolyArrs' : 'Sign in'} · HolyArrs</title>
</svelte:head>

<div class="wrap">
	<aside class="context" aria-hidden="true">
		<p class="eyebrow">HolyArrs</p>
		<p class="context-title">Your library, downloads and Wishlist in one place.</p>
		<p class="context-copy">One account keeps your settings, decisions and signed-in devices together.</p>
	</aside>
	<div class="panel rise">
		<div class="brand">
			<img src={to(mark)} alt="" width="44" height="44" />
			<span>HolyArrs</span>
		</div>

		{#if session.s.state === 'unknown' || ssoFinishing}
			<div class="sk" style="height: 260px"></div>
			{#if ssoFinishing}<p class="small muted" role="status">Finishing your sign-in…</p>{/if}
		{:else}
			<h1 class="h2">{mode === 'setup' ? 'Set up HolyArrs' : mode === 'code' ? 'Two-factor sign-in' : 'Sign in'}</h1>
			<p class="muted small lead">
				{#if mode === 'setup'}
					First start. Make the owner account: the one person who runs this HolyArrs.
				{:else if mode === 'code'}
					Type the six-digit code from your authenticator app, or one of your backup codes.
				{:else if via !== 'password'}
					Sign in with your {wayWord[via]} name and password.
				{:else}
					Welcome back.
				{/if}
			</p>

			{#if mode === 'code'}
				<form onsubmit={submitCode} novalidate>
					<div class="field">
						<label for="otp">Code</label>
						<input id="otp" class="input mono" bind:value={otp} autocomplete="one-time-code" maxlength="11" required />
					</div>
					{#if error}
						<p class="form-error" role="alert">
							{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}
						</p>
					{/if}
					<button class="btn btn-red btn-block submit" type="submit" disabled={busy || waiting > 0 || !otp.trim()} aria-busy={busy}>Sign in</button>
					<button class="btn btn-quiet btn-block again" type="button" onclick={restart}>Start again</button>
				</form>
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
						{#if mode === 'setup'}
							<input id="password" class="input" type="password" bind:value={pw} autocomplete="new-password" required />
						{:else}
							<input id="password" class="input" type="password" bind:value={pw} autocomplete="current-password" required />
						{/if}
						{#if mode === 'setup'}<span class="hint">At least {MIN_LENGTH} characters.</span>{/if}
					</div>
					{#if mode === 'setup'}
						<div class="field">
							<label for="pwAgain">Password again</label>
							<input id="pwAgain" class="input" type="password" bind:value={pwAgain} autocomplete="new-password" required />
						</div>
					{/if}

					{#if error}
						<p class="form-error" role="alert">
							{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}
						</p>
					{/if}

					<button class="btn btn-red btn-block submit" type="submit" disabled={busy || waiting > 0} aria-busy={busy}>
						{mode === 'setup' ? 'Make the owner and sign in' : via === 'password' ? 'Sign in' : `Sign in with ${wayWord[via]}`}
					</button>
				</form>

				{#if mode === 'login' && outside.length}
					<div class="ways">
						<p class="or xs muted"><span>or</span></p>
						{#if via !== 'password'}
							<button class="btn btn-ghost btn-block" type="button" onclick={() => pick('password')}>Sign in with a HolyArrs password</button>
						{/if}
						{#each outside as w (w)}
							{#if w === 'plex'}
								<button class="btn btn-ghost btn-block" type="button" onclick={plex} disabled={plexWaiting} aria-busy={plexWaiting}>
									{plexWaiting ? 'Waiting for Plex…' : 'Sign in with Plex'}
								</button>
							{:else if (w === 'jellyfin' || w === 'emby') && via !== w}
								<button class="btn btn-ghost btn-block" type="button" onclick={() => pick(w)}>Sign in with {wayWord[w]}</button>
							{:else if w === 'oidc'}
								<button class="btn btn-ghost btn-block" type="button" onclick={oidc} disabled={busy} aria-busy={busy}>Sign in with single sign-on</button>
							{:else if w === 'forward_auth'}
								<button class="btn btn-ghost btn-block" type="button" onclick={forwardAuth} disabled={busy} aria-busy={busy}>Sign in through the trusted proxy</button>
							{/if}
						{/each}
						{#if plexWaiting}<p class="xs muted" role="status">Finish signing in on the Plex page that opened, then come back here.</p>{/if}
					</div>
				{/if}

				{#if mode === 'login'}
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
		{/if}
	</div>
</div>

<style>
	.wrap {
		min-height: 100dvh;
		display: grid;
		grid-template-columns: minmax(280px, 1fr) minmax(360px, 440px);
		align-items: center;
		gap: clamp(var(--s-8), 8vw, 8rem);
		width: min(1120px, 100%);
		margin: 0 auto;
		padding: clamp(var(--s-6), 6vw, var(--s-10));
		background: var(--bg);
	}
	.panel {
		width: 100%;
		background: var(--surface-1);
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		padding: var(--s-8);
		box-shadow: none;
	}
	.context {
		max-width: 560px;
		padding-left: var(--s-5);
		border-left: 2px solid var(--ha-red);
	}
	.eyebrow {
		margin-bottom: var(--s-4);
		color: var(--ha-red);
		font-size: var(--text-xs);
		font-weight: var(--weight-semibold);
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}
	.context-title {
		max-width: 15ch;
		font: var(--weight-semibold) clamp(2rem, 4vw, 3.75rem) / 1.03 var(--font-display);
		letter-spacing: var(--tracking-display);
		text-wrap: balance;
	}
	.context-copy {
		max-width: 46ch;
		margin-top: var(--s-5);
		color: var(--text-muted);
		line-height: 1.6;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		margin-bottom: var(--s-6);
		font: var(--weight-bold) 22px/1 var(--font-display);
		letter-spacing: var(--tracking-display);
	}
	.lead {
		margin: var(--s-2) 0 var(--s-6);
	}
	.submit {
		margin-top: var(--s-6);
	}
	.again {
		margin-top: var(--s-2);
	}
	code {
		font-family: var(--font-mono);
		color: var(--text);
	}
	.help {
		margin-top: var(--s-5);
	}
	.help summary {
		cursor: pointer;
		min-height: 32px;
		display: inline-flex;
		align-items: center;
		border-radius: var(--r-sm);
	}
	.help p {
		margin-top: var(--s-2);
	}
	.ways {
		display: grid;
		gap: var(--s-2);
		margin-top: var(--s-5);
	}
	.or {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.or::before,
	.or::after {
		content: '';
		flex: 1;
		height: 1px;
		background: var(--line);
	}
	@media (max-width: 800px) {
		.wrap {
			grid-template-columns: minmax(0, 1fr);
			width: min(500px, 100%);
		}
		.context {
			display: none;
		}
	}
	@media (max-width: 480px) {
		.wrap {
			align-items: start;
			padding: var(--s-4);
		}
		.panel {
			padding: var(--s-6) var(--s-4);
		}
	}
</style>
