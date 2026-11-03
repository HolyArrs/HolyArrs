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
	import { filterSections, grouped, sectionFor, sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { settingsNow, refreshSettingsStatus } from '#lib/settings-status.svelte.ts';
	import { pageState, stateWords } from '#lib/settings-status.ts';
	import '#lib/ui/settings-looks.css';

	// Settings is one layout: folding groups on the left (look 6, Luis
	// 2026-11-02), the page on the right. Ten finishes still sit on top of it
	// until one is picked: ?finish=<id> or the Finish box, kept in this browser.
	const finishes: { id: string; name: string }[] = [
		{ id: 'plain', name: '1 Plain' },
		{ id: 'ink', name: '2 Ink' },
		{ id: 'grain', name: '3 Paper grain' },
		{ id: 'glass', name: '4 Glass' },
		{ id: 'outline', name: '5 Outline' },
		{ id: 'soft', name: '6 Soft' },
		{ id: 'carbon', name: '7 Carbon' },
		{ id: 'spot', name: '8 Spotlight' },
		{ id: 'brushed', name: '9 Brushed' },
		{ id: 'ember', name: '10 Ember' }
	];
	const FIN_KEY = 'holyarrs.settings-finish';
	let finishId = $state('plain');
	function pickFinish(id: string) {
		finishId = id;
		try { localStorage.setItem(FIN_KEY, id); } catch {}
	}

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

	// Some pages (sign-in details, users, logs, backups) want the password
	// typed again. The server answers 403 "reauth"; instead of leaving that
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
		const fq = page.url.searchParams.get('finish');
		let fsaved: string | null = null;
		try { fsaved = localStorage.getItem(FIN_KEY); } catch {}
		const fid = fq ?? fsaved;
		if (fid && finishes.some((f) => f.id === fid)) pickFinish(fid);
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

<div class="settings-shell look-fold fin-{finishId}" class:home data-finish={finishId}>
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
			{#if reauth}
				<SettingsCard id="settings-reauth" title="Confirm your password" description="This page holds sign-in details, so HolyArrs asks for your password again." icon="shield">
					<Confirm action="password">{#snippet children()}{/snippet}</Confirm>
				</SettingsCard>
			{/if}
			{#if reauth !== 'load'}
				{#key pageKey}{@render children()}{/key}
			{/if}
		</div>
	</div>
</div>

<!-- Picks one of the ten finishes until Luis chooses. -->
<div class="look-picker" data-testid="look-picker">
	<label for="settings-finish">Finish</label>
	<select id="settings-finish" value={finishId} onchange={(e) => pickFinish((e.currentTarget as HTMLSelectElement).value)}>
		{#each finishes as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
	</select>
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

	/* ======================================================================
	   Finishes for look 6 (Luis, 2026-11-02). Each sets a backdrop behind the
	   Settings area, the card surface, the buttons and the feel. Variables
	   first, then the few rules a finish needs beyond them.
	   ====================================================================== */
	.look-fold { position: relative; isolation: isolate;
		--fin-r: 12px; --fin-card: var(--surface-1); --fin-edge: var(--line); --fin-shadow: 0 1px 2px rgb(0 0 0 / 25%), 0 8px 24px -12px rgb(0 0 0 / 45%);
		--fin-btn-r: var(--r-btn); --fin-lift: 0; }
	.look-fold::before { content: ''; position: absolute; inset: -24px -16px -48px; z-index: -1; pointer-events: none; border-radius: 28px; }
	.look-fold .settings-content :global(.ui-settings) { border-radius: var(--fin-r); background: var(--fin-card); box-shadow: var(--fin-shadow), 0 0 0 1px var(--fin-edge); transition: box-shadow var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.look-fold :global(.ui-btn) { border-radius: var(--fin-btn-r); transition: background var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.look-fold :global(.ui-btn:active:not(:disabled)) { transform: scale(0.98); }
	.look-fold :global(.input) { border-radius: var(--fin-btn-r); }
	.look-fold .nav-row, .look-fold .fold-head { border-radius: calc(var(--fin-btn-r) - 2px); }

	/* 2 Ink: near-black, crisp edges, tighter corners, buttons inverted to white; nothing animates. */
	.fin-ink { --fin-r: 8px; --fin-card: #161618; --fin-edge: rgb(255 255 255 / 9%); --fin-shadow: none; --fin-btn-r: 6px; }
	.fin-ink::before { background: #0b0b0c; }
	.fin-ink .settings-content :global(.ui-settings), .fin-ink :global(.ui-btn), .fin-ink .nav-row, .fin-ink .fold-head { transition: none; }
	.fin-ink :global(.ui-btn.primary) { background: #f4f4f5; color: #0b0b0c; }
	.fin-ink :global(.ui-btn.primary:hover:not(:disabled)) { background: #ffffff; }
	.fin-ink :global(.ui-btn.secondary) { background: transparent; border-color: rgb(255 255 255 / 18%); }
	.fin-ink :global(.ui-btn:active:not(:disabled)) { transform: none; }
	.fin-ink :global(.input) { background: #0b0b0c; border-color: rgb(255 255 255 / 12%); }
	.fin-ink .nav-row[aria-current='page'] { background: #f4f4f5; color: #0b0b0c; }
	.fin-ink .nav-row[aria-current='page'] .glyph { color: #0b0b0c; }

	/* 3 Paper grain: a warm grey sheet with fine noise, cards one tone lighter, a 1px bevel on buttons. */
	.fin-grain { --fin-r: 10px; --fin-card: rgb(255 250 240 / 5%); --fin-edge: rgb(255 250 240 / 10%); --fin-shadow: inset 0 1px 0 rgb(255 250 240 / 6%); --fin-btn-r: 8px; }
	.fin-grain::before { background: linear-gradient(180deg, rgb(255 250 240 / 3%), transparent 40%), url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='120' height='120'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='1.1' numOctaves='2' stitchTiles='stitch'/></filter><rect width='120' height='120' filter='url(%23n)' opacity='0.07'/></svg>"); }
	.fin-grain :global(.ui-btn) { box-shadow: inset 0 1px 0 rgb(255 255 255 / 16%), inset 0 -1px 0 rgb(0 0 0 / 30%); }
	.fin-grain :global(.ui-btn.secondary) { background: rgb(255 250 240 / 9%); }
	.fin-grain :global(.input) { background: rgb(0 0 0 / 22%); box-shadow: inset 0 1px 2px rgb(0 0 0 / 35%); border-color: rgb(255 250 240 / 10%); }

	/* 4 Glass: frosted cards over a soft red wash; buttons translucent with a light edge. */
	.fin-glass { --fin-r: 16px; --fin-card: rgb(255 255 255 / 5%); --fin-edge: rgb(255 255 255 / 10%); --fin-shadow: inset 0 1px 0 rgb(255 255 255 / 12%), 0 20px 50px -30px rgb(0 0 0 / 70%); --fin-btn-r: 12px; }
	.fin-glass::before { background: radial-gradient(60% 50% at 10% 0%, color-mix(in srgb, var(--accent) 18%, transparent), transparent 70%), radial-gradient(50% 40% at 100% 100%, rgb(255 255 255 / 5%), transparent 70%); }
	.fin-glass .settings-content :global(:is(.ui-settings, .card)), .fin-glass .settings-nav { backdrop-filter: blur(16px) saturate(140%); }
	.fin-glass .settings-nav { padding: 10px; border-radius: 18px; background: rgb(255 255 255 / 5%); box-shadow: inset 0 1px 0 rgb(255 255 255 / 12%), 0 0 0 1px rgb(255 255 255 / 9%); }
	.fin-glass :global(.ui-btn.secondary) { background: rgb(255 255 255 / 9%); box-shadow: inset 0 1px 0 rgb(255 255 255 / 14%), 0 0 0 1px rgb(255 255 255 / 8%); }
	.fin-glass :global(.ui-btn.primary) { box-shadow: inset 0 1px 0 rgb(255 255 255 / 25%); }
	.fin-glass :global(.input) { background: rgb(255 255 255 / 6%); border-color: rgb(255 255 255 / 10%); }
	@media (prefers-reduced-transparency: reduce) { .fin-glass .settings-content :global(:is(.ui-settings, .card)), .fin-glass .settings-nav { backdrop-filter: none; background: var(--surface-1); } }

	/* 5 Outline: no fills anywhere, 1px lines do the work; the primary button is the only solid thing. */
	.fin-outline { --fin-r: 10px; --fin-card: transparent; --fin-edge: rgb(255 255 255 / 16%); --fin-shadow: none; --fin-btn-r: 8px; }
	.fin-outline :global(.ui-btn.secondary), .fin-outline :global(.ui-btn.plain) { background: transparent; border-color: rgb(255 255 255 / 20%); color: var(--text); }
	.fin-outline :global(.ui-btn.secondary:hover:not(:disabled)) { background: rgb(255 255 255 / 6%); }
	.fin-outline :global(.input) { background: transparent; border-color: rgb(255 255 255 / 20%); }
	.fin-outline .nav-row[aria-current='page'] { background: transparent; box-shadow: inset 0 0 0 1px var(--accent); }
	.fin-outline .settings-content :global(.ui-settings > header) { border-bottom: 1px solid var(--fin-edge); padding-bottom: 10px; }
	.fin-outline .nav-find input { background: transparent; box-shadow: inset 0 0 0 1px rgb(255 255 255 / 16%); }

	/* 6 Soft: big corners, lighter cards, pill buttons, things lift on hover. */
	.fin-soft { --fin-r: 20px; --fin-card: rgb(255 255 255 / 7%); --fin-edge: transparent; --fin-shadow: 0 10px 30px -18px rgb(0 0 0 / 60%); --fin-btn-r: 999px; }
	.fin-soft .settings-content :global(.ui-settings) { padding: 4px; }
	.fin-soft .settings-content :global(.ui-settings:hover) { box-shadow: 0 14px 36px -18px rgb(0 0 0 / 70%), 0 0 0 1px rgb(255 255 255 / 6%); }
	.fin-soft :global(.ui-btn:hover:not(:disabled)) { transform: translateY(-1px); box-shadow: 0 6px 16px -8px rgb(0 0 0 / 60%); }
	.fin-soft :global(.ui-btn:active:not(:disabled)) { transform: translateY(0) scale(0.98); box-shadow: none; }
	.fin-soft :global(.input) { border-radius: 14px; background: rgb(255 255 255 / 6%); border-color: transparent; }
	.fin-soft .nav-find input { border-radius: 999px; }
	.fin-soft .nav-row, .fin-soft .fold-head { border-radius: 999px; padding-inline: 12px; }

	/* 7 Carbon: a fine diagonal hatch behind, flat cards with a light top edge, squared buttons, mono values. */
	.fin-carbon { --fin-r: 6px; --fin-card: #151617; --fin-edge: rgb(255 255 255 / 7%); --fin-shadow: inset 0 1px 0 rgb(255 255 255 / 7%); --fin-btn-r: 4px; }
	.fin-carbon::before { background: repeating-linear-gradient(135deg, rgb(255 255 255 / 2.5%) 0 1px, transparent 1px 7px); }
	.fin-carbon .settings-content :global(.value), .fin-carbon .settings-content :global(.input) { font-family: var(--font-mono); font-size: 13px; }
	.fin-carbon :global(.ui-btn) { text-transform: uppercase; letter-spacing: 0.06em; font-size: 12.5px; font-weight: 600; }
	.fin-carbon :global(.ui-btn.secondary) { background: #1d1e20; border-color: rgb(255 255 255 / 10%); }
	.fin-carbon :global(.input) { background: #0f1011; border-color: rgb(255 255 255 / 10%); }
	.fin-carbon .fold-head .label { text-transform: uppercase; letter-spacing: 0.08em; font-size: 11px; }
	.fin-carbon .nav-row[aria-current='page'] { background: transparent; box-shadow: inset 2px 0 0 var(--accent); border-radius: 0; color: var(--text); }

	/* 8 Spotlight: a pool of light behind the content, cards fade from lighter top to darker base, buttons glow a little on hover. */
	.fin-spot { --fin-r: 14px; --fin-card: linear-gradient(180deg, rgb(255 255 255 / 8%), rgb(255 255 255 / 4%)); --fin-edge: rgb(255 255 255 / 9%); --fin-shadow: 0 1px 0 rgb(255 255 255 / 6%) inset, 0 24px 60px -36px rgb(0 0 0 / 80%); --fin-btn-r: 10px; }
	.fin-spot::before { background: radial-gradient(55% 45% at 60% 10%, rgb(255 255 255 / 7%), transparent 70%); }
	.fin-spot :global(.ui-btn.primary:hover:not(:disabled)) { box-shadow: 0 8px 24px -10px color-mix(in srgb, var(--accent) 70%, transparent); }
	.fin-spot :global(.ui-btn.secondary) { background: linear-gradient(180deg, rgb(255 255 255 / 12%), rgb(255 255 255 / 7%)); box-shadow: inset 0 1px 0 rgb(255 255 255 / 10%); }
	.fin-spot :global(.input) { background: rgb(0 0 0 / 25%); border-color: rgb(255 255 255 / 8%); }
	.fin-spot .nav-row[aria-current='page'] { background: linear-gradient(90deg, var(--accent-soft), transparent); }

	/* 9 Brushed: a faint vertical brush texture on cards, metallic two-tone buttons, a firm press. */
	.fin-brushed { --fin-r: 10px; --fin-card: #19191b; --fin-edge: rgb(255 255 255 / 8%); --fin-shadow: inset 0 1px 0 rgb(255 255 255 / 9%), 0 2px 4px rgb(0 0 0 / 40%); --fin-btn-r: 8px; }
	.fin-brushed .settings-content :global(:is(.ui-settings, .card)) { background-image: repeating-linear-gradient(90deg, rgb(255 255 255 / 1.6%) 0 1px, transparent 1px 3px); }
	.fin-brushed :global(.ui-btn.secondary) { background: linear-gradient(180deg, #2c2d30, #1f2022); border-color: rgb(255 255 255 / 10%); box-shadow: inset 0 1px 0 rgb(255 255 255 / 12%), 0 1px 2px rgb(0 0 0 / 50%); }
	.fin-brushed :global(.ui-btn.primary) { background: linear-gradient(180deg, color-mix(in srgb, var(--accent-fill) 85%, #fff), var(--accent-fill)); box-shadow: inset 0 1px 0 rgb(255 255 255 / 25%), 0 1px 2px rgb(0 0 0 / 50%); }
	.fin-brushed :global(.ui-btn:active:not(:disabled)) { transform: translateY(1px); box-shadow: inset 0 2px 4px rgb(0 0 0 / 40%); }
	.fin-brushed :global(.input) { background: #101012; box-shadow: inset 0 1px 3px rgb(0 0 0 / 50%); border-color: rgb(255 255 255 / 9%); }
	.fin-brushed .settings-nav { padding: 8px; border-radius: 12px; background: #141416; box-shadow: inset 0 1px 0 rgb(255 255 255 / 7%), 0 0 0 1px rgb(255 255 255 / 7%); }

	/* 10 Ember: a warm red glow low in the corner, deep cards, red-gradient primary, embers on hover. */
	.fin-ember { --fin-r: 14px; --fin-card: rgb(20 14 14 / 85%); --fin-edge: color-mix(in srgb, var(--accent) 18%, transparent); --fin-shadow: 0 20px 50px -30px rgb(0 0 0 / 80%); --fin-btn-r: 10px; }
	.fin-ember::before { background: radial-gradient(45% 40% at 0% 100%, color-mix(in srgb, var(--accent) 28%, transparent), transparent 70%), radial-gradient(40% 35% at 100% 0%, color-mix(in srgb, #ffb074 10%, transparent), transparent 70%); }
	.fin-ember :global(.ui-btn.primary) { background: linear-gradient(135deg, color-mix(in srgb, var(--accent-fill) 80%, #ffb074), var(--accent-fill)); }
	.fin-ember :global(.ui-btn.primary:hover:not(:disabled)) { box-shadow: 0 10px 28px -12px color-mix(in srgb, var(--accent) 80%, transparent); }
	.fin-ember :global(.ui-btn.secondary) { background: color-mix(in srgb, var(--accent) 10%, transparent); border-color: color-mix(in srgb, var(--accent) 22%, transparent); }
	.fin-ember :global(.input) { background: rgb(0 0 0 / 30%); border-color: color-mix(in srgb, var(--accent) 16%, transparent); }
	.fin-ember .nav-row[aria-current='page'] { background: color-mix(in srgb, var(--accent) 22%, transparent); color: #fff; }
	.fin-ember .nav-row[aria-current='page'] .glyph { color: #fff; }

	@media (prefers-reduced-motion: reduce) {
		.look-fold .settings-content :global(.ui-settings), .look-fold :global(.ui-btn) { transition: none; }
		.look-fold :global(.ui-btn:hover:not(:disabled)), .look-fold :global(.ui-btn:active:not(:disabled)) { transform: none; }
	}
	@media (max-width: 767px) { .look-fold::before { inset: -16px -16px -48px; border-radius: 0; } }

	/* ---------- the finish picker ---------- */
	.look-picker { position: fixed; right: 16px; bottom: 16px; z-index: 50; display: flex; align-items: center; gap: 8px; padding: 6px 8px 6px 12px; border-radius: var(--r-pill); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), 0 10px 30px -12px rgb(0 0 0 / 60%); backdrop-filter: blur(10px); font: var(--t-footnote); color: var(--text-muted); }
	.look-picker select { min-height: 30px; padding: 0 8px; border: 0; border-radius: 8px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; }

	/* ---------- phones: Settings home is the index; a page opens alone with a back link ---------- */
	@media (max-width: 1023px) {
		.settings-shell { grid-template-columns: minmax(0, 1fr); }
		.settings-nav { position: static; max-height: none; }
	}
	@media (max-width: 767px) {
		.settings-shell:not(.home) .settings-nav, .settings-shell.home .settings-workspace { display: none; }
		.back-home { display: inline-flex; }
		.phone-top, .phone-top:not(:has(.page-status)) { display: flex; margin-bottom: 4px; }
		.look-picker { right: 12px; bottom: calc(12px + var(--tabbar, 56px) + env(safe-area-inset-bottom)); }
	}
</style>
