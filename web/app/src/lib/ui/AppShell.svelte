<script lang="ts">
	import type { Snippet } from 'svelte';

	// Page frame: sidebar (tablet/desktop), sticky top bar, content, and the
	// phone tab bar. The content margin follows the sidebar width; on tablets
	// an expanded sidebar floats over the content with a scrim.
	let {
		collapsed,
		overlay = false,
		onscrim,
		sidebar,
		topbar,
		tabbar,
		children
	}: {
		/** Desktop sidebar is icons-only. */
		collapsed: boolean;
		/** Tablet sidebar is open over the content. */
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
	{#if overlay}<button class="scrim" type="button" tabindex="-1" aria-label="Close sidebar" onclick={onscrim}></button>{/if}
	<div class="content">
		{#if topbar}{@render topbar()}{/if}
		{@render children()}
	</div>
	{#if tabbar}{@render tabbar()}{/if}
</div>

<style>
	.ui-shell { --space: var(--sidebar-expanded); min-height: 100dvh; }
	.ui-shell.collapsed { --space: var(--sidebar-collapsed); }
	.content { min-width: 0; min-height: 100dvh; margin-left: var(--space); transition: margin-left var(--dur) var(--ease-out); }
	.collapsed .content { transition-delay: var(--dur-fast); }
	.scrim { position: fixed; inset: 0; z-index: 39; background: var(--scrim); border: 0; animation: fade var(--dur-fast) ease both; }
	@keyframes fade { from { opacity: 0; } }
	@media (min-width: 768px) and (max-width: 1023px) { .ui-shell { --space: var(--sidebar-collapsed); } }
	@media (max-width: 767px) { .content { margin-left: 0; } .scrim { display: none; } }
</style>
