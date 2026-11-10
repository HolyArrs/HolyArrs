<script lang="ts">
	// A glass menu trigger, round or pill, that opens a Menu of items.
	import Icon, { type IconName } from './Icon.svelte';
	import Menu, { type MenuItem } from './Menu.svelte';

	let {
		label,
		caption,
		icon = 'more',
		round,
		items,
		width = 260,
		align = 'start',
		open = $bindable(false)
	}: { label: string; caption?: string; icon?: IconName; round?: boolean; items: MenuItem[]; width?: number; align?: 'start' | 'end'; open?: boolean } = $props();
	// Plain more-dots menus are round, labelled ones are pills.
	const isRound = $derived(round ?? icon === 'more');
</script>

<span class="ui-actmenu" class:round={isRound}>
	<Menu {label} {items} {width} {align} bind:open>
		{#snippet trigger()}
			{#if isRound}
				<Icon name={icon} size={20} />
				<span class="tip" aria-hidden="true">{caption ?? label}</span>
			{:else}
				<Icon name={icon} size={18} />
				<span class="cap" aria-hidden="true">{caption ?? label}</span>
				<svg class="chev" width="12" height="12" viewBox="0 0 12 12" aria-hidden="true"><path d="M3 4.5 6 7.5 9 4.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
			{/if}
		{/snippet}
	</Menu>
</span>

<style>
	.ui-actmenu { display: inline-flex; }
	/* Restyle the menu trigger to match ActionButton. */
	.ui-actmenu :global(.ui-pop > .trigger) {
		position: relative; width: auto; height: 44px; min-width: 44px; padding: 0 16px 0 16px; gap: 8px; justify-content: center;
		border-radius: 999px; border: 1px solid rgb(255 255 255 / 22%); background: rgb(255 255 255 / 14%); color: #fff;
		backdrop-filter: blur(20px) saturate(170%); -webkit-backdrop-filter: blur(20px) saturate(170%);
		font: 600 15px/1 var(--font-ui); white-space: nowrap;
		transition: background var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), scale var(--dur) var(--ease-spring);
	}
	.round :global(.ui-pop > .trigger) { width: 44px; padding: 0; }
	.ui-actmenu :global(.ui-pop > .trigger:hover) { background: rgb(255 255 255 / 24%); border-color: rgb(255 255 255 / 34%); color: #fff; }
	.ui-actmenu :global(.ui-pop > .trigger[aria-expanded='true']) { background: rgb(255 255 255 / 92%); border-color: transparent; color: #111; }
	.ui-actmenu :global(.ui-pop > .trigger:active) { scale: .95; transition-duration: var(--dur-instant); }
	.ui-actmenu :global(.ui-pop > .trigger:focus-visible) { outline: none; box-shadow: 0 0 0 2px #111, 0 0 0 4px #fff; }
	.ui-actmenu :global(.panel) { color: var(--text); }
	.chev { margin-inline-start: -2px; opacity: .8; }
	.tip {
		position: absolute; left: 50%; top: calc(100% + 8px); z-index: 5; translate: -50% -4px; padding: 5px 9px; border-radius: 7px;
		background: rgb(20 20 22 / 92%); color: #fff; font: 600 12px/1.2 var(--font-ui); white-space: nowrap; pointer-events: none;
		opacity: 0; transition: opacity var(--dur-fast) var(--ease-out), translate var(--dur-fast) var(--ease-out);
	}
	.ui-actmenu :global(.ui-pop > .trigger:hover:not([aria-expanded='true']) .tip) { opacity: 1; translate: -50% 0; transition-delay: 500ms; }
	/* Touch has no hover, so the label would stick. */
	@media (hover: none) { .tip { display: none; } }
	/* Drop the caption on very narrow screens to keep the trigger small. */
	@media (max-width: 359px) { .cap { display: none; } .ui-actmenu :global(.ui-pop > .trigger) { padding: 0 12px; } }
	@media (prefers-reduced-motion: reduce) { .ui-actmenu :global(.ui-pop > .trigger:active) { scale: 1; } }
</style>
