<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
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
	let showPw = $state(false);
</script>

<svelte:head>
	<title>{mode === 'setup' ? 'Set up HolyArrs' : 'Sign in'} · HolyArrs</title>
</svelte:head>

<main class="wrap">
	<div class="panel rise">
		<div class="brand">
			<img src={to(mark)} alt="" width="44" height="44" />
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
							{mode === 'setup' ? 'Make the owner and sign in' : via === 'password' ? 'Sign in' : `Sign in with ${wayWord[via]}`}
						</Button>
					</div>
				</form>

				{#if mode === 'login' && outside.length}
					<div class="ways">
						<p class="or"><span>or</span></p>
						{#if via !== 'password'}
							<Button full type="button" onclick={() => pick('password')}>Sign in with a HolyArrs password</Button>
						{/if}
						{#each outside as w (w)}
							{#if w === 'plex'}
								<Button full type="button" onclick={plex} disabled={plexWaiting} aria-busy={plexWaiting}>
									{plexWaiting ? 'Waiting for Plex…' : 'Sign in with Plex'}
								</Button>
							{:else if (w === 'jellyfin' || w === 'emby') && via !== w}
								<Button full type="button" onclick={() => pick(w)}>Sign in with {wayWord[w]}</Button>
							{:else if w === 'oidc'}
								<Button full type="button" onclick={oidc} disabled={busy} aria-busy={busy}>Sign in with single sign-on</Button>
							{:else if w === 'forward_auth'}
								<Button full type="button" onclick={forwardAuth} disabled={busy} aria-busy={busy}>Sign in through the trusted proxy</Button>
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
</main>

<style>
	.wrap {
		min-height: 100dvh;
		display: grid;
		place-items: center;
		padding: clamp(16px, 5vw, 48px);
		background: radial-gradient(120% 80% at 50% 0%, var(--accent-soft), transparent 60%), var(--bg);
	}
	.panel {
		width: min(420px, 100%);
		padding: clamp(24px, 5vw, 40px);
		border-radius: var(--r-modal);
		background: var(--surface-1);
		box-shadow: var(--shadow-modal), 0 0 0 1px var(--line);
	}
	.brand {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 10px;
		margin-bottom: 20px;
		font: var(--t-headline);
		color: var(--text-muted);
	}
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; }
	.lead { margin: 8px 0 24px; color: var(--text-muted); font: var(--t-callout); text-align: center; }
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
	.ways { display: grid; gap: 8px; margin-top: 20px; }
	.or { display: flex; align-items: center; gap: 12px; color: var(--text-muted); font: var(--t-footnote); }
	.or::before, .or::after { content: ''; flex: 1; height: 1px; background: var(--line); }
	@media (max-width: 480px) {
		.wrap { place-items: start center; padding: 24px 16px; }
		.panel { box-shadow: none; background: transparent; padding: 8px 0; }
	}
</style>
