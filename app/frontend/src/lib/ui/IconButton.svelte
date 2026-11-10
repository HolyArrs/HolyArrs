<script lang="ts">
	// Round icon button that renders as a link when given an href.
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Icon, { type IconName } from './Icon.svelte';
	import { press } from '../core/motion.ts';

	type Props = Omit<HTMLButtonAttributes, 'children'> & {
		icon: IconName;
		label: string;
		variant?: 'plain' | 'tinted' | 'filled' | 'glass';
		size?: 'sm' | 'md' | 'lg';
		href?: string;
		badge?: number;
		tooltip?: boolean;
		tipSide?: 'below' | 'below-end' | 'above' | 'right';
		node?: HTMLElement;
	};
	let { icon, label, variant = 'plain', size = 'md', href, badge = 0, tooltip = true, tipSide = 'below', type = 'button', class: cls = '', node = $bindable(), ...rest }: Props = $props();
	// Glyph size follows the button size.
	const px = $derived(size === 'lg' ? 22 : size === 'sm' ? 16 : 19);
</script>

{#if href}
	<a bind:this={node} class="ui-ibtn {variant} {size} tip-{tipSide} {cls}" {href} aria-label={label} data-tip={tooltip ? label : undefined} use:press={'round'} {...rest as Record<string, unknown>}>
		<Icon name={icon} size={px} />
		{#if badge > 0}<b class="count" aria-hidden="true">{badge > 99 ? '99+' : badge}</b>{/if}
	</a>
{:else}
	<button bind:this={node} class="ui-ibtn {variant} {size} tip-{tipSide} {cls}" {type} aria-label={label} data-tip={tooltip ? label : undefined} use:press={'round'} {...rest}>
		<Icon name={icon} size={px} />
		{#if badge > 0}<b class="count" aria-hidden="true">{badge > 99 ? '99+' : badge}</b>{/if}
	</button>
{/if}

<style>
	.ui-ibtn { position: relative; display: inline-grid; place-items: center; flex: none; border-radius: 50%; border: 0; color: var(--text-muted); background: transparent; cursor: pointer; }
	.sm { width: 30px; height: 30px; }
	.md { width: 36px; height: 36px; }
	.lg { width: 44px; height: 44px; }
	.plain:hover { background: var(--surface-3); color: var(--text); }
	.tinted { background: var(--surface-2); color: var(--text); }
	.tinted:hover { background: var(--surface-3); }
	.filled { background: var(--accent-fill); color: var(--text-on-red); }
	.filled:hover { background: var(--accent-fill-hover); }
	/* Blur keeps the button readable over artwork. */
	.glass { background: var(--on-art-fill); color: var(--on-art); backdrop-filter: blur(16px) saturate(160%); -webkit-backdrop-filter: blur(16px) saturate(160%); box-shadow: inset 0 0 0 1px var(--on-art-ring); }
	.glass:hover { background: var(--on-art-fill-hover); }
	.ui-ibtn:disabled { opacity: .45; cursor: not-allowed; }
	.ui-ibtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ui-ibtn[aria-pressed='true'], .ui-ibtn[aria-expanded='true'] { background: var(--surface-3); color: var(--text); }
	.count { position: absolute; top: -3px; right: -4px; min-width: 18px; height: 18px; padding: 0 5px; display: grid; place-items: center; border-radius: 999px; background: var(--accent-fill); color: var(--text-on-red); font: 600 11px/1 var(--font-ui); box-shadow: 0 0 0 2px var(--bg); }
	/* Tooltip is drawn in CSS so it needs no extra element. */
	[data-tip]::after {
		content: attr(data-tip); position: absolute; z-index: 60; left: 50%; top: calc(100% + 6px);
		translate: -50% -2px; padding: 4px 8px; border-radius: 6px; white-space: nowrap; pointer-events: none;
		background: var(--text); color: var(--bg); font: var(--t-footnote); opacity: 0;
		transition: opacity var(--dur-fast) var(--ease-out) 0ms, translate var(--dur-fast) var(--ease-out) 0ms;
	}
	.tip-above[data-tip]::after { top: auto; bottom: calc(100% + 6px); }
	.tip-below-end[data-tip]::after { left: auto; right: 0; translate: 0 -2px; }
	:global([dir='rtl']) .tip-below-end[data-tip]::after { right: auto; left: 0; }
	.tip-below-end[data-tip]:hover::after, .tip-below-end[data-tip]:focus-visible::after { translate: 0 0; }
	.tip-right[data-tip]::after { left: calc(100% + 8px); top: 50%; translate: -2px -50%; }
	[data-tip]:hover::after, [data-tip]:focus-visible::after { opacity: 1; translate: -50% 0; transition-delay: 500ms; }
	.tip-right[data-tip]:hover::after, .tip-right[data-tip]:focus-visible::after { translate: 0 -50%; }
	/* Touch screens have no hover, so skip tooltips. */
	@media (hover: none) { [data-tip]::after { display: none; } }
	/* Minimum touch target size. */
	@media (pointer: coarse) { .sm, .md { width: 44px; height: 44px; } }
</style>
