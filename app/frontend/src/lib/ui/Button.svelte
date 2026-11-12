<script lang="ts">
	// Button or link with variants, sizes, icons and a busy spinner.
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Icon, { type IconName } from './Icon.svelte';
	import { press } from '../core/motion.ts';

	type Props = Omit<HTMLButtonAttributes, 'children'> & {
		variant?: 'primary' | 'secondary' | 'plain' | 'destructive';
		size?: 'lg' | 'md' | 'sm';
		href?: string;
		icon?: IconName;
		iconAfter?: IconName;
		busy?: boolean;
		full?: boolean;
		children: Snippet;
	};
	let { variant = 'secondary', size = 'md', href, icon, iconAfter, busy = false, full = false, type = 'button', disabled, children, class: cls = '', ...rest }: Props = $props();
	const iconSize = $derived(size === 'lg' ? 20 : size === 'sm' ? 16 : 18);
	// Busy can come from the prop or a passed aria-busy.
	const working = $derived(busy || rest['aria-busy'] === true || rest['aria-busy'] === 'true');
</script>

{#if href && !disabled}
	<a class="ui-btn {variant} {size} {cls}" class:full {href} use:press={'button'} aria-busy={busy || undefined}>
		{#if icon}<Icon name={icon} size={iconSize} />{/if}<span>{@render children()}</span>{#if iconAfter}<Icon name={iconAfter} size={iconSize} />{/if}
	</a>
{:else}
	<button class="ui-btn {variant} {size} {cls}" class:full {type} use:press={'button'} {...rest} disabled={disabled || working} aria-busy={working || undefined}>
		{#if working}<span class="spin" aria-hidden="true"></span>{:else if icon}<Icon name={icon} size={iconSize} />{/if}<span>{@render children()}</span>{#if iconAfter}<Icon name={iconAfter} size={iconSize} />{/if}
	</button>
{/if}

<style>
	.ui-btn {
		display: inline-flex; align-items: center; justify-content: center; gap: 8px;
		border: 1px solid transparent; border-radius: var(--r-btn);
		font: var(--t-headline); font-size: 15px; letter-spacing: -0.01em; white-space: nowrap;
		cursor: pointer; user-select: none; text-decoration: none;
	}
	.ui-btn > span { overflow: hidden; text-overflow: ellipsis; }
	/* Long labels wrap on phones instead of overflowing. */
	@media (max-width: 767px) { .ui-btn { max-width: 100%; white-space: normal; text-align: center; padding-block: 6px; } }
	.lg { min-height: 44px; padding: 0 20px; font-size: 16px; }
	.md { min-height: 36px; padding: 0 16px; }
	.sm { min-height: 30px; padding: 0 12px; font: var(--t-subhead); font-weight: 600; border-radius: 8px; }
	.full { width: 100%; }
	.primary { background: var(--accent-fill); color: var(--text-on-red); }
	.primary:hover:not(:disabled) { background: var(--accent-fill-hover); }
	.secondary { background: var(--surface-2); color: var(--text); }
	.secondary:hover:not(:disabled) { background: var(--surface-3); }
	.plain { background: transparent; color: var(--accent-text); }
	.plain:hover:not(:disabled) { background: var(--surface-2); }
	.destructive { background: color-mix(in srgb, var(--fail) 12%, transparent); color: var(--fail-text); }
	.destructive:hover:not(:disabled) { background: color-mix(in srgb, var(--fail) 18%, transparent); }
	.ui-btn:disabled { opacity: var(--disabled-opacity); cursor: not-allowed; }
	.ui-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.spin { width: 16px; height: 16px; border-radius: 50%; border: 2px solid currentColor; border-right-color: transparent; animation: spin .8s linear infinite; }
	@keyframes spin { to { transform: rotate(1turn); } }
	@media (max-width: 767px) { .sm { min-height: 36px; padding: 0 14px; font-size: 14px; } }
	/* 44px is the minimum comfortable touch target. */
	@media (pointer: coarse) { .ui-btn { min-height: 44px; } }
</style>
