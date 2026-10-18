<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { afterNavigate } from '$app/navigation';
	import Icon from '#lib/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import { grouped, sectionFor, sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { settingsNow, refreshSettingsStatus } from '#lib/settings-status.svelte.ts';
	import { pageState, stateWords } from '#lib/settings-status.ts';

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = $derived(grouped(sections));

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
	<nav class="settings-nav" aria-label="Settings sections">
		<a class="nav-row top" href={to('/settings')} aria-current={current === '/settings' ? 'page' : undefined}>
			<span class="ic"><Icon name="home" size={16} /></span>
			<span>All settings</span>
		</a>

		{#each groups as { group, items } (group.id)}
			<section class="nav-group" aria-labelledby="settings-nav-{group.id}">
				<h2 id="settings-nav-{group.id}"><Icon name={group.icon} size={14} />{group.label}</h2>
				{#each items as section (section.href)}
					<a class="nav-row" href={to(section.href)} aria-current={currentSection?.href === section.href ? 'page' : undefined}>
						<span>{section.label}</span>
					</a>
				{/each}
			</section>
		{/each}
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

<style>
	.settings-shell {
		display: grid;
		grid-template-columns: 240px minmax(0, 1fr);
		gap: clamp(24px, 3vw, 48px);
		align-items: start;
	}
	.settings-nav {
		position: sticky;
		top: calc(var(--topbar) + 8px);
		max-height: calc(100dvh - var(--topbar) - 24px);
		overflow-y: auto;
		overscroll-behavior: contain;
		padding: 4px 4px 24px;
		scrollbar-width: thin;
	}
	.nav-group { margin-top: 16px; }
	.nav-group h2 {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 0 10px 4px;
		font: var(--t-footnote);
		font-weight: 600;
		color: var(--text-muted);
	}
	.nav-row {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 34px;
		padding: 0 10px 0 32px;
		border-radius: var(--r-btn);
		color: var(--text);
		font: var(--t-callout);
		font-weight: 500;
		transition: background var(--dur-fast) var(--ease-out);
	}
	.nav-row.top { padding-left: 6px; }
	.nav-row:hover { background: var(--surface-2); }
	.nav-row[aria-current='page'] { background: var(--surface-3); font-weight: 600; box-shadow: inset 3px 0 0 var(--accent); }
	.nav-row.top[aria-current='page'] { box-shadow: none; }
	.nav-row.top[aria-current='page'] .ic { background: var(--accent-fill); color: var(--text-on-red); }
	.ic {
		width: 26px;
		height: 26px;
		flex: none;
		display: grid;
		place-items: center;
		border-radius: 7px;
		background: var(--surface-2);
		color: var(--text-muted);
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
	@media (pointer: coarse) { .nav-row { min-height: 44px; } }
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
