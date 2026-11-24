<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import { SigninBrand, SigninTheme, liftAboveKeyboard, SigninCode, SigninHelp, SigninForgot, SigninLoginForm, SigninPerson, SigninRecent, SigninServerForm, SigninTiles, SigninWait, signInNext, takeHandoff, staggerParts, focusFirstField, focusStepIn, SigninBackdrop, SigninPortals, SIGNIN_GL_EVENT, createSigninWays, providerTiles, type Way, forgetPerson, keepPersonPic, recentPeople, rememberPerson, type RecentPerson, type RecentWay, pixelIn, blockDissolve, siteCell, theme } from '#lib/accounts/index.ts';
	import { SetupBar, phasesFor, PickLanguage, createSetupSignin, setupSaved, forgetSetup, passwordProblem, uploadAvatar, saveAccount, SetupIntro } from '#lib/setup/index.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { to } from '#lib/core/paths.ts';
	import { loaderReady, playHandoff, playSignIn, SIGNIN_HOLD_MS, VEIL_MS } from '#lib/core/loader.ts';
	import { ApiError, api, message } from '#lib/core/api.ts';
	import { session, type SignInResult } from '#lib/core/session.svelte.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);
	let name = $state('');
	// People who signed in on this device, with the way each one used.
	let recent = $state<RecentPerson[]>([]);
	// Which account view: 'auto' (one person: their step; more: the list), the list, another account (the usual form), or one person by index.
	let acct = $state<'auto' | 'list' | 'new' | number>('auto');
	let editing = $state(false);
	// The way's name for people: Password, Plex, Jellyfin, Emby, Single sign-on.
	const wayName = (w: RecentWay) => (w === 'oidc' ? t('accounts.tiles.sso') : w === 'password' ? t('accounts.signin.passwordLabel') : w.slice(0, 1).toUpperCase() + w.slice(1));
	const focusStep = () => focusStepIn(() => panelEl);
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
	let forgot = $state(false);
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
	const ways = createSigninWays({
		set error(v: string) { error = v; },
		set busy(v: boolean) { busy = v; },
		set lastWay(v: Way | '') { lastWay = v; },
		set firstRun(v: boolean) { firstRun = v; },
		get mode() { return mode; },
		get panelEl() { return panelEl; },
		took,
		failed,
		pick,
	});

	// Back from the wizard's first step: the account step again, still signed in, to change the saved account (?setup=account&edit=1).
	// Once the owner exists and this browser is signed in, the account step always edits it (never Create account); justMade covers the moment the account step itself made the owner.
	const editAsked = page.url.searchParams.get('setup') === 'account';
	let justMade = $state(false);
	const editAcct = $derived(editAsked && !justMade && session.s.state === 'signed_in' && session.s.setupOpen);
	let editLoaded = false;
	// What the edit form opened with: nothing changed means nothing to save.
	let loadedAcct: { name: string; display: string } | null = null;
	// The account change needs a fresh password (the sign-in is older than setup's window).
	let needProof = $state(false);
	$effect(() => {
		if (!editAcct || editLoaded) return;
		editLoaded = true;
		void api.get<{ username: string; display_name: string }>('/my/profile').then(
			(p) => {
				name = p.username;
				displayName = p.display_name || p.username;
				loadedAcct = { name: name, display: displayName };
			},
			() => ((name = session.s.name ?? ''), (displayName = name), (loadedAcct = { name, display: displayName })) // Not loaded: the signed-in name, never an empty one.
		).finally(() => (editReady = true));
	});
	// A submit right as the account step opens (the Enter that pressed Get started) is not the person's.
	let acctShownAt = 0;
	$effect(() => {
		if (mode === 'setup' && setup.welcomed) acctShownAt = Date.now();
	});
	// The saved values are in before the form shows, so it opens filled.
	let editReady = $state(false);
	let avatarGone = $state(false);
	let madeOwner = $state(false); // keeps Create account up while the new owner's session opens: no sign-in card flash
	const mode = $derived(session.s.state === 'needs_setup' || editAcct || madeOwner ? 'setup' : ticket ? 'code' : 'login');
	// Waits for the picture upload before leaving for the wizard.
	let holdLeave = $state(false);
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
	// The owner exists but first-run setup is not finished: sign-in stays the setup's Sign in step (step bar, owner only, no account list) until it is.
	const setupSignin = $derived(mode !== 'setup' && session.s.setupPending);
	const setup = createSetupSignin({
		welcomed: page.url.searchParams.get('out') === '1' || page.url.searchParams.get('setup') === 'account',
		get mode() { return mode; },
		get editAcct() { return editAcct; },
		get name() { return name; },
		get displayName() { return displayName; },
		get error() { return error; },
		set error(v) { error = v; },
		get pw() { return pw; },
		set pw(v) { pw = v; },
		set pwAgain(v: string) { pwAgain = v; },
		set lastWay(v: Way | '') { lastWay = v; },
		set busy(v: boolean) { busy = v; },
		set firstRun(v: boolean) { firstRun = v; },
		set firstRunWay(v: RecentWay | '') { firstRunWay = v; },
		wayWord,
		focusStep,
		press: (w: Way) => ways.press(w),
		pick,
		failed,
	});
	let displayName = $state('');
	{
		const kept = setupSaved();
		if (kept?.name) name = kept.name;
		if (kept?.display) displayName = kept.display;
	}
	// The owner's picture, picked on the account step; sent once the account exists.
	let avatarFile = $state<File | null>(null);
	const acctView = $derived(
		mode !== 'login' || setupSignin || !shown.length || acct === 'new' ? 'none' : acct === 'list' || (acct === 'auto' && shown.length > 1) ? 'list' : 'person'
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
		// Welcome goes back to the language step, for a new owner and for one editing their account during setup.
		const toLang = setup.langDone ? { label: t('welcome.back'), go: setup.backToLang } : null;
		if (editAcct) return setup.welcomed ? { label: t('welcome.back'), go: () => { setup.welcomed = false; focusStep(); } } : toLang;
		if (mode === 'setup') return setup.welcomed ? { label: t('welcome.back'), go: () => { setup.welcomed = false; focusStep(); } } : toLang;
		if (setupSignin && mode === 'login') return setup.ownerWays ? null : chosen !== 'choose' && outside.length ? { label: t('accounts.login.otherWays'), go: () => pick('choose') } : { label: t('welcome.back'), go: () => ((setup.ownerWays = true), focusStep()) };
		if (mode !== 'login' || !shown.length) return null;
		if (acctView === 'list' && shown.length === 1) return { label: t('accounts.login.backTo', { name: shown[0].name }), go: () => openPerson(0) };
		if ((acctView === 'person' && shown.length > 1) || acct === 'new') return { label: t('accounts.login.allAccounts'), go: showList };
		return null;
	});
	// One key per step: the panel's content rises in when it changes.
	const step = $derived(forgot ? 'forgot' : ways.plexWaiting ? 'plex' : `${mode}-${mode === 'setup' ? `${setup.langDone}-${setup.welcomed}-${setup.setupWay}` : via}-${acctView}-${person?.name ?? ''}-${setup.ownerWays}`);
	let calm = $state(false);
	// The first step is there on the first frame; only later step changes rise in.
	let firstStep = '';
	const rise = $derived.by(() => {
		if (session.s.state === 'unknown') return { duration: 0 };
		if (!firstStep) firstStep = step;
		// Setup steps swap at once: the press shows only on the button, never as a dissolve over the whole card.
		return { duration: calm || step === firstStep || mode === 'setup' || setupSignin ? 0 : 300 };
	});
	let leaving = $state(false);
	let importPaused = $state(false);
	let panelEl = $state<HTMLDivElement>();
	let coverEl = $state<HTMLDivElement>();
	let left = false;

	// The card is there on the first frame; its contents come in as a soft stagger (logo, tiles, "or", fields, button: 70 ms apart, done in about 1 s). Focus goes to the name field at once, so typing never waits.
	// Back from the setup wizard lands here still signed in (?out=1): it signs out in place, with no entrance, so it reads as the same card going back one step.
	const fromSetup = page.url.searchParams.get('out') === '1' || page.url.searchParams.get('edit') === '1';
	let outDone = $state(page.url.searchParams.get('out') !== '1');
	let intro = $state(!fromSetup);
	// The stagger waits (paused, at its first frame) while the wallpaper's wave shader compiles: on a first visit the GPU is busy with it for about 100 ms, and a fade caught in that would hitch. SigninBackdrop signals when it is ready.
	let glWait = $state(!fromSetup);
	const GL_WAIT_CAP_MS = 450;
	let staggered = false;
	$effect(() => {
		if (staggered || session.s.state === 'unknown' || !panelEl) return;
		staggered = true;
		if (fromSetup) return;
		staggerParts(panelEl);
		// Later step changes use their own rise, not this entrance.
		setTimeout(() => (intro = false), 1400);
	});
	let focusedOnce = false;
	$effect(() => {
		if (focusedOnce || session.s.state === 'unknown' || !panelEl || (editAcct && !editReady)) return;
		if (focusFirstField(panelEl)) focusedOnce = true;
	});
	onMount(() => {
		if (fromSetup && page.url.searchParams.get('out') === '1')
			void session.logout().then(() => {
				outDone = true;
				history.replaceState(history.state, '', to('/login'));
			});
		const handed = takeHandoff();
		[ticket, importPaused] = [handed.ticket || ticket, handed.importPaused];
		calm = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		recent = recentPeople();
		const go = () => (glWait = false);
		if (calm || document.documentElement.dataset.signinGl) go();
		window.addEventListener(SIGNIN_GL_EVENT, go, { once: true });
		const cap = setTimeout(go, GL_WAIT_CAP_MS);
		const clock = setInterval(() => (now = Date.now()), 1000);
		if (page.url.searchParams.get('sso') === 'finish') void ways.finishOIDC();
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
			ways.stop();
			// Elsewhere the boot screen works as usual (reconnecting, slow page changes).
			document.documentElement.classList.remove('hl-skip');
		};
	});

	// Right after the owner is made, the first-run step asks which parts to use.
	let firstRun = false;
	$effect(() => {
		// A visit let in by the local-network skip is not signed in: this page stays, so a password sign-in can follow.
		if (session.s.state !== 'signed_in' || session.s.via === 'local_network') return;
		if (left || !outDone || holdLeave || editAcct) return;
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
		// Setup not finished yet: go on at its first unfinished step (the wizard picks it).
		const nx = signInNext(page.url.searchParams); // first run: back to the setup step it was on, else setup's start
		const target = to(firstRun || session.s.setupOpen ? (nx.startsWith('/welcome?') ? nx : '/welcome?first=1') : nx);
		// Setup goes on in the same card: no sign-in animation, no app reveal in between.
		if (firstRun || session.s.setupOpen) {
			void goto(target, { replace: true }).finally(loaderReady);
			return;
		}
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
		if (mode === 'setup' && Date.now() - acctShownAt < 400) return;
		if (mode === 'setup' && !name.trim()) return void (error = t('accounts.login.enterName', { way: 'HolyArrs' }));
		if (editAcct) {
			const bad = pw ? passwordProblem(pw, pwAgain, MIN_LENGTH) : pwAgain ? t('accounts.login.noMatch') : '';
			if (bad) return void (error = bad);
			const same = loadedAcct && name.trim() === loadedAcct.name && displayName.trim() === loadedAcct.display.trim() && !pw && !avatarFile && !avatarGone;
			if (same) {
				await goto(to('/welcome?first=1'), { replace: true });
				return;
			}
			busy = true;
			try {
				await saveAccount({ username: name.trim(), display: displayName.trim(), password: pw, avatar: avatarFile, avatarGone });
				await goto(to('/welcome?first=1'), { replace: true });
			} catch (err) {
				if (err instanceof ApiError && err.status === 403 && session.s.hasPassword) {
					// Ask for the password once (below), then Save again; the typed changes stay.
					needProof = true;
					error = '';
					return;
				}
				failed(err);
				pw = '';
				pwAgain = '';
			} finally {
				busy = false;
			}
			return;
		}
		if (mode === 'setup' && (error = passwordProblem(pw, pwAgain, MIN_LENGTH))) return;
		busy = true;
		try {
			if (mode === 'setup') {
				firstRun = true;
				justMade = madeOwner = true;
				// The picture goes up once the account exists; the step after already shows Home's work, so a failed upload is only skipped (My account can add it).
				holdLeave = !!avatarFile;
				await session.setup(name.trim(), pw, displayName.trim());
				forgetSetup();
				if (avatarFile) {
					try {
						await uploadAvatar(avatarFile);
					} catch {
						// Skipped; see above.
					}
					holdLeave = false;
				}
			}
			else if (via === 'password') {
				// Setup's sign-in goes on to the wizard, not the edit view.
				if (setupSignin) justMade = true;
				took(await session.login(name.trim(), pw));
			}
			else if (via !== 'choose') took(await session.mediaServer(via, name.trim(), pw));
		} catch (err) {
			madeOwner = false;
			if (err instanceof ApiError && err.status === 409 && mode === 'setup') {
				error = t('accounts.login.ownerExists');
				await session.load();
			} else {
				failed(err);
			}
			pw = '';
			pwAgain = '';
			holdLeave = false;
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

	let showPw = $state(false);
	// "Forgot password?" (owner only). HOLYARRS_RESET_OWNER opens it at the code step.
	$effect(() => {
		if (session.s.ownerResetEnv && mode === 'login') untrack(() => (forgot = true));
	});
	function openForgot() {
		forgot = true;
		error = '';
	}
	async function forgotDone(n: string, p: string) {
		forgot = false;
		name = n;
		pw = '';
		took(await session.login(n, p));
	}
</script>

<svelte:head>
	<title>{t('accounts.login.pageTitle', { what: mode === 'setup' || setupSignin ?t('accounts.login.setUp') : t('accounts.signin.signIn') })}</title>
</svelte:head>

<main class="wrap" class:leaving data-theme="dark">
	<SigninBackdrop />
	<div class="cover" bind:this={coverEl} aria-hidden="true"></div>
	<div class="panel" class:setup={mode === 'setup' || setupSignin} class:welcome-intro={mode === 'setup' && !setup.welcomed} class:intro class:gl-wait={glWait} class:light={theme.value === 'light'} data-theme={theme.value} bind:this={panelEl} data-signin-panel>
		<SigninBrand />
		{#if mode === 'setup' && session.s.state !== 'unknown'}<SetupBar current={setup.welcomed ? 'signin' : setup.langDone ? 'welcome' : 'language'} phases={phasesFor({ withServer: true })} />{/if}
		{#if setupSignin && session.s.state !== 'unknown'}<SetupBar current="signin" phases={phasesFor({ withServer: true })} />{/if}
		{#if backTo && session.s.state !== 'unknown' && !ways.ssoFinishing && !ways.plexWaiting}
			<span class="corner"><IconButton icon="chevronLeft" variant="glass" label={backTo.label} onclick={backTo.go} /></span>
		{/if}

		{#if session.s.state === 'unknown' || ways.ssoFinishing}
			<!-- As tall as the usual sign-in form, so the card does not jump when the session answers. -->
			<div class="sk" class:tall={recent.length > 1} style="border-radius: var(--r-card)"></div>
			{#if ways.ssoFinishing}<p class="small muted" role="status">{t('accounts.login.finishing')}</p>{/if}
		{:else}
			{#if importPaused}<p class="form-error" role="status" data-testid="import-paused">{t('accounts.login.importPaused')}</p>{/if}
			{#key step}
				<div class="step" class:intro-step={mode === 'setup' && setup.langDone && !setup.welcomed && !ways.plexWaiting} in:pixelIn={rise}>
					{#if forgot && mode === 'login'}
						<SigninForgot minLength={MIN_LENGTH} haveCode={session.s.ownerResetEnv} ondone={forgotDone} onback={() => (forgot = false)} />
					{:else if ways.plexWaiting}
						<SigninWait cancelPlex={ways.cancelPlex} />
					{:else if setupSignin && setup.ownerWays && mode === 'login'}
						<h1 class="title">{t('welcome.pickServer')}</h1>
						<p class="lead">{t('welcome.ownerSignIn')}</p>
						<SigninTiles setup setupTiles={setup.ownerTiles} setupName={setup.setupName} setupDesc={setup.setupDesc} {lastWay} {failN} {busy} onsetup={setup.ownerPick} />
					{:else if mode === 'setup' && !setup.langDone}
						<PickLanguage ondone={setup.finishLang} />
					{:else if mode === 'setup' && !setup.welcomed}
						<SetupIntro onstart={() => ((setup.welcomed = true), focusStep())} />
					{:else if acctView !== 'none'}
						<!-- One person: their face and name lead; the heading stays for screen readers. -->
						<h1 class="title" class:sr-only={acctView === 'person'}>{acctView === 'list' ? t('accounts.login.who') : t('accounts.signin.signIn')}</h1>
						{#if acctView === 'list'}
							<SigninRecent {shown} {editing} {wayName} {dropPerson} {openPerson} {another} />
							<button class="link" type="button" aria-pressed={editing} onclick={() => (editing = !editing)}>{editing ? t('accounts.login.done') : t('accounts.login.editList')}</button>
						{:else if person}
							<SigninPerson {person} {name} bind:pw bind:showPw {error} {waiting} {busy} {lastWay} {tileWay} {failN} {wayName} press={ways.press} {submit} />
							{#if shown.length === 1}<button class="link" type="button" onclick={showList}>{t('accounts.login.switch')}</button>{/if}
						{/if}
						{#if formOn}<SigninHelp {calm} onforgot={openForgot} />{/if}
					{:else}
						<h1 class="title">{mode === 'setup' ? (setup.setupWay === 'choose' ? t('welcome.pickServer') : setup.setupWay === 'password' ? t('welcome.account.title') : t('accounts.signin.signInWith', { way: setup.setupName(setup.setupWay) })) : mode === 'code' ? t('accounts.login.twoStep') : via === 'jellyfin' || via === 'emby' ? t('accounts.signin.signInWith', { way: wayWord[via] }) : setupSignin ? t('welcome.account.title') : t('accounts.signin.signIn')}</h1>
						{#if mode === 'setup'}
							<p class="lead">{setup.setupWay === 'choose' ? t('welcome.pickServerLead') : setup.setupWay === 'plex' ? t('welcome.plexLead') : setup.setupServer ? t('accounts.login.useAdmin', { way: wayWord[setup.setupServer] }) : t('welcome.account.lead')}</p>
							{#if session.s.configUnmapped}
								<p class="form-error" role="status">{t('accounts.login.unmapped')}</p>
							{/if}
						{:else if mode === 'code'}
							<p class="lead">{useBackup ? t('accounts.login.enterBackup') : t('accounts.login.enterSix')}</p>
						{:else if via === 'jellyfin' || via === 'emby'}
							<p class="lead">{t('accounts.login.useYour', { way: wayWord[via] })}</p>
						{:else if setupSignin}
							<p class="lead">{t('welcome.ownerSignIn')}</p>
						{:else if via === 'choose'}
							<p class="lead">{t('accounts.login.choose')}</p>
						{/if}

						{#if mode === 'setup' && setup.setupWay === 'choose'}
							<SigninTiles setup setupTiles={setup.setupTiles} setupName={setup.setupName} setupDesc={setup.setupDesc} {lastWay} {failN} {busy} onsetup={setup.setupPress} />
							{#if error}
								<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
							{/if}
						{:else if mode === 'setup' && setup.setupWay === 'plex'}
							<SigninTiles setup setupTiles={['plex']} setupName={setup.setupName} {lastWay} {failN} {busy} onsetup={() => ((lastWay = 'plex'), ways.plex())} />
							{#if error}<p class="form-error" role="alert">{error}</p>{/if}
						{:else if mode === 'setup' && setup.setupServer}
							<SigninServerForm kind={setup.setupServer} bind:address={setup.address} bind:name bind:pw bind:showPw foundHere={setup.foundHere} looking={setup.looking} onrefresh={setup.refreshServers} {error} {waiting} {busy} {failN} {wayWord} submitServer={setup.submitServer} {focusStep} />
						{:else if mode === 'code'}
							<SigninCode bind:ticket bind:pw bind:otp bind:useBackup bind:busy bind:error bind:lastWay {waiting} {failN} {tileWay} {failed} />
						{:else}
							{#if mode === 'login' && (via === 'password' || via === 'choose') && tiles.length}
								<SigninTiles {tiles} {lastWay} {failN} {busy} onpress={ways.press} />
								{#if via === 'password'}<div class="or">{t('accounts.login.or')}</div>{/if}
							{/if}

							{#if via !== 'choose' || mode === 'setup'}
								{#if editAcct && needProof}<Confirm action="password" reveal={false} onCancel={() => (needProof = false)}>{#snippet children()}{/snippet}</Confirm>{/if}
								{#if !editAcct || editReady}<SigninLoginForm {mode} {via} setupLook={setupSignin && mode === 'login' && via === 'password'} edit={editAcct} currentPic={editAcct ? session.s.avatar : null} bind:avatarGone bind:name bind:display={displayName} bind:avatar={avatarFile} bind:pw bind:pwAgain bind:showPw {error} {waiting} {busy} {tileWay} {failN} minLength={MIN_LENGTH} {wayWord} {submit} {pick} />{/if}
							{:else if error}
								<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
							{/if}

							{#if mode === 'login' && via === 'password' && !setupSignin}<SigninHelp {calm} onforgot={openForgot} />{/if}
						{/if}
					{/if}
				</div>
			{/key}
		{/if}
	</div>
	<SigninPortals symbols={mode !== 'setup'} />
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
	/* First-run setup: wider, so the six-step bar and the way tiles get room; welcome uses the same 672px content width, so steps never jump. */
	.panel.setup { width: min(calc(740px / var(--z)), calc(100% / var(--z))); }
	/* Setup header matches the wizard's: the 28px logo alone, the step bar right under it. */
	.panel.setup :global(.brand) { margin-bottom: 0; min-height: 28px; }
	.panel.setup :global(.brand span) { display: none; }
	.panel.setup :global(.brand img) { width: 28px; height: 28px; }
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
	.light .lead { color: rgb(0 0 0 / 0.74); }
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
	/* One spacing scale for the card: --gap between blocks, --gap-field between fields. */
	.panel { --gap: var(--s-5); --gap-field: var(--s-3); }
	.lead { margin: var(--s-2) 0 var(--gap); color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; text-wrap: pretty; }
	/* Welcome fills the setup card: brand, headline and rows at the top, Get started at the foot like Continue. */
	.panel.welcome-intro :global(.brand) { font-size: 30px; gap: 16px; margin-bottom: 20px; }
	.panel.welcome-intro :global(.brand img) { width: 52px; height: 52px; }
	/* Welcome is one group, centred in the card: no gap between the rows and Get started. */
	.step.intro-step { display: flex; flex-direction: column; justify-content: center; }
	/* Global: the next element often belongs to a child form component, which a scoped selector never matches. */
	.title:not(.sr-only) + :global(:not(.lead)) { margin-top: var(--gap); }
	.panel :global(.field + .field) { margin-top: var(--gap-field); }
	/* Labels, hints and errors centre like the title and button; typing stays left. */
	.panel :global(.field) { text-align: center; }
	.panel :global(.field .input) { text-align: start; }
	/* Errors: a soft red note centred under the fields, with a small warning mark. */
	/* Same gap above and below, so a warning never touches the buttons under it. */
	.form-error {
		margin: var(--gap) 0;
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
		margin: 0; margin-inline-end: 7px;
		vertical-align: -3px;
		background: currentColor;
		-webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 5h2v7h-2zm0 9h2v2h-2z'/%3E%3C/svg%3E") center / contain no-repeat;
		mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Cpath d='M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm-1 5h2v7h-2zm0 9h2v2h-2z'/%3E%3C/svg%3E") center / contain no-repeat;
	}
	/* Back to the list: a round button in the panel's top-left corner, level with the logo. */
	.corner { position: absolute; top: 32px; inset-inline-start: 34px; z-index: 1; }
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
	/* Desktop setup: the one setup card (setup-glass.css), the same box as every wizard step, centred inside the frame; the step sits in its middle. */
	@media (min-width: 1100px) {
		.wrap:has(.panel.setup) { align-items: center; padding: 0; }
		.panel.setup { --z: 1; box-sizing: border-box; width: var(--setup-w); height: var(--setup-h); display: flex; flex-direction: column; overflow: hidden; }
		/* One spacing rhythm on every sign-in step: 44px from the lead to the form, 28px between fields, 32px before the button. */
		.panel.setup { --gap-field: 28px; --gap: 32px; }
		.panel.setup .lead + :global(*) { margin-top: clamp(20px, 3vh, 44px); }
		.panel.setup .lead + .form-error { margin-bottom: clamp(16px, 2.5vh, 32px); }
		/* Every sign-in step starts at the same spot under the step bar, so titles never jump between steps. */
		.panel.setup > .step { width: min(100%, 560px); margin: clamp(24px, 4vh, 56px) auto auto; }
		.panel.setup > .step.intro-step { flex: 1 1 auto; margin: 0 auto; min-height: 0; }
		.panel.setup > .step :global(.tiles) { max-width: 560px; }
		/* Every setup step fills the one card: bigger type, tiles and fields, as one centred group (scales with the window height so 900 px still fits). */
		.panel.setup > .step:not(.intro-step) { width: min(100%, 900px); }
		.panel.setup .title { font-size: clamp(32px, 3.6vh, 40px); line-height: 1.15; }
		.panel.setup .lead { font-size: 17px; }
		/* The ways: a 2x2 grid of large tiles, each with a plain line under its name. */
		.panel.setup :global(.provs[data-testid='setup-ways']:not([data-n='1'])) { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
		.panel.setup :global(.provs .prov) { min-height: clamp(170px, 21vh, 220px); justify-content: center; gap: 12px; padding: 20px 24px; font: 600 20px/1.2 var(--font-ui); border-radius: 22px; }
		.panel.setup :global(.provs .prov svg) { width: 48px; height: 48px; }
		.panel.setup :global(.provs .prov .desc) { font-size: 15px; }
		.panel.setup :global(.provs[data-n='1'] .prov) { min-height: 64px; flex-direction: row; }
		.panel.setup :global(.field label) { font-size: 15px; }
		.panel.setup :global(.input) { min-height: 52px; height: 52px; font-size: 17px; }
		.panel.setup :global(.submit .ui-btn) { min-height: 48px; height: 48px; font-size: 17px; } .panel.setup :global(.submit .ui-btn.primary) { border-radius: var(--r-card); }
		/* Same frame as the wizard steps: the logo and step bar at one spot, the title at one spot, and the main button on the card's foot at the card's full width, so nothing moves between steps. */
		.panel.setup.welcome-intro :global(.brand) { font-size: inherit; gap: 10px; margin-bottom: 0; }
		.panel.setup.welcome-intro :global(.brand img) { width: 28px; height: 28px; }
		.panel.setup > .step, .panel.setup > .step.intro-step, .panel.setup > .step:not(.intro-step) { flex: 1 1 auto; min-height: 0; width: 100%; margin: clamp(24px, 4vh, 56px) 0 0; display: flex; flex-direction: column; justify-content: flex-start; }
		.panel.setup > .step > :global(form) { flex: 1 1 auto; display: flex; flex-direction: column; min-height: 0; }
		.panel.setup > .step :global(.acct-grid), .panel.setup > .step :global(.tiles), .panel.setup > .step :global(.provs), .panel.setup > .step > :global(.form-error) { width: min(100%, 900px); margin-inline: auto; box-sizing: border-box; }
		.panel.setup { padding-bottom: 34px; }
		.panel.setup > .step :global(.submit.submit) { margin-top: auto; margin-inline-start: 0; padding-top: var(--s-4); }
		.panel.setup > .step :global(.input.input) { min-height: 52px; height: 52px; font-size: 17px; border-radius: var(--g-r, 12px); }
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
		.panel { --gap: var(--s-4); }
		.lead { margin: var(--s-1) 0 var(--gap); }
		.corner { top: 6px; inset-inline-start: 16px; }
		.panel :global(.field) { gap: 6px; }
		.panel :global(.field label) { font-size: 14px; line-height: 1.25; }
		.panel :global(.input) { min-height: 48px; }
		.panel > .sk.tall { height: 375px; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
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
