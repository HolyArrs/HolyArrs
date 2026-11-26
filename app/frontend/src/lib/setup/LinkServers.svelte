<!-- First-run "Link your media servers": the owner links any of Plex, Jellyfin and Emby (each optional). A link is an extra way to sign in to this account, and the server is then connected like the Media server step does (address, key, libraries). -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api, ApiError, message } from '#lib/core/api.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { SigninServerForm } from '#lib/accounts/index.ts';
	import { closePlexWindow, openPlexWindow, sendToPlex } from '#lib/accounts/index.ts';
	import ConnectServer from './ConnectServer.svelte';
	import type { Library, ServerKind } from './welcome-steps.ts';
	import { onDestroy, onMount, tick } from 'svelte';
	import { session, type FoundServer } from '#lib/core/session.svelte.ts';

	// connecting: the linked server being connected (its own sub-screen; the page's title follows it, and Back clears it).
	let { busy = $bindable(false), connecting = $bindable(null), open = $bindable(null), onnext }: { busy?: boolean; connecting?: ServerKind | null; open?: ServerKind | null; onnext: (libs: Library[]) => void } = $props();

	const KINDS: ServerKind[] = ['plex', 'jellyfin', 'emby'];
	const word: Record<ServerKind, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' };
	const wayWord = { jellyfin: 'Jellyfin', emby: 'Emby' } as const;

	// The account name each kind is linked as ('' = not linked).
	let linked = $state<Record<ServerKind, string>>({ plex: '', jellyfin: '', emby: '' });
	// The row whose sign-in is open, and the row connecting its server (libraries).
	// The picked libraries per server, so an unlink drops that server's and a second link replaces them.
	let libs: Partial<Record<ServerKind, Library[]>> = {};
	let error = $state('');
	let failN = $state(0);
	// Jellyfin or Emby sign-in fields.
	let address = $state('');
	let name = $state('');
	let pw = $state('');
	let showPw = $state(false);
	const anyLinked = $derived(KINDS.some((k) => linked[k]));
	// The saved links are known (a reload on the Plex screen waits for them instead of asking to sign in again).
	let loaded = $state(false);

	onMount(loadLinks);
	// Back from a server screen (Continue or Back): the cards show what is linked now.
	let wasOpen = false;
	$effect(() => {
		if (connecting) wasOpen = true;
		// Page Back closed the Plex screen: stop waiting on the Plex window too.
		else plexStop = true;
		if (!connecting && wasOpen) {
			wasOpen = false;
			void loadLinks();
		}
	});
	async function loadLinks() {
		try {
			const r = await api.get<{ links: { provider: string; name: string }[] }>('/auth/media-link');
			for (const l of r.links ?? []) if (l.provider === 'jellyfin' || l.provider === 'emby') linked[l.provider] = l.name || word[l.provider];
		} catch {
			// None yet.
		}
		try {
			const p = await api.get<{ linked: boolean; name: string }>('/auth/plex/my-link');
			if (p.linked) linked.plex = p.name || 'Plex';
		} catch {
			// None yet.
		}
		// A linked server counts as done only once it watches at least one library.
		try {
			const ms = (await api.get<{ media_servers: { id: number; kind: string }[] }>('/media-servers')).media_servers ?? [];
			// All servers at once, and set together: the rows never show a half-checked state.
			const got = await Promise.all(KINDS.map(async (k) => {
				const m = linked[k] ? ms.find((x) => x.kind === k) : undefined;
				return !!m && ((await api.get<{ libraries: Library[] }>(`/media-servers/${m.id}/libraries`)).libraries ?? []).some((l) => l.on);
			}));
			KINDS.forEach((k, i) => (ready[k] = got[i]));
		} catch {
			// Unknown: trust the links.
			for (const k of KINDS) ready[k] = !!linked[k];
		}
		loaded = true;
	}
	let ready = $state<Record<ServerKind, boolean>>({ plex: false, jellyfin: false, emby: false });
	// Continue: a server linked but left before its libraries were picked opens again first.
	function go() {
		const half = KINDS.find((k) => linked[k] && !ready[k] && !libs[k]);
		if (half) return start(half);
		onnext(KINDS.flatMap((k) => (linked[k] ? (libs[k] ?? []) : [])));
	}

	// Keyboard focus follows the screen: the new screen's first control, or back on the tile the person left from.
	let lastKind: ServerKind = 'plex';
	const view = $derived(connecting === 'plex' && !linked.plex ? 'plex' : connecting ? 'connect' : open === 'jellyfin' || open === 'emby' ? 'form' : 'tiles');
	let firstView = true;
	$effect(() => {
		const v = view;
		if (firstView) return void (firstView = false);
		const sel = v === 'tiles' ? `[data-testid="link-${lastKind}"], [data-testid="edit-${lastKind}"]` : v === 'plex' ? '[data-testid="link-plex-start"]' : v === 'form' ? '[data-testid="link-form"] input' : '[data-testid="welcome-connect"] input:not(:disabled), [data-testid="welcome-connect"] :is(select, button):not(:disabled)';
		// The server screen draws its form after a load: try for a few frames until a control is there.
		let n = 0;
		const tryFocus = () => {
			const el = document.querySelector<HTMLElement>(sel);
			if (el) return el.focus();
			if (++n < 60) requestAnimationFrame(tryFocus);
		};
		void tick().then(tryFocus);
	});

	function start(k: ServerKind) {
		lastKind = k;
		error = '';
		// Each server opens its own screen: Plex signs in there first, Jellyfin and Emby take address, name and password, then libraries.
		connecting = k;
	}

	async function linkServer(e: SubmitEvent) {
		e.preventDefault();
		const k = open;
		if (k !== 'jellyfin' && k !== 'emby') return;
		error = '';
		if (!address.trim()) return void (error = t('accounts.login.enterAddress', { way: wayWord[k] }));
		if (!name.trim()) return void (error = t('accounts.login.enterName', { way: wayWord[k] }));
		busy = true;
		try {
			const r = await api.post<{ name: string }>(`/auth/setup/link/${k}`, { url: /^[a-z]+:\/\//i.test(address.trim()) ? address.trim() : `http://${address.trim()}`, name: name.trim(), password: pw });
			linked[k] = r.name || name.trim();
			open = null;
			connecting = k;
		} catch (err) {
			failN++;
			error = message(err);
		} finally {
			pw = '';
			busy = false;
		}
	}

	// Waiting on the Plex window: Cancel, or closing the window, stops the wait and leaves the cards as they were.
	let plexWaiting = $state(false);
	let plexStop = false;
	onDestroy(() => (plexStop = true));
	async function linkPlex() {
		busy = true;
		plexWaiting = true;
		plexStop = false;
		const win = openPlexWindow('plex-link');
		// The pressed button is now busy: keep focus on the step's Cancel.
		void tick().then(() => document.querySelector<HTMLElement>('[data-testid="link-plex-cancel"]')?.focus());
		try {
			const r = await api.post<{ id: number; auth_url: string }>('/auth/plex/my-link/start', {});
			sendToPlex(win, r.auth_url);
			const until = Date.now() + 10 * 60 * 1000;
			let tick = 0;
			while (Date.now() < until) {
				await new Promise((ok) => setTimeout(ok, 500));
				if (plexStop) return;
				// Closed window: one last ask (the done page closes itself after a sign-in), then stop quietly.
				const shut = !!win?.closed;
				if (!shut && ++tick % 4) continue;
				let done: { name: string } | { waiting: true };
				try {
					done = await api.post<{ name: string } | { waiting: true }>('/auth/plex/my-link/finish', { id: r.id });
				} catch (err) {
					if (err instanceof ApiError && err.status === 429) continue;
					throw err;
				}
				if ('waiting' in done) {
					if (shut) return;
					continue;
				}
				linked.plex = done.name || 'Plex';
				if (!plexStop) connecting = 'plex';
				return;
			}
			error = t('accounts.security.plexTimeout');
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
			plexWaiting = false;
			closePlexWindow(win);
			void tick().then(() => document.querySelector<HTMLElement>('[data-testid="link-plex-start"]')?.focus());
		}
	}

	async function unlink(k: ServerKind) {
		error = '';
		busy = true;
		try {
			await api.del(k === 'plex' ? '/auth/plex/my-link' : `/auth/media-link/${k}`);
			// In setup, Unlink also drops the server saved for it here, so its screen starts empty again.
			const have = (await api.get<{ media_servers: { id: number; kind: string }[] }>('/media-servers')).media_servers ?? [];
			for (const m of have.filter((x) => x.kind === k)) await api.del(`/media-servers/${m.id}`);
			linked[k] = '';
			ready[k] = false;
			delete libs[k];
			if (k !== 'plex') ((address = ''), (name = ''));
			lastKind = k;
			await tick();
			document.querySelector<HTMLElement>(`[data-testid="link-${k}"]`)?.focus();
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	function connected(l: Library[]) {
		if (connecting) ((libs[connecting] = l), (ready[connecting] = true));
		connecting = null;
		void loadLinks();
	}
	// The network search runs only when the person presses refresh (never on its own).
	let found = $state<FoundServer[]>([]);
	let looking = $state(false);
	async function findServers() {
		looking = true;
		found = await session.setupFindServers();
		looking = false;
	}
</script>

{#if connecting === 'plex' && !loaded}
	<div class="card plex-first" aria-busy="true"></div>
{:else if connecting === 'plex' && !linked.plex}
	<!-- Your Plex server: sign in with Plex first; then the same screen lists the account's servers. -->
	<div class="card plex-first" data-testid="link-plex">
		<div class="plex-mark"><svg viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg></div>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<!-- Always in the page, so screen readers announce the text when it changes. -->
		<p class="hint wait" role="status">{plexWaiting ? t('accounts.signin.waitLead') : ''}</p>
		<div class="acts">
			<Button variant="primary" size="lg" full type="button" disabled={busy} busy={plexWaiting} onclick={linkPlex} data-testid="link-plex-start">{t('accounts.signin.signInWith', { way: 'Plex' })}</Button>
			<!-- Always there (hidden while not waiting), so the main button never moves. -->
			<button class="alt" class:off={!plexWaiting} type="button" tabindex={plexWaiting ? 0 : -1} aria-hidden={!plexWaiting} onclick={() => (plexStop = true)} data-testid="link-plex-cancel">{t('welcome.link.cancel')}</button>
		</div>
	</div>
{:else if connecting}
	<ConnectServer kind={connecting} bind:busy onnext={connected} />
{:else if open === 'jellyfin' || open === 'emby'}
	<div class="card link-form" data-testid="link-form">
		<SigninServerForm kind={open} bind:address bind:name bind:pw bind:showPw foundHere={found.filter((f) => f.kind === open)} {looking} onrefresh={findServers} {error} waiting={0} {busy} {failN} {wayWord} submitServer={linkServer} focusStep={() => {}} />
		<button class="alt back" type="button" disabled={busy} onclick={() => ((open = null), (error = ''))}>{t('welcome.link.cancel')}</button>
	</div>
{:else}
	<div class="card" data-testid="welcome-link">
		<!-- The sign-in way tiles' look: one big card per server, three in a row (stacked on phones). Not linked: the card opens that server's sign-in; linked: a tick, who it is linked as, and Unlink. -->
		<div class="lrows">
			{#each KINDS as k (k)}
				<!-- One row like Parts: tick, logo, name with what it is linked as, and Edit / Unlink as pills. Not linked: the row opens that server's sign-in. -->
				<div class="lrow" class:on={!!linked[k]} data-kind={k}>
					{#if !linked[k]}<button class="lhit" type="button" disabled={busy} onclick={() => start(k)} data-testid="link-{k}" aria-label={word[k]} aria-describedby="desc-{k}"></button>{/if}
					<span class="ltick" class:todo={!(linked[k] && (!loaded || ready[k] || libs[k]))} aria-hidden="true">{#if linked[k] && (!loaded || ready[k] || libs[k])}<Icon name="check" size={14} stroke={2.5} />{/if}</span>
					<span class="llogo" aria-hidden="true">{#if k === 'plex'}<svg viewBox="0 0 24 24"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
					{:else if k === 'jellyfin'}<svg viewBox="0 0 24 24"><defs><linearGradient id="jg-link" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg-link)" /></svg>
					{:else}<svg viewBox="0 0 24 24"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>{/if}</span>
					<span class="lwords">
						<span class="lname" id="lname-{k}">{word[k]}</span>
						{#if linked[k]}<small class="lstate">{t('welcome.link.linkedAs', { name: linked[k] })}{#if loaded && !ready[k] && !libs[k]} · {t('welcome.link.finish')}{/if}</small>
						{:else}<small class="desc" id="desc-{k}">{t('welcome.way.server', { name: word[k] })}</small>{/if}
					</span>
					{#if linked[k]}<span class="lpills"><button class="pb" type="button" disabled={busy} onclick={() => ((error = ''), (lastKind = k), (connecting = k))} aria-describedby="lname-{k}" data-testid="edit-{k}">{t('welcome.link.edit')}</button><button class="pb" type="button" disabled={busy} onclick={() => unlink(k)} aria-describedby="lname-{k}" data-testid="unlink-{k}">{t('welcome.link.unlink')}</button></span>{/if}
				</div>
			{/each}
		</div>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<Button variant="primary" size="lg" full type="button" disabled={busy} onclick={go} data-testid="link-continue">{anyLinked ? t('welcome.continue') : t('welcome.link.skip')}</Button>
	</div>
{/if}

<style>
	/* Same tiles as the sign-in ways (48 px mark, 20 px name, one plain line), three across. */
	.lname { font: 600 20px/1.2 var(--font-ui); color: #fff; }
	.desc { max-width: 30ch; font: 400 15px/1.35 var(--font-ui); opacity: 0.72; text-wrap: pretty; }
	/* The whole card is the button while not linked. */
	.lhit { position: absolute; inset: 0; z-index: 2; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.lhit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.lhit:disabled { cursor: default; }
	.lstate { color: rgb(120 220 140); font: 500 15px/1.3 var(--font-ui); overflow-wrap: anywhere; }
	.alt:disabled { cursor: not-allowed; }
	@media (hover: hover) { .alt:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.1); } }
	.back { display: block; margin: 12px auto 0; }
	/* The sign-in page's rhythm for the Jellyfin or Emby sign-in inside the wizard card. */
	.link-form { --gap-field: 28px; --gap: 32px; max-width: 900px; width: 100%; margin-inline: auto; box-sizing: border-box; overflow-x: hidden; }
	.link-form :global(label) { font-size: 15px; }
	:global(.light) .lname { color: #1d1d1f; }
	/* Light glass: dark ink that passes 4.5:1, and a hover that gets whiter instead of grey. */
	:global(.light) .desc { opacity: 1; color: rgb(29 29 31 / 0.78); }
	:global(.light) .lstate { color: rgb(21 128 61); }
	.plex-first { display: flex; flex-direction: column; }
	.plex-mark { display: grid; place-items: center; min-height: 160px; }
	.plex-mark svg { width: 72px; height: 72px; }
	.alt { margin: 8px auto 0; min-height: 44px; padding: 0 18px; border: 0; border-radius: var(--r-sm); background: none; color: rgb(var(--ink-rgb, 255 255 255) / 0.72); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.alt.off { visibility: hidden; }
	.wait { min-height: 1.4em; margin: 0 0 8px; text-align: center; }
	.alt:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.acts { display: flex; flex-direction: column; margin-top: var(--gap, 24px); }
	/* Continue keeps one spot at the card's foot. */
	@media (min-width: 1100px) {
		[data-testid='welcome-link'] { display: flex; flex-direction: column; }
		.card.card[data-testid='welcome-link'] > :global(.ui-btn:last-child) { margin-top: auto; }
		.plex-first > .acts { margin-top: auto; }
	}
	/* Rows like Parts (Import from my apps): one 82 px row per server. */
	.lrows { display: grid; grid-template-columns: 1fr; gap: 10px; }
	.lrow { position: relative; display: flex; align-items: center; gap: 24px; min-height: 82px; padding: 0 28px; border-radius: var(--g-r, 16px); border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, transparent); color: var(--ink, var(--text)); }
	.lrow.on { border-color: color-mix(in srgb, var(--ok-text) 45%, var(--g-line, var(--line))); }
	.lhit { position: absolute; inset: 0; border: 0; background: none; border-radius: inherit; cursor: pointer; }
	.lhit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (hover: hover) { .lrow:has(.lhit:hover:not(:disabled)) { background: var(--g-bg-hover, transparent); border-color: var(--g-line-hover, var(--line-strong)); } }
	.ltick { flex: none; width: 22px; height: 22px; border-radius: 50%; display: grid; place-items: center; background: var(--ok-text); color: #fff; }
	.ltick.todo { background: none; border: 1.5px solid rgb(var(--ink-rgb, 255 255 255) / 0.3); }
	.llogo { flex: none; width: 56px; height: 56px; display: grid; place-items: center; }
	.llogo svg { width: 48px; height: 48px; }
	.lwords { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; text-align: start; }
	.lwords .lname { font: 700 20px/1.2 var(--font-ui); }
	.lwords .lstate { color: var(--ok-text); font: 400 15px/1.3 var(--font-ui); }
	.lwords .desc { color: var(--text-muted); font: 400 14px/1.3 var(--font-ui); }
	.lpills { position: relative; z-index: 1; display: flex; gap: 6px; }
	.lpills .pb { min-height: 40px; height: 40px; padding: 0 16px; border-radius: 999px; border: 1px solid var(--g-line-hover, var(--line-strong)); background: none; color: var(--ink, var(--text)); font: 600 14px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.lpills .pb:disabled { opacity: 0.5; cursor: not-allowed; }
	@media (hover: hover) { .lpills .pb:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.12); } }
	@media (max-width: 640px) { .lrow { flex-wrap: wrap; gap: 8px 12px; padding: 10px 14px; min-height: 60px; } .llogo, .llogo svg { width: 32px; height: 32px; } .lwords .lname { font-size: 16px; } .lpills { width: 100%; } }
</style>
