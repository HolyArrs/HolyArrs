<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Collapsible Library section on a title page.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	let { note = t('ui.librarySection.note'), children }: { note?: string; children: Snippet } = $props();
	let open = $state(true);
</script>

<section class="tp-library" aria-labelledby="h-library" data-testid="title-library">
	<details bind:open>
		<summary>
			<span class="t">
				<h2 id="h-library" class="tp-h2">{t('ui.librarySection.library')}</h2>
				<span class="note">{note}</span>
			</span>
			<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={20} /></span>
		</summary>
		<div class="body">{@render children()}</div>
	</details>
</section>

<style>
	/* Offset keeps the heading clear of the sticky top bar when scrolled to. */
	.tp-library { border-top: 1px solid var(--line); padding-top: 8px; scroll-margin-top: calc(var(--topbar, 56px) + 16px); }
	summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 56px; cursor: pointer; list-style: none; border-radius: 8px; }
	/* Hide the default marker; the chevron replaces it. */
	summary::-webkit-details-marker { display: none; }
	summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.t { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 14px; min-width: 0; }
	.t :global(.tp-h2) { margin: 0; }
	.note { color: var(--text-muted); font: var(--t-subhead); }
	.chev { display: grid; color: var(--text-muted); transition: transform var(--dur-fast) var(--ease-out); }
	details[open] > summary .chev { transform: rotate(180deg); }
	.body { display: flex; flex-direction: column; gap: clamp(28px, 4vw, 44px); padding-top: 18px; min-width: 0; }
	@media (prefers-reduced-motion: reduce) { .chev { transition: none; } }
</style>
