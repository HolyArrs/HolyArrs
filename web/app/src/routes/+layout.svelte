<script lang="ts">
	import { resolve } from '$app/paths';
	import { setBase } from '#lib/paths.ts';
	setBase(resolve('/'));
	import { to, appPath } from '#lib/paths.ts';
	import '../app.css';
	import { onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate, goto } from '$app/navigation';
	import Icon, { type IconName } from '#lib/Icon.svelte';
	import ToastList from '#lib/ToastList.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { live } from '#lib/live.svelte.ts';
	import { theme, toggleTheme } from '#lib/theme.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { inbox, refreshInbox } from '#lib/wishlist.svelte.ts';
	import { decisions, refreshDecisions } from '#lib/decisions.svelte.ts';
	import { partsNow, refreshParts } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';

	let { children }: { children: Snippet } = $props();

	type Need = 'manage_library' | 'settings' | 'wishlist' | 'staff' | null;
	const allPlaces: { href: string; label: string; icon: IconName; need: Need }[] = [
		{ href: '/', label: 'Home', icon: 'home', need: 'manage_library' },
		{ href: '/library', label: 'Library', icon: 'library', need: null },
		{ href: '/search', label: 'Search', icon: 'search', need: null },
		{ href: '/wishlist', label: 'Wishlist', icon: 'wish', need: 'wishlist' },
		{ href: '/downloads', label: 'Downloads', icon: 'download', need: 'manage_library' },
		{ href: '/calendar', label: 'Calendar', icon: 'calendar', need: 'manage_library' }
	];
	const extraDesktopPlaces: { href: string; label: string; icon: IconName; need: Need }[] = [
		{ href: '/downloads/history', label: 'History', icon: 'archive', need: 'manage_library' },
		{ href: '/downloads/decide', label: 'Needs a decision', icon: 'info', need: 'manage_library' },
		{ href: '/settings/alerts', label: 'Alerts', icon: 'bell', need: 'settings' }
	];

	function allowed(need: Need): boolean {
		if (need === null) return true;
		if (need === 'wishlist') return session.can('ask') || session.can('approve');
		if (need === 'staff') return session.can('settings') || session.can('users');
		if (need === 'settings') return session.can('settings');
		return session.can(need);
	}

	// A part that is off hides its pages from the menu.
	const places = $derived(allPlaces.filter((p) => allowed(p.need) && !offPartName(partsNow.answer, p.href)));
	const secondaryPlaces = $derived(extraDesktopPlaces.filter((p) => allowed(p.need) && !offPartName(partsNow.answer, p.href)));
	const desktopPlaces = $derived([...places, ...secondaryPlaces]);
	const mobilePlaces = $derived(places.filter((p) => ['/library', '/search', '/wishlist', '/downloads'].includes(p.href)));
	let libraryExpanded = $state(true);
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
		} catch { libraryResumeLinks = {}; }
	}
	const libraryKinds = $derived([
		{ kind: 'movie', part: 'movies', label: 'Movies' },
		{ kind: 'show', part: 'shows', label: 'TV Shows' },
		{ kind: 'artist', part: 'music', label: 'Music' }
	].filter((item) => partsNow.answer?.parts.find((part) => part.part === item.part)?.on !== false));
	let menu = $state<HTMLDialogElement>();
	let libraryChooser = $state(false);
	function openNavigation(onlyLibrary = false) {
		libraryChooser = onlyLibrary;
		if (onlyLibrary) libraryExpanded = true;
		menu?.showModal();
	}
	afterNavigate(() => { menu?.close(); readLibraryResumeLinks(); });
	// A direct link to a page of a part that is off says so instead.
	const offHere = $derived(offPartName(partsNow.answer, appPath(page.url.pathname)));
	const staff = $derived(allowed('staff'));
	// Members reach their account from the bottom menu instead of Settings.
	const settingsPlace = $derived(
		staff ? { href: '/settings', label: 'Settings', icon: 'settings' as IconName } : { href: '/account', label: 'My account', icon: 'user' as IconName }
	);
	const sectionKey = $derived.by(() => {
		const p = appPath(page.url.pathname);
		if (p.startsWith('/library')) return 'library';
		if (p.startsWith('/downloads')) return 'downloads';
		if (p.startsWith('/search') || p.startsWith('/wishlist')) return 'discover';
		if (p.startsWith('/calendar')) return 'calendar';
		if (p.startsWith('/settings') || p.startsWith('/account')) return 'settings';
		return 'home';
	});
	const sectionTitle = $derived(
		({ home: 'Home', library: 'Library', discover: 'Discover', downloads: 'Downloads', calendar: 'Calendar', settings: settingsPlace.label } as const)[sectionKey]
	);
	const contextPlaces = $derived.by(() => {
		if (sectionKey === 'downloads') return desktopPlaces.filter((p) => p.href.startsWith('/downloads'));
		if (sectionKey === 'discover') return desktopPlaces.filter((p) => p.href === '/search' || p.href === '/wishlist');
		if (sectionKey === 'calendar') return desktopPlaces.filter((p) => p.href === '/calendar');
		if (sectionKey === 'settings') return [settingsPlace];
		return desktopPlaces.filter((p) => p.href === '/');
	});

	const mark = $derived(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg');
	const bare = $derived(appPath(page.url.pathname) === '/login' || appPath(page.url.pathname).startsWith('/invite/'));
	const firstRunWelcome = $derived(appPath(page.url.pathname) === '/welcome' && page.url.searchParams.get('first') === '1');
	const ready = $derived(session.s.state === 'signed_in');

	// Pages a person may not use send them somewhere they may.
	function pageAllowed(p: string): boolean {
		if (p === '/account') return true;
		if (session.s.mustTwoFactor) return false;
		if (p === '/' || p.startsWith('/downloads') || p.startsWith('/calendar')) return allowed('manage_library');
		if (p === '/library/held' || p === '/library/turned-down' || p === '/library/rename' || p === '/library/import' || p === '/library/missing-subtitles') return allowed('manage_library');
		if (p.startsWith('/wishlist')) return allowed('wishlist');
		if (p === '/settings/users') return session.can('users');
		if (p === '/settings/account') return true;
		if (p.startsWith('/settings')) return p === '/settings' ? staff : session.can('settings');
		return true;
	}

	function current(href: string): boolean {
		const p = appPath(page.url.pathname);
		return href === '/' ? p === '/' : p === href || p.startsWith(href + '/');
	}

	function desktopCurrent(href: string): boolean {
		const peers = [...desktopPlaces, settingsPlace].filter((p) => current(p.href));
		return peers.sort((a, b) => b.href.length - a.href.length)[0]?.href === href;
	}

	onMount(() => {
		void session.load();
	});

	// Send anyone who is not signed in to the sign-in page, and start or stop
	// the live stream with the session.
	$effect(() => {
		const st = session.s.state;
		if ((st === 'signed_out' || st === 'needs_setup') && !bare) {
			const back = appPath(page.url.pathname) + page.url.search;
			void goto(to(back && back !== '/' ? `/login?next=${encodeURIComponent(back)}` : '/login'), { replace: true });
		}
		if (st === 'signed_in' && !bare && !pageAllowed(appPath(page.url.pathname))) {
			void goto(to(session.s.mustTwoFactor ? '/account' : allowed('manage_library') ? '/' : '/library'), { replace: true });
		}
		// The live stream carries Library and download news for staff.
		if (st === 'signed_in' && allowed('manage_library')) live.start();
		else live.stop();
	});

	// Which parts are on: asked on page changes and every minute.
	$effect(() => {
		void appPath(page.url.pathname);
		if (session.s.state !== 'signed_in') return;
		void refreshParts();
		const t = setInterval(() => void refreshParts(), 60_000);
		return () => clearInterval(t);
	});

	// The Wishlist badge: asks waiting for people who approve, else unread messages.
	const badge = $derived(session.can('approve') ? inbox.waiting : inbox.unread);
	$effect(() => {
		void appPath(page.url.pathname);
		if (session.s.state !== 'signed_in' || !allowed('wishlist') || offPartName(partsNow.answer, '/wishlist')) return;
		void refreshInbox();
		const t = setInterval(() => void refreshInbox(), 60_000);
		return () => clearInterval(t);
	});

	// The Downloads badge: movies that need a decision (staff only).
	$effect(() => {
		void appPath(page.url.pathname);
		if (session.s.state !== 'signed_in' || !allowed('manage_library')) return;
		void refreshDecisions();
		const off = live.on('slot', () => void refreshDecisions());
		const t = setInterval(() => void refreshDecisions(), 60_000);
		return () => {
			off();
			clearInterval(t);
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

	const liveWord = $derived(
		{ off: 'Live updates off', connecting: 'Connecting', live: 'Live', reconnecting: 'Reconnecting' }[live.status.state]
	);
</script>

{#snippet libraryMenu()}
	<div class="library-group">
		<button class="nav-item library-parent" class:library-active={current('/library')} type="button" title="Library" aria-label="Library" aria-expanded={libraryExpanded} onclick={() => libraryExpanded = !libraryExpanded}>
			<Icon name="library" size={19} /><span class="nav-label">Library</span>
		</button>
		<div class="library-reveal" class:is-open={libraryExpanded} inert={!libraryExpanded}>
			<div class="library-children">
				{#each libraryKinds as item (item.kind)}
					<a class="nav-item library-child" href={libraryResumeLinks[item.kind] ?? to(`/library?kind=${item.kind}`)} onclick={() => menu?.close()} title={item.label} aria-current={current('/library') && (page.shallow?.url ?? page.url).searchParams.get('kind') === item.kind ? 'page' : undefined}><span>{item.label}</span></a>
				{/each}
			</div>
		</div>
	</div>
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
	<div class="app" class:settings-section={sectionKey === 'settings'}>
		<aside class="global-rail" aria-label="Main">
			<a class="rail-brand" href={to('/')} aria-label="HolyArrs home"><img src={to(mark)} alt="" width="30" height="30" /></a>
			<nav class="rail-nav">
				{#each places as p (p.href)}
					{#if p.href === '/library'}
						<button class="rail-item" type="button" title="Library" aria-label="Library" aria-haspopup="dialog" aria-current={sectionKey === 'library' ? 'page' : undefined} onclick={() => openNavigation(true)}><Icon name={p.icon} size={20} /></button>
					{:else}
					<a class="rail-item" href={to(p.href)} title={p.label} aria-label={p.label} aria-current={desktopCurrent(p.href) ? 'page' : undefined}>
						<Icon name={p.icon} size={20} />
						{#if p.href === '/wishlist' && badge > 0}<b class="rail-badge" aria-label="{badge} new">{badge}</b>{/if}
					</a>
					{/if}
				{/each}
			</nav>
			<div class="rail-footer">
				{#if allowed('manage_library')}<span class="rail-live {live.status.state}" role="status" aria-label={liveWord} title={liveWord}><i aria-hidden="true"></i></span>{/if}
				<button class="rail-item rail-theme" type="button" onclick={toggleTheme} aria-label={theme.value === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'} title={theme.value === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}><Icon name={theme.value === 'dark' ? 'sun' : 'moon'} size={20} /></button>
				{#if staff}<a class="rail-item" href={to('/settings')} title="Settings" aria-label="Settings" aria-current={appPath(page.url.pathname).startsWith('/settings') ? 'page' : undefined}><Icon name="settings" size={20} /></a>{/if}
				<a class="rail-item" href={to('/account')} title="My account" aria-label="My account" aria-current={appPath(page.url.pathname) === '/account' ? 'page' : undefined}><Icon name="user" size={20} /></a>
			</div>
		</aside>
		<aside class="context-sidebar" aria-label={`${sectionTitle} navigation`}>
			<div class="context-head">
				<a class="context-brand" href={to('/')} aria-label="HolyArrs home"><span>HolyArrs</span></a>
				<h2>{sectionTitle}</h2>
			</div>
			<nav class="nav" aria-label={sectionTitle}>
				{#if sectionKey === 'library'}
					{@render libraryMenu()}
				{:else}
					{#each contextPlaces as p (p.href)}
						<a class="nav-item" href={to(p.href)} aria-current={desktopCurrent(p.href) ? 'page' : undefined}>
							<Icon name={p.icon} size={18} /><span class="nav-label">{p.label}</span>
							{#if p.href === '/wishlist' && badge > 0}<b class="badge" aria-label="{badge} new">{badge}</b>{/if}
							{#if p.href === '/downloads/decide' && decisions.count > 0}<b class="badge" data-testid="nav-decide" aria-label="{decisions.count} need a decision">{decisions.count}</b>{/if}
						</a>
					{/each}
				{/if}
			</nav>
			<div class="context-footer">
				<a class="who" href={to('/account')} title="My account"><span class="account-avatar"><Icon name="user" size={17} /></span><span class="nav-label">{session.s.name}</span></a>
				<div class="account-actions">
					<button class="icon-btn" type="button" onclick={toggleTheme} aria-label={theme.value === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}><Icon name={theme.value === 'dark' ? 'sun' : 'moon'} size={17} /></button>
					<button class="icon-btn" type="button" onclick={signOut} aria-label="Sign out"><Icon name="signout" size={17} /></button>
				</div>
			</div>
		</aside>
		<div class="shell">
			<header class="topbar">
				<a class="brand" href={to('/')} aria-label="HolyArrs home"><img src={to(mark)} alt="" width="26" height="26" /><span>HolyArrs</span></a>
				<div class="top-actions">
					{#if allowed('manage_library')}<span class="live {live.status.state}" role="status" aria-label={liveWord} title={liveWord}><i aria-hidden="true"></i><span class="sr-only">{liveWord}</span></span>{/if}
					<button class="icon-btn" type="button" onclick={() => openNavigation()} aria-label="Open navigation"><Icon name="menu" /></button>
				</div>
			</header>
			<main id="main" tabindex="-1">
				{#if offHere}
					<section class="part-off" role="status" data-testid="part-off"><h1>This part is off</h1><p>{offHere} {offHere.includes(' and ') ? 'are' : 'is'} turned off, so this page is hidden. Nothing is deleted: everything comes back when it is turned on again.</p><p class="muted">The owner or an admin can turn it on again.</p></section>
				{:else}{@render children()}{/if}
			</main>
		</div>
		<nav class="tabbar" aria-label="Main">
			{#each mobilePlaces as p (p.href)}
				{#if p.href === '/library'}
					<button class="tab" class:active={desktopCurrent('/library')} type="button" aria-haspopup="dialog" onclick={() => openNavigation(true)}><Icon name={p.icon} size={21} /><span>{p.label}</span></button>
				{:else}
				<a class="tab" href={to(p.href)} aria-current={desktopCurrent(p.href) ? 'page' : undefined}><Icon name={p.icon} size={21} /><span>{p.label}</span>{#if p.href === '/wishlist' && badge > 0}<b class="badge" aria-label="{badge} new">{badge}</b>{/if}</a>
				{/if}
			{/each}
			<button class="tab" class:active={!mobilePlaces.some((p) => desktopCurrent(p.href))} type="button" onclick={() => openNavigation()} aria-haspopup="dialog"><Icon name="menu" size={21} /><span>More</span>{#if badge + decisions.count > 0}<span class="attention-dot" aria-label="Items need attention"></span>{/if}</button>
		</nav>
		<dialog class="navigation-dialog" bind:this={menu} aria-labelledby="navigation-title">
			<div class="menu-head"><h2 id="navigation-title" class="h2">{libraryChooser ? 'Library' : 'Your workspace'}</h2><button class="icon-btn" type="button" onclick={() => menu?.close()} aria-label="Close navigation"><Icon name="x" /></button></div>
			<nav class="nav" aria-label="All destinations">
				{#if libraryChooser}
					{@render libraryMenu()}
				{:else}
				{#each [...desktopPlaces, settingsPlace] as p (p.href)}
					{#if p.href === '/library'}
						{@render libraryMenu()}
					{:else}
					<a class="nav-item" href={to(p.href)} onclick={() => menu?.close()} aria-current={desktopCurrent(p.href) ? 'page' : undefined}><Icon name={p.icon} size={20} /><span>{p.label}</span>{#if p.href === '/wishlist' && badge > 0}<b class="badge" aria-label="{badge} new">{badge}</b>{/if}{#if p.href === '/downloads/decide' && decisions.count > 0}<b class="badge" aria-label="{decisions.count} need a decision">{decisions.count}</b>{/if}</a>
					{/if}
				{/each}
				{/if}
			</nav>
			<div class="menu-account"><a class="nav-item" href={to('/account')} onclick={() => menu?.close()}><Icon name="user" /><span class="grow">{session.s.name}</span></a><button class="nav-item" type="button" onclick={toggleTheme}><Icon name={theme.value === 'dark' ? 'sun' : 'moon'} /><span>{theme.value === 'dark' ? 'Use light theme' : 'Use dark theme'}</span></button><button class="nav-item" type="button" onclick={() => { menu?.close(); void signOut(); }}><Icon name="signout" /><span>Sign out</span></button></div>
		</dialog>
	</div>
{/if}
<ToastList />

<style>
	.app { min-height: 100dvh; }
	.boot { min-height: 100dvh; display: grid; place-items: center; }
	.safe-banner { position: relative; z-index: 50; padding: var(--s-3) var(--s-4); background: var(--surface-2); color: var(--wait-text); border-bottom: 1px solid var(--line); font-size: var(--text-sm); overflow-wrap: anywhere; }
	.skip { position: fixed; left: 12px; top: -80px; z-index: 100; background: var(--accent-fill); color: var(--text-on-red); padding: 12px 16px; border-radius: var(--r-btn); }
	.skip:focus-visible { top: 12px; }
	.global-rail, .context-sidebar { display: none; }
	.brand { display: flex; align-items: center; gap: 10px; min-width: 0; font: var(--weight-semibold) 20px/1.2 var(--font-ui); letter-spacing: -.03em; }
	.brand img { flex: none; }
	.shell { min-height: 100dvh; min-width: 0; }
	.topbar { position: sticky; top: 0; z-index: 20; height: var(--topbar); display: flex; align-items: center; justify-content: space-between; padding-inline: var(--s-4); background: var(--sidebar-bg); border-bottom: 1px solid var(--line); }
	.top-actions { display: flex; align-items: center; gap: var(--s-2); }
	main { min-width: 0; padding: var(--s-6) var(--s-4) calc(var(--tabbar) + var(--s-8) + env(safe-area-inset-bottom)); outline: none; }
	.nav { display: flex; flex-direction: column; gap: 2px; }
	.nav-item { position: relative; display: flex; align-items: center; gap: 12px; min-height: 44px; width: 100%; padding: 8px 12px; border-radius: var(--r-btn); color: var(--text-muted); text-align: left; transition: background var(--dur-fast), color var(--dur-fast); }
	.nav-item:hover { background: var(--surface-2); color: var(--text); }
	.nav-item[aria-current='page'] { background: var(--surface-3); color: var(--text); font-weight: var(--weight-semibold); }
	.nav-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.library-parent { border-radius: var(--r-btn); transition: background var(--dur) var(--ease-out); }
	.library-parent:hover { background: var(--surface-2); color: var(--text); }
	.library-parent.library-active { background: var(--surface-3); color: var(--text); }
	.library-reveal { display: grid; grid-template-rows: 0fr; opacity: 0; transition: grid-template-rows var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.library-reveal.is-open { grid-template-rows: 1fr; opacity: 1; }
	.library-children { min-height: 0; overflow: hidden; margin-left: 21px; padding-left: 12px; position: relative; }
	.library-children::before { content: ''; position: absolute; left: 0; top: 6px; bottom: 8px; width: 1px; background: var(--line); }
	.library-child { position: relative; margin-block: 3px; padding-left: 13px; font-size: var(--text-sm); border-radius: 7px; }
	.library-child[aria-current='page'] { background: var(--surface-2); color: var(--text); font-weight: var(--weight-medium); }

	@media (prefers-reduced-motion: reduce) { .library-reveal { transition: none; } }
	.badge { min-width: 22px; height: 22px; margin-left: auto; padding: 0 6px; display: grid; place-items: center; border-radius: var(--r-pill); background: var(--surface-3); color: var(--text); font-size: var(--text-xs); font-weight: var(--weight-medium); }
	.live { font-family: var(--font-mono); letter-spacing: .025em; display: inline-flex; align-items: center; gap: 9px; color: var(--text-muted); font-size: var(--text-xs); }
	.live i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); flex: none; }
	.live.live i { background: var(--ok); }
	.live.reconnecting i, .live.connecting i { background: var(--wait); }
	.tabbar { position: fixed; inset: auto 0 0; z-index: 30; display: flex; padding: 4px 8px env(safe-area-inset-bottom); background: var(--sidebar-bg); border-top: 1px solid var(--line); }
	.tab { position: relative; display: flex; flex: 1; min-width: 0; min-height: var(--tabbar); flex-direction: column; align-items: center; justify-content: center; gap: 4px; border-radius: var(--r-btn); color: var(--text-muted); font-size: 11px; }
	.tab[aria-current='page'], .tab.active { color: var(--nav-active); }
	.tab .badge { position: absolute; top: 4px; left: calc(50% + 8px); min-width: 16px; height: 16px; padding-inline: 4px; font-size: 10px; }
	.attention-dot { position: absolute; top: 9px; left: calc(50% + 12px); width: 6px; height: 6px; border-radius: 50%; background: var(--wait); }
	.navigation-dialog { width: min(420px, calc(100vw - 24px)); max-height: calc(100dvh - 32px); padding: var(--s-4); border: 1px solid var(--line-strong); border-radius: var(--r-modal); color: var(--text); background: var(--sidebar-bg); box-shadow: var(--shadow-modal); }
	.menu-head { display: flex; align-items: center; justify-content: space-between; gap: var(--s-3); margin-bottom: var(--s-3); }
	.menu-account { margin-top: var(--s-4); padding-top: var(--s-3); border-top: 1px solid var(--line); }
	.part-off { max-width: 40rem; margin: var(--s-8) auto; padding: var(--s-6); border: 1px solid var(--line); border-radius: var(--r-card); }
	.part-off p { margin-top: var(--s-3); }
	@media (min-width: 768px) {
		.app { --rail: 64px; }
		.global-rail { position: fixed; inset: 0 auto 0 0; z-index: 32; display: flex; flex-direction: column; width: var(--rail); padding: 10px 7px; background: var(--bg); border-right: 1px solid var(--line); }
		.rail-brand { width: 42px; height: 42px; margin: 0 auto 12px; display: grid; place-items: center; border-radius: var(--r-btn); }
		.rail-nav, .rail-footer { display: flex; flex-direction: column; align-items: center; gap: 4px; }
		.rail-footer { margin-top: auto; }
		.rail-item { position: relative; width: 44px; height: 44px; display: grid; place-items: center; border-radius: var(--r-btn); color: var(--text-muted); transition: background var(--dur-fast), color var(--dur-fast), transform var(--dur-instant); }
		.rail-item:hover { background: var(--surface-2); color: var(--text); }
		.rail-item[aria-current='page'] { background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-control); }
		.rail-item:active { transform: scale(var(--press)); }
		.rail-badge { position: absolute; right: -1px; top: -2px; min-width: 17px; height: 17px; display: grid; place-items: center; padding-inline: 4px; border: 2px solid var(--bg); border-radius: var(--r-pill); background: var(--accent-fill); color: var(--text-on-red); font-size: 9px; line-height: 1; }
		.rail-live { width: 42px; height: 30px; display: grid; place-items: center; }
		.rail-live i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
		.rail-live.live i { background: var(--ok); }
		.rail-live.connecting i, .rail-live.reconnecting i { background: var(--wait); }
		.shell { margin-left: var(--rail); }
		.topbar, .tabbar { display: none; }
		main { padding: var(--s-6) var(--s-5) var(--s-12); }
	}
	@media (min-width: 1024px) {
		.app { --rail: 56px; }
		.rail-theme { display: none; }
		.settings-section .rail-theme { display: grid; }
		.context-sidebar { position: fixed; inset: 8px auto 8px var(--rail); z-index: 31; display: flex; flex-direction: column; width: var(--context-sidebar); padding: 18px 12px 12px; background: var(--sidebar-bg); border: 1px solid var(--line); border-radius: var(--r-panel); box-shadow: var(--shadow-panel); overflow-y: auto; }
		.context-head { padding: 0 8px 18px; }
		.context-brand { display: block; margin-bottom: 20px; color: var(--text-muted); font-size: var(--text-sm); font-weight: var(--weight-medium); }
		.context-head h2 { font-size: 22px; line-height: 1.2; letter-spacing: -.025em; }
		.context-sidebar .nav-item { min-height: 38px; padding: 7px 10px; gap: 10px; border-radius: var(--r-btn); font-size: var(--text-base); font-weight: var(--weight-medium); }
		.context-sidebar .library-child { min-height: 34px; font-size: var(--text-sm); }
		.context-sidebar .library-children { margin-left: 18px; }
		.context-footer { display: flex; align-items: center; gap: 8px; margin-top: auto; padding: 14px 6px 0; border-top: 1px solid var(--line); }
		.who { display: flex; align-items: center; gap: 9px; min-width: 0; min-height: 38px; color: var(--text-muted); font-size: var(--text-sm); }
		.account-avatar { width: 28px; height: 28px; display: grid; place-items: center; flex: none; border-radius: 50%; background: var(--surface-2); color: var(--text); }
		.context-footer .who { flex: 1; }
		.account-actions { display: flex; }
		.context-footer .icon-btn { width: 36px; height: 36px; }
		.shell { position: relative; margin: 8px 8px 8px calc(var(--rail) + var(--context-sidebar) + 8px); min-height: calc(100dvh - 16px); background: var(--surface-1); border: 1px solid var(--line); border-radius: var(--r-panel); box-shadow: var(--shadow-panel); overflow: clip; }
		.settings-section .context-sidebar { display: none; }
		.settings-section .shell { margin-left: calc(var(--rail) + 8px); }
		main { padding: 24px 28px var(--s-16); }
	}
	@media (pointer: coarse) { .rail-item, .context-sidebar .nav-item { min-height: 44px; } }
</style>
