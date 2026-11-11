<script lang="ts">
	import { t, startLang } from '#lib/core/i18n/i18n.svelte.ts';
	import { resolve } from '$app/paths';
	import { personName } from '#lib/core/plain.ts';
	import { setBase } from '#lib/core/paths.ts';
	setBase(resolve('/(home)'));
	import { to, appPath } from '#lib/core/paths.ts';
	import '../app.css';
	import { onMount, tick, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate, beforeNavigate, goto, onNavigate, preloadCode } from '$app/navigation';
	import { hideReconnecting, loaderReady, navEnded, navStarted, showReconnecting } from '#lib/core/loader.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Words from '#lib/ui/Words.svelte';
	import ToastList from './(frame)/ToastList.svelte';
	import Theater from './(frame)/Theater.svelte';
	import { theater } from '#lib/core/trailer/theater.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { keepPersonPic, theme, setTheme, type ThemeChoice, backdrop, loadProfile, forgetProfile } from '#lib/accounts/index.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { dlPush, decisions, refreshDecisions, activeDownloads, refreshActiveDownloads, resetActiveDownloads, dlTop } from '#lib/downloads/index.ts';
	import { alertsPanel, toasts } from '#lib/core/toasts.svelte.ts';
	import { decisionPins, feedPins, startBellFeeds } from './(frame)/bell-feeds.svelte.ts';
	import { inbox, refreshInbox } from '#lib/core/inbox.svelte.ts';
	import { partsNow, refreshParts, offPartName } from '#lib/mediafiles/index.ts';
	import { settingsNow, refreshSettingsStatus, needsAttention } from '#lib/alerts/index.ts';
	import { navigateWithTransition } from '#lib/core/motion.ts';
	import { installPersonPrefetch, installHeroAhead, checkHero, installScrollBar, showBar, arrived, leaving } from '#lib/discover/index.ts';
	import { forgetMusicPrefetch, installMusicPrefetch, prefetchMusicOnScreen, installArtistVideoPrefetch } from '#lib/music/index.ts';
	import { installHeroPrefetch } from '#lib/metadata/index.ts';
	import { idlePages, installRowAhead } from '#lib/core/smart-preload.ts';
	import { preloadHero } from '#lib/core/tmdb-image.ts';
	import { minimalLook } from '#lib/core/look.svelte.ts';
	import AppShell from './(frame)/AppShell.svelte';
	import PosterWall from './(frame)/PosterWall.svelte';
	import Sidebar from './(frame)/Sidebar.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import SidebarItem from './(frame)/SidebarItem.svelte';
	import TopBar from './(frame)/TopBar.svelte';
	import TabBar, { type Tab } from './(frame)/TabBar.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { trackInputModality } from './(frame)/focus.ts';
	import { focusField, rememberReturn, searchField } from '#lib/core/search-field.svelte.ts';
	import {
		buildNav,
		currentHref,
		homeHref,
		readSidebarPref,
		shellMode,
		writeSidebarPref,
		type Need,
		type ShellMode,
		type SidebarPref
	} from './(frame)/nav.ts';

	let { children }: { children: Snippet } = $props();

	// The saved or browser language, before the first page draws (English is already in the bundle).
	void startLang();

	function allowed(need: Need): boolean {
		if (need === null) return true;
		if (need === 'wishlist') return session.can('ask') || session.can('approve');
		if (need === 'staff') return session.can('settings') || session.can('users');
		if (need === 'manage') return session.can('manage_library') || session.can('approve') || session.can('settings');
		if (need === 'settings') return session.can('settings');
		return session.can(need);
	}

	// Settings attention (people who change settings only): a flag on the Settings row and the phone Manage tab.
	const settingsOn = $derived(session.s.state === 'signed_in' && allowed('staff') && allowed('settings'));
	const settingsAttention = $derived(settingsOn && needsAttention(settingsNow.answer));
	$effect(() => {
		if (!settingsOn) return;
		void refreshSettingsStatus();
		const off = live.on('source', () => void refreshSettingsStatus());
		const t = setInterval(() => void refreshSettingsStatus(), 120_000);
		return () => {
			off();
			clearInterval(t);
		};
	});

	const kinds = $derived(
		(
			[
				['movie', 'movies'],
				['show', 'shows'],
				['artist', 'music']
			] as const
		)
			.filter(([, part]) => partsNow.answer?.parts.find((p) => p.part === part)?.on !== false)
			.map(([k]) => k)
	);
	const sections = $derived(
		buildNav({
			allowed: (n) => allowed(n),
			off: (href) => !!offPartName(partsNow.answer, href),
			kinds,
			waitingAsks: inbox.waiting,
			decisions: decisions.count,
			active: activeDownloads,
			unread: inbox.unread,
			kid: session.kid,
			settingsAttention
		})
	);
	const allItems = $derived(sections.flatMap((s) => s.items));
	const path = $derived(appPath(page.url.pathname));
	const here = $derived(currentHref(allItems, path, (page.shallow?.url ?? page.url).searchParams));
	const brandHref = $derived(to(homeHref()));

	// The Library remembers the last view per kind for this tab.
	let libraryResumeLinks = $state<Record<string, string>>({});
	function readLibraryResumeLinks() {
		try {
			const views = JSON.parse(sessionStorage.getItem('holyarrs.preview.library-views') ?? '{}');
			const links: Record<string, string> = {};
			for (const kind of ['movie', 'show', 'artist']) {
				const href = views[kind]?.href;
				if (typeof href !== 'string') continue;
				const url = new URL(href, window.location.origin);
				if (url.origin === window.location.origin && url.pathname === to('/library') && url.searchParams.get('kind') === kind) links[kind] = url.pathname + url.search;
			}
			libraryResumeLinks = links;
		} catch {
			libraryResumeLinks = {};
		}
	}
	function link(href: string): string {
		const kind = href.startsWith('/library?kind=') ? href.slice('/library?kind='.length) : null;
		return (kind && libraryResumeLinks[kind]) || to(href);
	}

	function storage(): Storage | undefined {
		try {
			return localStorage;
		} catch {
			return undefined;
		}
	}
	let pref = $state<SidebarPref>(readSidebarPref(storage()));
	let mode = $state<ShellMode>(typeof window === 'undefined' ? 'desktop' : shellMode(window.innerWidth));
	let overlayOpen = $state(false);
	let sidebarToggle = $state<HTMLElement>();
	let topToggle = $state<HTMLElement>();
	const collapsed = $derived(mode === 'desktop' ? pref === 'collapsed' : mode === 'tablet' ? !overlayOpen : false);
	const overlay = $derived(mode === 'tablet' && overlayOpen);

	async function closeOverlay(returnFocus = true) {
		if (!overlayOpen) return;
		overlayOpen = false;
		if (returnFocus) {
			await tick();
			(topToggle ?? sidebarToggle)?.focus();
		}
	}
	async function toggleSidebar() {
		if (mode === 'tablet') {
			if (overlayOpen) return closeOverlay();
			overlayOpen = true;
			await tick();
			sidebarToggle?.focus();
			return;
		}
		if (mode !== 'desktop') return;
		const wasCollapsed = pref === 'collapsed';
		pref = wasCollapsed ? 'expanded' : 'collapsed';
		writeSidebarPref(storage(), pref);
		await tick();
		(wasCollapsed ? sidebarToggle : (topToggle ?? sidebarToggle))?.focus();
	}

	onMount(trackInputModality);
	onMount(installScrollBar);
	onMount(installPersonPrefetch);
	// Artist and album data on hover, touch and the first cards on screen.
	onMount(installMusicPrefetch);
	$effect(() => {
		if (session.s.state !== 'signed_in') forgetMusicPrefetch();
	});
	// Keep the sign-in page's copy of my picture current (only if this device remembers me).
	$effect(() => {
		if (session.s.state === 'signed_in') void keepPersonPic(session.s.name, session.s.avatar);
	});
	// After sign-in the account's own language (if one is saved) wins over this browser's.
	$effect(() => {
		if (session.s.state === 'signed_in' && session.s.id != null) void loadProfile();
		else if (session.s.state === 'signed_out') forgetProfile();
	});
	onMount(installArtistVideoPrefetch);
	onMount(() => installHeroPrefetch(preloadHero));
	// Music heroes on hover and focus, title heroes on focus, and the next card in a row.
	onMount(() => installHeroAhead(preloadHero));
	// The next row's pictures when a row comes into view.
	onMount(() => installRowAhead());
	// Once a page settles: the code of the side menu's places, on idle (fast connections only).
	let stopIdle: (() => void) | undefined;
	const menuPaths = () => [...new Set([...document.querySelectorAll<HTMLAnchorElement>('nav a[href^="/"]')].map((a) => a.getAttribute('href') ?? '').filter((h) => h && !h.includes('?')))];
	onMount(() => {
		void session.load();
		readLibraryResumeLinks();
		const mq = [matchMedia('(min-width: 768px)'), matchMedia('(min-width: 1024px)')];
		const sync = () => {
			mode = shellMode(window.innerWidth);
			if (mode !== 'tablet') overlayOpen = false;
		};
		mq.forEach((m) => m.addEventListener('change', sync));
		return () => mq.forEach((m) => m.removeEventListener('change', sync));
	});

	// ⌘\ or Ctrl+\ toggles the sidebar; ⌘K, Ctrl+K and / open the Search page; Escape closes the overlay.
	const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
	const searchKey = mac ? '⌘K' : 'Ctrl K';
	// Downloads has no top bar: its own toolbar carries the sidebar button and a plain search.
	$effect(() => {
		dlTop.collapsed = collapsed;
		dlTop.searchKey = searchKey;
		dlTop.toggle = () => void toggleSidebar();
	});
	// History has its own top row too (search, Live, Filter), so the shared bar steps aside there as well.
	const topHidden = $derived(path === '/downloads' || path === '/library' || path === '/downloads/history');
	const typingIn = (t: EventTarget | null) => t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
	function keys(e: KeyboardEvent) {
		if (!ready || bare) return;
		const mod = mac ? e.metaKey : e.ctrlKey;
		if (mod && e.key === '\\') {
			e.preventDefault();
			void toggleSidebar();
		} else if ((mod && (e.key === 'k' || e.key === 'K')) || (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey && !typingIn(e.target))) {
			// Ctrl+K and / focus the one search field (the top bar; on phones the Search page's) and open Search.
			e.preventDefault();
			showBar();
			if (!path.startsWith('/search')) rememberReturn();
			const shown = focusField(true);
			if (!path.startsWith('/search')) void goto(to(shown && searchField.term.trim() ? `/search?q=${encodeURIComponent(searchField.term.trim())}` : '/search'), { reset: !shown });
		} else if (e.key === 'Escape' && overlayOpen) {
			void closeOverlay();
		}
	}

	// Page changes cross-fade and rise (View Transitions, where supported). The full-screen trailer covers its own hand-over to the title page (it shrinks into the hero). Entering Search plays its own reveal (a mosaic from the top bar down), so no cross-fade there.
	const intoSearch = (n: { from: { url: URL } | null; to: { url: URL } | null }) => !!n.to && n.to.url.pathname.endsWith('/search') && !n.from?.url.pathname.endsWith('/search');
	// Leaving the sign-in page is covered by the Orbit hand-off, so no cross-fade under it.
	const fromLogin = (n: { from: { url: URL } | null }) => !!n.from?.url.pathname.endsWith('/login');
	onNavigate((navigation) => (theater.landing || intoSearch(navigation) || fromLogin(navigation) ? undefined : navigateWithTransition(navigation)));
	// The loading screen from app.html leaves once the session is known; slow page changes get a small one.
	$effect(() => {
		if (session.s.state !== 'unknown') void tick().then(loaderReady);
	});
	// The server went away (restart, update): Orbit with Reconnecting… until it answers.
	$effect(() => {
		if (live.status.state === 'reconnecting') showReconnecting();
		else if (live.status.state === 'live') hideReconnecting();
	});
	beforeNavigate((nav) => {
		if (!nav.willUnload) navStarted();
		leaving();
	});
	afterNavigate((nav) => {
		arrived(nav.type);
		prefetchMusicOnScreen();
		stopIdle?.();
		stopIdle = idlePages(menuPaths(), (p) => preloadCode(p as Parameters<typeof preloadCode>[0]));
		// The new page's hero (if any) decides the bar's glass; it may load a moment later.
		for (const t of [0, 250, 800, 2000]) setTimeout(checkHero, t);

		navEnded();
		readLibraryResumeLinks();
		moreOpen = false;
		void closeOverlay(false);
	});

	let moreOpen = $state(false);
	const manager = $derived(allowed('manage'));
	const tabs = $derived.by<Tab[]>(() => {
		const list: Tab[] = [];
		const add = (href: string, label: string, icon: Tab['icon']) => list.push({ href: to(href), label, icon, current: href === '/' ? path === '/' : path === href || path.startsWith(href + '/') });
		add('/', t('frame.nav.home'), 'home');
		add('/explore', t('frame.nav.explore'), 'compass');
		add('/library', t('frame.nav.library'), 'library');
		add('/search', t('frame.layout.search'), 'search');
		if (!manager && allowed('wishlist') && !offPartName(partsNow.answer, '/requests')) {
			list.push({ href: to('/requests'), label: t('frame.nav.requests'), icon: 'requests', current: path.startsWith('/requests'), badge: inbox.unread || undefined });
		}
		const tabbed = ['/', '/explore', '/library', '/search', '/requests'];
		const elsewhere = !tabbed.some((h) => (h === '/' ? path === '/' : path === h || path.startsWith(h + '/')));
		list.push({ label: manager ? t('frame.nav.manage') : t('frame.layout.more'), icon: manager ? 'inbox' : 'more', current: elsewhere, dot: ((inbox.waiting + decisions.count > 0 || settingsAttention) && manager) || toasts.unseen() > 0, onclick: () => (moreOpen = true) });
		return list;
	});

	const roleWord = $derived(({ owner: t('frame.layout.owner'), admin: t('frame.layout.admin'), member: t('frame.layout.member') } as Record<string, string>)[session.s.role] ?? t('frame.layout.member'));
	// An id-like account name (preview-..., e2e-...) reads as "You".
	const shownName = $derived(personName(session.s.name, t('frame.layout.you')));
	// The bell counts new alerts plus Settings warnings still open (those stay until fixed).
	const openFixes = $derived((settingsAttention ? (settingsNow.answer?.items.length ?? 0) : 0) + (allowed('manage_library') ? decisionPins().length : 0) + (feedPins.update ? 1 : 0));
	// Request updates, failed downloads and a new version reach the bell from any page.
	$effect(() => {
		if (session.s.state !== 'signed_in') return;
		return startBellFeeds(allowed('manage'));
	});
	const unseenAlerts = $derived(toasts.unseen() + openFixes);
	const hasFailAlert = $derived(openFixes > 0 || toasts.past.some((p) => p.fresh && p.tone === 'fail'));
	const accountItems = $derived<MenuItem[]>([
		{ label: t('frame.layout.account'), icon: 'user', href: to('/account') },
		{ kind: 'separator' },
		{ kind: 'heading', label: t('frame.layout.appearance') },
		{ label: t('frame.layout.light'), checked: theme.choice === 'light', onselect: () => setTheme('light') },
		{ label: t('frame.layout.dark'), checked: theme.choice === 'dark', onselect: () => setTheme('dark') },
		{ label: t('frame.layout.system'), checked: theme.choice === 'system', onselect: () => setTheme('system') },
		{ kind: 'separator' },
		{ label: t('frame.layout.switch'), icon: 'user', onselect: () => void switchAccount() },
		{ label: t('frame.layout.signOut'), icon: 'signout', onselect: () => void signOut() }
	]);
	const themeOptions = $derived<{ value: ThemeChoice; label: string }[]>([
		{ value: 'light', label: t('frame.layout.light') },
		{ value: 'dark', label: t('frame.layout.dark') },
		{ value: 'system', label: t('frame.layout.system') }
	]);
	let themeChoice = $state<ThemeChoice>(theme.choice);
	$effect(() => {
		themeChoice = theme.choice;
	});

	const offHere = $derived(offPartName(partsNow.answer, path));
	const staff = $derived(allowed('staff'));
	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');
	const bare = $derived(path === '/login' || path.startsWith('/invite/'));
	const firstRunWelcome = $derived(path === '/welcome' && page.url.searchParams.get('first') === '1');
	const ready = $derived(session.s.state === 'signed_in');

	// Pages a person may not use send them somewhere they may.
	function pageAllowed(p: string): boolean {
		if (p === '/account') return true;
		if (session.s.mustTwoFactor) return false;
		// Everyone signed in watches their own downloads; the rest of Downloads is for managers.
		if (p === '/downloads') return true;
		if (p.startsWith('/downloads')) return allowed('manage_library');
		if (p === '/library/held' || p === '/library/turned-down' || p === '/library/rename' || p === '/library/import' || p === '/library/import-files' || p === '/library/missing-subtitles' || p === '/library/copies') return allowed('manage_library');
		if (p.startsWith('/requests')) return allowed('wishlist');
		// Status sits under Manage, as in the side menu.
		if (p.startsWith('/health')) return allowed('manage_library');
		// A Settings page the person may not open shows a short note there instead of a bounce.
		if (p === '/settings') return staff;
		return true;
	}

	// Send anyone who is not signed in to the sign-in page, and start or stop the live stream with the session.
	$effect(() => {
		const st = session.s.state;
		if ((st === 'signed_out' || st === 'needs_setup') && !bare) {
			const back = path + page.url.search;
			void goto(to(back && back !== '/' ? `/login?next=${encodeURIComponent(back)}` : '/login'), { replace: true });
		}
		// The owner finishes the first-run wizard before the rest of the app opens.
		if (st === 'signed_in' && session.s.setupOpen && !firstRunWelcome && path !== '/login') {
			void goto(to('/welcome?first=1'), { replace: true });
			return;
		}
		if (st === 'signed_in' && !bare && !pageAllowed(path)) {
			void goto(to(session.s.mustTwoFactor ? '/account' : homeHref()), { replace: true });
		}
		// The live stream carries Library and download news for staff. On Downloads with push updates, everyone else gets their own rows' patches, and on Requests their own asks and downloads as they move.
		if (st === 'signed_in' && (allowed('manage_library') || (path === '/downloads' && dlPush.on) || path === '/requests')) live.start();
		else live.stop();
	});

	// Which parts are on: asked on page changes and every minute.
	$effect(() => {
		void path;
		if (session.s.state !== 'signed_in') return;
		void refreshParts();
		const t = setInterval(() => void refreshParts(), 60_000);
		return () => clearInterval(t);
	});

	// Requests counts: asks waiting for people who approve, else unread messages. A derived flag, so the Parts answer arriving does not ask a second time.
	const inboxOn = $derived(session.s.state === 'signed_in' && allowed('wishlist') && !offPartName(partsNow.answer, '/requests'));
	$effect(() => {
		void path;
		if (!inboxOn) return;
		void refreshInbox();
		const t = setInterval(() => void refreshInbox(), 60_000);
		return () => clearInterval(t);
	});

	// Movies that need a decision (staff only), shown on Downloads.
	const decisionsOn = $derived(session.s.state === 'signed_in' && allowed('manage_library'));
	$effect(() => {
		void path;
		if (!decisionsOn) return;
		void refreshDecisions();
		const off = live.on('slot', () => void refreshDecisions());
		const t = setInterval(() => void refreshDecisions(), 60_000);
		return () => {
			off();
			clearInterval(t);
		};
	});

	// Active downloads for the Downloads row: the live stream says when something moved; a 15 s poll runs only while the tab is visible.
	$effect(() => {
		if (!decisionsOn || offPartName(partsNow.answer, '/downloads')) {
			resetActiveDownloads();
			return;
		}
		// Reading the list can itself send "download" events, so live refreshes keep at least 10 s apart instead of looping.
		let timer: ReturnType<typeof setTimeout> | undefined;
		let last = 0;
		const soon = () => {
			if (timer) return;
			timer = setTimeout(() => {
				timer = undefined;
				last = Date.now();
				void refreshActiveDownloads();
			}, Math.max(800, last + 10_000 - Date.now()));
		};
		void refreshActiveDownloads();
		const offs = ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		const poll = setInterval(() => {
			if (document.visibilityState === 'visible') void refreshActiveDownloads();
		}, 15_000);
		const visible = () => {
			if (document.visibilityState === 'visible') void refreshActiveDownloads();
		};
		document.addEventListener('visibilitychange', visible);
		return () => {
			offs.forEach((off) => off());
			clearInterval(poll);
			clearTimeout(timer);
			document.removeEventListener('visibilitychange', visible);
		};
	});

	// A shared screen: sign out and open sign-in, where people who signed in here before are one tap away. Each switch is a full sign-in.
	async function switchAccount() {
		try {
			await session.logout();
		} catch {
			toasts.push(t('frame.layout.signOutFail'), 'fail');
		}
		void goto(to('/login'));
	}

	async function signOut() {
		try {
			await session.logout();
			toasts.push(t('frame.layout.signedOut'), 'ok');
		} catch {
			toasts.push(t('frame.layout.signOutFail'), 'fail');
		}
	}

	const detail = $derived(path.split('/').filter(Boolean).length >= 2);
