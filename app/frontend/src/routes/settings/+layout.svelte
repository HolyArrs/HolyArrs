<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import '../(frame)/settings-content.css';
	import Icon from '#lib/ui/Icon.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { homeArt, startArt } from './(home)/art.svelte.ts';
	import { wall } from './(home)/pick.ts';
	import { arrive, coverPage, push, settingsNav, swap } from './(home)/zoom.ts';
	import { backdrop } from '#lib/accounts/index.ts';
	import { minimalLook } from '#lib/core/look.svelte.ts';
	import { partsNow, offPartName } from '#lib/mediafiles/index.ts';
	import { settingsNow, refreshSettingsStatus, pageState, stateWords } from '#lib/alerts/index.ts';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate, goto, onNavigate } from '$app/navigation';
	import { to, appPath } from '#lib/core/paths.ts';
	import { grouped, mayOpen, sectionFor, sections as all, visible } from '#lib/core/settings.ts';
	import { session } from '#lib/core/session.svelte.ts';

	// Settings: the home is five group cards over a still, blurred wall of this Library's posters; a page shows its group's name and a row of capsules for the group's pages. The top bar's field searches settings while you are here.

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = $derived(grouped(sections));
	const flagged = (href: string) => {
		const a = settingsNow.answer;
		if (!a) return false;
		return pageState(a, href).state === 'attention';
	};

	let { children }: { children: Snippet } = $props();
	const current = $derived(appPath(page.url.pathname));
	const home = $derived(current === '/settings');
	const currentSection = $derived(sectionFor(sections, current));
	// A direct link to a page this person may not open gets a note, not a bounce.
	const blocked = $derived.by(() => {
		const sec = sectionFor(all, current);
		return !!sec && !mayOpen(sec, session.can);
	});
	const currentGroup = $derived(groups.find((g) => g.group.id === currentSection?.group));
	const tiles = $derived(wall(homeArt.posters));
	let workspace = $state<HTMLElement>();
	let capsRow = $state<HTMLElement>();
	const status = $derived(settingsNow.answer && currentSection ? pageState(settingsNow.answer, currentSection.href) : null);
	// Which way the next screen moves: into a group (zoom or push), back to the home (the home plays it), or along a group's capsules (a quiet fade).
	onNavigate(({ from, to: next }) => {
		if (!from || !next) return;
		const a = appPath(from.url.pathname);
		const b = appPath(next.url.pathname);
		if (!b.startsWith('/settings')) return;
		const fromSec = sectionFor(all, a);
		const toSec = sectionFor(all, b);
		if (b === '/settings' && fromSec) {
			settingsNav.move = 'back';
			settingsNav.group = fromSec.group;
			// A blank sheet covers the page first, so the home mounts unseen and the sheet shrinks into its card.
			return coverPage(fromSec.group);
		} else if (a === '/settings' && toSec) {
			settingsNav.move = 'open';
			settingsNav.group = toSec.group;
		} else if (fromSec && toSec && fromSec.href !== toSec.href) {
			settingsNav.move = 'swap';
			settingsNav.group = toSec.group;
		}
	});
	function moved() {
		const m = settingsNav.move;
		if (home || !workspace) return;
		settingsNav.move = null;
		if (m === 'open') {
			arrive();
			push(workspace, false);
		} else if (m === 'swap') swap(workspace);
		// Keep the current capsule in view on narrow screens.
		const cur = capsRow?.querySelector<HTMLElement>('[aria-current="page"]');
		cur?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
	}
	// Escape on a page (nothing open, not typing) goes back to the Settings home.
	function escape(e: KeyboardEvent) {
		if (e.key !== 'Escape' || e.defaultPrevented || home) return;
		const target = e.target as HTMLElement | null;
		if (target?.closest('input, textarea, select, [contenteditable="true"], dialog, [role="menu"], [role="listbox"], [role="dialog"]')) return;
		if (document.querySelector('dialog[open], [role="dialog"], [role="menu"]')) return;
		e.preventDefault();
		void goto(to('/settings'));
	}

	// Risky actions (keys, passwords, roles, backups, single sign-on) want the password typed again. The server answers 403 "reauth"; instead of leaving that bare error on the page, ask here, then load the page again. A failed save keeps the page (and its edits) under the box.
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
		startArt(session.s.id, session.can('users'), { name: session.s.name ?? '', avatar: session.s.avatar ?? null });
		const onReauth = (e: Event) => {
			const detail = (e as CustomEvent<{ method?: string; path?: string }>).detail;
			const method = detail?.method ?? 'GET';
			session.s.localRecent = false;
			// My account asks inline, next to the control that needs it, and so does Jackett or Prowlarr, above its Connect button.
			if (currentSection?.group === 'account' || detail?.path === '/bridges') return;
			// Move from other apps shows its own set-a-password card in the step.
			if (!session.s.hasPassword && detail?.path?.startsWith('/migrate')) return;
			if (reauth !== 'load') reauth = method === 'GET' ? 'load' : 'save';
		};
		window.addEventListener('holyarrs:reauth', onReauth);
		return () => window.removeEventListener('holyarrs:reauth', onReauth);
	});

	// A search result links to page#field: scroll to that field, open its Advanced disclosure, focus it and flash it softly. Pages load their data after they appear, so look for the field for a few seconds.
	let seek = 0;
	function highlight(id: string) {
		const mine = ++seek;
		let tries = 0;
		let done: HTMLElement | null = null;
		const look = () => {
			if (mine !== seek) return;
			const el = document.getElementById(id);
			// Keep watching for a few seconds: a page that finishes loading can redraw the field (closed again, focus lost), so redo it.
			if (tries++ < 50) setTimeout(look, 100);
			if (!el || el === done) return;
			done = el;
			for (let d = el.closest('details'); d; d = d.parentElement?.closest('details') ?? null) if (!d.open) d.open = true;
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
	// A direct load or reload of page#field: this layout can mount after the first navigation has finished, so afterNavigate never sees it.
	onMount(() => {
		if (location.hash) highlight(decodeURIComponent(location.hash.slice(1)));
	});
	afterNavigate(() => {
		const id = decodeURIComponent(page.url.hash.slice(1));
		if (id) highlight(id);
		reauth = null;
		moved();
		if (home) void refreshSettingsStatus();
	});
</script>

<svelte:window onhashchange={() => location.hash && highlight(decodeURIComponent(location.hash.slice(1)))} onkeydown={escape} />

{#snippet pill()}
	{#if status && stateWords(status)}<span class="page-status" data-testid="page-status"><StatusPill kind={status.state === 'ok' ? 'in_library' : status.state === 'attention' ? 'failed' : 'missing'} label={stateWords(status)} /></span>{/if}
{/snippet}

<!-- Background: this Library's posters, blurred and dimmed, still. -->
{#if tiles.length && home && backdrop.value === 'posters' && !minimalLook()}
	<div class="sx-wall" aria-hidden="true" data-poster-wall>
		<div class="tiles">{#each tiles as src, i (i)}<img {src} alt="" decoding="async" />{/each}</div>
		<div class="veil"></div>
	</div>
{/if}

<div class="settings-shell look-fold" class:home>
	{#if !home}
		<div class="group-top">
			<!-- Settings home is for staff; a member on their own pages goes back Home instead of being bounced. -->
			{#if session.can('settings') || session.can('users')}<a class="back-home" href={to('/settings')} data-testid="settings-back"><Icon name="chevronLeft" size={17} />{t('settings.layout.settings')}</a>{:else}<a class="back-home" href={to('/')} data-testid="settings-back"><Icon name="chevronLeft" size={17} />{t('settings.layout.home')}</a>{/if}
			{@render pill()}
		</div>
		{#if currentGroup}
			<p class="group-name" data-testid="settings-group-name">{currentGroup.group.label}</p>
			{#if currentGroup.items.length > 1}
				<nav class="caps" aria-label={t('settings.layout.sections')} data-testid="settings-nav" bind:this={capsRow}>
					{#each currentGroup.items as section (section.href)}
						<a class="cap" href={to(section.href)} aria-current={currentSection?.href === section.href ? 'page' : undefined}>
							{section.label}
							{#if flagged(section.href)}<span class="flag" title={t('settings.layout.needsAttention')}><span class="sr-only">, {t('settings.layout.needsAttentionLower')}</span></span>{/if}
						</a>
					{/each}
				</nav>
			{/if}
		{/if}
	{/if}
	<div class="settings-workspace" bind:this={workspace}>
		<div class="settings-content">
			{#if blocked}
				<SettingsCard id="settings-blocked" title={t('settings.layout.blocked')} icon="shield">
					<div><Button href={to('/settings')}>{t('settings.layout.backToSettings')}</Button></div>
				</SettingsCard>
			{:else if reauth && !session.s.hasPassword}
				<SettingsCard id="settings-reauth" title={t('settings.layout.setPasswordFirst')} description={t('settings.layout.setPasswordDesc')} icon="shield">
					<div><Button variant="primary" href={to('/account')}>{t('settings.layout.setPasswordLabel')}</Button></div>
				</SettingsCard>
			{:else if reauth}
				<SettingsCard id="settings-reauth" title={t('settings.layout.confirmPasswordLabel')} description={t('settings.layout.confirmPasswordDesc')} icon="shield">
					<Confirm action="password">{#snippet children()}{/snippet}</Confirm>
				</SettingsCard>
			{/if}
			{#if !blocked && reauth !== 'load'}
				{#key pageKey}{@render children()}{/key}
			{/if}
		</div>
	</div>
</div>

<style>
	/* Settings reads in one font: paths, keys, addresses and field values use the app font with even-width digits (the log viewer keeps its fixed-width lines). */
	.settings-content :global(:is(.mono, input, textarea, select, dl.facts dd, code, kbd)),
	.settings-content :global(:is([class*='path'], [class*='key'], [class*='url'], [class*='addr'], .hint)) { font-family: var(--font-ui) !important; font-variant-numeric: tabular-nums; letter-spacing: normal; }
	/* The wall: this Library's posters, blurred, dimmed, still. */
	.sx-wall { position: fixed; inset: 0 0 0 var(--space, 0px); z-index: -1; overflow: hidden; pointer-events: none; transition: opacity 500ms var(--ease-out); }
	.sx-wall .tiles { position: absolute; inset: -60px; display: grid; grid-template-columns: repeat(8, 1fr); gap: 14px; transform: rotate(-8deg) scale(1.15); filter: blur(22px) saturate(1.15); }
	.sx-wall img { width: 100%; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 10px; display: block; }
	.sx-wall .veil { position: absolute; inset: 0; background: linear-gradient(180deg, color-mix(in srgb, var(--bg) 58%, transparent), color-mix(in srgb, var(--bg) 70%, transparent)), radial-gradient(130% 90% at 50% 10%, transparent, color-mix(in srgb, var(--bg) 70%, transparent)); }
	@media (max-width: 767px) { .sx-wall { left: 0; } .sx-wall .tiles { grid-template-columns: repeat(4, 1fr); } }

	.settings-shell { position: relative; isolation: isolate; }
	.settings-workspace { min-width: 0; max-width: 980px; }
	.settings-shell.home .settings-workspace { max-width: none; }
	.settings-content { width: 100%; min-width: 0; }
	.flag { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--fail); }
	.group-top { display: flex; align-items: center; gap: 12px; min-height: 36px; }
	.page-status { margin-left: auto; }
	.back-home { display: inline-flex; align-items: center; gap: 2px; min-height: 44px; margin-left: -6px; padding: 0 12px 0 4px; border-radius: var(--r-pill); color: var(--accent-text); font: var(--t-callout); font-weight: 500; transition: background var(--dur-fast) var(--ease-out); }
	.back-home:hover { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.back-home:focus-visible, .cap:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.group-name { margin: 2px 0 12px; font: 700 34px/1.12 var(--font-display, var(--font-ui)); letter-spacing: -0.022em; }

	/* The group's pages, one capsule each; the current one is filled. */
	.caps { display: flex; gap: 8px; overflow-x: auto; margin: 0 -4px 22px; padding: 4px; scrollbar-width: none; overscroll-behavior-x: contain; }
	.caps::-webkit-scrollbar { display: none; }
	.cap {
		position: relative; flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 0 14px; border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-2) 72%, transparent); box-shadow: inset 0 0 0 1px var(--line); color: var(--text-muted);
		font: 500 14px/1 var(--font-ui); white-space: nowrap; text-decoration: none;
		transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	.cap:hover { color: var(--text); background: color-mix(in srgb, var(--surface-3) 80%, transparent); }
	.cap[aria-current='page'] { background: var(--text); color: var(--bg); box-shadow: none; }
	@media (pointer: coarse) { .cap { min-height: 44px; } }

	@media (max-width: 767px) {
		.group-name { font-size: 28px; margin-bottom: 10px; }
		/* Phones: the sections wrap onto a second line; nothing sits past the screen edge. */
		.caps { margin-bottom: 16px; flex-wrap: wrap; overflow-x: visible; }
		.back-home { margin-left: -10px; }
	}

	/* Content: the card grid from settings-content.css, plus the "flash" for search hits. */
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
	@media (prefers-reduced-motion: reduce) {
		.settings-content :global(.flash-target) { animation: none; background: var(--accent-soft); box-shadow: 0 0 0 6px var(--accent-soft); }
		.sx-wall, .cap, .back-home { transition: none; }
	}

	/* Plain cards and buttons on the page. */
	.look-fold {
		--fin-r: 12px; --fin-card: var(--surface-1); --fin-edge: var(--line);
		--fin-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 8px 24px -14px rgb(0 0 0 / 35%); --fin-btn-r: var(--r-btn); }
	.look-fold :global(.ui-btn) { border-radius: var(--fin-btn-r); transition: background var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.look-fold :global(.ui-btn:active:not(:disabled)) { transform: scale(0.98); }
	.look-fold :global(.input) { border-radius: var(--fin-btn-r); }
	@media (prefers-reduced-motion: reduce) {
		.look-fold .settings-content :global(.ui-settings), .look-fold :global(.ui-btn) { transition: none; }
		.look-fold :global(.ui-btn:active:not(:disabled)) { transform: none; }
	}
	/* The section pages match the Status page. */
	/* One centred column, as wide as Status reads well. */
	.settings-shell:not(.home) { max-width: 1120px; margin-inline: auto; }
	.settings-shell:not(.home) .settings-workspace { max-width: none; }
	.group-name { margin: 2px 0 14px; font: var(--t-large-title); letter-spacing: var(--t-large-title-track, -0.022em); }
	/* Section tabs: the Status tab track (one pill, the open tab lifted in white). */
	.caps { display: inline-flex; flex-wrap: nowrap; gap: 2px; max-width: 100%; margin: 0 0 20px; padding: 2px; border-radius: var(--r-pill); background: var(--surface-2); overflow-x: auto; }
	.cap { min-height: 32px; height: 32px; padding: 0 16px; background: none; box-shadow: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-subhead); font-size: 14px; }
	.cap:hover { background: none; color: var(--text); }
	.cap[aria-current='page'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	@media (pointer: coarse) { .cap { min-height: 40px; height: 40px; } }
	@media (max-width: 767px) {
		.group-name { font-size: 30px; }
		.caps { display: flex; flex-wrap: nowrap; overflow-x: auto; margin-bottom: 16px; }
		.cap { padding: 0 14px; }
	}
	/* The open section's own title: a quiet bar with its buttons on the right (the tab already names it). */
	.settings-content :global(> .ui-head), .settings-content :global(.ui-head) { margin-bottom: 4px; }
	.settings-content :global(.ui-head h1) { font: 600 20px/1.25 var(--font-ui); letter-spacing: -0.01em; }
	.settings-content :global(.ui-head .row) { align-items: center; min-height: 40px; }
	/* Every section is the Library folders pattern: a small grey heading, then one glass sheet of rows. */
	.settings-content { grid-template-columns: minmax(0, 1fr) !important; gap: 22px !important; }
	.settings-content :global(section.ui-settings) { padding: 0; border: 0; border-radius: 0; background: none; -webkit-backdrop-filter: none; backdrop-filter: none; box-shadow: none; }
	.settings-content :global(section.ui-settings > header) { display: flex; align-items: baseline; flex-wrap: wrap; gap: 2px 10px; padding: 0 6px 8px; border: 0; }
	.settings-content :global(section.ui-settings > header > div) { display: contents; }
	.settings-content :global(section.ui-settings > header h2) { color: var(--text-muted); font: 600 13px/1.2 var(--font-ui); letter-spacing: 0.02em; }
	.settings-content :global(section.ui-settings > header p) { margin: 0; color: var(--text-muted); font: 400 12px/1.2 var(--font-ui); }
	/* The sheet. */
	.settings-content :global(:is(section.ui-settings > .body, ul.card, div.card)) {
		border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); border-radius: 20px;
		background: color-mix(in srgb, var(--surface-1) 60%, transparent);
		-webkit-backdrop-filter: blur(28px) saturate(1.6); backdrop-filter: blur(28px) saturate(1.6);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 24px 50px -32px rgb(0 0 0 / 0.45);
	}
	/* Rows run edge to edge like the Library folders rows: the sheet has no side padding, each row has 18px. */
	.settings-content :global(section.ui-settings > .body) { padding: 0; gap: 0 !important; }
	.settings-content :global(section.ui-settings > .body > :not(form, .ui-table)), .settings-content :global(section.ui-settings > .body > form > *) { padding-inline: 18px; }
	.settings-content :global(section.ui-settings > .body > form) { padding: 0; }
	.settings-content :global(:is(ul.card, div.card)) { padding: 4px 18px; }
	.settings-content :global(section.ui-settings.danger > .body) { border-color: color-mix(in srgb, var(--fail) 40%, transparent); }
	/* Rows inside a sheet: room to breathe, a hairline between. */
	.settings-content :global(section.ui-settings > .body > :is(.srow, .field, ul > li, .row)), .settings-content :global(section.ui-settings > .body > form:not(.ask) > :is(.srow, .field)) { padding-block: 14px; min-height: 64px; box-sizing: border-box; }
	.settings-content :global(section.ui-settings > .body > * + *), .settings-content :global(section.ui-settings > .body > form:not(.ask) > * + *) { box-shadow: inset 0 1px 0 var(--line); margin-top: 0 !important; }
	.settings-content :global(section.ui-settings > .body > :first-child:last-child:not(.srow, .field)) { padding-block: 14px; }
	.settings-content :global(section.ui-settings > .body > :is(ul, ol, p, .row)) { padding-block: 12px; }
	/* One label style for every row: switch, text box or dropdown. */
	.settings-content :global(section.ui-settings :is(.srow > .txt > :is(.label, label), .body > .field > :is(label:first-child, .label, legend), .body > form:not(.ask) > .field > :is(label:first-child, .label, legend), .adv-body > .field > :is(label:first-child, .label, legend), .body :not(.ask, .row-fields) > .field > label:first-child)) { font: 600 15px/1.3 var(--font-ui) !important; color: var(--text) !important; letter-spacing: -0.005em; }
	.settings-content :global(section.ui-settings .srow > .txt) { align-self: center; }
	/* A table inside a sheet uses the sheet's surface; no second box. */
	.settings-content :global(section.ui-settings > .body > .ui-table) { margin: 4px -18px; padding: 0 8px; border-radius: 0; background: none; box-shadow: none; }
	/* Groups of choices inside a sheet: the group name is a small sub-heading, never cut by a border. */
	.settings-content :global(section.ui-settings fieldset) { border: 0; margin: 0; padding: 0; min-width: 0; }
	.settings-content :global(section.ui-settings fieldset > legend) { float: left; width: 100%; padding: 16px 0 8px; color: var(--text-muted); font: 600 11px/1.2 var(--font-ui); letter-spacing: 0.08em; text-transform: uppercase; }
	.settings-content :global(section.ui-settings fieldset > legend :is(b, strong)) { font-weight: 600; color: var(--text-muted); }
	.settings-content :global(section.ui-settings fieldset > legend + *) { clear: both; }
	.settings-content :global(section.ui-settings fieldset label:not(.switch, .checkline) > input[type='checkbox']) { width: 18px; height: 18px; accent-color: var(--accent); }
	/* A section with only Advanced: Advanced becomes its sheet, so the heading never floats alone. */
	.settings-content :global(section.ui-settings:not(:has(> .body)) > .advanced) { margin: 0; padding: 0 18px; border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); border-radius: 20px; background: color-mix(in srgb, var(--surface-1) 60%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 24px 50px -32px rgb(0 0 0 / 0.45); }
	.settings-content :global(section.ui-settings:not(:has(> .body)) > .advanced > summary) { min-height: 52px; color: var(--text); font: 600 15px/1.3 var(--font-ui); }
	.settings-content :global(section.ui-settings:not(:has(> .body)) > .advanced > .adv-body) { border: 0; background: none; padding: 0 0 12px; }
	/* Small show-more lists inside a sheet look like Advanced: a quiet line with a chevron. */
	.settings-content :global(section.ui-settings > .body details:not(.advanced) > summary) { display: flex; align-items: center; gap: 6px; min-height: 40px; list-style: none; cursor: pointer; color: var(--text-muted); font: 500 13px/1 var(--font-ui); }
	.settings-content :global(section.ui-settings > .body details:not(.advanced) > summary::-webkit-details-marker) { display: none; }
	.settings-content :global(section.ui-settings > .body details:not(.advanced) > summary::before) { content: ''; width: 6px; height: 6px; border-right: 1.5px solid currentColor; border-bottom: 1.5px solid currentColor; transform: rotate(-45deg); margin: 0 4px 0 2px; transition: transform var(--dur-fast) var(--ease-out); }
	.settings-content :global(section.ui-settings > .body details[open]:not(.advanced) > summary::before) { transform: rotate(45deg); }
	/* Hand-built boxes: their own title reads like every section heading (small, grey), with the rows under a hairline. */
	.settings-content :global(:is(section.card, div.card) > :is(h2, .h2):first-child),
	.settings-content :global(:is(section.card, div.card) > .card-head:first-child :is(h2, .h2)) { color: var(--text-muted); font: 600 13px/1.2 var(--font-ui) !important; letter-spacing: 0.02em; }
	.settings-content :global(:is(section.card, div.card) > :is(h2, .h2, .card-head):first-child) { padding: 14px 0 12px; margin: 0; box-shadow: inset 0 -1px 0 var(--line); }
	/* Status words in Settings: a dot and a word, no pill (as on Library folders). */
	.settings-content :global(.chip) { background: none !important; box-shadow: none !important; padding-inline: 0 !important; font: 500 12px/1 var(--font-ui); color: var(--text-muted); }
	/* One field style everywhere in Settings: soft fill, thin edge, 12px corners, 40px tall. */
	.settings-content :global(:is(input.input:not([type='checkbox'], [type='radio']), select.input, textarea.input)) { min-height: 40px; border-radius: 12px; border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); background: color-mix(in srgb, var(--text) 5%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.04); font: 400 14.5px/1.3 var(--font-ui); color: var(--text); }
	.settings-content :global(:is(input.input, select.input, textarea.input):focus-visible) { outline: none; border-color: color-mix(in srgb, var(--accent) 55%, transparent); box-shadow: var(--focus-ring); }
	.settings-content :global(textarea.input) { padding-top: 10px; }
	/* A row with a tall text box keeps its label at the top, beside the box. */
	.settings-content :global(:is(.field, .srow):has(textarea)) { align-items: start !important; }
	.settings-content :global(:is(.field, .srow):has(textarea) > :is(label, .txt)) { padding-top: 10px; }
	/* Row words, the same sizes as the Library folders rows. */
	.settings-content :global(section.ui-settings :is(.srow > .txt > .hint, .field > .hint, label.switch .hint, .checkline .hint)) { font: 400 13px/1.35 var(--font-ui) !important; color: var(--text-muted); }
	/* A field row's label lines up with its box, not with box and hint. */
	.settings-content :global(section.ui-settings :is(.srow, .field):not(.ask > *, .row-fields > *):has(> .ctl :is(input, select), > :is(input, select), > .ctl > .input)) { align-items: start !important; }
	.settings-content :global(section.ui-settings :is(.srow, .field):not(.ask > *, .row-fields > *):has(> .ctl :is(input, select), > :is(input, select), > .ctl > .input) > :is(.txt, label:first-child)) { padding-top: 10px; }
	/* Long values (patterns, addresses, paths) get half the row so they are never cut off. */
	@media (min-width: 761px) {
		.settings-content :global(.field:not(.ask > *, .row-fields > *):is(:has(> input:is([type='text'], [type='url'], :not([type])):not(.narrow)), :has(> .row > input.input))) { grid-template-columns: minmax(0, 1fr) minmax(0, 50%) !important; }
		.settings-content :global(section.ui-settings .srow:has(> .ctl > input:is([type='text'], [type='url'], :not([type])):not(.narrow)) > .ctl) { width: 50%; max-width: none; flex: 0 0 50%; }
	}
	/* Inner panels (examples, notes) sit inside the row padding with their own soft fill. */
	.settings-content :global(section.ui-settings > .body > :is(.example, .notice, .preview)) { margin: 12px 18px; padding: 12px 14px; border-radius: 12px; box-shadow: none !important; }
	/* Advanced, when open, continues the same sheet instead of starting a second box. */
	.settings-content :global(section.ui-settings:has(> .advanced[open]) > .body) { border-bottom-left-radius: 0; border-bottom-right-radius: 0; border-bottom: 0; box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2); }
	.settings-content :global(section.ui-settings:has(> .body) > .advanced[open]) { margin: 0; padding: 0; border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); border-top: 0; border-radius: 0 0 20px 20px; background: color-mix(in srgb, var(--surface-1) 60%, transparent); -webkit-backdrop-filter: blur(28px) saturate(1.6); backdrop-filter: blur(28px) saturate(1.6); box-shadow: 0 24px 50px -32px rgb(0 0 0 / 0.45); }
	.settings-content :global(section.ui-settings:has(> .body) > .advanced[open] > summary) { padding-inline: 18px; box-shadow: inset 0 1px 0 var(--line); }
	.settings-content :global(section.ui-settings:has(> .body) > .advanced[open] > .adv-body) { border: 0; border-radius: 0; background: none; padding: 0 18px 8px; }
	/* Show-more lists line up with the row labels. */
	.settings-content :global(section.ui-settings > .body details:not(.advanced) > summary) { margin-left: -2px; }
	.settings-content :global(section.ui-settings > .body > .field details:not(.advanced)), .settings-content :global(section.ui-settings > .body > details:not(.advanced)) { padding-left: 18px; margin-left: 0; }
	/* Room under the last row so the save bar never covers it. */
	.settings-content:has(:global([data-testid='savebar'])) { padding-bottom: 96px; }
	@media (max-width: 767px) {
		.settings-content :global(section.ui-settings:has(> .body) > .advanced[open]) { border-radius: 0 0 16px 16px; }
		.settings-content :global(ul.fl) { border-radius: 16px !important; }
	}
	@media (pointer: coarse) { .settings-content :global(.ui-seg.sm button), .settings-content :global(.ui-seg button) { min-height: 40px !important; height: 40px !important; } }
	/* Advanced and footer sit under the sheet, quiet. */
	.settings-content :global(section.ui-settings > .advanced) { margin: 6px 6px 0; border: 0; }
	.settings-content :global(section.ui-settings > .advanced > summary) { padding-inline: 0; color: var(--text-muted); font: 500 13px/1 var(--font-ui); }
	.settings-content :global(section.ui-settings > .advanced > .adv-body) { padding: 4px 18px; border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); border-radius: 20px; background: color-mix(in srgb, var(--surface-1) 60%, transparent); }
	.settings-content :global(section.ui-settings > footer) { padding: 10px 6px 0; border: 0; background: none; }
	@media (max-width: 767px) {
		.settings-content :global(:is(section.ui-settings > .body, ul.card, div.card)) { border-radius: 16px; }
		.settings-content :global(:is(ul.card, div.card)) { padding: 2px 14px; }
		.settings-content :global(section.ui-settings > .body > :not(form, .ui-table)), .settings-content :global(section.ui-settings > .body > form > *) { padding-inline: 14px; }
	}
</style>
