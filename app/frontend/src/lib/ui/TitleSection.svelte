<script lang="ts">
	// One titled block on the title page: a fold on phones, a plain section elsewhere.
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import Icon from './Icon.svelte';

	let {
		id,
		title,
		testid,
		primary = false,
		focusable = false,
		head,
		children
	}: { id: string; title: string; testid?: string; primary?: boolean; focusable?: boolean; head?: Snippet; children: Snippet } = $props();

	// Matches the layout breakpoint where sections turn into folds.
	const PHONE = '(max-width: 767px)';
	let phone = $state(typeof window !== 'undefined' && window.matchMedia(PHONE).matches);
	// Re-sync on mount because the first value is a guess before the page is in the browser.
	onMount(() => {
		const q = window.matchMedia(PHONE);
		const sync = () => (phone = q.matches);
		sync();
		q.addEventListener('change', sync);
		return () => q.removeEventListener('change', sync);
	});
</script>

{#if phone}
	<details class="tp-section ui-fold" data-testid={testid} open={primary}>
		<summary>
			<!-- svelte-ignore a11y_no_noninteractive_tabindex (only ever -1, for focus after a search) -->
			<h2 {id} class="tp-h2" tabindex={focusable ? -1 : undefined}>{title}</h2>
			<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
		</summary>
		{#if head}<div class="tp-head fold-head">{@render head()}</div>{/if}
		{@render children()}
	</details>
{:else}
	<section class="tp-section" aria-labelledby={id} data-testid={testid}>
		{#if head}
			<div class="tp-head">
				<!-- svelte-ignore a11y_no_noninteractive_tabindex (only ever -1, for focus after a search) -->
				<h2 {id} class="tp-h2" tabindex={focusable ? -1 : undefined}>{title}</h2>
				{@render head()}
			</div>
		{:else}
			<!-- svelte-ignore a11y_no_noninteractive_tabindex (only ever -1, for focus after a search) -->
			<h2 {id} class="tp-h2" tabindex={focusable ? -1 : undefined}>{title}</h2>
		{/if}
		{@render children()}
	</section>
{/if}

<style>
	.ui-fold { border-block: 1px solid var(--line); }
	summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 52px; cursor: pointer; list-style: none; border-radius: 8px; }
	summary::-webkit-details-marker { display: none; }
	summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	summary :global(.tp-h2) { margin: 0; font-size: 19px; }
	.chev { display: grid; color: var(--text-muted); transition: transform var(--dur-fast) var(--ease-out); }
	.ui-fold[open] > summary .chev { transform: rotate(180deg); }
	.ui-fold[open] { padding-bottom: 16px; }
	.fold-head { margin-top: 4px; }
	@media (prefers-reduced-motion: reduce) { .chev { transition: none; } }
</style>
