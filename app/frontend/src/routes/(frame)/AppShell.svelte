<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { Snippet } from 'svelte';

	// Page frame: sidebar (tablet and desktop), sticky top bar, content, and the phone tab bar. The content margin follows the sidebar width; on tablets an expanded sidebar floats over the content with a scrim.
	let {
		collapsed,
		overlay = false,
		onscrim,
		sidebar,
		topbar,
		tabbar,
		children
	}: {
		/** The desktop sidebar shows icons only. */
		collapsed: boolean;
		/** The tablet sidebar is open over the content. */
		overlay?: boolean;
		onscrim?: () => void;
		sidebar: Snippet;
		topbar?: Snippet;
		tabbar?: Snippet;
		children: Snippet;
	} = $props();
</script>

<div class="ui-shell" class:collapsed class:overlay>
	{@render sidebar()}
	{#if overlay}<button class="scrim" type="button" tabindex="-1" aria-label={t('frame.shell.closeSidebar')} onclick={onscrim}></button>{/if}
	<!-- The phone tab bar comes before the content in tab order (it is fixed at the bottom on screen). -->
	{#if tabbar}{@render tabbar()}{/if}
	<div class="content">
		{#if topbar}{@render topbar()}{/if}
		{@render children()}
	</div>
</div>

<style>
	.ui-shell { --space: var(--sidebar-expanded); min-height: 100dvh; }
	.ui-shell.collapsed { --space: var(--sidebar-collapsed); }
	.content { min-width: 0; min-height: 100dvh; margin-inline-start: var(--space); }
	/* The margin eases only when the sidebar is folded or opened. Dropping to phone width (no sidebar) removes the margin at once, so the page is never wider than the screen while it settles. */
	@media (min-width: 768px) {
		.content { transition: margin-inline-start var(--dur) var(--ease-out); }
		.collapsed .content { transition-delay: var(--dur-fast); }
	}
	.scrim { position: fixed; inset: 0; z-index: 39; background: var(--scrim); border: 0; animation: fade var(--dur-fast) var(--ease-out) both; }
	@keyframes fade { from { opacity: 0; } }
	@media (min-width: 768px) and (max-width: 1023px) { .ui-shell { --space: var(--sidebar-collapsed); } }
	@media (max-width: 767px) { .content { margin-inline-start: 0; } .scrim { display: none; } }
</style>
