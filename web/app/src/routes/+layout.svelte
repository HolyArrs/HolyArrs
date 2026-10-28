<script lang="ts">
	import { personName } from '#lib/plain.ts';
	import { resolve } from '$app/paths';
	import { setBase } from '#lib/paths.ts';
	setBase(resolve('/'));
	import { to, appPath } from '#lib/paths.ts';
	import '../app.css';
	import { onMount, tick, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate, beforeNavigate, goto, onNavigate, preloadCode } from '$app/navigation';
	import { hideReconnecting, loaderReady, navEnded, navStarted, showReconnecting } from '#lib/loader.ts';
	import Icon from '#lib/Icon.svelte';
	import ToastList from '#lib/ToastList.svelte';
	import Theater from '#lib/ui/Theater.svelte';
	import { theater } from '#lib/theater.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { live } from '#lib/live.svelte.ts';
	import { theme, setTheme, type ThemeChoice } from '#lib/theme.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { inbox, refreshInbox } from '#lib/wishlist.svelte.ts';
	import { decisions, refreshDecisions } from '#lib/decisions.svelte.ts';
	import { activeDownloads, refreshActiveDownloads, resetActiveDownloads } from '#lib/activeDownloads.svelte.ts';
	import { partsNow, refreshParts } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { navigateWithTransition } from '#lib/motion.ts';
	import { installPersonPrefetch } from '#lib/person-prefetch.ts';
	import { forgetMusicPrefetch, installMusicPrefetch, prefetchMusicOnScreen } from '#lib/music-prefetch.ts';
	import { installArtistVideoPrefetch } from '#lib/music/artist-video-prefetch.ts';
	import { installHeroPrefetch } from '#lib/title-details.ts';
	import { installHeroAhead } from '#lib/hero-prefetch.ts';
	import { idlePages, installRowAhead } from '#lib/smart-preload.ts';
	import { preloadHero } from '#lib/tmdb-image.ts';
	import AppShell from '#lib/ui/AppShell.svelte';
	import Sidebar from '#lib/ui/Sidebar.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import SidebarItem from '#lib/ui/SidebarItem.svelte';
	import TopBar from '#lib/ui/TopBar.svelte';
	import TabBar, { type Tab } from '#lib/ui/TabBar.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { trackInputModality } from '#lib/focus.ts';
	import { focusField, rememberReturn, searchField } from '#lib/search-field.svelte.ts';
	import { checkHero, installScrollBar, showBar } from '#lib/scroll-bar.svelte.ts';
	import { arrived, leaving } from '#lib/came-from.svelte.ts';
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
	} from '#lib/ui/nav.ts';

	let { children }: { children: Snippet } = $props();

	function allowed(need: Need): boolean {
		if (need === null) return true;
		if (need === 'wishlist') return session.can('ask') || session.can('approve');
		if (need === 'staff') return session.can('settings') || session.can('users');
		if (need === 'manage') return session.can('manage_library') || session.can('approve') || session.can('settings');
		if (need === 'settings') return session.can('settings');
		return session.can(need);
	}

	// ── Places ────────────────────────────────────────────────────────────
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
			approvals: inbox.waiting,
			decisions: decisions.count,
			active: activeDownloads,
			unread: inbox.unread,
			kid: session.kid
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

	// ── Sidebar state ─────────────────────────────────────────────────────
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
	// Artist and album data on hover, touch and the first cards on screen (lib/music-prefetch.ts).
	onMount(installMusicPrefetch);
	$effect(() => {
		if (session.s.state !== 'signed_in') forgetMusicPrefetch();
	});
	onMount(installArtistVideoPrefetch);
	onMount(() => installHeroPrefetch(preloadHero));
	// Music heroes on hover and focus, title heroes on focus, and the next card in a row.
	onMount(() => installHeroAhead(preloadHero));
	// The next row's pictures when a row comes into view (lib/smart-preload.ts).
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

	// ⌘\ / Ctrl+\ toggles the sidebar; ⌘K / Ctrl+K and / open the Search page; Escape closes the overlay.
	const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
	const searchKey = mac ? '⌘K' : 'Ctrl K';
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

	// Page changes cross-fade and rise (View Transitions, where supported).
	// The full-screen trailer covers its own hand-over to the title page (it shrinks into the hero).
	// Entering Search plays its own reveal (a mosaic from the top bar down), so no cross-fade there.
	const intoSearch = (n: { from: { url: URL } | null; to: { url: URL } | null }) => !!n.to && n.to.url.pathname.endsWith('/search') && !n.from?.url.pathname.endsWith('/search');
	// Leaving the sign-in page is covered by the Orbit hand-off (lib/loader.ts), so no cross-fade under it.
	const fromLogin = (n: { from: { url: URL } | null }) => !!n.from?.url.pathname.endsWith('/login');
	onNavigate((navigation) => (theater.landing || intoSearch(navigation) || fromLogin(navigation) ? undefined : navigateWithTransition(navigation)));
	// The loading screen from app.html leaves once the session is known;
	// slow page changes get a small one (lib/loader.ts).
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
		// The new page's hero (if any) decides the bar's glass; it may load in a moment after.
		for (const t of [0, 250, 800, 2000]) setTimeout(checkHero, t);

		navEnded();
		readLibraryResumeLinks();
		moreOpen = false;
		void closeOverlay(false);
	});

	// ── Phone ─────────────────────────────────────────────────────────────
	let moreOpen = $state(false);
	const manager = $derived(allowed('manage'));
	const tabs = $derived.by<Tab[]>(() => {
		const t: Tab[] = [];
		const add = (href: string, label: string, icon: Tab['icon']) => t.push({ href: to(href), label, icon, current: href === '/' ? path === '/' : path === href || path.startsWith(href + '/') });
		add('/', 'Home', 'home');
		add('/explore', 'Explore', 'compass');
		add('/library', 'Library', 'library');
		add('/search', 'Search', 'search');
		if (!manager && allowed('wishlist') && !offPartName(partsNow.answer, '/wishlist')) {
			t.push({ href: to('/wishlist'), label: 'Wishlist', icon: 'wish', current: path.startsWith('/wishlist'), badge: inbox.unread || undefined });
		}
		const tabbed = ['/', '/explore', '/library', '/search', '/wishlist'];
		const elsewhere = !tabbed.some((h) => (h === '/' ? path === '/' : path === h || path.startsWith(h + '/')));
		t.push({ label: manager ? 'Manage' : 'More', icon: manager ? 'inbox' : 'more', current: elsewhere, dot: inbox.waiting + decisions.count > 0 && manager, onclick: () => (moreOpen = true) });
		return t;
	});

	// ── Account ───────────────────────────────────────────────────────────
	const roleWord = $derived({ owner: 'Owner', admin: 'Admin', member: 'Member' }[session.s.role] ?? 'Member');
	// An id-like account name (preview-…, e2e-…) reads as "You".
	const shownName = $derived(personName(session.s.name, 'You'));
	const accountItems = $derived<MenuItem[]>([
		{ label: 'Account', icon: 'user', href: to('/account') },
		{ kind: 'separator' },
		{ kind: 'heading', label: 'Appearance' },
		{ label: 'Light', checked: theme.choice === 'light', onselect: () => setTheme('light') },
		{ label: 'Dark', checked: theme.choice === 'dark', onselect: () => setTheme('dark') },
		{ label: 'System', checked: theme.choice === 'system', onselect: () => setTheme('system') },
		{ kind: 'separator' },
		{ label: 'Sign out', icon: 'signout', onselect: () => void signOut() }
	]);
	const themeOptions: { value: ThemeChoice; label: string }[] = [
		{ value: 'light', label: 'Light' },
		{ value: 'dark', label: 'Dark' },
		{ value: 'system', label: 'System' }
	];
	let themeChoice = $state<ThemeChoice>(theme.choice);
	$effect(() => {
		themeChoice = theme.choice;
	});

	// ── Session, permissions and live data (unchanged rules) ──────────────
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
		if (p.startsWith('/downloads')) return allowed('manage_library');
		if (p === '/library/held' || p === '/library/turned-down' || p === '/library/rename' || p === '/library/import' || p === '/library/missing-subtitles' || p === '/library/copies') return allowed('manage_library');
		if (p.startsWith('/wishlist')) return allowed('wishlist');
		if (p === '/settings/users') return session.can('users');
		if (p === '/settings/account' || p === '/settings/appearance') return true;
		// People who manage the Library may look at Pick rules and Movies and TV defaults, read only.
		if (p === '/settings/title-defaults' || p === '/settings/pick-rules' || p.startsWith('/settings/pick-rules/')) return session.can('settings') || allowed('manage_library');
		if (p.startsWith('/settings')) return p === '/settings' ? staff : session.can('settings');
		return true;
	}

	// Send anyone who is not signed in to the sign-in page, and start or stop
	// the live stream with the session.
	$effect(() => {
		const st = session.s.state;
		if ((st === 'signed_out' || st === 'needs_setup') && !bare) {
			const back = path + page.url.search;
			void goto(to(back && back !== '/' ? `/login?next=${encodeURIComponent(back)}` : '/login'), { replace: true });
		}
		if (st === 'signed_in' && !bare && !pageAllowed(path)) {
			void goto(to(session.s.mustTwoFactor ? '/account' : homeHref()), { replace: true });
		}
		// The live stream carries Library and download news for staff.
		if (st === 'signed_in' && allowed('manage_library')) live.start();
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

	// Wishlist counts: asks waiting for people who approve, else unread messages.
	// A derived flag, so the Parts answer arriving does not ask a second time.
	const inboxOn = $derived(session.s.state === 'signed_in' && allowed('wishlist') && !offPartName(partsNow.answer, '/wishlist'));
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

	// Active downloads for the Downloads row: the live stream says when
	// something moved; a 15 s poll runs only while the tab is visible.
	$effect(() => {
		if (!decisionsOn || offPartName(partsNow.answer, '/downloads')) {
			resetActiveDownloads();
			return;
		}
		let timer: ReturnType<typeof setTimeout> | undefined;
		const soon = () => {
			if (timer) return;
			timer = setTimeout(() => {
				timer = undefined;
				void refreshActiveDownloads();
			}, 800);
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

	async function signOut() {
		try {
			await session.logout();
			toasts.push('Signed out.');
		} catch {
			toasts.push('Could not confirm sign-out with the server.', 'fail');
		}
	}

	const detail = $derived(path.split('/').filter(Boolean).length >= 2);
</script>

<svelte:window onkeydown={keys} />

{#snippet account()}
	<Menu label="{shownName}, {roleWord}: account and appearance" items={accountItems} variant="row" align="start" placement="above" width={240}>
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
{/snippet}

{#if session.s.safeMode}
	<div class="safe-banner" role="status" data-testid="safe-mode">
		<b>Safe mode.</b> Only the core runs: sign-in, the Library, settings, backups and health. Searches, downloads, imports, subtitles
		and schedules are off until HolyArrs starts again without safe mode.
	</div>
{/if}
{#if bare}
	{@render children()}
{:else if !ready}
	<div class="boot" aria-busy="true" aria-label="Loading HolyArrs"><img src={to(mark)} alt="" width="40" height="40" /></div>
{:else if firstRunWelcome}
	<main id="main">{@render children()}</main>
{:else}
	<a class="skip" href="#main">Skip to content</a>
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
			<TopBar
				showToggle={collapsed}
				controls="sidebar"
				bind:toggle={topToggle}
				ontoggle={toggleSidebar}
				history={detail}
				{searchKey}
			/>
		{/snippet}
		{#snippet tabbar()}<TabBar items={tabs} />{/snippet}
		<main id="main" tabindex="-1">
			{#if offHere}
				<section class="part-off" role="status" data-testid="part-off"><h1>This part is off</h1><p>{offHere} {offHere.includes(' and ') ? 'are' : 'is'} turned off, so this page is hidden. Nothing is deleted: everything comes back when it is turned on again.</p><p class="muted">The owner or an admin can turn it on again.</p></section>
			{:else}{@render children()}{/if}
		</main>
	</AppShell>


	<Sheet bind:open={moreOpen} title={manager ? 'Manage' : 'More'} size="sm">
		<!-- Only while open, so a closed sheet adds no second copy of every place to the page. -->
		{#if moreOpen}
		<nav class="more" aria-label="All places">
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
				<h3>You</h3>
				<ul><li><SidebarItem href={to('/account')} label="Account · {session.s.name}" icon="user" current={path === '/account'} onclick={() => (moreOpen = false)} /></li></ul>
				<div class="more-theme"><SegmentedControl label="Appearance" options={themeOptions} bind:value={themeChoice} onchange={(v) => setTheme(v)} /></div>
				<Button variant="destructive" icon="signout" full onclick={() => { moreOpen = false; void signOut(); }}>Sign out</Button>
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
	.part-off { max-width: 40rem; margin: var(--s-8) auto; padding: var(--s-6); border: 1px solid var(--line); border-radius: var(--r-card); }
	.part-off p { margin-top: var(--s-3); }
	.who { display: flex; align-items: center; gap: 10px; width: 100%; min-width: 0; color: var(--text-muted); }
	.who.only { justify-content: center; }
	.who-text { flex: 1; min-width: 0; display: flex; flex-direction: column; text-align: left; }
	.who-name { color: var(--text); font: var(--t-subhead); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.who-role { font: var(--t-footnote); color: var(--text-muted); }
	:global(.ui-sidebar .foot .ui-pop) { display: flex; width: 100%; }
	.more { display: flex; flex-direction: column; gap: 16px; }
	.more h3 { padding: 0 12px 4px; color: var(--text-muted); font: 600 12px/1.3 var(--font-ui); }
	.more ul { display: flex; flex-direction: column; gap: 2px; }
	.more :global(.ui-sbi.current) { background: var(--surface-3); }
	.more-theme { padding: 10px 4px 14px; }
	.more-theme :global(.ui-seg) { width: 100%; }
	@media (max-width: 767px) {
		main { padding: var(--s-4) var(--gutter) calc(var(--tabbar) + var(--s-8) + env(safe-area-inset-bottom)); }
	}
</style>
