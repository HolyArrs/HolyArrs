<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { SigninBrand, SigninTheme, liftAboveKeyboard, SigninCode, SigninHelp, SigninLoginForm, SigninPerson, SigninRecent, SigninServerForm, SigninTiles, SigninWait, plexSignIn, safeNext, SigninBackdrop, SigninPortals, SIGNIN_GL_EVENT, providerTiles, type Way, forgetPerson, keepPersonPic, recentPeople, rememberPerson, type RecentPerson, type RecentWay, pixelIn, blockDissolve, siteCell, theme } from '#lib/accounts/index.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { to } from '#lib/core/paths.ts';
	import { loaderReady, playHandoff, playSignIn, SIGNIN_HOLD_MS, VEIL_MS } from '#lib/core/loader.ts';
	import { ApiError, message } from '#lib/core/api.ts';
	import { session, type FoundServer, type SignInResult } from '#lib/core/session.svelte.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);

	let name = $state('');
	// People who signed in on this device, with the way each one used.
	let recent = $state<RecentPerson[]>([]);
	// Which account view: 'auto' (one person: their step; more: the list), the list, another account (the usual form), or one person by index.
	let acct = $state<'auto' | 'list' | 'new' | number>('auto');
	let editing = $state(false);
	// The way's name for people: Password, Plex, Jellyfin, Emby, Single sign-on.
	const wayName = (w: RecentWay) => (w === 'oidc' ? t('accounts.tiles.sso') : w === 'password' ? t('accounts.signin.passwordLabel') : w.slice(0, 1).toUpperCase() + w.slice(1));
	// After a view change the pressed button is gone, so focus would drop to the page: move it to the new step's first control. Touch screens are left alone (focusing a field there pops the keyboard).
	function focusStep() {
		if (!matchMedia('(pointer: fine)').matches && document.documentElement.dataset.input !== 'keyboard') return;
		void tick().then(() => {
			for (const s of ['.digit', '#otp', '#addr', '#name', '#password', '.acct-pick', '.step button']) {
				const el = panelEl?.querySelector<HTMLElement>(s);
				if (el) return el.focus({ preventScroll: true });
			}
		});
	}
	function openPerson(i: number) {
		editing = false;
		acct = i;
		focusStep();
	}
	function showList() {
		editing = false;
		acct = 'list';
		focusStep();
	}
	function another() {
		editing = false;
		acct = 'new';
		name = '';
		chosen = 'choose';
		pw = '';
		error = '';
		focusStep();
	}
	function dropPerson(n: string) {
		forgetPerson(n);
		recent = recentPeople();
		focusStep();
	}
	let pw = $state('');
	let pwAgain = $state('');
	let error = $state('');
	let busy = $state(false);
	// Counts failed sign-ins; each new one shakes and flickers the pressed button.
	let failN = $state(0);
	let lastWay = $state<Way | ''>('');
	// A tile that signs in by itself (not the form) owns the busy and failure looks.
	const tileWay = $derived(lastWay === 'plex' || lastWay === 'oidc' || lastWay === 'forward_auth');
	let waitUntil = $state(0);
	let now = $state(Date.now());

	// Which way: the list of buttons, the HolyArrs password, or a Jellyfin or Emby account. With only the password on, the form shows at once.
	let chosen = $state<'choose' | 'password' | 'jellyfin' | 'emby'>('choose');
	// Set once the first step passed and a two-factor code is needed.
	let ticket = $state('');
	let otp = $state('');
	let plexWaiting = $state(false);
	let ssoFinishing = $state(false);
	let plexRun: { stop: () => void } | undefined;

	const mode = $derived(session.s.state === 'needs_setup' ? 'setup' : ticket ? 'code' : 'login');
	// First run: which way makes the owner. 'choose' shows them all; Plex signs in from its tile, the others open their form.
	type SetupWay = 'password' | 'plex' | 'jellyfin' | 'emby';
	let setupWay = $state<'choose' | 'password' | 'jellyfin' | 'emby'>('choose');
	const setupName = (w: SetupWay) => (w === 'password' ? t('accounts.signin.passwordLabel') : w.charAt(0).toUpperCase() + w.slice(1));
	const setupTiles = $derived((['password', 'plex', 'jellyfin', 'emby'] as const).filter((w) => session.s.setupWays.includes(w)));
	const setupServer = $derived(setupWay === 'jellyfin' || setupWay === 'emby' ? setupWay : null);
	// The Jellyfin or Emby server's address, and the servers that answered on the network.
	let address = $state('');
	let found = $state<FoundServer[]>([]);
	let looking = $state(false);
	let looked = false;
	const foundHere = $derived(found.filter((f) => f.kind === setupServer));
	// How the owner was made, for remembering them on this device.
	let firstRunWay: RecentWay | '' = '';
	const waiting = $derived(Math.max(0, Math.ceil((waitUntil - now) / 1000)));
	const wayWord = { jellyfin: 'Jellyfin', emby: 'Emby' } as const;
	const outside = $derived(session.s.ways.filter((w) => w !== 'password'));
	const tiles = $derived(providerTiles(session.s.ways as Way[]));
	const formOn = $derived(session.s.ways.includes('password'));
	// The password form shows under the tiles when the HolyArrs password is on.
	const via = $derived(chosen === 'choose' && (formOn || !outside.length) ? 'password' : chosen);
	// Remembered people whose way is on now.
	const shown = $derived(recent.filter((p) => session.s.ways.includes(p.way)));
	const acctView = $derived(
		mode !== 'login' || !shown.length || acct === 'new' ? 'none' : acct === 'list' || (acct === 'auto' && shown.length > 1) ? 'list' : 'person'
	);
	const person = $derived(acctView === 'person' ? (shown[typeof acct === 'number' ? acct : 0] ?? shown[0]) : null);
	// A chosen person fills the name and way the form signs in with.
	$effect(() => {
		const p = person;
		if (!p) return;
		untrack(() => {
			name = p.login ?? p.name;
			chosen = p.way === 'jellyfin' || p.way === 'emby' ? p.way : 'choose';
			pw = '';
			error = '';
		});
	});
	// The round back button in the panel's corner, when there is somewhere to go back to.
	const backTo = $derived.by(() => {
		if (mode === 'code') return { label: t('accounts.login.backToSignIn'), go: restart };
		if (mode === 'setup') return setupWay !== 'choose' ? { label: t('accounts.login.otherWays'), go: () => setupPick('choose') } : null;
		if (mode !== 'login' || !shown.length) return null;
		if (acctView === 'list' && shown.length === 1) return { label: t('accounts.login.backTo', { name: shown[0].name }), go: () => openPerson(0) };
		if ((acctView === 'person' && shown.length > 1) || acct === 'new') return { label: t('accounts.login.allAccounts'), go: showList };
		return null;
	});
	// One key per step: the panel's content rises in when it changes.
	const step = $derived(plexWaiting ? 'plex' : `${mode}-${mode === 'setup' ? setupWay : via}-${acctView}-${person?.name ?? ''}`);
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

	// The card is there on the first frame; its contents come in as a soft stagger (logo, tiles, "or", fields, button: 70 ms apart, done in about 1 s). Focus goes to the name field at once, so typing never waits.
	let intro = $state(true);
	// The stagger waits (paused, at its first frame) while the wallpaper's wave shader compiles: on a first visit the GPU is busy with it for about 100 ms, and a fade caught in that would hitch. SigninBackdrop signals when it is ready.
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
		const field = panelEl.querySelector<HTMLInputElement>('#name') ?? panelEl.querySelector<HTMLInputElement>('#password');
		if (!field) return;
		focusedOnce = true;
		// Phones: no focus on load, so the keyboard does not cover the art and the card.
		if (!matchMedia('(pointer: fine)').matches) return;
		if (!document.activeElement || document.activeElement === document.body) field.focus({ preventScroll: true });
	});
	onMount(() => {
		// An invite join that needs a two-factor code hands its ticket over here (never in the address).
		try {
			const saved = sessionStorage.getItem('holyarrs-2fa-ticket');
			if (saved) {
				sessionStorage.removeItem('holyarrs-2fa-ticket');
				ticket = saved;
			}
		} catch {
			// No storage: they sign in again.
		}
		calm = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		recent = recentPeople();
		const go = () => (glWait = false);
		if (calm || document.documentElement.dataset.signinGl) go();
		window.addEventListener(SIGNIN_GL_EVENT, go, { once: true });
		const cap = setTimeout(go, GL_WAIT_CAP_MS);
		const clock = setInterval(() => (now = Date.now()), 1000);
		if (page.url.searchParams.get('sso') === 'finish') void finishOIDC();
		else if (page.url.searchParams.get('sso') === 'failed') {
			lastWay = 'oidc';
			error = t('accounts.login.ssoFailed');
		}
		const unlift = liftAboveKeyboard(() => panelEl);
		return () => {
			unlift();
			clearInterval(clock);
			clearTimeout(cap);
			window.removeEventListener(SIGNIN_GL_EVENT, go);
			plexRun?.stop();
			// Elsewhere the boot screen works as usual (reconnecting, slow page changes).
			document.documentElement.classList.remove('hl-skip');
		};
	});

	async function finishOIDC() {
		lastWay = 'oidc';
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
		// Remember who signed in here, and how (the trusted proxy signs in by itself).
		const way: RecentWay | '' =
			firstRunWay ||
			(lastWay === 'plex' || lastWay === 'oidc'
				? lastWay
				: !tileWay && name.trim() && (via === 'password' || via === 'jellyfin' || via === 'emby')
					? via
					: '');
		if (way) {
			rememberPerson(session.s.name, way, way === 'jellyfin' || way === 'emby' ? name.trim() : undefined);
			void keepPersonPic(session.s.name, session.s.avatar);
		}
		// Success, one continuous move: the Orbit grows out of the Sign in button and glides to the centre while the panel and the art dissolve under a veil in the app colour; the app opens under the same Orbit, which then leaves as the page appears.
		const target = to(firstRun ? '/welcome?first=1' : next());
		if (calm) {
			playSignIn(session.s.name, SIGNIN_HOLD_MS); // Reduced motion: a plain crossfade, no long hold.
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
			useBackup = false;
			void tick().then(() => panelEl?.querySelector<HTMLInputElement>('.digit')?.focus());
		}
	}

	let useBackup = $state(false);
	function failed(err: unknown) {
		failN++;
		if (err instanceof ApiError && err.status === 429) {
			waitUntil = Date.now() + (err.retryAfter ?? 30) * 1000;
			now = Date.now();
			error = t('accounts.login.tooMany');
		} else {
			const m = message(err);
			error = m.slice(0, 1).toUpperCase() + m.slice(1);
		}
	}

	async function submit(e: SubmitEvent) {
		lastWay = '';
		e.preventDefault();
		error = '';
		if (mode === 'setup') {
			if (pw.length < MIN_LENGTH) {
				error = t('accounts.login.useAtLeast', { n: MIN_LENGTH });
				return;
			}
			if (pw !== pwAgain) {
				error = t('accounts.login.noMatch');
				return;
			}
		}
		busy = true;
		try {
			if (mode === 'setup') {
				firstRun = true;
				await session.setup(name.trim(), pw);
			}
			else if (via === 'password') took(await session.login(name.trim(), pw));
			else if (via !== 'choose') took(await session.mediaServer(via, name.trim(), pw));
		} catch (err) {
			if (err instanceof ApiError && err.status === 409 && mode === 'setup') {
				error = t('accounts.login.ownerExists');
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

	function restart() {
		ticket = '';
		error = '';
		pw = '';
		useBackup = false;
		otp = '';
		focusStep();
	}

	function pick(w: 'choose' | 'password' | 'jellyfin' | 'emby') {
		chosen = w;
		error = '';
		pw = '';
	}

	function setupPick(w: 'choose' | 'password' | 'jellyfin' | 'emby') {
		setupWay = w;
		error = '';
		pw = '';
		pwAgain = '';
		if (w === 'jellyfin' || w === 'emby') void lookForServers(w);
		focusStep();
	}

	function setupPress(w: SetupWay) {
		lastWay = w === 'plex' ? 'plex' : '';
		if (w === 'plex') void plex();
		else setupPick(w);
	}

	// Asks the network once; a single server of the chosen kind fills the address.
	async function lookForServers(kind: 'jellyfin' | 'emby') {
		if (!looked) {
			looked = true;
			looking = true;
			found = await session.setupFindServers();
			looking = false;
		}
		const mine = found.filter((f) => f.kind === kind);
		if (!address.trim() && mine.length === 1 && setupWay === kind) address = mine[0].address;
	}

	async function submitServer(e: SubmitEvent) {
		e.preventDefault();
		lastWay = '';
		error = '';
		const kind = setupServer;
		if (!kind) return;
		if (!address.trim()) {
			error = t('accounts.login.enterAddress', { way: wayWord[kind] });
			return;
		}
		if (!name.trim()) {
			error = t('accounts.login.enterName', { way: wayWord[kind] });
			return;
		}
		busy = true;
		try {
			firstRun = true;
			firstRunWay = kind;
			await session.setupMediaServer(kind, address.trim(), name.trim(), pw);
		} catch (err) {
			firstRunWay = '';
			if (err instanceof ApiError && err.status === 409) {
				error = t('accounts.login.ownerExists');
				await session.load();
			} else {
				failed(err);
			}
			pw = '';
		} finally {
			busy = false;
		}
	}

	function press(w: Way) {
		lastWay = w;
		if (w === 'plex') void plex();
		else if (w === 'oidc') void oidc();
		else if (w === 'forward_auth') void forwardAuth();
		else pick(w);
	}

	function cancelPlex() {
		plexRun?.stop();
		plexWaiting = false;
		// Focus goes back to the Plex button (or the first choice), not the page.
		void tick().then(() => (panelEl?.querySelector<HTMLElement>('[data-way="plex"]') ?? panelEl?.querySelector<HTMLElement>('button:not([disabled])'))?.focus());
	}

	function plex() {
		error = '';
		// First run: the Plex account becomes the owner.
		const setup = mode === 'setup';
		if (setup) firstRun = true;
		plexRun = plexSignIn({ setup, waiting: (on) => (plexWaiting = on), done: took, failed, timedOut: () => (error = t('accounts.security.plexTimeout')) });
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
	<title>{t('accounts.login.pageTitle', { what: mode === 'setup' ? t('accounts.login.setUp') : t('accounts.signin.signIn') })}</title>
</svelte:head>

<main class="wrap" class:leaving data-theme="dark">
	<SigninBackdrop />
	<div class="cover" bind:this={coverEl} aria-hidden="true"></div>
	<div class="panel" class:intro class:gl-wait={glWait} class:light={theme.value === 'light'} data-theme={theme.value} bind:this={panelEl} data-signin-panel>
		<SigninBrand />
		{#if backTo && session.s.state !== 'unknown' && !ssoFinishing && !plexWaiting}
			<span class="corner"><IconButton icon="chevronLeft" variant="glass" label={backTo.label} onclick={backTo.go} /></span>
		{/if}

		{#if session.s.state === 'unknown' || ssoFinishing}
			<!-- As tall as the usual sign-in form, so the card does not jump when the session answers. -->
			<div class="sk" class:tall={recent.length > 1} style="border-radius: var(--r-card)"></div>
			{#if ssoFinishing}<p class="small muted" role="status">{t('accounts.login.finishing')}</p>{/if}
		{:else}
			{#key step}
				<div class="step" in:pixelIn={rise}>
					{#if plexWaiting}
						<SigninWait {cancelPlex} />
					{:else if acctView !== 'none'}
						<!-- One person: their face and name lead; the heading stays for screen readers. -->
						<h1 class="title" class:sr-only={acctView === 'person'}>{acctView === 'list' ? t('accounts.login.who') : t('accounts.signin.signIn')}</h1>
						{#if acctView === 'list'}
							<SigninRecent {shown} {editing} {wayName} {dropPerson} {openPerson} {another} />
							<button class="link" type="button" aria-pressed={editing} onclick={() => (editing = !editing)}>{editing ? t('accounts.login.done') : t('accounts.login.editList')}</button>
						{:else if person}
							<SigninPerson {person} {name} bind:pw bind:showPw {error} {waiting} {busy} {lastWay} {tileWay} {failN} {wayName} {press} {submit} />
							{#if shown.length === 1}<button class="link" type="button" onclick={showList}>{t('accounts.login.switch')}</button>{/if}
						{/if}
						{#if formOn}<SigninHelp {calm} />{/if}
					{:else}
						<h1 class="title">{mode === 'setup' ? t('accounts.login.setUp') : mode === 'code' ? t('accounts.login.twoStep') : via === 'jellyfin' || via === 'emby' ? t('accounts.signin.signInWith', { way: wayWord[via] }) : t('accounts.signin.signIn')}</h1>
						{#if mode === 'setup'}
							<p class="lead">{setupServer ? t('accounts.login.useAdmin', { way: wayWord[setupServer] }) : t('accounts.login.createOwner')}</p>
							{#if session.s.configUnmapped}
								<p class="form-error" role="status">{t('accounts.login.unmapped')}</p>
							{/if}
						{:else if mode === 'code'}
							<p class="lead">{useBackup ? t('accounts.login.enterBackup') : t('accounts.login.enterSix')}</p>
						{:else if via === 'jellyfin' || via === 'emby'}
							<p class="lead">{t('accounts.login.useYour', { way: wayWord[via] })}</p>
						{:else if via === 'choose'}
							<p class="lead">{t('accounts.login.choose')}</p>
						{/if}

						{#if mode === 'setup' && setupWay === 'choose'}
							<SigninTiles setup {setupTiles} {setupName} {lastWay} {failN} {busy} onsetup={setupPress} />
							{#if error}
								<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
							{/if}
						{:else if mode === 'setup' && setupServer}
							<SigninServerForm kind={setupServer} bind:address bind:name bind:pw bind:showPw {foundHere} {looking} {error} {waiting} {busy} {failN} {wayWord} {submitServer} {focusStep} />
						{:else if mode === 'code'}
							<SigninCode bind:ticket bind:pw bind:otp bind:useBackup bind:busy bind:error bind:lastWay {waiting} {failN} {tileWay} {failed} />
						{:else}
							{#if mode === 'login' && (via === 'password' || via === 'choose') && tiles.length}
								<SigninTiles {tiles} {lastWay} {failN} {busy} onpress={press} />
								{#if via === 'password'}<div class="or">{t('accounts.login.or')}</div>{/if}
							{/if}

							{#if via !== 'choose' || mode === 'setup'}
								<SigninLoginForm {mode} {via} bind:name bind:pw bind:pwAgain bind:showPw {error} {waiting} {busy} {tileWay} {failN} minLength={MIN_LENGTH} {wayWord} {submit} {pick} />
							{:else if error}
								<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
							{/if}

							{#if mode === 'login' && via === 'password'}<SigninHelp {calm} />{/if}
						{/if}
					{/if}
				</div>
			{/key}
		{/if}
	</div>
	<SigninPortals />
	<SigninTheme {panelEl} />
</main>

<style>
	/* One backdrop, the panel always centred on dark hairline glass. The page stays dark in light mode too. */
	.wrap {
		position: relative;
		min-height: 100dvh;
		display: grid;
		/* The card's top stays put: a step change, an error note or the open help grows it downward (and the page scrolls) instead of re-centring it. The top offset centres a usual-height card. */
		justify-items: center;
		align-items: start;
		/* --fr mirrors the edge frame's thickness (SigninPortals), so the panel clears it. */
		--fr: 14px;
		/* Centres a usual (about 560 px) card, but never so low that the tallest (about 640 px) would not fit. */
		padding: calc(var(--fr) + max(clamp(16px, 5vh, 48px), (100dvh - 640px) / 2, min((100dvh - 560px) / 2, 100dvh - 640px - 2 * var(--fr) - clamp(16px, 5vh, 48px)))) calc(clamp(16px, 5vw, 96px) + var(--fr)) calc(clamp(16px, 5vh, 48px) + var(--fr));
		background: #050507;
		color: var(--text);
		isolation: isolate;
	}
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		.intro .title, .intro .lead, .intro .or, .intro .step .link {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	.gl-wait .title, .gl-wait .lead, .gl-wait .or, .gl-wait .step .link { animation-play-state: paused; }
	@keyframes brand-in { from { opacity: 0; transform: scale(0.96); } }
	@keyframes part-in { from { opacity: 0; transform: translateY(6px); } }
	/* app.html painted the wallpaper's mosaic behind the page: let it show. */
	:global(html.sa-seeded) .wrap { background: transparent; }
	.panel {
		position: relative;
		z-index: 1;
		/* Visual width clamp(360px, 24vw, 440px); type and inputs scale with --z on big screens (zoom multiplies the width, so divide it out). */
		--z: 1;
		zoom: var(--z);
		width: min(calc(clamp(360px, 24vw, 440px) / var(--z)), calc(100% / var(--z)));
		padding: 34px 34px 32px;
		border-radius: 28px;
		/* Glass: a light tint over a deep blur, so the wallpaper's colour comes through; the darkening brightness() keeps white text above AA on bright art. */
		background: rgb(14 14 18 / 0.46);
		-webkit-backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		backdrop-filter: blur(40px) saturate(1.7) brightness(0.72);
		border: 1px solid rgb(255 255 255 / 0.07);
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.35), 0 30px 90px rgb(0 0 0 / 0.55), 0 6px 18px rgb(0 0 0 / 0.35);
	}
	/* Dev comparison (?panel=blur, set by SigninPortals): a heavier frosted material. */
	.panel:global([data-mat='blur']) {
		background: rgb(40 40 48 / 0.32);
		-webkit-backdrop-filter: blur(80px) saturate(1.3) brightness(0.62);
		backdrop-filter: blur(80px) saturate(1.3) brightness(0.62);
	}
	/* Success: the panel breaks into blocks, then the dark closes over the art in the site's hero cells (blockDissolve masks both). */
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
	/* A 44 px blur re-rendered under a changing mask costs about 30 ms a frame; the panel is breaking up anyway, so it leaves on its solid fallback colour. */
	.wrap.leaving .panel { -webkit-backdrop-filter: none; backdrop-filter: none; background: rgb(18 18 22 / 0.92); }
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.panel { background: rgb(18 18 22 / 0.92); }
		.panel.light { background: rgb(248 246 242 / 0.94); }
}

	/* Light mode: tinted glass. The blur lifts the poster under the panel and keeps its colour; text is dark ink, links are dark and underlined (red on tinted glass reads poorly), and red stays on the main button. */
	.panel.light {
		/* Thick enough that the poster's dark patches read as tint, never as smudges behind the text. */
		background: rgb(255 253 250 / 0.66);
		-webkit-backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		backdrop-filter: blur(64px) saturate(1.5) brightness(1.5);
		border-color: rgb(255 255 255 / 0.55);
		/* A light rim and a soft lift: dark shadows make the light glass look dirty. */
		box-shadow: 0 0 0 1px rgb(255 255 255 / 0.25), 0 24px 70px rgb(0 0 0 / 0.2), 0 4px 14px rgb(0 0 0 / 0.08);
		/* Focus rings in dark ink: the red ring is about 2:1 on the light glass. */
		--focus-ring: 0 0 0 2px rgb(255 255 255 / 0.9), 0 0 0 4px #1d1d1f;
	}
	.wrap.leaving .panel.light { background: rgb(250 250 250 / 0.92); }
	.light .title { color: #111; }
	.light .lead { color: rgb(0 0 0 / 0.64); }
	.light .or { color: rgb(0 0 0 / 0.62); }
	.light .or::before, .light .or::after { background: rgb(0 0 0 / 0.14); }
	.light :global(.input) { background: rgb(255 255 255 / 0.6); border-color: rgb(255 255 255 / 0.75); color: #111; }
	/* The resting light field above outranks app.css's hover, focus and error borders, so give them back. */
	.light :global(.input:hover) { border-color: rgb(0 0 0 / 0.16); }
	.light :global(.input:focus-visible) { border-color: var(--accent); box-shadow: 0 0 0 3px rgb(229 53 43 / 0.22); }
	.light :global(.input[aria-invalid='true']) { border-color: var(--fail); }
	.light .link {
		color: #1d1d1f;
		font-weight: 600;
		text-decoration: underline;
		text-underline-offset: 3px;
		text-decoration-color: rgb(0 0 0 / 0.25);
	}
	.light .form-error { background: rgb(255 241 240 / 0.88); border-color: rgb(215 0 21 / 0.35); color: #9b0010; }
	.light .corner :global(.ui-ibtn.glass) {
		color: #1d1d1f;
		background: rgb(255 255 255 / 0.55);
		border-color: rgb(255 255 255 / 0.6);
		box-shadow: 0 2px 10px rgb(0 0 0 / 0.12);
	}
	@media (hover: hover) { .light .corner :global(.ui-ibtn.glass:hover) { background: rgb(255 255 255 / 0.85); }
}
	.light .corner :global(.ui-ibtn.glass:focus-visible) { outline: none; box-shadow: var(--focus-ring); }
	.panel > .sk { height: 311px; background: rgb(255 255 255 / 0.06); }
	.panel.light > .sk { background: rgb(0 0 0 / 0.05); }
	/* Two or more remembered people open on the taller list. */
	.panel > .sk.tall { height: 415px; }
	.panel:global(.swap), .panel:global(.swap) :global(*) { transition: none !important; }
	@media (max-width: 600px) { .panel > .sk { height: 289px; }
}
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; text-wrap: balance; }
	.lead { margin: 8px 0 20px; color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; text-wrap: pretty; }
	.title + :not(.lead) { margin-top: 26px; }
	.panel :global(.field + .field) { margin-top: 12px; }
	/* Labels, hints and errors centre like the title and button; typing stays left. */
	.panel :global(.field) { text-align: center; }
	.panel :global(.field .input) { text-align: left; }
	/* Errors: a soft red note centred under the fields, with a small warning mark. */
	.form-error {
		margin-top: 14px;
		padding: 10px 14px;
		border-radius: 12px;
		background: rgb(255 69 58 / 0.12);
		border: 1px solid rgb(255 69 58 / 0.26);
		color: #ffb3ad;
		font: 500 13px/1.45 var(--font-ui);
		text-align: center;
	}
	.form-error::before {
		content: '';
		display: inline-block;
		width: 15px;
		height: 15px;
		margin: 0 7px 0 0;
		vertical-align: -3px;
		background: currentColor;
		-webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 5h2v7h-2zm0 9h2v2h-2z'/%3E%3C/svg%3E") center / contain no-repeat;
		mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 5h2v7h-2zm0 9h2v2h-2z'/%3E%3C/svg%3E") center / contain no-repeat;
	}
	/* Back to the list: a round button in the panel's top-left corner, level with the logo. */
	.corner { position: absolute; top: 34px; left: 34px; z-index: 1; }
	.or { display: flex; align-items: center; gap: 10px; margin: 18px 0; color: rgb(255 255 255 / 0.45); font: var(--t-footnote); }
	.or::before, .or::after { content: ''; flex: 1; height: 1px; background: rgb(255 255 255 / 0.12); }

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
	/* Text links, the help summary and Copy: a faint wash on hover, a firmer one on press. */
	@media (hover: hover) {
		.link:hover { background: rgb(255 255 255 / 0.08); }
		.light .link:hover { background: rgb(0 0 0 / 0.05); }
}
	.link:active { background: rgb(255 255 255 / 0.14); }
	.light .link:active { background: rgb(0 0 0 / 0.09); }
	@keyframes orbit { to { transform: rotate(360deg); } }

	@media (prefers-reduced-motion: reduce) {
		.panel { transition: none; }
		.panel { transform: none; }
}
	@media (min-width: 601px) { .wrap { --fr: 18px; }
}
	@media (min-width: 1280px) { .wrap { --fr: 22px; }
}
	@media (min-width: 1440px) { .wrap { --fr: 24px; }
}
	@media (min-width: 1920px) { .wrap { --fr: 32px; }
}
	@media (min-width: 2560px) { .wrap { --fr: 40px; }
}
	/* Large screens: type, inputs and spacing grow with the panel. */
	@media (min-width: 1600px) and (min-height: 760px) { .panel { --z: 1.08; }
}
	@media (min-width: 2200px) and (min-height: 1000px) { .panel { --z: 1.2; }
}
	@media (min-width: 3000px) and (min-height: 1300px) { .panel { --z: 1.32; }
}
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.wrap {
			--fr: 0px;
			place-items: end center;
			padding: max(24px, env(safe-area-inset-top)) max(24px, env(safe-area-inset-right)) max(24px, calc(env(safe-area-inset-bottom) + 12px)) max(24px, env(safe-area-inset-left));
		}
		.panel {
			--z: 1;
			width: 100%;
			max-width: 440px;
			padding: 14px 16px 4px;
			border-radius: 24px;
			transform: translateY(calc(-1 * var(--kb, 0px)));
			transition: transform var(--dur) var(--ease-out);
		}
		.title { font-size: 20px; line-height: 1.15; }
		.lead { margin: 4px 0 10px; }
		.title + :not(.lead) { margin-top: 16px; }
		.corner { top: 6px; left: 16px; }
		.panel :global(.field) { gap: 2px; }
		.panel :global(.field label) { font-size: 14px; line-height: 1.25; }
		.panel :global(.field + .field) { margin-top: 4px; }
		.panel :global(.input) { min-height: 48px; }
		.panel > .sk.tall { height: 375px; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
		.step > .form-error:last-child { margin-bottom: 12px; }
		.or { margin: 10px 0; }
		.link { min-height: 40px; margin-bottom: 0; margin-top: 0; }
}
	/* Touch screens: 44 px tap targets for the small controls (after the phone block, so it wins). */
	@media (pointer: coarse) {
		.link { min-height: 44px; }
}
	/* Reduced motion: no keyboard-lift slide on phones either. */
	@media (prefers-reduced-motion: reduce) and (max-width: 600px) {
		.panel { transition: none; }
}
</style>
