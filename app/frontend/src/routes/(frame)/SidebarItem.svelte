<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import { press } from '#lib/core/motion.ts';

	// One sidebar row. Collapsed, the label fades out but stays the accessible name, and a tooltip shows it on hover or focus.
	let {
		href,
		label,
		icon,
		current = false,
		collapsed = false,
		badge = 0,
		badgeLabel,
		ring,
		dot = false,
		kbd,
		onclick,
		testid,
		badgeTestid
	}: {
		href?: string;
		label: string;
		icon: IconName;
		current?: boolean;
		collapsed?: boolean;
		badge?: number;
		badgeLabel?: string;
		/** Overall progress 0 to 100: a ring around the icon. Decorative; the name carries the words. */
		ring?: number;
		/** A small flag that something here needs attention. */
		dot?: boolean;
		kbd?: string;
		onclick?: (e: MouseEvent) => void;
		testid?: string;
		/** Test id on the count bubble itself. */
		badgeTestid?: string;
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
	const name = $derived((badge > 0 ? `${label}, ${badgeLabel ?? badge}` : label) + (dot ? t('frame.sidebar.needsAttentionComma') : ''));
</script>

{#snippet body()}
	<span class="ic">
		{#if ring !== undefined}<svg class="ring" viewBox="0 0 28 28" aria-hidden="true" data-testid="nav-ring"><circle class="track" cx="14" cy="14" r="12" /><circle class="arc" cx="14" cy="14" r="12" pathLength="100" stroke-dasharray="{Math.min(Math.max(ring, 0), 100)} 100" /></svg>{/if}
		<Icon name={icon} size={19} />
	</span>
	<span class="label">{label}</span>
	{#if kbd}<kbd class="label" aria-hidden="true">{kbd}</kbd>{/if}
	{#if dot && !(badge > 0)}<span class="flag" title={t('frame.sidebar.needsAttention')} aria-hidden="true" data-testid="nav-flag"></span>{/if}
	{#if badge > 0}<b class="count" aria-hidden="true" data-testid={badgeTestid}>{badge > 99 ? '99+' : badge}</b>{/if}
{/snippet}
{#if href}
	<a class="ui-sbi" class:current {href} aria-current={current ? 'page' : undefined} aria-label={collapsed || badge > 0 || dot ? name : undefined} data-testid={testid} {onclick} onpointerenter={show} onpointerleave={hide} onfocus={show} onblur={hide} use:press={'row'}>{@render body()}</a>
{:else}
	<button class="ui-sbi" class:current type="button" aria-label={collapsed || badge > 0 || dot ? name : undefined} data-testid={testid} {onclick} onpointerenter={show} onpointerleave={hide} onfocus={show} onblur={hide} use:press={'row'}>{@render body()}</button>
{/if}
{#if tip}<span class="tip" role="presentation" style="left: {tip.x}px; top: {tip.y}px">{label}</span>{/if}

<style>
	.ui-sbi {
		position: relative; z-index: 1; display: flex; align-items: center; gap: 10px; width: 100%; min-height: 36px;
		padding: 0 10px 0 12px; border-radius: 9px; color: var(--text); font: var(--t-callout); font-size: 15px; font-weight: 450; text-align: start;
		white-space: nowrap; overflow: hidden;
	}
	.ui-sbi:hover:not(.current) { background: color-mix(in srgb, var(--surface-3) 70%, transparent); }
	.ui-sbi:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.ic { position: relative; flex: none; width: 24px; display: grid; place-items: center; color: var(--text-muted); transition: color var(--dur-fast) var(--ease-out); }
	.ring { position: absolute; inset: -2px; width: 28px; height: 28px; transform: rotate(-90deg); fill: none; stroke-width: 2; pointer-events: none; }
	.ring .track { stroke: var(--line); }
	.ring .arc { stroke: var(--accent); stroke-linecap: round; transition: stroke-dasharray var(--dur) var(--ease-out); }
	@media (prefers-reduced-motion: reduce) { .ring .arc { transition: none; } }
	.current { font-weight: 600; }
	.current .ic { color: var(--accent); }
	.label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; transition: opacity var(--dur-fast) var(--ease-out) var(--label-delay, 0ms); }
	kbd.label { flex: none; color: var(--text-muted); font: var(--t-footnote); }
	.count { min-width: 22px; height: 20px; padding: 0 7px; display: grid; place-items: center; flex: none; border-radius: 999px; background: var(--surface-3); color: var(--text); font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; transition: opacity var(--dur-fast) var(--ease-out); }
	.flag { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--fail); }
	:global(.ui-sidebar.collapsed) .ui-sbi .flag { position: absolute; top: 6px; inset-inline-end: 8px; }
	.current .count { background: var(--surface-1); }
	:global(.ui-sidebar.collapsed) .ui-sbi .label { opacity: 0; }
	:global(.ui-sidebar.collapsed) .ui-sbi .count { position: absolute; top: 1px; inset-inline: auto 1px; min-width: 16px; height: 16px; padding: 0 4px; font-size: 10px; background: var(--accent-fill); color: var(--text-on-red); }
	.tip { position: fixed; z-index: 80; translate: 0 -50%; padding: 5px 9px; border-radius: 7px; background: var(--text); color: var(--bg); font: var(--t-footnote); white-space: nowrap; pointer-events: none; box-shadow: var(--shadow-lift); }
	@media (pointer: coarse) { .ui-sbi { min-height: 44px; } }
</style>
