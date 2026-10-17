<script lang="ts">
	import { to, appPath } from '#lib/paths.ts';
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import Icon from '#lib/Icon.svelte';
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
			label: 'Connections',
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

	function pickSection(event: Event) {
		const href = (event.currentTarget as HTMLSelectElement).value;
		if (href) void goto(to(href));
	}
</script>

<div class="settings-shell">
	<nav class="settings-nav" aria-label="Settings sections">
		<div class="nav-heading"><h1>Settings</h1></div>

		<a class="overview-link" href={to('/settings')} aria-current={current === '/settings' ? 'page' : undefined}>
			<Icon name="home" size={18} />
			<span>Home</span>
		</a>

		{#each groups as group (group.label)}
			{@const items = sections.filter((section) => group.hrefs.includes(section.href))}
			{#if items.length}
				<section class="nav-group" aria-labelledby="settings-nav-{group.label.toLowerCase()}">
					<h2 id="settings-nav-{group.label.toLowerCase()}">{group.label}</h2>
					<div class="nav-items">
						{#each items as section (section.href)}
							<a href={to(section.href)} aria-current={currentSection?.href === section.href ? 'page' : undefined}>
								<Icon name={section.icon} size={17} />
								<span>{section.label}</span>
							</a>
						{/each}
					</div>
				</section>
			{/if}
		{/each}
	</nav>

	<main class="settings-workspace">
		<div class="section-picker">
			<label for="settings-section">Settings section</label>
			<div class="select-wrap">
				<Icon name={currentSection?.icon ?? 'settings'} size={18} />
				<select id="settings-section" value={currentSection?.href ?? '/settings'} onchange={pickSection}>
					<option value="/settings">Settings overview</option>
					{#each sections as section (section.href)}
						<option value={section.href}>{section.label}</option>
					{/each}
				</select>
			</div>
		</div>
		<div class:overview-content={current === '/settings'} class="settings-content">{@render children()}</div>
	</main>
</div>

<style>
	.settings-shell {
		display: grid;
		grid-template-columns: 288px minmax(0, 1fr);
		gap: 10px;
		min-height: calc(100dvh - var(--topbar) - var(--s-8));
		margin: calc(var(--s-4) * -1);
		background: var(--surface-3);
	}
	.settings-nav,
	.settings-workspace {
		border: 1px solid var(--line);
		background: var(--surface-1);
	}
	.settings-nav {
		position: sticky;
		top: var(--topbar);
		align-self: start;
		height: calc(100dvh - var(--topbar) - var(--s-8));
		overflow-y: auto;
		padding: var(--s-5) var(--s-3) var(--s-8);
		border-radius: var(--r-panel);
		scrollbar-width: thin;
	}
	.nav-heading { padding: 0 var(--s-2) var(--s-4); }
	.nav-heading h1 {
		font-size: var(--text-xl);
		line-height: 1.2;
		letter-spacing: -0.025em;
	}
	.overview-link,
	.nav-items a {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 40px;
		padding: 0 var(--s-3);
		border: 1px solid transparent;
		border-radius: var(--r-btn);
		color: var(--text);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		transition: background var(--dur-fast), border-color var(--dur-fast), box-shadow var(--dur-fast);
	}
	.overview-link:hover,
	.nav-items a:hover { background: var(--surface-2); }
	.overview-link[aria-current='page'],
	.nav-items a[aria-current='page'] {
		border-color: var(--line);
		background: var(--surface-1);
		box-shadow: 0 1px 3px rgb(24 24 21 / 8%);
		font-weight: var(--weight-semibold);
	}
	.nav-group { margin-top: var(--s-4); }
	.nav-group h2 {
		padding: 0 var(--s-3) var(--s-1);
		font-size: var(--text-xs);
		font-weight: var(--weight-semibold);
		color: var(--text-muted);
		letter-spacing: 0.01em;
	}
	.nav-items {
		position: relative;
		padding-left: var(--s-2);
	}
	.nav-items::before {
		content: '';
		position: absolute;
		inset-block: 6px;
		left: 4px;
		width: 1px;
		background: var(--line);
	}
	.nav-items a { position: relative; z-index: 1; }
	.settings-workspace {
		min-width: 0;
		border-radius: var(--r-panel);
		overflow: hidden;
	}
	.settings-content {
		width: 100%;
		min-width: 0;
		padding: var(--s-6);
	}
	.settings-content.overview-content { padding: 0; }
	.section-picker {
		display: none;
		padding: var(--s-4) var(--s-5) 0;
	}
	.section-picker label {
		display: block;
		margin-bottom: var(--s-2);
		font-size: var(--text-xs);
		font-weight: var(--weight-semibold);
		color: var(--text-muted);
	}
	.select-wrap { position: relative; display: flex; align-items: center; }
	.select-wrap > :global(.icon) {
		position: absolute;
		left: var(--s-3);
		z-index: 1;
		color: var(--text-muted);
		pointer-events: none;
	}
	.select-wrap select {
		width: 100%;
		min-height: 44px;
		padding: 0 var(--s-4) 0 calc(var(--s-3) + 26px);
		border: 1px solid var(--line);
		border-radius: var(--r-btn);
		background: var(--surface-1);
		color: var(--text);
		font: inherit;
		font-weight: var(--weight-medium);
	}
	@media (max-width: 1000px) {
		.settings-shell {
			grid-template-columns: minmax(0, 1fr);
			margin: 0;
			min-height: 0;
			background: transparent;
		}
		.settings-nav { display: none; }
		.settings-workspace { border-radius: var(--r-panel); }
		.section-picker { display: block; }
	}
	@media (max-width: 600px) {
		.settings-workspace { border: 0; border-radius: 0; }
		.section-picker { padding: var(--s-3) var(--s-4) 0; }
		.settings-content { padding: var(--s-4); }
		.settings-content.overview-content { padding: 0; }
	}
</style>
