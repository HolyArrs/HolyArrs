<script lang="ts">
	import Icon from '../Icon.svelte';
	import type { IconName } from '../Icon.svelte';

	// One notification, iOS style: a frosted card with an app-style icon tile,
	// a bold title, a one-line body and a time. ToastList stacks and animates them.
	// `compact` shrinks it to a pill (tap to open again); swipe down or × dismisses.
	type Tone = 'ok' | 'fail' | 'info' | 'warn';
	let {
		text,
		title,
		tone = 'info',
		icon,
		time,
		compact = false,
		action,
		ondismiss,
		onexpand
	}: {
		text: string;
		title?: string;
		tone?: Tone;
		icon?: IconName;
		time?: string;
		compact?: boolean;
		action?: { label: string; run: () => void };
		ondismiss?: () => void;
		onexpand?: () => void;
	} = $props();
	const glyph = $derived<IconName>(icon ?? (tone === 'ok' ? 'check' : tone === 'fail' || tone === 'warn' ? 'alert' : 'info'));

	// Swipe up to dismiss.
	let dy = $state(0);
	let startY: number | null = null;
	function down(e: PointerEvent) {
		if (!ondismiss || e.button !== 0 || (e.target as Element).closest('button')) return;
		startY = e.clientY;
		(e.currentTarget as Element).setPointerCapture?.(e.pointerId);
	}
	function move(e: PointerEvent) {
		if (startY === null) return;
		dy = Math.max(0, e.clientY - startY);
	}
	function up() {
		if (startY === null) return;
		startY = null;
		if (dy > 28) ondismiss?.();
		dy = 0;
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'Escape' && ondismiss) {
			e.stopPropagation();
			ondismiss();
		}
	}
</script>

{#if compact}
	<button class="ui-toast pill {tone}" type="button" data-testid="toast-pill" aria-label="{title ?? text}. Show more" onclick={onexpand} onkeydown={key}>
		<span class="tile sm"><Icon name={glyph} size={13} stroke={2.2} /></span>
		<span class="pill-t">{title ?? text}</span>
	</button>
{:else}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="ui-toast card {tone}" style:transform={dy ? `translateY(${dy}px)` : undefined} style:opacity={dy ? Math.max(0.3, 1 - dy / 120) : undefined} class:drag={dy !== 0} onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up} onkeydown={key}>
		<span class="tile"><Icon name={glyph} size={20} stroke={2} /></span>
		<span class="body">
			{#if title}
				<span class="row"><strong class="title">{title}</strong>{#if time}<span class="time">{time}</span>{/if}</span>
				<span class="text">{text}</span>
			{:else}
				<span class="title solo">{text}</span>
			{/if}
		</span>
		{#if action}<button class="act" type="button" data-testid="toast-action" onclick={action.run}>{action.label}</button>{/if}
		{#if ondismiss}<button class="x" type="button" aria-label="Close message" onclick={ondismiss}><Icon name="x" size={14} stroke={2} /></button>{/if}
	</div>
{/if}

<style>
	.ui-toast {
		--tint: var(--info);
		pointer-events: auto;
		color: var(--text);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent);
		-webkit-backdrop-filter: saturate(180%) blur(24px);
		backdrop-filter: saturate(180%) blur(24px);
		box-shadow: 0 10px 30px rgb(0 0 0 / 14%), 0 1px 3px rgb(0 0 0 / 8%), inset 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent);
	}
	.ok { --tint: var(--ok); }
	.fail { --tint: var(--fail); }
	.warn { --tint: var(--wait); }
	.card { display: flex; align-items: center; gap: 12px; min-height: 64px; padding: 10px 8px 10px 12px; border-radius: 22px; touch-action: pan-x; transition: transform var(--dur) var(--ease-spring), opacity var(--dur) var(--ease-out); }
	.card.drag { transition: none; }
	.tile { width: 38px; height: 38px; flex: none; display: grid; place-items: center; border-radius: 10px; color: var(--tint); background: color-mix(in srgb, var(--tint) 18%, var(--surface-1)); box-shadow: inset 0 0 0 0.5px color-mix(in srgb, var(--tint) 30%, transparent); }
	.tile.sm { width: 22px; height: 22px; border-radius: 7px; }
	.body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.row { display: flex; align-items: baseline; gap: 8px; }
	.title { flex: 1; min-width: 0; font: var(--t-callout); font-weight: 650; letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.title.solo { font-weight: 600; white-space: normal; }
	.time { flex: none; font: var(--t-footnote, var(--t-subhead)); font-size: 12px; color: var(--text-muted); }
	.text { font: var(--t-subhead); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pill { align-self: center; display: inline-flex; align-items: center; gap: 8px; height: 34px; max-width: 100%; padding: 0 14px 0 6px; border-radius: 999px; font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.pill-t { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pill:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.x { width: 30px; height: 30px; flex: none; align-self: flex-start; display: grid; place-items: center; border-radius: 50%; color: var(--text-muted); }
	.act { min-height: 34px; padding: 0 12px; border-radius: 999px; color: var(--text); font: var(--t-subhead); font-weight: 700; flex: none; text-decoration: underline; text-underline-offset: 3px; text-decoration-color: color-mix(in srgb, var(--text) 35%, transparent); }
	.act:hover, .x:hover { background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); }
	.act:focus-visible, .x:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .x { width: 44px; height: 44px; margin: -7px -4px -7px 0; } }
	@media (prefers-reduced-motion: reduce) { .card { transition: opacity var(--dur-fast) var(--ease-out); } }
</style>
