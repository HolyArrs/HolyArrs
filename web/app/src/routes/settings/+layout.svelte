<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate, goto } from '$app/navigation';
	import Icon from '#lib/Icon.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Confirm from '#lib/account/Confirm.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { filterSections, grouped, sectionFor, sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { settingsNow, refreshSettingsStatus } from '#lib/settings-status.svelte.ts';
	import { pageState, stateWords } from '#lib/settings-status.ts';
	import '#lib/ui/settings-looks.css';

	// Settings is one layout: folding groups on the left (look 6, Luis
	// 2026-11-02), the page on the right. Plain cards and buttons on one
	// surface that holds the menu and the page together (Luis, 2026-11-03).

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = $derived(grouped(sections));
	const menuGroups = $derived(groups.filter((g) => g.group.id !== 'account'));
	const account = $derived(sections.find((s) => s.group === 'account'));
	// The search box: Enter opens the first page that matches.
	let find = $state('');
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
			void goto(firstShown ? to(firstShown.href) : to('/settings') + '?q=' + encodeURIComponent(find.trim()));
			find = '';
		}
	}

	let { children }: { children: Snippet } = $props();
	const current = $derived(appPath(page.url.pathname));
	const home = $derived(current === '/settings');
	const currentSection = $derived(sectionFor(sections, current));
	const currentGroup = $derived(groups.find((g) => g.group.id === currentSection?.group));
	const status = $derived(settingsNow.answer && currentSection ? pageState(settingsNow.answer, currentSection.href) : null);
	// The current group is open; a heading opens another.
	let openGroup = $state<string | null>(null);
	const isOpen = (id: string) => (openGroup ?? currentGroup?.group.id ?? menuGroups[0]?.group.id) === id || !!find.trim();

	// Risky actions (keys, passwords, roles, backups, single sign-on) want
	// the password typed again. The server answers 403 "reauth"; instead of leaving that
	// bare error on the page, ask here, then load the page again. A failed
	// save keeps the page (and its edits) under the box.
	let reauth = $state<'load' | 'save' | null>(null);
	let pageKey = $state(0);
	$effect(() => {
		if (reauth && session.s.localRecent) {
			if (reauth === 'load') pageKey++;
			reauth = null;
		}
	});

	onMount(() => {
		void refreshSettingsStatus();
		const onReauth = (e: Event) => {
			const method = (e as CustomEvent<{ method?: string }>).detail?.method ?? 'GET';
			session.s.localRecent = false;
			// My account asks inline, next to the control that needs it.
			if (currentSection?.group === 'account') return;
			if (reauth !== 'load') reauth = method === 'GET' ? 'load' : 'save';
		};
		window.addEventListener('holyarrs:reauth', onReauth);
		return () => window.removeEventListener('holyarrs:reauth', onReauth);
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
		openGroup = null;
		reauth = null;
		if (home) void refreshSettingsStatus();
	});
</script>

<svelte:window onhashchange={() => location.hash && highlight(decodeURIComponent(location.hash.slice(1)))} />

