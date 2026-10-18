<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import Icon from '#lib/Icon.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = [
		{
			label: 'Library',
			hrefs: ['/settings/library-folders', '/settings/naming', '/settings/search', '/settings/subtitles', '/settings/music', '/settings/wishlist', '/settings/schedules']
		},
		{
			label: 'Linked apps',
			hrefs: ['/settings/sources', '/settings/downloaders', '/settings/outside-lists', '/settings/media-servers', '/settings/tmdb', '/settings/alerts']
		},
		{
			label: 'Access',
			hrefs: ['/settings/users', '/settings/account', '/settings/sign-in', '/settings/api']
		},
		{
			label: 'System',
			hrefs: ['/settings/backups', '/settings/move', '/settings/parts', '/settings/this-app', '/settings/logs']
		}
	];

	let { children }: { children: Snippet } = $props();
	const current = $derived(appPath(page.url.pathname));
	const currentSection = $derived(sections.find((section) => section.href === current || current.startsWith(section.href + '/')));
	// Below the two-column width the section list is a Menu, so it stays one tap away on tablets and phones.
	const menuItems = $derived<MenuItem[]>([
		{ label: 'All settings', icon: 'home', href: to('/settings'), checked: current === '/settings' },
		...groups.flatMap((group) => {
			const items = sections.filter((section) => group.hrefs.includes(section.href));
			return items.length
				? [
						{ kind: 'heading' as const, label: group.label },
						...items.map((section) => ({ label: section.label, icon: section.icon, href: to(section.href), checked: currentSection?.href === section.href }))
					]
				: [];
		})
	]);
</script>

<div class="settings-shell">
	<nav class="settings-nav" aria-label="Settings sections">
		<a class="nav-row" href={to('/settings')} aria-current={current === '/settings' ? 'page' : undefined}>
			<span class="ic"><Icon name="home" size={16} /></span>
			<span>All settings</span>
		</a>

		{#each groups as group (group.label)}
			{@const items = sections.filter((section) => group.hrefs.includes(section.href))}
			{#if items.length}
				<section class="nav-group" aria-labelledby="settings-nav-{group.label.toLowerCase().replace(' ', '-')}">
					<h2 id="settings-nav-{group.label.toLowerCase().replace(' ', '-')}">{group.label}</h2>
					{#each items as section (section.href)}
						<a class="nav-row" href={to(section.href)} aria-current={currentSection?.href === section.href ? 'page' : undefined}>
							<span class="ic"><Icon name={section.icon} size={16} /></span>
							<span>{section.label}</span>
						</a>
					{/each}
				</section>
			{/if}
		{/each}
	</nav>

	<div class="settings-workspace">
		{#if current !== '/settings'}
			<div class="section-picker">
				<Menu label="Settings sections" variant="button" icon={currentSection?.icon ?? 'settings'} text={currentSection?.label ?? 'All settings'} align="start" width={280} items={menuItems} />
			</div>
		{/if}
		<div class="settings-content">{@render children()}</div>
	</div>
</div>

<style>
	.settings-shell {
		display: grid;
		grid-template-columns: 236px minmax(0, 1fr);
		gap: clamp(24px, 3vw, 48px);
		align-items: start;
	}
	.settings-nav {
		position: sticky;
		top: calc(var(--topbar) + 8px);
		max-height: calc(100dvh - var(--topbar) - 24px);
		overflow-y: auto;
		padding: 4px 4px 24px;
		scrollbar-width: thin;
	}
	.nav-group { margin-top: 18px; }
	.nav-group h2 {
		padding: 0 10px 6px;
		font: var(--t-footnote);
		font-weight: 600;
		color: var(--text-muted);
	}
	.nav-row {
		display: flex;
		align-items: center;
		gap: 10px;
		min-height: 36px;
		padding: 0 10px 0 6px;
		border-radius: var(--r-btn);
		color: var(--text);
		font: var(--t-callout);
		font-weight: 500;
		transition: background var(--dur-fast) var(--ease-out);
	}
	.nav-row:hover { background: var(--surface-2); }
	.nav-row[aria-current='page'] { background: var(--surface-3); font-weight: 600; }
	.nav-row[aria-current='page'] .ic { background: var(--accent-fill); color: var(--text-on-red); }
	.ic {
		width: 26px;
		height: 26px;
		flex: none;
		display: grid;
		place-items: center;
		border-radius: 7px;
		background: var(--surface-2);
		color: var(--text-muted);
		transition: background var(--dur-fast), color var(--dur-fast);
	}
	.settings-workspace { min-width: 0; max-width: 820px; }
	.settings-content { width: 100%; min-width: 0; }
	.section-picker { display: none; margin-bottom: 16px; }
	/* Section pages: every group is a SettingsCard (or a list card shaped like
	   one), stacked with one rhythm. */
	.settings-content > :global(:is(.ui-settings, .sc-form, .card, div[data-testid], ul.card) + :is(.ui-settings, .sc-form, .card, div[data-testid], ul, p, section)) { margin-top: 16px; }
	.settings-content :global(.card) { border: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); padding: 18px 20px; }
	.settings-content :global(.card .h2) { font: var(--t-headline); font-size: 17px; letter-spacing: normal; }
	.settings-content :global(.sc-form) { display: block; }
	.settings-content :global(.ui-settings .h2) { font: var(--t-headline); font-size: 16px; letter-spacing: normal; }
	@media (pointer: coarse) { .nav-row { min-height: 44px; } }
	@media (max-width: 1099px) {
		.settings-shell { grid-template-columns: minmax(0, 1fr); }
		.settings-nav { display: none; }
		.settings-workspace { max-width: none; }
		.section-picker { display: block; }
	}
</style>
