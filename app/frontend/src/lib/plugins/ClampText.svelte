<script lang="ts">
	// Long text cut to a few lines with More / Less (the fit-to-screen layout in app.css): a window keeps its size until the person asks for the rest.
	import type { Snippet } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { lines = 4, children }: { lines?: number; children: Snippet } = $props();
	let open = $state(false);
	let box = $state<HTMLElement>();
	let cut = $state(false);
	$effect(() => {
		if (!box) return;
		const check = () => (cut = !open && box!.scrollHeight > box!.clientHeight + 1);
		check();
		const ro = new ResizeObserver(check);
		ro.observe(box);
		return () => ro.disconnect();
	});
</script>

<div class="clamp" class:fit-clamp={!open} style:--fit-lines={lines} bind:this={box}>{@render children()}</div>
{#if cut || open}<button type="button" class="more xs" aria-expanded={open} onclick={() => (open = !open)}>{open ? t('discover.people.less') : t('discover.people.more')}</button>{/if}

<style>
	.clamp { min-width: 0; }
	.more { justify-self: start; align-self: start; padding: 0; border: 0; background: none; color: var(--accent); font-weight: 600; cursor: pointer; }
	.more:focus-visible { outline: 2px solid var(--focus, var(--accent)); outline-offset: 2px; border-radius: 4px; }
</style>