{#snippet pill()}
	{#if status && stateWords(status)}<span class="page-status" data-testid="page-status"><StatusPill kind={status.state === 'ok' ? 'in_library' : status.state === 'attention' ? 'failed' : 'missing'} label={stateWords(status)} /></span>{/if}
{/snippet}

<div class="settings-shell look-fold" class:home>
	<nav class="settings-nav" aria-label="Settings sections" data-testid="settings-nav">
		<div class="nav-find">
			<Icon name="search" size={15} />
			<label class="sr-only" for="settings-nav-find">Find a setting</label>
			<input id="settings-nav-find" type="search" placeholder="Search" autocomplete="off" spellcheck="false" bind:value={find} onkeydown={findKey} />
		</div>
		{#each shown as { group, items } (group.id)}
			<div class="nav-group" class:closed={!isOpen(group.id)} role="group" aria-labelledby="settings-nav-{group.id}">
				<button type="button" class="fold-head" id="settings-nav-{group.id}" aria-expanded={isOpen(group.id)} onclick={() => (openGroup = isOpen(group.id) ? '' : group.id)}>
					<span class="glyph"><Icon name={group.icon} size={15} /></span>
					<span class="label">{group.label}</span>
					{#if items.some((i) => flagged(i.href))}<span class="flag" title="Needs attention"><span class="sr-only">, needs attention</span></span>{/if}
					<Icon name="chevronDown" size={14} />
				</button>
				{#if isOpen(group.id)}
					<ul class="nav-section">
						{#each items as section (section.href)}
							<li>
								<a class="nav-row" href={to(section.href)} aria-current={currentSection?.href === section.href ? 'page' : undefined} onclick={() => (find = '')}>
									<span class="label">{section.label}</span>
									{#if flagged(section.href)}<span class="flag" title="Needs attention"><span class="sr-only">, needs attention</span></span>{/if}
								</a>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/each}
		{#if account && !find.trim()}
			<a class="nav-row nav-account" href={to(account.href)} aria-current={currentSection?.href === account.href ? 'page' : undefined}>
				<span class="glyph"><Icon name="user" size={16} /></span><span class="label">My account</span>
			</a>
		{/if}
		{#if find.trim() && !firstShown}
			<p class="nav-none">No page is called that. <a href={to('/settings') + '?q=' + encodeURIComponent(find.trim())} onclick={() => (find = '')}>Search every setting</a></p>
		{/if}
	</nav>
	<div class="settings-workspace">
		{#if !home}
			<div class="page-top phone-top">
				<a class="back-home" href={to('/settings')}><Icon name="chevronLeft" size={17} />Settings</a>
				{@render pill()}
			</div>
		{/if}
		<div class="settings-content">
			{#if reauth && !session.s.hasPassword}
				<SettingsCard id="settings-reauth" title="Set a password first" description="This touches keys, passwords, people or backups, so HolyArrs asks for your HolyArrs password. You sign in another way and have none yet." icon="shield">
					<div><Button variant="primary" href={to('/account')}>Set a password</Button></div>
				</SettingsCard>
			{:else if reauth}
				<SettingsCard id="settings-reauth" title="Confirm your password" description="This touches keys, passwords, people or backups, so HolyArrs asks for your password again. It then won't ask for 10 minutes." icon="shield">
					<Confirm action="password">{#snippet children()}{/snippet}</Confirm>
				</SettingsCard>
			{/if}
			{#if reauth !== 'load'}
				{#key pageKey}{@render children()}{/key}
			{/if}
		</div>
	</div>
</div>


<style>
	/* ---------- shared ---------- */
	.settings-shell { display: grid; grid-template-columns: 252px minmax(0, 1fr); gap: clamp(24px, 3vw, 44px); align-items: start; }
	.settings-workspace { min-width: 0; max-width: 820px; }
	.settings-content { width: 100%; min-width: 0; }
	.flag { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--fail); }
	.nav-find { position: relative; display: flex; align-items: center; margin-bottom: 8px; }
	.nav-find > :global(.icon) { position: absolute; left: 10px; color: var(--text-muted); pointer-events: none; }
	.nav-find input {
		width: 100%; height: 30px; padding: 0 10px 0 31px; border: 0; border-radius: 9px;
		background: var(--surface-3); color: var(--text); font: var(--t-callout); outline: none;
	}
	.nav-find input::placeholder { color: var(--text-muted); }
	.nav-find input:focus-visible { box-shadow: 0 0 0 3px var(--accent-soft), 0 0 0 1px var(--accent); }
	.settings-nav { position: sticky; top: calc(var(--topbar) + 8px); max-height: calc(100dvh - var(--topbar) - 16px); overflow-y: auto; overscroll-behavior: contain; padding: 2px 4px 6px; scrollbar-width: thin; }
	.nav-group { margin-top: 10px; }
	.nav-section { margin: 0; padding: 0; list-style: none; }
	.nav-row {
		position: relative; display: flex; align-items: center; gap: 9px; width: 100%; min-height: 30px; padding: 2px 8px;
		border: 0; border-radius: 7px; background: none; color: var(--text); font: var(--t-callout); font-size: 13.5px; text-align: left; cursor: pointer;
		transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	.nav-row .label { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.nav-row:hover { background: var(--surface-2); }
	.nav-row:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
	.nav-row[aria-current='page'] { background: var(--accent-soft); color: var(--accent-text); font-weight: 500; }
	.nav-row[aria-current='page'] .glyph { color: var(--accent-text); }
	.nav-row > :global(.icon:last-child) { color: var(--text-muted); }
	.glyph { flex: none; display: grid; place-items: center; width: 18px; color: var(--text-muted); }
	.nav-account { margin-top: 14px; }
	.nav-none { padding: 10px 8px; color: var(--text-muted); font: var(--t-footnote); }
	.nav-none a { color: var(--accent-text); font-weight: 600; }
	.page-top { display: flex; align-items: center; gap: 12px; min-height: 32px; margin-bottom: 8px; }
	.page-status { margin-left: auto; }
	.back-home { display: none; align-items: center; gap: 2px; min-height: 44px; margin-left: -6px; padding-right: 8px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	.phone-top:not(:has(.page-status)) { display: none; }
	@media (pointer: coarse) { .nav-row { min-height: 40px; } }
	@media (max-width: 767px) and (pointer: coarse) {
		/* The shell prefix outranks the base .fold-head rule further down. */
		.settings-shell .nav-row, .settings-shell .fold-head { min-height: 44px; }
		.nav-find input { height: 44px; border-radius: 11px; }
	}
	@media (prefers-reduced-motion: reduce) { .nav-row, .fold-head > :global(.icon:last-child) { transition: none; } }

	/* content: the card grid from settings-looks.css, plus the "flash" for search hits */
	/* Hand-built cards wear the same surface, corners and title as SettingsCards. */
	.settings-content :global(.card) { border: 0; border-radius: var(--fin-r); background: var(--fin-card); box-shadow: var(--fin-shadow), 0 0 0 1px var(--fin-edge); padding: 12px 14px; }
	.settings-content :global(:is(.card, .ui-settings) :is(h2, .h2)) { font: var(--t-headline); font-size: 15px; font-weight: 600; letter-spacing: normal; }
	.settings-content > :global(:is(.ui-settings, .sc-form, .card, div[data-testid], ul.card) + :is(.ui-settings, .sc-form, .card, div[data-testid], ul, p, section)) { margin-top: 16px; }
	.settings-content :global(.sc-form > .ui-settings + .ui-settings) { margin-top: 0; }
	.settings-content :global(.sc-form) { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; align-items: stretch; }
	.settings-content :global(.sc-form > :not(.ui-settings)) { grid-column: 1 / -1; }
	.settings-content :global(.sc-form > section.ui-settings:last-of-type:nth-of-type(odd)) { grid-column: 1 / -1; }
	@media (max-width: 760px) { .settings-content :global(.sc-form) { grid-template-columns: minmax(0, 1fr); } }
	.settings-content :global(.flash-target) { border-radius: 10px; animation: flash 2.2s var(--ease-out); }
	@keyframes flash { 0%, 30% { background: var(--accent-soft); box-shadow: 0 0 0 6px var(--accent-soft); } 100% { background: transparent; box-shadow: 0 0 0 6px transparent; } }
	@media (prefers-reduced-motion: reduce) { .settings-content :global(.flash-target) { animation: none; background: var(--accent-soft); box-shadow: 0 0 0 6px var(--accent-soft); } }

	/* ---------- folding groups: headings are buttons, one group open ---------- */
	.look-fold .nav-group { margin-top: 4px; }
	.fold-head { display: flex; align-items: center; gap: 9px; width: 100%; min-height: 36px; padding: 2px 8px; border: 0; border-radius: 8px; background: none; color: var(--text); font: var(--t-callout); font-weight: 600; text-align: left; cursor: pointer; }
	.fold-head .label { flex: 1; }
	.fold-head > :global(.icon:last-child) { color: var(--text-muted); transition: transform var(--dur-fast) var(--ease-out); }
	.fold-head[aria-expanded='false'] > :global(.icon:last-child) { transform: rotate(-90deg); }
	.fold-head:hover { background: var(--surface-2); }
	.look-fold .nav-section { padding: 2px 0 6px 8px; margin-left: 16px; border-left: 1px solid var(--line); }

	/* One surface for all of Settings (Luis, 2026-11-03): a quiet panel holds
	   the side menu and the page, so the menu is not left on the bare page.
	   Cards and buttons stay plain. The panel is a light tint of the text
	   colour: soft grey under white cards in light, a slight lift in dark. */
	.look-fold { position: relative; isolation: isolate;
		--fin-r: 12px; --fin-card: var(--surface-1); --fin-edge: var(--line);
		--fin-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 8px 24px -14px rgb(0 0 0 / 35%); --fin-btn-r: var(--r-btn); }
	.look-fold::before { content: ''; position: absolute; inset: -20px -20px -40px; z-index: -1; pointer-events: none; border-radius: 24px;
		background: color-mix(in srgb, var(--text) 4%, transparent); box-shadow: 0 0 0 1px color-mix(in srgb, var(--line) 70%, transparent); }
	.look-fold .settings-nav { padding-right: 16px; border-right: 1px solid var(--line); }
	.look-fold .settings-content :global(.ui-settings) { border-radius: var(--fin-r); background: var(--fin-card); box-shadow: var(--fin-shadow), 0 0 0 1px var(--fin-edge); }
	.look-fold :global(.ui-btn) { border-radius: var(--fin-btn-r); transition: background var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.look-fold :global(.ui-btn:active:not(:disabled)) { transform: scale(0.98); }
	.look-fold :global(.input) { border-radius: var(--fin-btn-r); }
	.look-fold .nav-row, .look-fold .fold-head { border-radius: calc(var(--fin-btn-r) - 2px); }

	@media (prefers-reduced-motion: reduce) {
		.look-fold .settings-content :global(.ui-settings), .look-fold :global(.ui-btn) { transition: none; }
		.look-fold :global(.ui-btn:hover:not(:disabled)), .look-fold :global(.ui-btn:active:not(:disabled)) { transform: none; }
	}
	@media (max-width: 1023px) { .look-fold .settings-nav { padding-right: 4px; border-right: 0; } }
	@media (max-width: 767px) { .look-fold::before { inset: -16px -16px -48px; border-radius: 0; box-shadow: none; } }

	/* ---------- phones: Settings home is the index; a page opens alone with a back link ---------- */
	@media (max-width: 1023px) {
		.settings-shell { grid-template-columns: minmax(0, 1fr); }
		.settings-nav { position: static; max-height: none; }
	}
	@media (max-width: 767px) {
		.settings-shell:not(.home) .settings-nav { display: none; }
		/* Home on a phone: the page title stays (screen readers, the sticky
		   mini title) above the menu; the rest of the index is the menu itself. */
		.settings-shell.home .settings-workspace { order: -1; }
		.settings-shell.home .settings-content > :global(:not(.ui-head)) { display: none; }
		.back-home { display: inline-flex; }
		.phone-top, .phone-top:not(:has(.page-status)) { display: flex; margin-bottom: 4px; }
	}
</style>
