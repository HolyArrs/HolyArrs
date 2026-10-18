<script lang="ts">
	import Icon, { type IconName } from '../Icon.svelte';
	import Menu, { type MenuItem } from './Menu.svelte';

	// A Menu whose trigger looks like an ActionButton (round, word under it),
	// for the Library ▾ and ⋯ menus of a title hero.
	let {
		label,
		caption,
		icon = 'more',
		items,
		width = 260,
		align = 'start',
		open = $bindable(false)
	}: { label: string; caption?: string; icon?: IconName; items: MenuItem[]; width?: number; align?: 'start' | 'end'; open?: boolean } = $props();
</script>

<span class="ui-actmenu">
	<Menu {label} {items} {width} {align} bind:open>
		{#snippet trigger()}
			<span class="dot" aria-hidden="true"><Icon name={icon} size={22} /></span>
			<span class="cap" aria-hidden="true">{caption ?? label}</span>
		{/snippet}
	</Menu>
</span>

<style>
	.ui-actmenu { display: inline-flex; }
	.ui-actmenu :global(.ui-pop > .trigger) { width: auto; height: auto; min-width: 64px; max-width: 84px; padding: 2px; flex-direction: column; gap: 6px; border-radius: 14px; background: none; color: var(--on-art); }
	.ui-actmenu :global(.ui-pop > .trigger:hover), .ui-actmenu :global(.ui-pop > .trigger[aria-expanded='true']) { background: none; color: var(--on-art); }
	.ui-actmenu :global(.ui-pop > .trigger:focus-visible) { box-shadow: none; }
	.ui-actmenu :global(.ui-pop > .trigger:focus-visible .dot) { box-shadow: 0 0 0 2px var(--on-art-ink), 0 0 0 4px var(--on-art); }
	.ui-actmenu :global(.ui-pop > .trigger[aria-expanded='true'] .dot) { background: var(--on-art); color: var(--on-art-ink); }
	.ui-actmenu :global(.panel) { color: var(--text); }
	.dot { width: 52px; height: 52px; display: grid; place-items: center; border-radius: 50%; background: var(--on-art-fill); box-shadow: inset 0 0 0 1px var(--on-art-ring); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); transition: background var(--dur-fast) var(--ease-out); }
	.ui-actmenu :global(.ui-pop > .trigger:hover .dot) { background: var(--on-art-fill-hover); }
	.cap { font: var(--t-footnote); font-size: 12px; font-weight: 600; line-height: 1.2; text-align: center; color: var(--on-art-soft); }
	@media (max-width: 400px) { .ui-actmenu :global(.ui-pop > .trigger) { min-width: 56px; } .dot { width: 48px; height: 48px; } }
</style>
