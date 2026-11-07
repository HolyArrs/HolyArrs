<script lang="ts">
	// A link styled as a button, with variants and sizes.
	import type { Snippet } from 'svelte';
	import type { HTMLAnchorAttributes } from 'svelte/elements';
	import Icon, { type IconName } from './Icon.svelte';
	import { press } from '../core/motion.ts';

	type Props = Omit<HTMLAnchorAttributes, 'children'> & {
		href: string;
		variant?: 'primary' | 'secondary' | 'plain' | 'destructive';
		size?: 'lg' | 'md' | 'sm';
		icon?: IconName;
		children: Snippet;
	};
	let { href, variant = 'secondary', size = 'md', icon, children, class: cls = '', ...rest }: Props = $props();
	// Icon grows with the button size.
	const iconSize = $derived(size === 'lg' ? 20 : size === 'sm' ? 16 : 18);
</script>

<a class="ui-link-btn {variant} {size} {cls}" {href} use:press={'button'} {...rest}>
	{#if icon}<Icon name={icon} size={iconSize} />{/if}<span>{@render children()}</span>
</a>

<style>
	.ui-link-btn {
		display: inline-flex; align-items: center; justify-content: center; gap: 8px;
		border: 1px solid transparent; border-radius: var(--r-btn);
		font: var(--t-headline); font-size: 15px; letter-spacing: -0.01em; white-space: nowrap;
		cursor: pointer; user-select: none; text-decoration: none;
		transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	.ui-link-btn > span { overflow: hidden; text-overflow: ellipsis; }
	.lg { min-height: 44px; padding: 0 20px; font-size: 16px; }
	.md { min-height: 36px; padding: 0 16px; }
	.sm { min-height: 30px; padding: 0 12px; font: var(--t-subhead); font-weight: 600; border-radius: 8px; }
	.primary { background: var(--accent-fill); color: var(--text-on-red); }
	.primary:hover { background: var(--accent-fill-hover); }
	.secondary { background: var(--surface-2); color: var(--text); }
	.secondary:hover { background: var(--surface-3); }
	.plain { background: transparent; color: var(--accent-text); }
	.plain:hover { background: var(--accent-soft); }
	.destructive { background: color-mix(in srgb, var(--fail) 12%, transparent); color: var(--fail-text); }
	.ui-link-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Touch targets stay at least 44px tall. */
	@media (pointer: coarse) { .ui-link-btn { min-height: 44px; } }
	@media (max-width: 767px) { .sm { min-height: 36px; padding: 0 14px; font-size: 14px; } }
</style>
