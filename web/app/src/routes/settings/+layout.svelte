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

	let { children }: { children: Snippet } = $props();
	const atIndex = $derived(appPath(page.url.pathname) === '/settings');
	const current = $derived(appPath(page.url.pathname));
	const currentSection = $derived(sections.find((section) => section.href === current || current.startsWith(section.href + '/')));

	function pickSection(event: Event) {
		const href = (event.currentTarget as HTMLSelectElement).value;
		if (href) void goto(to(href));
	}
</script>

<div class="settings" class:at-index={atIndex}>
	<nav class="subnav" aria-label="Settings sections">
		<p class="label xs muted">Settings</p>
		{#each sections as s (s.href)}
			<a href={to(s.href)} aria-current={currentSection?.href === s.href ? 'page' : undefined}>
				<Icon name={s.icon} size={18} />{s.label}
			</a>
		{/each}
	</nav>
	<div class="body">
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
		{@render children()}
	</div>
</div>

<style>
	.settings {
		display: grid;
		grid-template-columns: 208px minmax(0, 1fr);
		gap: clamp(var(--s-6), 4vw, var(--s-10));
		align-items: start;
	}
	.subnav {
		position: sticky;
		top: calc(var(--topbar) + var(--s-6));
		display: flex;
		flex-direction: column;
		gap: 1px;
		max-height: calc(100dvh - var(--topbar) - var(--s-10));
		overflow-y: auto;
		padding-right: var(--s-2);
		scrollbar-width: thin;
	}
	.label {
		text-transform: uppercase;
		letter-spacing: 0.06em;
		padding: 0 var(--s-3) var(--s-2);
	}
	.subnav a {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 36px;
		padding: 0 var(--s-3);
		border-radius: var(--r-btn);
		color: var(--text-muted);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		transition:
			background var(--dur-fast),
			color var(--dur-fast);
	}
	.subnav a:hover {
		background: var(--surface-3);
		color: var(--text);
	}
	.subnav a[aria-current='page'] {
		background: var(--ha-red-soft);
		color: var(--text);
		box-shadow: inset 2px 0 0 var(--ha-red);
	}
	.subnav a[aria-current='page'] :global(.icon) {
		color: var(--ha-red);
	}
	.body {
		width: 100%;
		max-width: 960px;
		min-width: 0;
	}
	.section-picker {
		display: none;
		margin-bottom: var(--s-5);
	}
	.section-picker label {
		display: block;
		margin-bottom: var(--s-2);
		font-size: var(--text-xs);
		font-weight: var(--weight-semibold);
		color: var(--text-muted);
	}
	.select-wrap {
		position: relative;
		display: flex;
		align-items: center;
	}
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
	@media (max-width: 900px) {
		.settings {
			grid-template-columns: minmax(0, 1fr);
		}
		.subnav {
			display: none;
		}
		.section-picker {
			display: block;
		}
	}
</style>