</script>

<svelte:window onkeydown={keys} />

{#snippet account()}
	<Menu label={t('frame.layout.accountMenu', { name: shownName, role: roleWord })} items={accountItems} variant="row" align="start" placement="above" width={240}>
		{#snippet trigger()}
			<span class="who" class:only={collapsed}>
				<Avatar name={shownName} src={session.s.avatar} size={36} />
				{#if !collapsed}
					<span class="who-text"><span class="who-name">{shownName}</span><span class="who-role">{roleWord}</span></span>
					<Icon name="chevronUpDown" size={15} />
				{/if}
			</span>
		{/snippet}
	</Menu>
	<!-- Recent alerts: a bell beside your name, with how many arrived since you last looked. -->
	<button type="button" class="alerts-bell" class:only={collapsed} data-alerts-open aria-label={unseenAlerts ? t('frame.layout.alertsNew', { n: unseenAlerts }) : t('frame.layout.alerts')} aria-expanded={alertsPanel.open} onclick={() => (alertsPanel.open = !alertsPanel.open)}>
		<Icon name="bell" size={18} />
		{#if unseenAlerts}<span class="alerts-count" class:bad={hasFailAlert}>{unseenAlerts > 9 ? '9+' : unseenAlerts}</span>{/if}
	</button>
{/snippet}

{#if session.s.safeMode}
	<div class="safe-banner" role="status" data-testid="safe-mode">
		<b>{t('frame.layout.safeTitle')}</b> {t('frame.layout.safeBody')}
	</div>
{/if}
{#if session.s.hostRefused}
	<div class="safe-banner" role="status" data-testid="host-refused">
		<Words key="frame.layout.hostRefused" vars={{ host: session.s.hostRefused }} />
	</div>
{/if}
{#if bare}
	{@render children()}
{:else if !ready}
	<div class="boot" aria-busy="true" aria-label={t('frame.layout.loading')}><img src={to(mark)} alt="" width="40" height="40" /></div>
{:else if firstRunWelcome}
	<main id="main">{@render children()}</main>
{:else}
	<a class="skip" href="#main">{t('frame.layout.skip')}</a>
	<!-- Background behind every page: plain by default, or this Library's posters (Settings > Appearance). Settings home draws its own. -->
	{#if backdrop.value === 'posters' && path !== '/settings' && !minimalLook()}<PosterWall />{/if}
	<AppShell {collapsed} {overlay} onscrim={() => closeOverlay()}>
		{#snippet sidebar()}
			<Sidebar
				id="sidebar"
				{sections}
				{collapsed}
				{overlay}
				currentHref={here}
				{link}
				{brandHref}
				markSrc={to(mark)}
				bind:toggle={sidebarToggle}
				ontoggle={toggleSidebar}
				onpick={() => void closeOverlay(false)}
				footer={account}
			/>
		{/snippet}
		{#snippet topbar()}
			{#if !topHidden}<TopBar
				showToggle={collapsed}
				controls="sidebar"
				bind:toggle={topToggle}
				ontoggle={toggleSidebar}
				history={detail}
				{searchKey}
			/>{/if}
		{/snippet}
		{#snippet tabbar()}<TabBar items={tabs} />{/snippet}
		<main id="main" tabindex="-1" class:bleed={path === '/downloads' || (path.startsWith('/downloads/') && path !== '/downloads/history')}>
			{#if offHere}
				<section class="part-off" role="status" data-testid="part-off"><h1>{t('frame.layout.partOff')}</h1><p>{t(offHere.includes(' and ') ? 'frame.layout.partsAreOff' : 'frame.layout.partIsOff', { parts: offHere })}</p><p class="muted">{t('frame.layout.partOffHint')}</p></section>
			{:else}{@render children()}{/if}
		</main>
	</AppShell>

	<Sheet bind:open={moreOpen} title={manager ? t('frame.nav.manage') : t('frame.layout.more')} size="sm">
		<!-- Only while open, so a closed sheet adds no second copy of every place to the page. -->
		{#if moreOpen}
		<nav class="more" aria-label={t('frame.layout.allPlaces')}>
			{#each sections as s (s.id)}
				<div class="more-sec">
					{#if s.label}<h3>{s.label}</h3>{/if}
					<ul>
						{#each s.items as item (item.href + item.label)}
							<li><SidebarItem href={link(item.href)} label={item.label} icon={item.icon} current={here === item.href} badge={item.badge} badgeLabel={item.badgeLabel} onclick={() => (moreOpen = false)} /></li>
						{/each}
					</ul>
				</div>
			{/each}
			<div class="more-sec">
				<ul><li><SidebarItem href="#alerts" label={t('frame.layout.alerts')} icon="bell" current={false} badge={unseenAlerts} badgeLabel={t('frame.layout.newWord')} onclick={(e) => { e.preventDefault(); moreOpen = false; alertsPanel.open = true; }} /></li></ul>
			</div>
			<div class="more-sec">
				<h3>{t('frame.layout.you')}</h3>
				<ul><li><SidebarItem href={to('/account')} label={t('frame.layout.accountName', { name: session.s.name })} icon="user" current={path === '/account'} onclick={() => (moreOpen = false)} /></li></ul>
				<div class="more-theme"><SegmentedControl label={t('frame.layout.appearance')} options={themeOptions} bind:value={themeChoice} onchange={(v) => setTheme(v)} /></div>
				<Button icon="user" full onclick={() => { moreOpen = false; void switchAccount(); }}>{t('frame.layout.switch')}</Button>
				<Button variant="destructive" icon="signout" full onclick={() => { moreOpen = false; void signOut(); }}>{t('frame.layout.signOut')}</Button>
			</div>
		</nav>
		{/if}
	</Sheet>
{/if}
<ToastList />
<Theater />

<style>
	.boot { min-height: 100dvh; display: grid; place-items: center; }
	.safe-banner { position: relative; z-index: 50; padding: var(--s-3) var(--s-4); background: var(--surface-2); color: var(--wait-text); border-bottom: 1px solid var(--line); font-size: var(--text-sm); overflow-wrap: anywhere; }
	.skip { position: fixed; left: 12px; top: -80px; z-index: 100; background: var(--accent-fill); color: var(--text-on-red); padding: 12px 16px; border-radius: var(--r-btn); }
	.skip:focus-visible { top: 12px; }
	main { min-width: 0; padding: var(--s-6) var(--gutter) var(--s-16); outline: none; view-transition-name: page; }
	/* Focus lands on main after a page change (for screen readers); it never draws a ring around the page. */
	main:focus, main:focus-visible { outline: none; box-shadow: none; }
	/* The Downloads app fills everything under the top bar, edge to edge. */
	main.bleed { padding: 0; }
	.part-off { max-width: 40rem; margin: var(--s-8) auto; padding: var(--s-6); border: 1px solid var(--line); border-radius: var(--r-card); }
	.part-off p { margin-top: var(--s-3); }
	.who { display: flex; align-items: center; gap: 10px; width: 100%; min-width: 0; color: var(--text-muted); }
	.who.only { justify-content: center; }
	.who-text { flex: 1; min-width: 0; display: flex; flex-direction: column; text-align: left; }
	.who-name { color: var(--text); font: var(--t-subhead); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.who-role { font: var(--t-footnote); color: var(--text-muted); }
	:global(.ui-sidebar .foot) { display: flex; align-items: center; gap: 4px; }
	:global(.ui-sidebar .foot .ui-pop) { display: flex; flex: 1; min-width: 0; }
	/* Collapsed menu: the bell sits under the picture. */
	:global(.ui-sidebar .foot:has(.alerts-bell.only)) { flex-direction: column; }
	.alerts-bell { position: relative; flex: none; width: 34px; height: 34px; display: grid; place-items: center; border-radius: 50%; color: var(--text-muted); }
	.alerts-bell:hover, .alerts-bell[aria-expanded='true'] { color: var(--text); background: var(--surface-2); }
	.alerts-bell:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.alerts-count { position: absolute; top: 2px; right: 0; min-width: 16px; height: 16px; padding: 0 4px; border-radius: 8px; display: grid; place-items: center; font: 700 10px/1 var(--font-ui); font-variant-numeric: tabular-nums; color: #fff; background: var(--text-muted); box-shadow: 0 0 0 2px var(--sidebar-solid, var(--bg)); }
	.alerts-count.bad { background: var(--fail, #d70015); }
	.more { display: flex; flex-direction: column; gap: 16px; }
	.more h3 { padding: 0 12px 4px; color: var(--text-muted); font: 600 12px/1.3 var(--font-ui); }
	.more ul { display: flex; flex-direction: column; gap: 2px; }
	.more :global(.ui-sbi.current) { background: var(--surface-3); }
	.more-theme { padding: 10px 4px 14px; }
	.more-theme :global(.ui-seg) { width: 100%; }
	@media (max-width: 767px) {
		main { padding: var(--s-4) var(--gutter) calc(var(--tabbar) + var(--s-8) + env(safe-area-inset-bottom)); }
		main.bleed { padding: 0; }
	}
</style>
