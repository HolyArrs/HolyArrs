<script lang="ts">
	import { resolve } from '$app/paths';
	import { setBase } from '#lib/paths.ts';
	setBase(resolve('/'));
	import { to, appPath } from '#lib/paths.ts';
	import '../app.css';
	import { onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
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
	const desktopPlaces = $derived([...places, ...extraDesktopPlaces.filter((p) => allowed(p.need) && !offPartName(partsNow.answer, p.href))]);
	// A direct link to a page of a part that is off says so instead.
	const offHere = $derived(offPartName(partsNow.answer, appPath(page.url.pathname)));
	const staff = $derived(allowed('staff'));
	// Members reach their account from the bottom menu instead of Settings.
	const settingsPlace = $derived(
		staff ? { href: '/settings', label: 'Settings', icon: 'settings' as IconName } : { href: '/account', label: 'My account', icon: 'user' as IconName }
	);

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
		await session.logout();
		toasts.push('Signed out.');
	}

	const liveWord = $derived(
		{ off: 'Live updates off', connecting: 'Connecting', live: 'Live', reconnecting: 'Reconnecting' }[live.status.state]
	);
</script>

{#if session.s.safeMode}
	<div class="safe-banner" role="status" data-testid="safe-mode">
		<b>Safe mode.</b> Only the core runs: sign-in, the Library, settings, backups and health. Searches, downloads, imports, subtitles
		and schedules are off until HolyArrs starts again without safe mode.
	</div>
{/if}
{#if bare}
	{@render children()}
{:else if !ready}
	<div class="boot" aria-busy="true" aria-label="Loading HolyArrs">
		<img src={to(mark)} alt="" width="48" height="48" />
	</div>
{:else if firstRunWelcome}
	<main id="main">{@render children()}</main>
{:else}
	<a class="skip" href="#main">Skip to content</a>
	<div class="app">
		<aside class="sidebar">
			<a class="brand" href={to(`/`)} aria-label="HolyArrs home">
				<img src={to(mark)} alt="" width="34" height="34" />
				<span>HolyArrs</span>
			</a>
			<nav class="nav" aria-label="Main">
				{#each desktopPlaces as p (p.href)}
					<a class="nav-item" href={to(p.href)} aria-current={desktopCurrent(p.href) ? 'page' : undefined}>
						<Icon name={p.icon} size={21} /><span>{p.label}</span>
						{#if p.href === '/wishlist' && badge > 0}<b class="badge" aria-label="{badge} new">{badge}</b>{/if}
						{#if p.href === '/downloads' && decisions.count > 0}<b class="badge" data-testid="nav-decide" aria-label="{decisions.count} need a decision">{decisions.count}</b>{/if}
					</a>
				{/each}
			</nav>
			<nav class="nav nav-bottom" aria-label="Settings">
				<a class="nav-item" href={to(settingsPlace.href)} aria-current={desktopCurrent(settingsPlace.href) ? 'page' : undefined}>
					<Icon name={settingsPlace.icon} size={21} /><span>{settingsPlace.label}</span>
				</a>
			</nav>
		</aside>

		<div class="shell">
			<header class="topbar">
				<a class="brand brand-m" href={to(`/`)} aria-label="HolyArrs home">
					<img src={to(mark)} alt="" width="30" height="30" />
					<span>HolyArrs</span>
				</a>
				<div class="top-actions">
					{#if allowed('manage_library')}
						<span class="live {live.status.state}" title={liveWord}>
							<i aria-hidden="true"></i><span class="live-word">{liveWord}</span>
						</span>
					{/if}
					<button
						class="icon-btn"
						type="button"
						onclick={toggleTheme}
						aria-label={theme.value === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
					>
						<Icon name={theme.value === 'dark' ? 'sun' : 'moon'} />
					</button>
					<a class="who small muted" href={to(`/account`)} title="My account">{session.s.name}</a>
					<button class="icon-btn" type="button" onclick={signOut} aria-label="Sign out">
						<Icon name="signout" />
					</button>
				</div>
			</header>
			<main id="main" tabindex="-1">
				{#if offHere}
					<section class="part-off" role="status" data-testid="part-off">
						<h1>This part is off</h1>
						<p>{offHere} {offHere.includes(' and ') ? 'are' : 'is'} turned off, so this page is hidden. Nothing is deleted: everything comes back when it is turned on again.</p>
						<p class="muted">The owner or an admin can turn it on again.</p>
					</section>
				{:else}
					{@render children()}
				{/if}
			</main>
		</div>

		<nav class="tabbar" aria-label="Main">
			{#each [...places, settingsPlace] as p (p.href)}
				<a class="tab" href={to(p.href)} aria-current={current(p.href) ? 'page' : undefined}>
					<Icon name={p.icon} size={22} /><span>{p.label}</span>
					{#if p.href === '/wishlist' && badge > 0}<b class="badge" aria-label="{badge} new">{badge}</b>{/if}
					{#if p.href === '/downloads' && decisions.count > 0}<b class="badge" aria-label="{decisions.count} need a decision">{decisions.count}</b>{/if}
				</a>
			{/each}
		</nav>
	</div>
{/if}
<ToastList />

<style>
	.part-off {
		max-width: 36rem;
		margin: var(--s-10) auto;
		padding: var(--s-6);
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
	}
	.part-off h1 {
		margin: 0 0 var(--s-2);
	}
	.safe-banner {
		padding: var(--s-2) var(--s-4);
		background: color-mix(in srgb, var(--wait) 14%, var(--bg));
		color: var(--wait-text);
		border-bottom: 1px solid var(--border);
		font: var(--weight-regular) var(--text-sm) / 1.4 var(--font-ui);
		overflow-wrap: anywhere;
	}
	.badge {
		min-width: 20px;
		height: 20px;
		padding: 0 6px;
		margin-left: auto;
		border-radius: var(--r-pill);
		background: var(--ha-red-press);
		color: var(--text-on-red);
		font: var(--weight-semibold) var(--text-xs) / 20px var(--font-ui);
		text-align: center;
	}
	.tab .badge {
		position: absolute;
		top: 4px;
		left: calc(50% + 6px);
		margin: 0;
	}
	.boot {
		min-height: 100vh;
		display: grid;
		place-items: center;
	}
	.boot img {
		opacity: 0.8;
	}
	.skip {
		position: fixed;
		left: 12px;
		top: -60px;
		z-index: 100;
		background: var(--ha-red-press);
		color: var(--text-on-red);
		padding: 10px 14px;
		border-radius: var(--r-btn);
	}
	.skip:focus-visible {
		top: 12px;
	}
	.sidebar {
		position: fixed;
		inset: 0 auto 0 0;
		width: var(--sidebar);
		z-index: 30;
		background: var(--surface-1);
		border-right: 1px solid var(--line);
		display: flex;
		flex-direction: column;
		padding: var(--s-5) var(--s-3) var(--s-4);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		padding: 0 var(--s-2) var(--s-6);
		border-radius: var(--r-btn);
		font: var(--weight-bold) 21px/1 var(--font-display);
		letter-spacing: var(--tracking-display);
	}
	.brand-m {
		display: none;
		padding: 0;
	}
	.nav {
		display: flex;
		flex-direction: column;
		gap: var(--s-1);
	}
	.nav-bottom {
		margin-top: auto;
	}
	.nav-item {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: var(--tap);
		padding: 0 var(--s-3);
		border-radius: var(--r-btn);
		color: var(--text-muted);
		font-weight: var(--weight-medium);
		transition:
			background var(--dur-fast),
			color var(--dur-fast),
			transform var(--dur-instant);
	}
	.nav-item:hover {
		background: var(--surface-3);
		color: var(--text);
	}
	.nav-item:active {
		transform: scale(var(--press));
	}
	.nav-item[aria-current='page'] {
		background: var(--ha-red-soft);
		color: var(--text);
	}
	.nav-item[aria-current='page'] :global(.icon) {
		color: var(--ha-red);
	}
	.nav-item[aria-current='page']::before {
		content: '';
		position: absolute;
		left: 0;
		top: 10px;
		bottom: 10px;
		width: 3px;
		border-radius: 3px;
		background: var(--ha-red);
	}
	.shell {
		margin-left: var(--sidebar);
		min-height: 100vh;
	}
	.topbar {
		position: sticky;
		top: 0;
		z-index: 20;
		height: var(--topbar);
		display: flex;
		align-items: center;
		gap: var(--s-4);
		padding: 0 var(--s-8);
		background: color-mix(in srgb, var(--bg) 88%, transparent);
		backdrop-filter: blur(10px);
		border-bottom: 1px solid var(--line);
	}
	.top-actions {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: var(--s-2);
	}
	.who {
		max-width: 14ch;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.live {
		display: inline-flex;
		align-items: center;
		gap: var(--s-2);
		font-size: var(--text-xs);
		color: var(--text-muted);
		padding: 0 var(--s-2);
	}
	.live i {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--text-faint);
	}
	.live.live i {
		background: var(--ok);
	}
	.live.reconnecting i,
	.live.connecting i {
		background: var(--wait);
	}
	main {
		padding: var(--s-8) var(--s-8) var(--s-16);
		max-width: 1280px;
		outline: none;
	}
	.tabbar {
		display: none;
	}

	@media (max-width: 767px) {
		.sidebar {
			display: none;
		}
		.shell {
			margin-left: 0;
		}
		.topbar {
			padding: 0 var(--s-4);
			gap: var(--s-2);
		}
		.brand-m {
			display: flex;
			gap: var(--s-2);
			font-size: 19px;
		}
		.live-word,
		.who {
			display: none;
		}
		main {
			padding: var(--s-5) var(--s-4) calc(var(--tabbar) + var(--s-10) + env(safe-area-inset-bottom));
		}
		.tabbar {
			position: fixed;
			z-index: 40;
			left: 0;
			right: 0;
			bottom: 0;
			display: grid;
			grid-auto-columns: 1fr;
			grid-auto-flow: column;
			padding-bottom: env(safe-area-inset-bottom);
			background: var(--surface-1);
			border-top: 1px solid var(--line);
		}
		.tab {
			display: flex;
			flex-direction: column;
			align-items: center;
			justify-content: center;
			gap: 3px;
			min-height: var(--tabbar);
			min-width: 0;
			position: relative;
			font-size: 10.5px;
			font-weight: var(--weight-medium);
			color: var(--text-muted);
			transition:
				color var(--dur-fast),
				transform var(--dur-instant);
		}
		.tab:active {
			transform: scale(var(--press));
		}
		.tab[aria-current='page'] {
			color: var(--text);
		}
		.tab[aria-current='page'] :global(.icon) {
			color: var(--ha-red);
		}
	}
	@media (min-width: 768px) and (max-width: 1100px) {
		main {
			padding: var(--s-6);
		}
	}
</style>
