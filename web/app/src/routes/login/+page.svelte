<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { loaderReady, playHandoff, playSignIn, SIGNIN_HOLD_MS, VEIL_MS } from '#lib/loader.ts';
	import { page } from '$app/state';
	import { ApiError, message } from '#lib/api.ts';
	import { session, type SignInResult } from '#lib/session.svelte.ts';
	import { safeNext } from '#lib/next.ts';
	import SigninBackdrop from '#lib/SigninBackdrop.svelte';
	import SigninPortals from '#lib/SigninPortals.svelte';
	import { SIGNIN_GL_EVENT, providerTiles, type Way } from '#lib/signinArt.ts';
	import { pixelTrail, pixelIn, blockDissolve, siteCell } from '#lib/pixelTrail.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);

	let name = $state('');
	let pw = $state('');
	let pwAgain = $state('');
	let code = $state('');
	let error = $state('');
	let busy = $state(false);
	// Counts failed sign-ins; each new one shakes and flickers the pressed button.
	let failN = $state(0);
	let lastWay = $state<Way | ''>('');
	// A tile that signs in by itself (not the form) owns the busy and failure looks.
	const tileWay = $derived(lastWay === 'plex' || lastWay === 'oidc' || lastWay === 'forward_auth');
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
	const tiles = $derived(providerTiles(session.s.ways as Way[]));
	const formOn = $derived(session.s.ways.includes('password'));
	// The password form shows under the tiles when the HolyArrs password is on.
	const via = $derived(chosen === 'choose' && (formOn || !outside.length) ? 'password' : chosen);
	// One key per step: the panel's content rises in when it changes.
	const step = $derived(plexWaiting ? 'plex' : `${mode}-${via}`);
	let calm = $state(false);
	// The first step is there on the first frame; only later step changes rise in.
	let firstStep = '';
	const rise = $derived.by(() => {
		if (session.s.state === 'unknown') return { duration: 0 };
		if (!firstStep) firstStep = step;
		return { duration: calm || step === firstStep ? 0 : 300 };
	});
	let leaving = $state(false);
	let panelEl = $state<HTMLDivElement>();
	let coverEl = $state<HTMLDivElement>();
	let left = false;

	function next(): string {
		const n = page.url.searchParams.get('next') ?? '/';
		// Only paths inside this app; never another site.
		return safeNext(n);
	}

	// The card is there on the first frame; its contents come in as a soft
	// stagger (logo, tiles, "or", fields, button: 70 ms apart, done in about
	// 1 s). Focus goes to the name field at once, so typing never waits.
	let intro = $state(true);
	// The stagger waits (paused, at its first frame) while the wallpaper's
	// wave shader compiles: on a first visit the GPU is busy with it for about
	// 100 ms, and a fade caught in that would hitch. SigninBackdrop signals
	// when it is done; repeat visits (shader cached) wait two frames at most.
	let glWait = $state(true);
	const GL_WAIT_CAP_MS = 450;
	let staggered = false;
	$effect(() => {
		if (staggered || session.s.state === 'unknown' || !panelEl) return;
		staggered = true;
		const parts = panelEl.querySelectorAll<HTMLElement>(
			'.title, .lead, .provs, .or, .field, .submit, .step .link, .help'
		);
		parts.forEach((el, i) => el.style.setProperty('--i', String(Math.min(i + 1, 9))));
		// Later step changes use their own rise, not this entrance.
		setTimeout(() => (intro = false), 1400);
	});
	let focusedOnce = false;
	$effect(() => {
		if (focusedOnce || session.s.state === 'unknown' || !panelEl) return;
		const field = panelEl.querySelector<HTMLInputElement>('#name');
		if (!field) return;
		focusedOnce = true;
		if (!document.activeElement || document.activeElement === document.body) field.focus({ preventScroll: true });
	});
	onMount(() => {
		calm = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		const go = () => (glWait = false);
		if (calm || document.documentElement.dataset.signinGl) go();
		window.addEventListener(SIGNIN_GL_EVENT, go, { once: true });
		const cap = setTimeout(go, GL_WAIT_CAP_MS);
		const t = setInterval(() => (now = Date.now()), 1000);
		if (page.url.searchParams.get('sso') === 'finish') void finishOIDC();
		return () => {
			clearInterval(t);
			clearTimeout(cap);
			window.removeEventListener(SIGNIN_GL_EVENT, go);
			clearInterval(plexTimer);
			// Elsewhere the boot screen works as usual (reconnecting, slow page changes).
			document.documentElement.classList.remove('hl-skip');
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
		if (left) return;
		left = true;
		// Success, one continuous move: the Orbit grows out of the Sign in
		// button and glides to the centre while the panel and the art dissolve
		// under a veil in the app colour; the app opens under the same Orbit,
		// which then leaves as the first page rises (lib/loader.ts).
		const target = to(firstRun ? '/welcome?first=1' : next());
		if (calm) {
			playSignIn(session.s.name, SIGNIN_HOLD_MS); // reduced motion: a plain crossfade, no long hold
			void goto(target, { replace: true }).finally(loaderReady);
			return;
		}
		const btn = panelEl?.querySelector<HTMLElement>('button[type=submit]') ?? panelEl ?? null;
		playHandoff(btn ? btn.getBoundingClientRect() : null, session.s.name);
		leaving = true;
		void tick().then(() => {
			if (panelEl) void blockDissolve(panelEl, { dir: -1, duration: 320, cell: 24 });
			if (coverEl) void blockDissolve(coverEl, { dir: 1, duration: 380, delay: 120, cell: siteCell(window.innerWidth) });
		});
		setTimeout(() => void goto(target, { replace: true }).finally(loaderReady), VEIL_MS);
	});

	function took(r: SignInResult) {
		if (!r.done) {
			ticket = r.ticket;
			otp = '';
		}
	}

	function failed(err: unknown) {
		failN++;
		if (err instanceof ApiError && err.status === 429) {
			waitUntil = Date.now() + (err.retryAfter ?? 30) * 1000;
			now = Date.now();
			error = 'Too many failed sign-ins. Wait a little and try again.';
		} else {
			error = message(err);
		}
	}

	async function submit(e: SubmitEvent) {
		lastWay = '';
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
		lastWay = '';
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
		lastWay = w;
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

<main class="wrap" class:leaving data-theme="dark">
	<SigninBackdrop />
	<div class="cover" bind:this={coverEl} aria-hidden="true"></div>
	<div class="panel" class:intro class:gl-wait={glWait} bind:this={panelEl} data-signin-panel>
		<div class="brand">
			<img src={to('/brand/mark-color-on-dark.svg')} alt="" width="36" height="36" />
			<span>HolyArrs</span>
		</div>

		{#if session.s.state === 'unknown' || ssoFinishing}
			<!-- As tall as the usual sign-in form, so the card does not jump when the session answers. -->
			<div class="sk" style="border-radius: var(--r-card)"></div>
			{#if ssoFinishing}<p class="small muted" role="status">Finishing your sign-in…</p>{/if}
		{:else}
			{#key step}
				<div class="step" in:pixelIn={rise}>
					{#if plexWaiting}
						<div class="wait" role="status" aria-live="polite" data-testid="plex-wait">
							<div class="ring" aria-hidden="true">
								<span class="orbit"></span>
								<svg viewBox="0 0 24 24"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
							</div>
							<h1 class="title">Waiting for Plex</h1>
							<p class="lead">Finish signing in on the Plex window that opened, then come back here.</p>
							<button class="link" type="button" onclick={cancelPlex}>Cancel</button>
						</div>
					{:else}
						<h1 class="title">{mode === 'setup' ? 'Set up HolyArrs' : mode === 'code' ? 'Two-factor sign-in' : via === 'jellyfin' || via === 'emby' ? `Sign in with ${wayWord[via]}` : 'Sign in'}</h1>
						{#if mode === 'setup'}
							<p class="lead">First start. Make the owner account: the one person who runs this HolyArrs.</p>
						{:else if mode === 'code'}
							<p class="lead">Type the six-digit code from your authenticator app, or one of your backup codes.</p>
						{:else if via === 'jellyfin' || via === 'emby'}
							<p class="lead">Use your {wayWord[via]} name and password.</p>
						{:else if via === 'choose'}
							<p class="lead">Choose how to sign in.</p>
						{/if}

						{#if mode === 'code'}
							<form onsubmit={submitCode} novalidate>
								<div class="field">
									<label for="otp">Code</label>
									<input id="otp" class="input mono otp" inputmode="numeric" bind:value={otp} autocomplete="one-time-code" maxlength="11" required />
								</div>
								{#if error}
									<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}</p>
								{/if}
								<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}><Button variant="primary" size="lg" full type="submit" disabled={waiting > 0 || !otp.trim()} {busy}>Sign in</Button></div>
								<button class="link" type="button" onclick={restart}>Start again</button>
							</form>
						{:else}
							{#if mode === 'login' && (via === 'password' || via === 'choose') && tiles.length}
								<div class="provs" data-n={tiles.length} data-testid="signin-ways">
									{#each tiles as t (t.way)}
										<button class="prov" type="button" use:pixelTrail={{ busy: busy && lastWay === t.way, fail: lastWay === t.way ? failN : 0 }} data-way={t.way} disabled={busy} onclick={() => press(t.way)}>
											{#if t.way === 'plex'}
												<svg viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
											{:else if t.way === 'jellyfin'}
												<svg viewBox="0 0 24 24" aria-hidden="true"><defs><linearGradient id="jg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg)" /></svg>
											{:else if t.way === 'emby'}
												<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>
											{:else if t.way === 'oidc'}
												<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linecap="round"><circle cx="8" cy="12" r="4" /><path d="M12 12h9M18 12v3M21 12v2" /></svg>
											{:else}
												<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linejoin="round"><path d="M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z" /></svg>
											{/if}
											<span>{t.label}</span>
										</button>
									{/each}
								</div>
								{#if via === 'password'}<div class="or">or</div>{/if}
							{/if}

							{#if via !== 'choose' || mode === 'setup'}
								<form onsubmit={submit} novalidate>
									{#if mode === 'setup'}
										<div class="field">
											<label for="code">Setup code</label>
											<input id="code" class="input mono" bind:value={code} autocomplete="one-time-code" spellcheck="false" required />
											<span class="hint">Printed in the HolyArrs log at start, and saved in <code>setup-code.txt</code> in the config folder.</span>
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
										<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}</p>
									{/if}
									<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}>
										<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>
											{mode === 'setup' ? 'Make the owner and sign in' : via === 'jellyfin' || via === 'emby' ? `Sign in with ${wayWord[via]}` : 'Sign in'}
										</Button>
									</div>
									{#if mode === 'login' && (via === 'jellyfin' || via === 'emby')}
										<button class="link" type="button" onclick={() => pick('choose')}>Other ways to sign in</button>
									{/if}
								</form>
							{:else if error}
								<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono"> ({waiting}s)</span>{/if}</p>
							{/if}

							{#if mode === 'login' && via === 'password'}
								<details class="help">
									<summary>Forgot password?</summary>
									<p>
										Ask the owner. The owner runs <code>holyarrs reset-owner</code> on the machine that runs HolyArrs. It asks for a new name and
										password, turns off two-factor sign-in, and signs out every device and API key.
									</p>
									<p data-testid="reset-owner-help">
										In Docker: <code>docker exec -i holyarrs /holyarrs reset-owner</code>, then type the name on one line and the new password on the
										next, and press Ctrl+D. It works while HolyArrs runs; nothing else changes.
									</p>
								</details>
							{/if}
						{/if}
					{/if}
				</div>
			{/key}
		{/if}
	</div>
	<SigninPortals />
</main>

<style>
	/* Direction B2: one cinematic backdrop, the panel always centred on dark
	   hairline glass. The page stays dark in light mode too. */
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
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one
	   before; the logo settles from a hair smaller instead. Opacity and
	   transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		.intro .brand { animation: brand-in 520ms cubic-bezier(0.16, 1, 0.3, 1) 60ms both; }
		.intro :is(.title, .lead, .provs, .or, .field, .submit, .step .link, .help) {
			animation: part-in 460ms cubic-bezier(0.22, 1, 0.36, 1) calc(90ms + var(--i, 1) * 70ms) both;
		}
	}
	.gl-wait .brand,
	.gl-wait :is(.title, .lead, .provs, .or, .field, .submit, .step .link, .help) { animation-play-state: paused; }
	@keyframes brand-in { from { opacity: 0; transform: scale(0.96); } }
	@keyframes part-in { from { opacity: 0; transform: translateY(6px); } }
	/* app.html painted the wallpaper's mosaic behind the page: let it show. */
	:global(html.sa-seeded) .wrap { background: transparent; }
	.panel {
		position: relative;
		z-index: 1;
		/* Visual width clamp(360px, 24vw, 440px); type and inputs scale with
		   --z on big screens (zoom multiplies the width, so divide it out). */
		--z: 1;
		zoom: var(--z);
		width: min(calc(clamp(360px, 24vw, 440px) / var(--z)), calc(100% / var(--z)));
		padding: 28px 28px 22px;
		border-radius: 28px;
		/* Glass: a light tint over a deep blur, so the wallpaper's colour comes
		   through; the darkening brightness() keeps white text above AA on bright art. */
		background: rgb(14 14 18 / 0.46);
		-webkit-backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		border: 1px solid rgb(255 255 255 / 0.07);
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.35), 0 30px 90px rgb(0 0 0 / 0.55), 0 6px 18px rgb(0 0 0 / 0.35);
	}
	/* Dev comparison (?panel=blur, set by SigninPortals): a heavier frosted material. */
	.panel[data-mat='blur'] {
		background: rgb(40 40 48 / 0.32);
		-webkit-backdrop-filter: blur(80px) saturate(1.3) brightness(0.62);
		backdrop-filter: blur(80px) saturate(1.3) brightness(0.62);
	}
	/* Success: the panel breaks into blocks, then the dark closes over the
	   art in the site's hero cells (blockDissolve masks both). */
	.cover {
		position: fixed;
		inset: 0;
		z-index: 2;
		background: #0b0b0c;
		pointer-events: none;
		display: none;
	}
	.wrap.leaving .cover { display: block; }
	:global(html[data-theme='light']) .cover { background: #f5f5f7; }
	/* A 44 px blur re-rendered under a changing mask costs ~30 ms a frame; the
	   panel is breaking up anyway, so it leaves on its solid fallback colour. */
	.wrap.leaving .panel { -webkit-backdrop-filter: none; backdrop-filter: none; background: rgb(18 18 22 / 0.92); }
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.panel { background: rgb(18 18 22 / 0.92); }
	}
	.brand {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 11px;
		margin-bottom: 22px;
		font: 650 24px/1 var(--font-ui);
		letter-spacing: -0.015em;
		color: #fff;
	}
	.panel > .sk { height: 311px; }
	@media (max-width: 600px) { .panel > .sk { height: 289px; } }
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; }
	.lead { margin: 8px 0 20px; color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; }
	.title + :not(.lead) { margin-top: 20px; }
	.panel :global(.field + .field) { margin-top: 12px; }
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 52px; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	.otp { text-align: center; font-size: 22px; letter-spacing: 0.3em; }
	.submit { margin-top: 20px; }
	code { font-family: var(--font-mono); color: var(--text); overflow-wrap: anywhere; }

	/* Provider tiles: one row, mark over name; a lone tile is a wide button. */
	.provs { display: grid; gap: 10px; grid-template-columns: repeat(auto-fit, minmax(0, 1fr)); }
	.prov {
		position: relative;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 6px;
		min-height: 76px;
		padding: 0 6px;
		border: 1px solid rgb(255 255 255 / 0.08);
		border-radius: 16px;
		background: rgb(118 118 128 / 0.22);
		color: rgb(255 255 255 / 0.78);
		font: 500 12px/1.2 var(--font-ui);
		cursor: pointer;
		transition: transform 160ms ease, box-shadow 160ms ease, background-color 160ms ease, color 160ms ease;
	}
	.prov svg { width: 26px; height: 26px; flex: none; }
	.provs[data-n='1'] .prov { flex-direction: row; min-height: 50px; gap: 10px; font-size: 16px; color: #fff; }
	.prov:active:not(:disabled) { transform: scale(0.97); }
	.prov:disabled { opacity: 0.5; cursor: default; }
	.prov:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.or { display: flex; align-items: center; gap: 10px; margin: 18px 0; color: rgb(255 255 255 / 0.45); font: var(--t-footnote); }
	.or::before, .or::after { content: ''; flex: 1; height: 1px; background: rgb(255 255 255 / 0.12); }

	/* Hover lives on the buttons only: a 1 px lift, a soft glow and the
	   site's pixel trail inside the button (pixelTrail.ts paints a canvas
	   clipped to the button's radius). Focus rings come from :focus-visible. */
	.submit :global(.ui-btn) { position: relative; overflow: hidden; isolation: isolate; transition: transform 160ms ease, box-shadow 160ms ease, background-color 160ms ease; }
	.prov { isolation: isolate; }
	.prov > :global(:not(canvas)),
	.submit :global(.ui-btn > :not(canvas)) { position: relative; z-index: 1; }
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		.prov:not(:disabled):hover,
		.submit :global(.ui-btn:not(:disabled):hover) {
			transform: translateY(-1px);
			box-shadow: 0 6px 22px rgb(0 0 0 / 0.35), 0 0 18px rgb(255 255 255 / 0.08);
		}
		.prov:not(:disabled):hover { background: rgb(118 118 128 / 0.34); color: #fff; }
	}

	.link {
		display: block;
		margin: 12px auto 0;
		padding: 0 10px;
		min-height: 36px;
		border: 0;
		border-radius: var(--r-sm);
		background: none;
		color: var(--accent-text);
		font: 500 14px/1 var(--font-ui);
		cursor: pointer;
	}
	.link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.help { margin-top: 12px; color: var(--text-muted); font: var(--t-footnote); }
	.help summary { cursor: pointer; min-height: 36px; display: flex; align-items: center; justify-content: center; border-radius: var(--r-sm); color: var(--accent-text); font: 500 14px/1 var(--font-ui); list-style: none; }
	.help summary::-webkit-details-marker { display: none; }
	.help summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.help p { margin-top: 8px; }

	/* The waiting step: the provider's mark inside a turning ring. */
	.wait { text-align: center; }
	.ring { position: relative; width: 64px; height: 64px; margin: 4px auto 16px; display: grid; place-items: center; }
	.ring svg { width: 34px; height: 34px; }
	.orbit {
		position: absolute;
		inset: 0;
		border-radius: 50%;
		border: 2px solid rgb(255 255 255 / 0.14);
		border-top-color: #e5a00d;
		animation: orbit 0.9s linear infinite;
	}
	@keyframes orbit { to { transform: rotate(360deg); } }

	@media (prefers-reduced-motion: reduce) {
		.panel { transition: none; }
		.panel { transform: none; }
		.orbit { animation-duration: 2.4s; }
		/* A static press: no scale, just a dimmer face; busy keeps the Button spinner. */
		.prov:active:not(:disabled),
		.submit :global(.ui-btn:active:not(:disabled)) { transform: none; filter: brightness(0.86); }
	}
	/* Large screens: type, inputs and spacing grow with the panel. */
	@media (min-width: 1600px) and (min-height: 760px) { .panel { --z: 1.08; } }
	@media (min-width: 2200px) and (min-height: 1000px) { .panel { --z: 1.2; } }
	@media (min-width: 3000px) and (min-height: 1300px) { .panel { --z: 1.32; } }
	/* Phones: a full-width bottom sheet over the art, clear of the home bar. */
	@media (max-width: 600px) {
		.wrap { place-items: end stretch; padding: 0; }
		.panel {
			--z: 1;
			width: 100%;
			margin-top: 34dvh;
			border-radius: 26px 26px 0 0;
			padding: 20px max(22px, env(safe-area-inset-right)) max(22px, calc(env(safe-area-inset-bottom) + 14px)) max(22px, env(safe-area-inset-left));
			border-width: 1px 0 0;
			box-shadow: 0 -20px 60px rgb(0 0 0 / 0.45);
		}
		/* Compact: the art keeps the top half. The mark sits on the art just above
		   the sheet (no drag handle: the sheet does not move). */
		.brand { position: absolute; left: 0; right: 0; bottom: calc(100% + 18px); margin: 0; font-size: 26px; text-shadow: 0 2px 16px rgb(0 0 0 / 0.6); }
		.brand img { width: 40px; height: 40px; filter: drop-shadow(0 2px 10px rgb(0 0 0 / 0.5)); }
		.title { font-size: 24px; }
		.lead { margin: 4px 0 14px; }
		.title + :not(.lead) { margin-top: 14px; }
		.panel :global(.field + .field) { margin-top: 10px; }
		.submit { margin-top: 16px; }
		.prov { min-height: 64px; }
		.or { margin: 14px 0; }
		.link, .help { margin-top: 6px; }
	}
</style>
