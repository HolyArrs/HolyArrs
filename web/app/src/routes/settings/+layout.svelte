<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate, goto, onNavigate } from '$app/navigation';
	import Icon from '#lib/Icon.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Confirm from '#lib/account/Confirm.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { grouped, mayOpen, sectionFor, sections as all, visible } from '#lib/settings.ts';
	import { homeArt, startArt } from '#lib/settings-home/art.svelte.ts';
	import { wall } from '#lib/settings-home/pick.ts';
	import { arrive, coverPage, push, settingsNav, swap } from '#lib/settings-home/zoom.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { settingsNow, refreshSettingsStatus } from '#lib/settings-status.svelte.ts';
	import { pageState, stateWords } from '#lib/settings-status.ts';
	import '#lib/ui/settings-looks.css';

	// Settings (Luis, 2026-11-06, design settings-views v4): the home is five
	// group cards over a still, blurred wall of this Library's posters; a page
	// shows its group's name and a row of capsules for the group's pages. The
	// top bar's field searches settings while you are here.

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
	// Which way the next screen moves: into a group (zoom or push), back to
	// the home (the home plays it), or along a group's capsules (a quiet fade).
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
		const t = e.target as HTMLElement | null;
		if (t?.closest('input, textarea, select, [contenteditable="true"], dialog, [role="menu"], [role="listbox"], [role="dialog"]')) return;
		if (document.querySelector('dialog[open], [role="dialog"], [role="menu"]')) return;
		e.preventDefault();
		void goto(to('/settings'));
	}

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
		startArt(session.s.id, session.can('users'), { name: session.s.name ?? '', avatar: session.s.avatar ?? null });
		const onReauth = (e: Event) => {
			const detail = (e as CustomEvent<{ method?: string; path?: string }>).detail;
			const method = detail?.method ?? 'GET';
			session.s.localRecent = false;
			// My account asks inline, next to the control that needs it, and so
			// does Jackett or Prowlarr, above its Connect button.
			if (currentSection?.group === 'account' || detail?.path === '/bridges') return;
			// Move from other apps shows its own set-a-password card in the step.
			if (!session.s.hasPassword && detail?.path?.startsWith('/migrate')) return;
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
		let done: HTMLElement | null = null;
		const look = () => {
			if (mine !== seek) return;
			const el = document.getElementById(id);
			// Keep watching for a few seconds: a page that finishes loading
			// can redraw the field (closed again, focus lost), so redo it.
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
	// A direct load or reload of page#field: this layout can mount after
	// the first navigation has finished, so afterNavigate never sees it.
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
{#if tiles.length && home}
	<div class="sx-wall" aria-hidden="true">
		<div class="tiles">{#each tiles as src, i (i)}<img {src} alt="" decoding="async" />{/each}</div>
		<div class="veil"></div>
	</div>
{/if}

<div class="settings-shell look-fold" class:home>
	{#if !home}
		<div class="group-top">
			<!-- Settings home is for staff; a member on their own pages goes back Home instead of being bounced. -->
			{#if session.can('settings') || session.can('users')}<a class="back-home" href={to('/settings')} data-testid="settings-back"><Icon name="chevronLeft" size={17} />Settings</a>{:else}<a class="back-home" href={to('/')} data-testid="settings-back"><Icon name="chevronLeft" size={17} />Home</a>{/if}
			{@render pill()}
		</div>
		{#if currentGroup}
			<p class="group-name" data-testid="settings-group-name">{currentGroup.group.label}</p>
			{#if currentGroup.items.length > 1}
				<nav class="caps" aria-label="Settings sections" data-testid="settings-nav" bind:this={capsRow}>
					{#each currentGroup.items as section (section.href)}
						<a class="cap" href={to(section.href)} aria-current={currentSection?.href === section.href ? 'page' : undefined}>
							{section.label}
							{#if flagged(section.href)}<span class="flag" title="Needs attention"><span class="sr-only">, needs attention</span></span>{/if}
						</a>
					{/each}
				</nav>
			{/if}
		{/if}
	{/if}
	<div class="settings-workspace" bind:this={workspace}>
		<div class="settings-content">
			{#if blocked}
				<SettingsCard id="settings-blocked" title="You can't change this" icon="shield">
					<div><Button href={to('/settings')}>Back to Settings</Button></div>
				</SettingsCard>
			{:else if reauth && !session.s.hasPassword}
				<SettingsCard id="settings-reauth" title="Set a password first" description="Keys, passwords, people and backups need a HolyArrs password. You don't have one yet." icon="shield">
					<div><Button variant="primary" href={to('/account')}>Set a password</Button></div>
				</SettingsCard>
			{:else if reauth}
				<SettingsCard id="settings-reauth" title="Confirm your password" description="Keys, passwords, people and backups need your password. Good for 10 minutes." icon="shield">
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
	/* Settings reads in one font: paths, keys, addresses and field values use the app font
	   with even-width digits (the log viewer keeps its fixed-width lines). */
	.settings-content :global(:is(.mono, input, textarea, select, dl.facts dd, code, kbd)),
	.settings-content :global(:is([class*='path'], [class*='key'], [class*='url'], [class*='addr'], .hint)) { font-family: var(--font-ui) !important; font-variant-numeric: tabular-nums; letter-spacing: normal; }
	.settings-content :global([data-testid='log-lines'] .text), .settings-content :global(pre) { font-family: var(--font-mono) !important; }
	/* ---------- the wall: this Library's posters, blurred, dimmed, still ---------- */
	.sx-wall { position: fixed; inset: 0 0 0 var(--space, 0px); z-index: -1; overflow: hidden; pointer-events: none; transition: opacity 500ms var(--ease-out); }
	.sx-wall.dim { opacity: 0.55; }
	.sx-wall .tiles { position: absolute; inset: -60px; display: grid; grid-template-columns: repeat(8, 1fr); gap: 14px; transform: rotate(-8deg) scale(1.15); filter: blur(22px) saturate(1.15); }
	.sx-wall img { width: 100%; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 10px; display: block; }
	.sx-wall .veil { position: absolute; inset: 0; background: linear-gradient(180deg, color-mix(in srgb, var(--bg) 58%, transparent), color-mix(in srgb, var(--bg) 70%, transparent)), radial-gradient(130% 90% at 50% 10%, transparent, color-mix(in srgb, var(--bg) 70%, transparent)); }
	@media (max-width: 767px) { .sx-wall { left: 0; } .sx-wall .tiles { grid-template-columns: repeat(4, 1fr); } }

	/* ---------- frame ---------- */
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
		flex: none; display: inline-flex; align-items: center; gap: 6px; min-height: 36px; padding: 0 14px; border-radius: var(--r-pill);
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
	@media (prefers-reduced-motion: reduce) {
		.settings-content :global(.flash-target) { animation: none; background: var(--accent-soft); box-shadow: 0 0 0 6px var(--accent-soft); }
		.sx-wall, .cap, .back-home { transition: none; }
	}

	/* Plain cards and buttons on the page (Luis, 2026-11-03). */
	.look-fold {
		--fin-r: 12px; --fin-card: var(--surface-1); --fin-edge: var(--line);
		--fin-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 8px 24px -14px rgb(0 0 0 / 35%); --fin-btn-r: var(--r-btn); }
	.look-fold .settings-content :global(.ui-settings) { border-radius: var(--fin-r); background: var(--fin-card); box-shadow: var(--fin-shadow), 0 0 0 1px var(--fin-edge); }
	.look-fold :global(.ui-btn) { border-radius: var(--fin-btn-r); transition: background var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.look-fold :global(.ui-btn:active:not(:disabled)) { transform: scale(0.98); }
	.look-fold :global(.input) { border-radius: var(--fin-btn-r); }
	@media (prefers-reduced-motion: reduce) {
		.look-fold .settings-content :global(.ui-settings), .look-fold :global(.ui-btn) { transition: none; }
		.look-fold :global(.ui-btn:active:not(:disabled)) { transform: none; }
	}
	/* ---------- Status look (Luis, 2026-11-06): the section pages match the Status page ---------- */
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
	/* Every section box is a Status plate: frosted sheet, red tick, hairline under the title. */
	.settings-content :global(:is(section.ui-settings, .card)) {
		border: 1px solid color-mix(in srgb, var(--text) 12%, transparent); border-radius: 20px;
		background: color-mix(in srgb, var(--surface-1) 55%, transparent);
		-webkit-backdrop-filter: blur(28px) saturate(1.6); backdrop-filter: blur(28px) saturate(1.6);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.18), 0 20px 50px -30px rgb(0 0 0 / 0.5);
	}
	.settings-content :global(section.ui-settings) { padding: 0; }
	.settings-content :global(section.ui-settings > header) { display: flex; align-items: center; gap: 10px; padding: 16px 20px 14px; border-bottom: 1px solid var(--line); }
	.settings-content :global(section.ui-settings > header::before) { content: ''; flex: none; width: 4px; height: 16px; border-radius: 2px; background: linear-gradient(var(--accent), color-mix(in srgb, var(--accent) 30%, transparent)); }
	.settings-content :global(section.ui-settings > header h2) { font: 600 17px/1.25 var(--font-ui); letter-spacing: -0.01em; }
	.settings-content :global(section.ui-settings > header p) { margin-top: 2px; font: 400 13px/1.35 var(--font-ui); color: var(--text-muted); }
	.settings-content :global(section.ui-settings > .body) { padding: 14px 20px 18px; }
	.settings-content :global(section.ui-settings > :is(.advanced, footer)) { margin: 0 20px; }
	.settings-content :global(section.ui-settings > footer) { padding-bottom: 16px; }
	.settings-content :global(ul.card), .settings-content :global(div.card) { padding: 6px 20px; }
	@media (max-width: 767px) {
		.settings-content :global(:is(section.ui-settings, .card)) { border-radius: 16px; }
		.settings-content :global(section.ui-settings > header) { padding: 14px 14px 12px; }
		.settings-content :global(section.ui-settings > .body) { padding: 12px 14px 14px; }
		.settings-content :global(ul.card), .settings-content :global(div.card) { padding: 4px 14px; }
	}
</style>
