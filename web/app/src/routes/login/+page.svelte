<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { loaderReady, playHandoff, playSignIn, SIGNIN_HOLD_MS, VEIL_MS } from '#lib/loader.ts';
	import { page } from '$app/state';
	import { ApiError, message } from '#lib/api.ts';
	import { session, type FoundServer, type SignInResult } from '#lib/session.svelte.ts';
	import { safeNext } from '#lib/next.ts';
	import SigninBackdrop from '#lib/SigninBackdrop.svelte';
	import SigninPortals from '#lib/SigninPortals.svelte';
	import { SIGNIN_GL_EVENT, providerTiles, type Way } from '#lib/signinArt.ts';
	import { forgetPerson, keepPersonPic, recentPeople, rememberPerson, type RecentPerson, type RecentWay } from '#lib/recentPeople.ts';
	import { pixelTrail, pixelIn, blockDissolve, siteCell } from '#lib/pixelTrail.ts';
	import { setTheme, theme } from '#lib/theme.svelte.ts';

	const MIN_LENGTH = $derived(session.s.minPassword);

	let name = $state('');
	// People who signed in on this device, with the way each one used.
	let recent = $state<RecentPerson[]>([]);
	// Which account view: 'auto' (one person: their step; more: the list),
	// the list, another account (the usual form), or one person by index.
	let acct = $state<'auto' | 'list' | 'new' | number>('auto');
	let editing = $state(false);
	// The way's name for people: Password, Plex, Jellyfin, Emby, Single sign-on.
	const wayName = (w: RecentWay) => (w === 'oidc' ? 'Single sign-on' : w.slice(0, 1).toUpperCase() + w.slice(1));
	const hue = (n: string) => [...n].reduce((a, c) => (a * 31 + c.charCodeAt(0)) % 360, 7);
	// After a view change the pressed button is gone, so focus would drop to the
	// page: move it to the new step's first control. Touch screens are left alone
	// (focusing a field there pops the keyboard).
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
	const RESET_CMD = 'docker exec -i holyarrs /holyarrs reset-owner';
	let copied = $state(false);
	let copyNote = $state('');
	let cmdEl = $state<HTMLElement>();
	async function copyCmd() {
		try {
			await navigator.clipboard.writeText(RESET_CMD);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			// Plain http has no clipboard: select the command so Ctrl+C copies it.
			const sel = window.getSelection();
			if (!cmdEl || !sel) return;
			const r = document.createRange();
			r.selectNodeContents(cmdEl);
			sel.removeAllRanges();
			sel.addRange(r);
			copyNote = 'Selected. Press Ctrl+C to copy.';
			setTimeout(() => (copyNote = ''), 3000);
		}
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
	// First run: which way makes the owner. 'choose' shows them all; Plex
	// signs in from its tile, the others open their form.
	type SetupWay = 'password' | 'plex' | 'jellyfin' | 'emby';
	let setupWay = $state<'choose' | 'password' | 'jellyfin' | 'emby'>('choose');
	const setupName = (w: SetupWay) => w.charAt(0).toUpperCase() + w.slice(1);
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
		if (mode === 'code') return { label: 'Back to sign in', go: restart };
		if (mode === 'setup') return setupWay !== 'choose' ? { label: 'Other ways', go: () => setupPick('choose') } : null;
		if (mode !== 'login' || !shown.length) return null;
		if (acctView === 'list' && shown.length === 1) return { label: `Back to ${shown[0].name}`, go: () => openPerson(0) };
		if ((acctView === 'person' && shown.length > 1) || acct === 'new') return { label: 'All accounts', go: showList };
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
		const field = panelEl.querySelector<HTMLInputElement>('#name') ?? panelEl.querySelector<HTMLInputElement>('#password');
		if (!field) return;
		focusedOnce = true;
		// Phones: no focus on load, so the keyboard does not cover the art and the card.
		if (!matchMedia('(pointer: fine)').matches) return;
		if (!document.activeElement || document.activeElement === document.body) field.focus({ preventScroll: true });
	});
	onMount(() => {
		calm = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		recent = recentPeople();
		const go = () => (glWait = false);
		if (calm || document.documentElement.dataset.signinGl) go();
		window.addEventListener(SIGNIN_GL_EVENT, go, { once: true });
		const cap = setTimeout(go, GL_WAIT_CAP_MS);
		const t = setInterval(() => (now = Date.now()), 1000);
		if (page.url.searchParams.get('sso') === 'finish') void finishOIDC();
		else if (page.url.searchParams.get('sso') === 'failed') {
			lastWay = 'oidc';
			error = "Single sign-on didn't finish. Try again, or sign in another way.";
		}
		// Phones: lift the card above the on-screen keyboard (--kb on <main>).
		const vv = window.visualViewport;
		const lift = () => {
			if (!vv || !panelEl) return;
			let kb = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop));
			// Never lift the card's top past 16 px of the visible area.
			kb = Math.min(kb, Math.max(0, panelEl.offsetTop - 16));
			panelEl.parentElement?.style.setProperty('--kb', `${kb}px`);
		};
		vv?.addEventListener('resize', lift);
		vv?.addEventListener('scroll', lift);
		return () => {
			vv?.removeEventListener('resize', lift);
			vv?.removeEventListener('scroll', lift);
			clearInterval(t);
			clearTimeout(cap);
			window.removeEventListener(SIGNIN_GL_EVENT, go);
			clearInterval(plexTimer);
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
			useBackup = false;
			clearDigits();
		}
	}

	// Two-factor: six digit boxes (typing moves on, a pasted or autofilled code
	// fills them all and sends), or one box for a backup code.
	let useBackup = $state(false);
	let digits = $state(['', '', '', '', '', '']);
	let digitsEl = $state<HTMLDivElement>();
	const digitInputs = () => [...(digitsEl?.querySelectorAll<HTMLInputElement>('input') ?? [])];
	function clearDigits() {
		digits = ['', '', '', '', '', ''];
		otp = '';
		void tick().then(() => (useBackup ? panelEl?.querySelector<HTMLInputElement>('#otp') : digitInputs()[0])?.focus());
	}
	function fillDigits(from: number, text: string): boolean {
		const ds = text.replace(/\D/g, '').slice(0, 6 - from).split('');
		if (!ds.length) return false;
		ds.forEach((d, k) => (digits[from + k] = d));
		otp = digits.join('');
		const ins = digitInputs();
		ds.forEach((d, k) => ins[from + k] && (ins[from + k].value = d));
		ins[Math.min(from + ds.length, 5)]?.focus();
		if (otp.length === 6 && !busy) digitsEl?.closest('form')?.requestSubmit();
		return true;
	}
	function typeDigit(i: number, e: Event) {
		const el = e.currentTarget as HTMLInputElement;
		if (fillDigits(i, el.value)) return;
		digits[i] = '';
		otp = digits.join('');
		el.value = '';
	}
	function digitKey(i: number, e: KeyboardEvent) {
		const ins = digitInputs();
		if (e.key === 'Backspace' && !digits[i] && i > 0) {
			e.preventDefault();
			digits[i - 1] = '';
			otp = digits.join('');
			ins[i - 1].value = '';
			ins[i - 1].focus();
		} else if (e.key === 'ArrowLeft' && i > 0) {
			e.preventDefault();
			ins[i - 1].focus();
		} else if (e.key === 'ArrowRight' && i < 5) {
			e.preventDefault();
			ins[i + 1].focus();
		}
	}
	function pasteDigits(i: number, e: ClipboardEvent) {
		const t = (e.clipboardData?.getData('text') ?? '').replace(/\D/g, '');
		if (!t) return;
		e.preventDefault();
		fillDigits(t.length >= 6 ? 0 : i, t);
	}
	function toggleBackup() {
		useBackup = !useBackup;
		error = '';
		clearDigits();
	}

	function failed(err: unknown) {
		failN++;
		if (err instanceof ApiError && err.status === 429) {
			waitUntil = Date.now() + (err.retryAfter ?? 30) * 1000;
			now = Date.now();
			error = 'Too many failed sign-ins. Wait a little and try again.';
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
				error = `Use at least ${MIN_LENGTH} characters.`;
				return;
			}
			if (pw !== pwAgain) {
				error = "Passwords don't match.";
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
				error = 'An owner already exists. Sign in instead.';
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
		// The button stays on; a short code gets a plain note instead of a dead button.
		if (!useBackup && otp.length < 6) {
			error = 'Enter all 6 digits.';
			digitInputs()[Math.max(0, digits.findIndex((d) => !d))]?.focus();
			return;
		}
		if (useBackup && !otp.trim()) {
			error = 'Enter a backup code.';
			panelEl?.querySelector<HTMLInputElement>('#otp')?.focus();
			return;
		}
		busy = true;
		try {
			await session.loginCode(ticket, otp.trim());
		} catch (err) {
			if (err instanceof ApiError && err.status === 401 && err.message.includes('ended')) {
				ticket = '';
				pw = '';
			}
			failed(err);
			clearDigits();
		} finally {
			busy = false;
		}
	}

	function restart() {
		ticket = '';
		error = '';
		pw = '';
		useBackup = false;
		digits = ['', '', '', '', '', ''];
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
			error = `Enter the ${wayWord[kind]} address.`;
			return;
		}
		if (!name.trim()) {
			error = `Enter your ${wayWord[kind]} name.`;
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
				error = 'An owner already exists. Sign in instead.';
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
		clearInterval(plexTimer);
		plexWaiting = false;
		// Focus goes back to the Plex button (or the first choice), not the page.
		void tick().then(() => (panelEl?.querySelector<HTMLElement>('[data-way="plex"]') ?? panelEl?.querySelector<HTMLElement>('button:not([disabled])'))?.focus());
	}

	async function plex() {
		error = '';
		// First run: the Plex account becomes the owner.
		const setup = mode === 'setup';
		if (setup) firstRun = true;
		// Open the window while the click still counts, then point it at Plex.
		const win = window.open('', 'plex-sign-in', 'width=600,height=720');
		try {
			const p = await (setup ? session.setupPlexStart() : session.plexStart());
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
					error = 'Plex sign-in timed out. Try again.';
					return;
				}
				try {
					const r = await (setup ? session.setupPlexFinish(p.id) : session.plexFinish(p.id));
					if (r === 'waiting') return;
					clearInterval(plexTimer);
					plexWaiting = false;
					win?.close();
					took(r);
				} catch (err) {
					clearInterval(plexTimer);
					plexWaiting = false;
					win?.close();
					if (setup && err instanceof ApiError && err.status === 409) void session.load();
					failed(err);
				}
			}, 2000);
		} catch (err) {
			win?.close();
			if (setup && err instanceof ApiError && err.status === 409) void session.load();
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

{#snippet face(p: RecentPerson, size: number)}
	<span class="face" style="--s: {size}px; --h: {hue(p.name)}" aria-hidden="true">
		{p.name.slice(0, 1).toUpperCase()}
		{#if p.pic}<img src={p.pic} alt="" />{/if}
		{#if p.way !== 'password'}
			<span class="badge">
				{#if p.way === 'plex'}
					<svg viewBox="0 0 24 24"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
				{:else if p.way === 'jellyfin'}
					<svg viewBox="0 0 24 24"><defs><linearGradient id="jg-face" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg-face)" /></svg>
				{:else if p.way === 'emby'}
					<svg viewBox="0 0 24 24"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>
				{:else}
					<svg viewBox="0 0 24 24" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linecap="round"><circle cx="8" cy="12" r="4" /><path d="M12 12h9M18 12v3M21 12v2" /></svg>
				{/if}
			</span>
		{/if}
	</span>
{/snippet}

{#snippet help()}
	<!-- Opening brings the answers into view (the frame would otherwise hide the last lines). -->
	<details class="help" ontoggle={(e) => { const d = e.currentTarget; if (d.open) d.scrollIntoView({ block: 'nearest', behavior: calm ? 'auto' : 'smooth' }); }}>
		<summary>Can't sign in?</summary>
		<div class="ans">
			<b>You use this server</b>
			<p>Ask the owner. They can give you a new password in Settings › Users.</p>
		</div>
		<div class="ans" data-testid="reset-owner-help">
			<b>You run this server</b>
			<p>Run this where HolyArrs runs:</p>
			<div class="cmd">
				<code bind:this={cmdEl}>{RESET_CMD}</code>
				<span class="sr-only" role="status">{copied ? 'Copied' : copyNote}</span>
				<button type="button" class="cp" onclick={copyCmd}>{copied ? 'Copied' : 'Copy'}</button>
			</div>
			<p>Type your name, press Enter, type a new password, then press Ctrl+D.</p>
			<p class="note">Everyone gets signed out and two-factor turns off.</p>
		</div>
	</details>
{/snippet}

<svelte:head>
	<title>{mode === 'setup' ? 'Set up HolyArrs' : 'Sign in'} · HolyArrs</title>
</svelte:head>

<main class="wrap" class:leaving data-theme="dark">
	<SigninBackdrop />
	<div class="cover" bind:this={coverEl} aria-hidden="true"></div>
	<div class="panel" class:intro class:gl-wait={glWait} class:light={theme.value === 'light'} data-theme={theme.value} bind:this={panelEl} data-signin-panel>
		<div class="brand">
			<!-- Both marks load up front, so switching the appearance never shows a blank logo. -->
			<img class="mark-dark" src={to('/brand/mark-color-on-dark.svg')} alt="" width="36" height="36" />
			<img class="mark-light" src={to('/brand/mark-color.svg')} alt="" width="36" height="36" />
			<span>HolyArrs</span>
		</div>
		{#if backTo && session.s.state !== 'unknown' && !ssoFinishing && !plexWaiting}
			<span class="corner"><IconButton icon="chevronLeft" variant="glass" label={backTo.label} onclick={backTo.go} /></span>
		{/if}

		{#if session.s.state === 'unknown' || ssoFinishing}
			<!-- As tall as the usual sign-in form, so the card does not jump when the session answers. -->
			<div class="sk" class:tall={recent.length > 1} style="border-radius: var(--r-card)"></div>
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
							<p class="lead">Finish in the Plex window.</p>
							<button class="link" type="button" onclick={cancelPlex}>Cancel</button>
						</div>
					{:else if acctView !== 'none'}
						<!-- One person: their face and name lead; the heading stays for screen readers. -->
						<h1 class="title" class:sr-only={acctView === 'person'}>{acctView === 'list' ? "Who's signing in?" : 'Sign in'}</h1>
						{#if acctView === 'list'}
							<div class="accts" class:editing role="list" aria-label="People who signed in on this device" data-testid="recent-people">
								{#each shown as p, i (p.name)}
									<div class="acct" role="listitem">
										<button type="button" class="acct-pick" aria-label={editing ? `Forget ${p.name} on this device` : undefined} onclick={() => (editing ? dropPerson(p.name) : openPerson(i))}>
											{@render face(p, 36)}
											<span class="acct-name"><span class="acct-n">{p.name}</span><span class="acct-way">{wayName(p.way)}</span></span>
											<span class="acct-end" aria-hidden="true">{editing ? 'Forget' : '›'}</span>
										</button>
									</div>
								{/each}
								<!-- While editing, every row forgets; the add row steps aside. -->
								{#if !editing}
									<div class="acct" role="listitem">
										<button type="button" class="acct-pick" onclick={another}>
											<span class="face plus" aria-hidden="true">+</span>
											<span class="acct-name"><span class="acct-n">Use another account</span></span>
											<span class="acct-end" aria-hidden="true">›</span>
										</button>
									</div>
								{/if}
							</div>
							<button class="link" type="button" aria-pressed={editing} onclick={() => (editing = !editing)}>{editing ? 'Done' : 'Edit list'}</button>
						{:else if person}
							<div class="who" data-testid="signin-person">
								{@render face(person, 84)}
								<span class="who-n">{person.name}</span>
								<!-- The password box below already says it; the list keeps the label. -->
								{#if person.way !== 'password'}<span class="acct-way">{wayName(person.way)}</span>{/if}
							</div>
							{#if person.way === 'plex' || person.way === 'oidc'}
								{@const w = person.way}
								<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && lastWay === w, fail: lastWay === w ? failN : 0 }}>
									<Button variant="primary" size="lg" full type="button" {busy} onclick={() => press(w)}>
										{w === 'plex' ? 'Continue with Plex' : 'Continue with single sign-on'}
									</Button>
								</div>
								<p class="note">{w === 'plex' ? 'Opens Plex to confirm it is you.' : 'Opens your sign-in page.'}</p>
								{#if error}<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>{/if}
							{:else}
								<form onsubmit={submit} novalidate>
									<input type="text" name="username" autocomplete="username" value={name} hidden readonly />
									<div class="field">
										<label for="password">{person.way === 'password' ? 'Password' : `${wayName(person.way)} password`}</label>
										<div class="pw">
											<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" required />
											<IconButton icon={showPw ? 'eyeOff' : 'eye'} label="Show password" aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
										</div>
									</div>
									{#if error}
										<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
									{/if}
									<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}>
										<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>
											{person.way === 'password' ? 'Sign in' : `Sign in with ${wayName(person.way)}`}
										</Button>
									</div>
								</form>
							{/if}
							{#if shown.length === 1}<button class="link" type="button" onclick={showList}>Switch account</button>{/if}
						{/if}
						{#if formOn}{@render help()}{/if}
					{:else}
						<h1 class="title">{mode === 'setup' ? 'Set up HolyArrs' : mode === 'code' ? 'Two-step check' : via === 'jellyfin' || via === 'emby' ? `Sign in with ${wayWord[via]}` : 'Sign in'}</h1>
						{#if mode === 'setup'}
							<p class="lead">{setupServer ? `Use an admin account on your ${wayWord[setupServer]} server.` : 'Create the owner account.'}</p>
							{#if session.s.configUnmapped}
								<p class="form-error" role="status">/config isn't mapped. This setup is lost when the container is recreated. See "Forgot to map /config?" in the install guide.</p>
							{/if}
						{:else if mode === 'code'}
							<p class="lead">{useBackup ? 'Enter one of your saved backup codes.' : 'Enter the 6-digit code from your authenticator app to finish signing in.'}</p>
						{:else if via === 'jellyfin' || via === 'emby'}
							<p class="lead">Use your {wayWord[via]} name and password.</p>
						{:else if via === 'choose'}
							<p class="lead">Choose how to sign in.</p>
						{/if}

						{#if mode === 'setup' && setupWay === 'choose'}
							<div class="provs" data-n={setupTiles.length} data-testid="setup-ways">
								{#each setupTiles as w (w)}
									<button class="prov" type="button" use:pixelTrail={{ busy: false, fail: lastWay === w ? failN : 0 }} data-way={w} disabled={busy} onclick={() => setupPress(w)}>
										{#if w === 'password'}
											<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linejoin="round"><rect x="5" y="10.5" width="14" height="9.5" rx="2.5" /><path d="M8.5 10.5V7.8a3.5 3.5 0 0 1 7 0v2.7" stroke-linecap="round" /></svg>
										{:else if w === 'plex'}
											<svg viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
										{:else if w === 'jellyfin'}
											<svg viewBox="0 0 24 24" aria-hidden="true"><defs><linearGradient id="jg-setup" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg-setup)" /></svg>
										{:else}
											<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>
										{/if}
										<span>{setupName(w)}</span>
									</button>
								{/each}
							</div>
							{#if error}
								<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
							{/if}
						{:else if mode === 'setup' && setupServer}
							{@const kind = setupServer}
							<form onsubmit={submitServer} novalidate>
								<div class="field">
									<label for="addr">Address</label>
									<input id="addr" class="input mono" bind:value={address} placeholder="http://192.168.1.10:8096" inputmode="url" autocomplete="url" autocapitalize="none" spellcheck="false" maxlength="500" required />
									{#if foundHere.length}
										<div class="accts found" role="group" aria-label="Found on your network" data-testid="setup-found">
											{#each foundHere as f (f.address)}
												<div class="acct">
													<button type="button" class="acct-pick" aria-pressed={address === f.address} onclick={() => { address = f.address; focusStep(); }}>
														<span class="acct-name"><span class="acct-n">{f.name}</span><span class="acct-way mono">{f.address}</span></span>
														<span class="acct-end" aria-hidden="true">{address === f.address ? '✓' : '›'}</span>
													</button>
												</div>
											{/each}
										</div>
									{:else if looking}
										<p class="note looking" role="status">Looking on your network…</p>
									{/if}
								</div>
								<div class="field">
									<label for="name">Name</label>
									<input id="name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="128" required />
								</div>
								<div class="field">
									<label for="password">Password</label>
									<div class="pw">
										<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" />
										<IconButton icon={showPw ? 'eyeOff' : 'eye'} label="Show password" aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
									</div>
								</div>
								{#if error}
									<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
								{/if}
								<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy, fail: failN }}>
									<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>Sign in with {wayWord[kind]}</Button>
								</div>
							</form>
						{:else if mode === 'code'}
							<form onsubmit={submitCode} novalidate>
								{#if useBackup}
									<div class="field">
										<label for="otp">Backup code</label>
										<input id="otp" class="input mono otp" inputmode="text" bind:value={otp} autocomplete="off" spellcheck="false" maxlength="11" required />
									</div>
								{:else}
									<div class="digits" role="group" aria-label="6-digit code" bind:this={digitsEl}>
										{#each digits as d, i (i)}
											{#if i === 3}<span class="dash" aria-hidden="true">–</span>{/if}
											<input
												class="digit"
												value={d}
												inputmode="numeric"
												autocomplete={i === 0 ? 'one-time-code' : 'off'}
												aria-label="Digit {i + 1}"
												onfocus={(e) => e.currentTarget.select()}
												oninput={(e) => typeDigit(i, e)}
												onkeydown={(e) => digitKey(i, e)}
												onpaste={(e) => pasteDigits(i, e)}
											/>
										{/each}
									</div>
								{/if}
								{#if error}
									<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
								{/if}
								<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}><Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>Sign in</Button></div>
								<button class="link" type="button" onclick={toggleBackup}>{useBackup ? 'Use the 6-digit code' : 'Use a backup code'}</button>
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
									<div class="field">
										<label for="name">Name</label>
										<input id="name" class="input" bind:value={name} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength={via === 'password' ? 64 : 128} required />
									</div>
									<div class="field">
										<label for="password">Password</label>
										<div class="pw">
											{#if mode === 'setup'}
												<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="new-password" required />
											{:else}
												<input id="password" class="input" type={showPw ? 'text' : 'password'} bind:value={pw} autocomplete="current-password" required />
											{/if}
											<IconButton icon={showPw ? 'eyeOff' : 'eye'} label="Show password" aria-pressed={showPw} onclick={() => (showPw = !showPw)} />
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
										<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
									{/if}
									<div class="submit" use:pixelTrail={{ selector: '.ui-btn', busy: busy && !tileWay, fail: tileWay ? 0 : failN }}>
										<Button variant="primary" size="lg" full type="submit" disabled={waiting > 0} {busy}>
											{mode === 'setup' ? 'Create account' : via === 'jellyfin' || via === 'emby' ? `Sign in with ${wayWord[via]}` : 'Sign in'}
										</Button>
									</div>
									{#if mode === 'login' && (via === 'jellyfin' || via === 'emby')}
										<button class="link" type="button" onclick={() => pick('choose')}>Other ways to sign in</button>
									{/if}
								</form>
							{:else if error}
								<p class="form-error" role="alert">{error}{#if waiting > 0}<span class="mono" aria-hidden="true"> ({waiting}s)</span>{/if}</p>
							{/if}

							{#if mode === 'login' && via === 'password'}{@render help()}{/if}
						{/if}
					{/if}
				</div>
			{/key}
		{/if}
	</div>
	<SigninPortals />
	<!-- Light or dark, quietly in the top-right corner; it changes this device's appearance. -->
	<span class="mode" data-theme={theme.value}>
		<IconButton
			icon={theme.value === 'light' ? 'moon' : 'sun'}
			variant="glass"
			tipSide="below-end"
			label={theme.value === 'light' ? 'Dark appearance' : 'Light appearance'}
			onclick={() => {
				// Switch in one frame: no 150 ms fades on rows and fields while the glass changes at once.
				panelEl?.classList.add('swap');
				setTheme(theme.value === 'light' ? 'dark' : 'light');
				requestAnimationFrame(() => requestAnimationFrame(() => panelEl?.classList.remove('swap')));
			}}
		/>
	</span>
</main>

<style>
	/* Direction B2: one cinematic backdrop, the panel always centred on dark
	   hairline glass. The page stays dark in light mode too. */
	.wrap {
		position: relative;
		min-height: 100dvh;
		display: grid;
		/* The card's top stays put: a step change, an error note or the open help
		   grows it downward (and the page scrolls) instead of re-centring it. The
		   top offset centres a usual-height card. */
		justify-items: center;
		align-items: start;
		/* --fr mirrors the edge frame's thickness (SigninPortals), so the panel clears it. */
		--fr: 14px;
		/* Centres a usual (~560 px) card, but never so low that the tallest (~640 px) would not fit. */
		padding: calc(var(--fr) + max(clamp(16px, 5vh, 48px), (100dvh - 640px) / 2, min((100dvh - 560px) / 2, 100dvh - 640px - 2 * var(--fr) - clamp(16px, 5vh, 48px)))) calc(clamp(16px, 5vw, 96px) + var(--fr)) calc(clamp(16px, 5vh, 48px) + var(--fr));
		background: #050507;
		color: var(--text);
		isolation: isolate;
	}
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one
	   before; the logo settles from a hair smaller instead. Opacity and
	   transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		.intro .brand { animation: brand-in var(--dur-slow) var(--ease-out) 60ms both; }
		.intro :is(.title, .lead, .provs, .or, .field, .submit, .step .link, .help) {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
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
		padding: 34px 34px 32px;
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
	.panel:global([data-mat='blur']) {
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
		.panel.light { background: rgb(248 246 242 / 0.94); }
	}

	/* Light mode: tinted glass. The blur lifts the poster under the panel and
	   keeps its colour; text is dark ink, links are dark and underlined (red on
	   tinted glass reads poorly), and red stays on the main button. */
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
	.mark-light, .light .mark-dark { display: none; }
	.light .mark-light { display: block; }
	.light .brand, .light .title, .light .who-n { color: #111; }
	.light .lead, .light .note, .light .acct-way, .light .help { color: rgb(0 0 0 / 0.64); }
	.light .help .note { color: rgb(0 0 0 / 0.56); }
	.light .or { color: rgb(0 0 0 / 0.62); }
	.light .or::before, .light .or::after { background: rgb(0 0 0 / 0.14); }
	.light :global(.input) { background: rgb(255 255 255 / 0.6); border-color: rgb(255 255 255 / 0.75); color: #111; }
	/* The resting light field above outranks app.css's hover, focus and error borders: give them back. */
	.light :global(.input:hover) { border-color: rgb(0 0 0 / 0.16); }
	.light :global(.input:focus-visible) { border-color: var(--accent); box-shadow: 0 0 0 3px rgb(229 53 43 / 0.22); }
	.light :global(.input[aria-invalid='true']) { border-color: var(--fail); }
	.light .link, .light .help summary {
		color: #1d1d1f;
		font-weight: 600;
		text-decoration: underline;
		text-underline-offset: 3px;
		text-decoration-color: rgb(0 0 0 / 0.25);
	}
	.light .form-error { background: rgb(255 241 240 / 0.88); border-color: rgb(215 0 21 / 0.35); color: #9b0010; }
	.light .who .face { box-shadow: 0 0 0 3px rgb(255 255 255 / 0.85), 0 10px 28px rgb(0 0 0 / 0.18); }
	.light .accts { border-color: rgb(0 0 0 / 0.08); }
	.light .acct + .acct { border-top-color: rgb(0 0 0 / 0.07); }
	/* Rest faint, hover clearly lit: a small step reads as the wrong row when the poster's tint shifts under the glass. */
	.light .acct-pick { background: rgb(255 255 255 / 0.22); color: #111; }
	@media (hover: hover) { .light .acct-pick:hover { background: rgb(255 255 255 / 0.8); } }
	.light .acct-pick:active { background: rgb(255 255 255 / 0.92); }
	.light .acct-pick:focus-visible { outline-color: #1d1d1f; }
	.light .acct-end { color: rgb(0 0 0 / 0.35); }
	.light .accts.editing .acct-end { color: #1d1d1f; }
	.light .face.plus { background: rgb(0 0 0 / 0.05); border-color: rgb(0 0 0 / 0.25); color: rgb(0 0 0 / 0.6); }
	.light .prov { background: rgb(255 255 255 / 0.45); border-color: rgb(255 255 255 / 0.6); color: #1d1d1f; }
	.light .provs[data-n='1'] .prov { color: #111; }
	.light .prov svg[stroke='#f5f5f7'] { stroke: #1d1d1f; }
	@media (hover: hover) { .light .prov:not(:disabled):hover { background: rgb(255 255 255 / 0.65); color: #111; } }
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		/* A soft lift on light glass; the dark mode's deep shadow looks like a smudge here. */
		.light .prov:not(:disabled):hover,
		.light .submit :global(.ui-btn:not(:disabled):hover) { box-shadow: 0 4px 14px rgb(0 0 0 / 0.1), 0 1px 3px rgb(0 0 0 / 0.06); }
	}
	.light .cmd { background: rgb(255 255 255 / 0.5); border-color: rgb(0 0 0 / 0.08); }
	.light .cp { border-color: rgb(0 0 0 / 0.15); color: #111; }
	/* Light badges: a white disc, not a heavy black one. */
	.light .badge { background: #fff; box-shadow: 0 0 0 2px #fff; }
	.light .badge svg[stroke='#f5f5f7'] { stroke: #1d1d1f; }
	.light .help .ans + .ans { border-top-color: rgb(0 0 0 / 0.1); }
	.light .orbit { border-color: rgb(0 0 0 / 0.14); border-top-color: #e5a00d; }

	/* The appearance switch: inside the frame's top-right corner, above the art. */
	.mode { position: absolute; z-index: 3; top: calc(var(--fr) + 14px); right: calc(var(--fr) + 14px); }
	.wrap.leaving .mode { display: none; }
	/* Barely there, like a watermark: a faint icon with no fill until hover or focus. */
	.mode :global(.ui-ibtn.glass) {
		color: rgb(255 255 255 / 0.5);
		background: transparent;
		border-color: transparent;
		box-shadow: none;
		-webkit-backdrop-filter: none;
		backdrop-filter: none;
		filter: drop-shadow(0 1px 2px rgb(0 0 0 / 0.35));
		transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
	}
	.mode :global(.ui-ibtn.glass:focus-visible) { color: rgb(255 255 255 / 0.9); background: rgb(255 255 255 / 0.1); }
	/* Hover: a soft dark disc and a full-white icon, readable over bright art too. */
	@media (hover: hover) { .mode :global(.ui-ibtn.glass:hover) { color: #fff; background: rgb(0 0 0 / 0.3); } }
	.mode :global(.ui-ibtn.glass:focus-visible) { outline: none; box-shadow: 0 0 0 2px rgb(255 255 255 / 0.85); }
	.light .corner :global(.ui-ibtn.glass) {
		color: #1d1d1f;
		background: rgb(255 255 255 / 0.55);
		border-color: rgb(255 255 255 / 0.6);
		box-shadow: 0 2px 10px rgb(0 0 0 / 0.12);
	}
	@media (hover: hover) { .light .corner :global(.ui-ibtn.glass:hover) { background: rgb(255 255 255 / 0.85); } }
	.light .corner :global(.ui-ibtn.glass:focus-visible) { outline: none; box-shadow: var(--focus-ring); }

	/* Two-factor: six digit boxes, a dash after the third. */
	.digits { display: flex; justify-content: center; gap: 8px; }
	/* The boxes share the row, up to 44 px each, so six always fit (360 px phones too). */
	.digit {
		flex: 1 1 0;
		/* width 0: an empty input's own width must not widen the row (and the card) on 360 px phones. */
		width: 0;
		min-width: 0;
		max-width: 44px;
		height: 54px;
		padding: 0;
		text-align: center;
		border-radius: 12px;
		border: 1px solid var(--line);
		background: var(--surface-2);
		color: var(--text);
		font: 600 22px/1 var(--font-mono);
		caret-color: var(--accent);
	}
	.digit:focus { outline: none; border-color: var(--accent) !important; }
	.dash { align-self: center; color: rgb(255 255 255 / 0.4); }
	.light .digit { background: rgb(255 255 255 / 0.6); border-color: rgb(255 255 255 / 0.75); color: #111; }
	.light .dash { color: rgb(0 0 0 / 0.35); }
	.brand {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 14px;
		margin-bottom: 28px;
		font: 650 24px/1 var(--font-ui);
		letter-spacing: -0.015em;
		color: #fff;
	}
	.panel > .sk { height: 311px; background: rgb(255 255 255 / 0.06); }
	.panel.light > .sk { background: rgb(0 0 0 / 0.05); }
	/* Two or more remembered people open on the taller list. */
	.panel > .sk.tall { height: 415px; }
	.panel:global(.swap), .panel:global(.swap) :global(*) { transition: none !important; }
	@media (max-width: 600px) { .panel > .sk { height: 289px; } }
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; text-wrap: balance; }
	.lead { margin: 8px 0 20px; color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; text-wrap: pretty; }
	.title + :not(.lead) { margin-top: 26px; }
	.panel :global(.field + .field) { margin-top: 12px; }
	/* Labels, hints and errors centre like the title and button; typing stays left. */
	.panel :global(.field) { text-align: center; }
	.panel :global(.field .input) { text-align: left; }
	.pw { position: relative; display: flex; align-items: center; }
	.pw .input { padding-right: 52px; }
	/* Edge draws its own eye in password fields; ours is the only one. */
	.pw .input::-ms-reveal { display: none; }
	.pw > :global(*:last-child) { position: absolute; right: 4px; }
	.otp { text-align: center; font-size: 22px; letter-spacing: 0.3em; }
	.submit { margin-top: 20px; }
	/* Busy keeps the button's full colour (only the spinner says it is working); disabled stays dimmed. */
	.submit :global(.ui-btn[aria-busy='true']:disabled) { opacity: 1; }
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
	code { font-family: var(--font-mono); color: var(--text); overflow-wrap: anywhere; }

	/* Provider tiles: one row, mark over name; a lone tile is a wide button. */
	.provs { display: grid; gap: 10px; grid-template-columns: repeat(auto-fit, minmax(0, 1fr)); }
	/* Marks sit on one line across the row: a two-line label ("Single sign-on")
	   grows downward instead of pushing its mark up. */
	.prov {
		position: relative;
		overflow: hidden;
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
	/* "Single / sign-on", never "Single sign- / on". */
	.prov span { text-wrap: balance; }
	.provs[data-n='1'] .prov { flex-direction: row; justify-content: center; padding: 0 6px; min-height: 50px; gap: 10px; font-size: 16px; color: #fff; }
	.prov:active:not(:disabled) { transform: scale(0.97); }
	.prov:disabled { opacity: 0.5; cursor: default; }
	.prov:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* People who signed in here: a list of rows, or one person's step. */
	.accts { display: flex; flex-direction: column; border: 1px solid rgb(255 255 255 / 0.1); border-radius: 16px; overflow: hidden; }
	.acct + .acct { border-top: 1px solid rgb(255 255 255 / 0.08); }
	.acct-pick {
		display: flex;
		align-items: center;
		gap: 14px;
		width: 100%;
		min-height: 64px;
		padding: 12px 18px 12px 14px;
		border: 0;
		background: rgb(255 255 255 / 0.03);
		color: #f5f5f7;
		font: 500 15px/1.2 var(--font-ui);
		text-align: left;
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease-out);
	}
	/* One ring, inside the row (the global ring would bleed onto the next row). */
	.acct-pick:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; box-shadow: none; }
	@media (hover: hover) {
		.acct-pick:hover { background: rgb(255 255 255 / 0.12); }
		/* The arrow of the row under the pointer steps forward and darkens/brightens. */
		.acct-pick:hover .acct-end { color: currentColor; transform: translateX(2px); }
	}
	.acct-pick:active { background: rgb(255 255 255 / 0.16); }
	.acct-end { transition: transform var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.acct-name { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.acct-n, .who-n { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.acct-way { font: 400 12px/1.2 var(--font-ui); color: rgb(255 255 255 / 0.55); white-space: nowrap; }
	.acct-end { margin-left: auto; flex: none; color: rgb(255 255 255 / 0.4); font-size: 18px; }
	.accts.editing .acct-end { color: var(--accent-text); font: 500 13px/1 var(--font-ui); }
	.face {
		position: relative;
		flex: none;
		display: inline-grid;
		place-items: center;
		width: var(--s, 36px);
		height: var(--s, 36px);
		border-radius: 50%;
		background: hsl(var(--h, 0) 45% 32%);
		color: #fff;
		font: 600 calc(var(--s, 36px) * 0.42)/1 var(--font-ui);
	}
	.face.plus { background: rgb(255 255 255 / 0.08); border: 1px dashed rgb(255 255 255 / 0.3); color: rgb(255 255 255 / 0.75); }
	.badge { position: absolute; right: -4px; bottom: -4px; width: 18px; height: 18px; border-radius: 50%; display: grid; place-items: center; background: #111; box-shadow: 0 0 0 2px #111; }
	.badge svg { width: 14px; height: 14px; }
	/* One remembered person: a big face in a soft ring, their name under it. */
	.who { display: flex; flex-direction: column; align-items: center; gap: 12px; margin: 4px 0 24px; }
	.title.sr-only + .who { margin-top: 4px; }
	.who .face { box-shadow: 0 0 0 3px rgb(255 255 255 / 0.22), 0 10px 28px rgb(0 0 0 / 0.35); }
	.who .badge { right: 0; bottom: 0; width: 26px; height: 26px; }
	.who .badge svg { width: 18px; height: 18px; }
	.who-n { max-width: 100%; color: #fff; font: 600 20px/1.2 var(--font-ui); letter-spacing: -0.01em; }
	/* The way sits close under the name, as in the list rows. */
	.who .acct-way { margin-top: -8px; }
	/* Back to the list: a round button in the panel's top-left corner, level with the logo. */
	.corner { position: absolute; top: 34px; left: 34px; z-index: 1; }
	.face img { position: absolute; inset: 0; width: 100%; height: 100%; border-radius: 50%; object-fit: cover; }
	.note { margin-top: 10px; text-align: center; color: rgb(255 255 255 / 0.55); font: var(--t-footnote); }
	/* First run: servers found on the network sit above the address field. */
	.found { margin-top: 8px; }
	.found .acct-pick { min-height: 56px; }
	.found .acct-pick[aria-pressed='true'] .acct-end { color: var(--accent-text); }
	.light .found .acct-pick[aria-pressed='true'] .acct-end { color: #1d1d1f; }
	.found .acct-way { overflow: hidden; text-overflow: ellipsis; }
	.looking { margin: 6px 0 0; }
	.or { display: flex; align-items: center; gap: 10px; margin: 18px 0; color: rgb(255 255 255 / 0.45); font: var(--t-footnote); }
	.or::before, .or::after { content: ''; flex: 1; height: 1px; background: rgb(255 255 255 / 0.12); }

	/* Hover lives on the buttons only: a 1 px lift, a soft glow and the
	   site's pixel trail inside the button (pixelTrail.ts paints a canvas
	   clipped to the button's radius). Focus rings come from :focus-visible. */
	.submit :global(.ui-btn) { position: relative; overflow: hidden; isolation: isolate; transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.prov { isolation: isolate; }
	.prov > :global(:not(canvas)),
	.submit :global(.ui-btn > :not(canvas)) { position: relative; z-index: 1; }
	/* Colour on hover for everyone; the lift only when motion is welcome. */
	@media (hover: hover) { .prov:not(:disabled):hover { background: rgb(118 118 128 / 0.34); color: #fff; } }
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		.prov:not(:disabled):hover,
		.submit :global(.ui-btn:not(:disabled):hover) {
			transform: translateY(-1px);
			box-shadow: 0 6px 22px rgb(0 0 0 / 0.35), 0 0 18px rgb(255 255 255 / 0.08);
		}
		/* Pressed: settle back down, so a click reads as a press. */
		.prov:not(:disabled):active,
		.submit :global(.ui-btn:not(:disabled):active) { transform: translateY(0) scale(0.98); box-shadow: none; }
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
	.help { margin-top: 12px; color: var(--text-muted); font: var(--t-footnote); scroll-margin-bottom: calc(var(--fr, 0px) + 16px); }
	/* Sized to its words, like the other text links (not the whole panel width). */
	.help summary { cursor: pointer; width: fit-content; margin: 0 auto; padding: 0 10px; min-height: 36px; display: flex; align-items: center; justify-content: center; border-radius: var(--r-sm); color: var(--accent-text); font: 500 14px/1 var(--font-ui); list-style: none; }
	.help summary::-webkit-details-marker { display: none; }
	.help summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) and (pointer: coarse) { .help summary { min-height: 44px; } }
	.help p { margin-top: 6px; }
	.help .ans { padding: 12px 0; text-align: left; }
	.help .ans + .ans { border-top: 1px solid rgb(255 255 255 / 0.08); }
	.help .ans b { display: block; color: var(--text); font-weight: 600; }
	.help .note { margin-top: 10px; text-align: left; font-size: 12px; }
	.cmd { display: flex; align-items: center; gap: 8px; margin-top: 8px; padding: 6px 6px 6px 12px; border-radius: 10px; background: rgb(255 255 255 / 0.06); border: 1px solid rgb(255 255 255 / 0.08); }
	.cmd code { flex: 1; min-width: 0; font-size: 12px; line-height: 1.45; }
	.cp { flex: none; min-height: 32px; min-width: 64px; padding: 0 12px; border-radius: 8px; border: 1px solid rgb(255 255 255 / 0.14); background: transparent; color: var(--text); font: 500 12px/1 var(--font-ui); cursor: pointer; }
	.cp:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Text links, the help summary and Copy: a faint wash on hover, a firmer one on press. */
	@media (hover: hover) {
		.link:hover, .help summary:hover, .cp:hover { background: rgb(255 255 255 / 0.08); }
		.light .link:hover, .light .help summary:hover, .light .cp:hover { background: rgb(0 0 0 / 0.05); }
	}
	.link:active, .help summary:active, .cp:active { background: rgb(255 255 255 / 0.14); }
	.light .link:active, .light .help summary:active, .light .cp:active { background: rgb(0 0 0 / 0.09); }

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
	@media (min-width: 601px) { .wrap { --fr: 18px; } }
	@media (min-width: 1280px) { .wrap { --fr: 22px; } }
	@media (min-width: 1440px) { .wrap { --fr: 24px; } }
	@media (min-width: 1920px) { .wrap { --fr: 32px; } }
	@media (min-width: 2560px) { .wrap { --fr: 40px; } }
	/* Large screens: type, inputs and spacing grow with the panel. */
	@media (min-width: 1600px) and (min-height: 760px) { .panel { --z: 1.08; } }
	@media (min-width: 2200px) and (min-height: 1000px) { .panel { --z: 1.2; } }
	@media (min-width: 3000px) and (min-height: 1300px) { .panel { --z: 1.32; } }
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of
	   art on every side; it rides above the keyboard (--kb) with a smooth lift. */
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
		.brand { gap: 12px; margin-bottom: 16px; font-size: 18px; }
		.brand img { width: 26px; height: 26px; }
		.title { font-size: 20px; line-height: 1.15; }
		.lead { margin: 4px 0 10px; }
		.title + :not(.lead) { margin-top: 16px; }
		.who { gap: 10px; margin-bottom: 16px; }
		.who .face { width: 72px; height: 72px; font-size: 30px; }
		.who-n { font-size: 18px; }
		.corner { top: 6px; left: 16px; }
		.mode { top: max(14px, env(safe-area-inset-top)); right: max(14px, env(safe-area-inset-right)); }
		.digits { gap: 6px; }
		.digit { max-width: 40px; height: 50px; }
		.acct-pick { min-height: 60px; }
		.panel :global(.field) { gap: 2px; }
		.panel :global(.field label) { font-size: 14px; line-height: 1.25; }
		.panel :global(.field + .field) { margin-top: 4px; }
		.panel :global(.input), .submit :global(.ui-btn), .prov { min-height: 48px; }
		.submit { margin-top: 8px; }
		.submit :global(.ui-btn) { width: 100%; }
		.prov { min-height: 64px; padding-top: 11px; }
		.provs[data-n='5'] { gap: 6px; }
		.provs[data-n='5'] .prov { padding-inline: 2px; font-size: 11px; }
		.panel > .sk.tall { height: 375px; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
		.step > :is(.provs, .note, .form-error):last-child { margin-bottom: 12px; }
		.or { margin: 10px 0; }
		.link { min-height: 40px; margin-bottom: 0; margin-top: 0; }
		.help { margin-top: 2px; }
	}
	/* Touch screens: 44 px tap targets for the small controls (after the phone block, so it wins). */
	@media (pointer: coarse) {
		.link, .help summary, .cp { min-height: 44px; }
	}
	/* Reduced motion: no keyboard-lift slide on phones either. */
	@media (prefers-reduced-motion: reduce) and (max-width: 600px) {
		.panel { transition: none; }
	}
</style>
