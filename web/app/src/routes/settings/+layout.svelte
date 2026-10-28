<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate, goto } from '$app/navigation';
	import Icon from '#lib/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import { filterSections, grouped, sectionFor, sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { settingsNow, refreshSettingsStatus } from '#lib/settings-status.svelte.ts';
	import { pageState, stateWords } from '#lib/settings-status.ts';
	import VariantBar from '#lib/ui/VariantBar.svelte';
	import { MENUS, menuView, GROUP_DOT } from '#lib/ui/settings-looks.ts';
	import '#lib/ui/settings-looks.css';
	import { profile, loadProfile, shownName } from '#lib/account/profile.svelte.ts';
	import { roleWord } from '#lib/account/profile.ts';

	// ?menu=1…5 picks the side menu's look (Luis is comparing); no pick is view 1.
	let menuPick = $state(0);
	const view = $derived(menuView(menuPick));
	// View 5: which groups are folded shut (the current page's group always opens).
	let shut = $state<Record<string, boolean>>({});

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = $derived(grouped(sections));
	// The side menu: My account sits on top like a profile card; the rest in
	// grouped, rounded sections. The search box narrows it as you type.
	let find = $state('');
	const account = $derived(sections.find((s) => s.group === 'account'));
	const shown = $derived(grouped(filterSections(sections, find)).filter((g) => g.group.id !== 'account'));
	const firstShown = $derived(filterSections(sections, find)[0]);
	const flagged = (href: string) => {
		const a = settingsNow.answer;
		if (!a) return false;
		return pageState(a, href).state === 'attention';
	};
	function findKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && find) {
			e.preventDefault();
			find = '';
		} else if (e.key === 'Enter' && find.trim()) {
			e.preventDefault();
			// Enter opens the first page that matches, or searches every setting on Settings home.
			void goto(firstShown ? to(firstShown.href) : to('/settings') + '?q=' + encodeURIComponent(find.trim()));
			find = '';
		}
	}

	let { children }: { children: Snippet } = $props();
	const current = $derived(appPath(page.url.pathname));
	const currentSection = $derived(sectionFor(sections, current));
	const currentGroup = $derived(groups.find((g) => g.group.id === currentSection?.group));
	const status = $derived(settingsNow.answer && currentSection ? pageState(settingsNow.answer, currentSection.href) : null);

	// Tablets: one menu button named after the current group, holding every group and its pages.
	const menuItems = $derived<MenuItem[]>([
		{ label: 'All settings', icon: 'home', href: to('/settings'), checked: current === '/settings' },
		...groups.flatMap(({ group, items }) => [
			{ kind: 'heading' as const, label: group.label },
			...items.map((section) => ({ label: section.label, icon: section.icon, href: to(section.href), checked: currentSection?.href === section.href }))
		])
	]);

	onMount(() => {
		void refreshSettingsStatus();
		void loadProfile();
	});

	// A search result links to page#field: scroll to that field, open its
	// Advanced disclosure, focus it and flash it softly. Pages load their data
	// after they appear, so look for the field for a few seconds.
	let seek = 0;
	function highlight(id: string) {
		const mine = ++seek;
		let tries = 0;
		const look = () => {
			if (mine !== seek) return;
			const el = document.getElementById(id);
			if (!el) {
				if (tries++ < 50) setTimeout(look, 100);
				return;
			}
			const details = el.closest('details');
			if (details && !details.open) details.open = true;
			const spot = (el.closest('.field, .checkline, fieldset') as HTMLElement | null) ?? el;
			spot.scrollIntoView({ block: 'center', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
			const focusable = el.matches('input, select, textarea, button, a[href], [tabindex]') ? el : (el.querySelector('input, select, textarea, button') as HTMLElement | null);
			focusable?.focus({ preventScroll: true });
			spot.classList.remove('flash-target');
			void spot.offsetWidth;
			spot.classList.add('flash-target');
			setTimeout(() => spot.classList.remove('flash-target'), 2200);
		};
		look();
	}
	afterNavigate(() => {
		const id = decodeURIComponent(page.url.hash.slice(1));
		if (id) highlight(id);
		if (current === '/settings') void refreshSettingsStatus();
	});
</script>

<svelte:window onhashchange={() => location.hash && highlight(decodeURIComponent(location.hash.slice(1)))} />

<div class="settings-shell" class:home={current === '/settings'}>
	<nav class="settings-nav" aria-label="Settings sections" data-menu={view} data-testid="settings-nav">
		<div class="nav-find">
			<Icon name="search" size={15} />
			<label class="sr-only" for="settings-nav-find">Find a settings page</label>
			<input id="settings-nav-find" type="search" placeholder="Search" autocomplete="off" spellcheck="false" bind:value={find} onkeydown={findKey} />
		</div>

		{#if !find.trim()}
			{#if account}
				<a class="nav-account" href={to(account.href)} aria-current={currentSection?.href === account.href ? 'page' : undefined} data-testid="nav-account">
					<Avatar name={shownName(profile.displayName, session.s.name) || 'Me'} src={session.s.avatar} size={32} />
					<span class="who"><b>{shownName(profile.displayName, session.s.name) || 'My account'}</b><span>{roleWord(session.s.role)}</span></span>
				</a>
			{/if}
		{/if}

		{#each shown as { group, items } (group.id)}
			{@const open = view !== 5 || !!find.trim() || currentSection?.group === group.id || !shut[group.id]}
			<div class="nav-group" class:folded={!open} role="group" aria-labelledby="settings-nav-{group.id}" style:--dot={GROUP_DOT[group.id]}>
				{#if view === 5}
					<h2 id="settings-nav-{group.id}">
						<button type="button" class="fold" aria-expanded={open} onclick={() => (shut = { ...shut, [group.id]: open })}>
							<Icon name="chevronRight" size={12} /><span>{group.label}</span>
						</button>
					</h2>
				{:else}
					<h2 id="settings-nav-{group.id}">{group.label}</h2>
				{/if}
				{#if open}
					<ul class="nav-section">
						{#each items as section (section.href)}
							<li>
								<a class="nav-row" href={to(section.href)} aria-current={currentSection?.href === section.href ? 'page' : undefined} onclick={() => (find = '')}>
									{#if view === 1 || view === 5}
										<span class="glyph"><Icon name={section.icon} size={16} /></span>
									{:else if view === 3}
										<span class="dot" aria-hidden="true"></span>
									{:else if view === 4}
										<span class="gtile" aria-hidden="true"><Icon name={section.icon} size={13} stroke={2.1} /></span>
									{/if}
									<span class="label">{section.label}</span>
									{#if flagged(section.href)}<span class="flag" title="Needs attention"><span class="sr-only">, needs attention</span></span>{/if}
								</a>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/each}
		{#if find.trim() && account && filterSections([account], find).length}
			<ul class="nav-section found-account">
				<li>
					<a class="nav-row" href={to(account.href)} onclick={() => (find = '')}>
						<span class="glyph"><Icon name="user" size={16} /></span>
						<span class="label">My account</span>
					</a>
				</li>
			</ul>
		{/if}
		{#if find.trim() && !firstShown}
			<p class="nav-none">No page is called that. <a href={to('/settings') + '?q=' + encodeURIComponent(find.trim())} onclick={() => (find = '')}>Search every setting</a></p>
		{/if}
	</nav>

	<div class="settings-workspace">
		{#if current !== '/settings'}
			<div class="page-top">
				<a class="back-home" href={to('/settings')}><Icon name="chevronLeft" size={17} />Settings</a>
				<div class="section-picker">
					<Menu label="Settings sections" variant="button" icon={currentGroup?.group.icon ?? 'settings'} text={currentGroup?.group.label ?? 'All settings'} align="start" width={300} items={menuItems} />
				</div>
				{#if status && stateWords(status)}
					<span class="page-status" data-testid="page-status">
						<StatusPill kind={status.state === 'ok' ? 'in_library' : status.state === 'attention' ? 'failed' : 'missing'} label={stateWords(status)} />
					</span>
				{/if}
			</div>
		{/if}
		<div class="settings-content">{@render children()}</div>
	</div>
</div>
<VariantBar options={MENUS} bind:value={menuPick} param="menu" storageKey="holyarrs.settings.menu" label="Settings menu views" />

<style>
	.settings-shell {
		display: grid;
		grid-template-columns: 252px minmax(0, 1fr);
		gap: clamp(24px, 3vw, 44px);
		align-items: start;
	}
	/* The side menu: a search box, the profile entry, then grouped rows
	   (five views under ?menu=1…5). */
	.settings-nav {
		position: sticky;
		top: calc(var(--topbar) + 8px);
		max-height: calc(100dvh - var(--topbar) - 16px);
		overflow-y: auto;
		overscroll-behavior: contain;
		padding: 2px 4px 6px;
		scrollbar-width: thin;
		scrollbar-gutter: stable;
	}
	.nav-find {
		position: relative;
		display: flex;
		align-items: center;
		margin-bottom: 8px;
	}
	.nav-find > :global(.icon) { position: absolute; left: 10px; color: var(--text-muted); pointer-events: none; }
	.nav-find input {
		width: 100%;
		height: 28px;
		padding: 0 10px 0 31px;
		border: 0;
		border-radius: 9px;
		background: var(--surface-3);
		color: var(--text);
		font: var(--t-callout);
		outline: none;
		transition: box-shadow var(--dur-fast) var(--ease-out);
	}
	.nav-find input::placeholder { color: var(--text-muted); }
	.nav-find input:focus-visible { box-shadow: 0 0 0 3px var(--accent-soft), 0 0 0 1px var(--accent); }
	/* The profile entry: avatar, name, role. Flat, no card. */
	.nav-account {
		display: flex;
		align-items: center;
		gap: 10px;
		margin: 2px 0 10px;
		padding: 6px 8px;
		border-radius: 10px;
		color: var(--text);
		transition: background var(--dur-fast) var(--ease-out);
	}
	.nav-account:hover { background: var(--surface-2); }
	.nav-account[aria-current='page'] { background: var(--accent-soft); }
	.nav-account:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.who { flex: 1; min-width: 0; display: grid; }
	.who b { font: var(--t-callout); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.who span { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	/* Groups: a small muted heading over flat rows (no rounded white panels). */
	.nav-group { margin-top: 10px; }
	.nav-group h2 {
		padding: 0 8px 2px;
		font: var(--t-caption);
		line-height: 16px;
		font-weight: 500;
		color: var(--text-muted);
	}
	.nav-section { margin: 0; padding: 0; list-style: none; }
	.found-account { margin-top: 8px; }
	.nav-row {
		position: relative;
		display: flex;
		align-items: center;
		gap: 9px;
		min-height: 28px;
		padding: 2px 8px;
		border-radius: 7px;
		color: var(--text);
		font: var(--t-callout);
		font-size: 13.5px;
		font-weight: 400;
		transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	.nav-row .label { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.nav-row:hover { background: var(--surface-2); }
	.nav-row:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
	.nav-row[aria-current='page'] { color: var(--accent-text); font-weight: 500; }
	.glyph { flex: none; display: grid; place-items: center; width: 18px; color: var(--text-muted); transition: color var(--dur-fast) var(--ease-out); }
	.nav-row[aria-current='page'] .glyph { color: var(--accent-text); }
	.flag { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--fail); }
	.nav-none { padding: 10px 8px; color: var(--text-muted); font: var(--t-footnote); }
	.nav-none a { color: var(--accent-text); font-weight: 600; }

	/* 1 Lines: line icons, the current row gets a soft accent wash. */
	[data-menu='1'] .nav-row[aria-current='page'],
	[data-menu='5'] .nav-row[aria-current='page'] { background: var(--accent-soft); }
	/* 2 Text: words only; a thin accent bar marks the current page. */
	[data-menu='2'] .nav-row { padding-left: 12px; }
	[data-menu='2'] .nav-row[aria-current='page']::before {
		content: '';
		position: absolute;
		left: 2px;
		top: 6px;
		bottom: 6px;
		width: 2px;
		border-radius: 2px;
		background: var(--accent);
	}
	[data-menu='2'] .nav-group h2 { padding-left: 12px; font-weight: 600; color: var(--text); }
	/* 3 Dots: one small tinted dot per group. */
	.dot { flex: none; width: 6px; height: 6px; margin: 0 4px 0 2px; border-radius: 50%; background: var(--dot); opacity: 0.85; }
	[data-menu='3'] .nav-row[aria-current='page'] { background: color-mix(in srgb, var(--dot) 12%, transparent); color: var(--text); }
	/* 4 Tiles: neutral grey tiles; the current one turns accent. */
	.gtile {
		flex: none;
		display: grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 6px;
		background: var(--surface-3);
		color: var(--text-muted);
		transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	.nav-row[aria-current='page'] .gtile { background: var(--accent-fill); color: var(--text-on-red); }
	[data-menu='4'] .nav-row[aria-current='page'] { color: var(--text); font-weight: 600; }
	/* 5 Folds: each group opens and shuts; the current group stays open. */
	.fold {
		display: flex;
		align-items: center;
		gap: 4px;
		width: 100%;
		min-height: 24px;
		padding: 0;
		border: 0;
		background: none;
		color: inherit;
		font: inherit;
		cursor: pointer;
	}
	.fold:hover { color: var(--text); }
	.fold:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.fold :global(.icon) { transition: transform var(--dur-fast) var(--ease-out); transform: rotate(90deg); }
	.folded .fold :global(.icon) { transform: none; }
	[data-menu='5'] .nav-group { margin-top: 4px; }
	@media (prefers-reduced-motion: reduce) {
		.nav-row, .nav-account, .nav-find input, .glyph, .gtile, .fold :global(.icon) { transition: none; }
	}
	.settings-workspace { min-width: 0; max-width: 820px; }
	.settings-content { width: 100%; min-width: 0; }
	.page-top { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; min-height: 32px; margin-bottom: 8px; }
	.page-top:not(:has(.page-status)) { display: none; }
	.page-status { margin-left: auto; }
	.back-home, .section-picker { display: none; }
	.back-home { align-items: center; gap: 2px; min-height: 44px; margin-left: -6px; padding-right: 8px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	/* Section pages: every group is a SettingsCard (or a list card shaped like
	   one), stacked with one rhythm. */
	.settings-content > :global(:is(.ui-settings, .sc-form, .card, div[data-testid], ul.card) + :is(.ui-settings, .sc-form, .card, div[data-testid], ul, p, section)) { margin-top: 16px; }
	.settings-content :global(.sc-form > .ui-settings + .ui-settings) { margin-top: 16px; }
	.settings-content :global(.card) { border: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); padding: 18px 20px; }
	.settings-content :global(.card .h2) { font: var(--t-headline); font-size: 17px; letter-spacing: normal; }
	.settings-content :global(.sc-form) { display: block; }
	.settings-content :global(.ui-settings .h2) { font: var(--t-headline); font-size: 16px; letter-spacing: normal; }
	/* The field a search result pointed at. */
	.settings-content :global(.flash-target) { border-radius: 10px; animation: flash 2.2s var(--ease-out); }
	@keyframes flash {
		0%, 30% { background: var(--accent-soft); box-shadow: 0 0 0 6px var(--accent-soft); }
		100% { background: transparent; box-shadow: 0 0 0 6px transparent; }
	}
	@media (prefers-reduced-motion: reduce) {
		.settings-content :global(.flash-target) { animation: none; background: var(--accent-soft); box-shadow: 0 0 0 6px var(--accent-soft); }
	}
	@media (min-height: 1000px) { .nav-row { min-height: 30px; } .nav-group { margin-top: 14px; } }
	@media (pointer: coarse) { .nav-row { min-height: 40px; } }
	/* Tablets: the list becomes one menu button named after the group. */
	@media (max-width: 1023px) {
		.settings-shell { grid-template-columns: minmax(0, 1fr); }
		.settings-nav { display: none; }
		.settings-workspace { max-width: none; }
		.page-top, .page-top:not(:has(.page-status)) { display: flex; margin-bottom: 16px; }
		.section-picker { display: block; }
	}
	/* Phones: Settings home is the index; a page opens on its own with a back link. */
	@media (max-width: 767px) {
		.section-picker { display: none; }
		.back-home { display: inline-flex; }
		.page-top { margin-bottom: 4px; }
	}
</style>
