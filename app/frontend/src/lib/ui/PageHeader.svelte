<script lang="ts">
	// Page title with optional subtitle and actions, plus a small title bar that appears on phones when scrolled.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	let {
		title,
		subtitle,
		actions,
		size = 'large'
	}: { title: string; subtitle?: string; back?: { href: string; label: string }; actions?: Snippet; size?: 'large' | 'title1' } = $props();

	// Watches the big title; once it scrolls above the screen the mini bar shows.
	let head = $state<HTMLElement>();
	let shrunk = $state(false);
	$effect(() => {
		if (!head || typeof IntersectionObserver === 'undefined') return;
		const io = new IntersectionObserver(([e]) => (shrunk = !e.isIntersecting && e.boundingClientRect.top < 0), { threshold: 0 });
		io.observe(head);
		return () => io.disconnect();
	});
</script>

<div class="mini" class:show={shrunk} aria-hidden="true">{title}</div>
<header class="ui-head {size}">
	<div class="row">
		<div class="titles">
			<h1 bind:this={head}>{title}</h1>
			{#if subtitle}<p>{subtitle}</p>{/if}
		</div>
		{#if actions}<div class="actions">{@render actions()}</div>{/if}
	</div>
</header>

<style>
	.ui-head { margin-bottom: 24px; }
	.row { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 12px 16px; }
	.titles { min-width: 0; }
	h1 { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.title1 h1 { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
	p { margin-top: 4px; color: var(--text-muted); font: var(--t-body); max-width: 62ch; }
	.actions { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
	/* Phone only bar that fades in once the page title has scrolled away. */
	.mini { display: none; }
	@media (max-width: 767px) {
		.mini {
			display: block; position: fixed; z-index: 25; inset: 0 0 auto; height: calc(44px + env(safe-area-inset-top)); padding: env(safe-area-inset-top) 56px 0;
			line-height: 44px; text-align: center; font: var(--t-headline); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
			background: var(--sidebar-bg); backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px);
			color: var(--text); border-bottom: 1px solid var(--line); opacity: 0; visibility: hidden; translate: 0 -6px; pointer-events: none;
			transition: opacity var(--dur-fast) var(--ease-out), translate var(--dur-fast) var(--ease-out), visibility 0s linear var(--dur-fast);
		}
		.mini.show { opacity: 1; visibility: visible; translate: 0 0; transition-delay: 0s; }
		h1 { font-size: 30px; }
	}
</style>
