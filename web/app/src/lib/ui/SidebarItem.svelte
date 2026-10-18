<script lang="ts">
	import Icon, { type IconName } from '../Icon.svelte';
	import { press } from '../motion.ts';

	// One sidebar row. Collapsed, the label fades out but stays the accessible
	// name, and a tooltip shows it on hover or focus.
	let {
		href,
		label,
		icon,
		current = false,
		collapsed = false,
		badge = 0,
		badgeLabel,
		kbd,
		onclick,
		testid
	}: {
		href?: string;
		label: string;
		icon: IconName;
		current?: boolean;
		collapsed?: boolean;
		badge?: number;
		badgeLabel?: string;
		kbd?: string;
		onclick?: (e: MouseEvent) => void;
		testid?: string;
	} = $props();

	let tip = $state<{ x: number; y: number } | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;
	function show(e: Event) {
		if (!collapsed) return;
		const el = e.currentTarget as HTMLElement;
		clearTimeout(timer);
		timer = setTimeout(() => {
			const r = el.getBoundingClientRect();
			tip = { x: r.right + 10, y: r.top + r.height / 2 };
		}, e.type === 'focus' ? 0 : 350);
	}
	function hide() {
		clearTimeout(timer);
		tip = null;
	}
	const name = $derived(badge > 0 ? `${label}, ${badgeLabel ?? badge}` : label);
</script>

{#snippet body()}
	<span class="ic"><Icon name={icon} size={19} /></span>
	<span class="label">{label}</span>
	{#if kbd}<kbd class="label" aria-hidden="true">{kbd}</kbd>{/if}
	{#if badge > 0}<b class="count" aria-hidden="true">{badge > 99 ? '99+' : badge}</b>{/if}
{/snippet}
{#if href}
	<a class="ui-sbi" class:current {href} aria-current={current ? 'page' : undefined} aria-label={collapsed || badge > 0 ? name : undefined} data-testid={testid} {onclick} onpointerenter={show} onpointerleave={hide} onfocus={show} onblur={hide} use:press={'row'}>{@render body()}</a>
{:else}
	<button class="ui-sbi" class:current type="button" aria-label={collapsed || badge > 0 ? name : undefined} data-testid={testid} {onclick} onpointerenter={show} onpointerleave={hide} onfocus={show} onblur={hide} use:press={'row'}>{@render body()}</button>
{/if}
{#if tip}<span class="tip" role="presentation" style="left: {tip.x}px; top: {tip.y}px">{label}</span>{/if}

<style>
	.ui-sbi {
		position: relative; z-index: 1; display: flex; align-items: center; gap: 10px; width: 100%; min-height: 36px;
		padding: 0 10px 0 12px; border-radius: 9px; color: var(--text); font: var(--t-callout); font-size: 15px; font-weight: 450; text-align: left;
		white-space: nowrap; overflow: hidden;
	}
	.ui-sbi:hover:not(.current) { background: color-mix(in srgb, var(--surface-3) 70%, transparent); }
	.ui-sbi:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.ic { flex: none; width: 24px; display: grid; place-items: center; color: var(--text-muted); transition: color var(--dur-fast); }
	.current { font-weight: 600; }
	.current .ic { color: var(--accent); }
	.label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; transition: opacity var(--dur-fast) ease var(--label-delay, 0ms); }
	kbd.label { flex: none; color: var(--text-muted); font: var(--t-footnote); }
	.count { min-width: 22px; height: 20px; padding: 0 7px; display: grid; place-items: center; flex: none; border-radius: 999px; background: var(--surface-3); color: var(--text); font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; transition: opacity var(--dur-fast); }
	.current .count { background: var(--surface-1); }
	:global(.ui-sidebar.collapsed) .ui-sbi .label { opacity: 0; }
	:global(.ui-sidebar.collapsed) .ui-sbi .count { position: absolute; top: 2px; left: 34px; min-width: 16px; height: 16px; padding: 0 4px; font-size: 10px; background: var(--accent-fill); color: var(--text-on-red); }
	.tip { position: fixed; z-index: 80; translate: 0 -50%; padding: 5px 9px; border-radius: 7px; background: var(--text); color: var(--bg); font: var(--t-footnote); white-space: nowrap; pointer-events: none; box-shadow: var(--shadow-lift); }
	@media (pointer: coarse) { .ui-sbi { min-height: 44px; } }
</style>
