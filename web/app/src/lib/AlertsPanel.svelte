<script lang="ts">
	import { onMount } from 'svelte';
	import Icon from './Icon.svelte';
	import AlertSymbol from './ui/AlertSymbol.svelte';
	import Sheet from './ui/Sheet.svelte';
	import { alertsPanel, toasts } from './toasts.svelte';

	// Recent alerts: the last 50 alerts on this device, newest first. Desktop: a glass
	// panel beside the side menu's bell. Phone: a sheet from the bottom.
	let phone = $state(false);
	onMount(() => {
		const mq = matchMedia('(max-width: 767px)');
		const set = () => (phone = mq.matches);
		set();
		mq.addEventListener('change', set);
		return () => mq.removeEventListener('change', set);
	});

	// Opening shows everything waiting and clears the count.
	$effect(() => {
		if (alertsPanel.open) toasts.seeAll();
	});

	// Times like "now", "2m", "1h"; refreshed while open.
	let now = $state(Date.now());
	$effect(() => {
		if (!alertsPanel.open) return;
		now = Date.now();
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});
	function when(at: number) {
		const s = Math.max(0, Math.round((now - at) / 1000));
		if (s < 45) return 'now';
		if (s < 3600) return `${Math.round(s / 60)}m`;
		if (s < 86400) return `${Math.round(s / 3600)}h`;
		return `${Math.round(s / 86400)}d`;
	}
	const live = (p: { action?: unknown; until?: number }) => !!p.action && (!p.until || now <= p.until);

	let box = $state<HTMLElement>();
	function close() {
		alertsPanel.open = false;
	}
	function outside(e: PointerEvent) {
		const el = e.target as Element;
		if (box && !box.contains(el) && !el.closest('[data-alerts-open]')) close();
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'Escape' && alertsPanel.open && !phone) close();
	}
</script>

<svelte:window onpointerdown={alertsPanel.open && !phone ? outside : undefined} onkeydown={key} />

{#snippet rows()}
	{#if toasts.past.length}
		<ul class="rows">
			{#each toasts.past as p (p.id)}
				<li class="row" class:fresh={p.fresh}>
					<AlertSymbol tone={p.tone} size={24} />
					<span class="tx">
						<span class="t">{p.text}</span>
						{#if p.body}<span class="b">{p.body}</span>{/if}
					</span>
					{#if live(p)}
						<button type="button" class="act" onclick={() => toasts.runPast(p.id)}>{p.action?.label}</button>
					{:else}
						<time>{when(p.at)}</time>
					{/if}
					<button type="button" class="x" aria-label="Remove {p.text}" onclick={() => toasts.forget(p.id)}><Icon name="x" size={11} stroke={2.4} /></button>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="empty">No alerts yet</p>
	{/if}
{/snippet}

{#if phone}
	<Sheet bind:open={alertsPanel.open} title="Recent alerts" size="sm">
		<div class="sheet-body">
			{#if toasts.past.length}<div class="tools"><button type="button" class="clear" onclick={() => toasts.forgetAll()}>Clear all</button></div>{/if}
			{@render rows()}
		</div>
	</Sheet>
{:else if alertsPanel.open}
	<div class="panel" role="dialog" aria-label="Recent alerts" bind:this={box}>
		<div class="head">
			<strong>Recent alerts</strong>
			{#if toasts.past.length}<button type="button" class="clear" onclick={() => toasts.forgetAll()}>Clear all</button>{/if}
		</div>
		{@render rows()}
	</div>
{/if}

<style>
	/* Same dark glass as the alert pills. */
	.panel {
		position: fixed;
		z-index: 91;
		left: calc(var(--sidebar, 260px) + 12px);
		bottom: calc(12px + env(safe-area-inset-bottom, 0px));
		width: min(380px, calc(100vw - 32px));
		max-height: min(560px, calc(100dvh - 32px));
		display: flex;
		flex-direction: column;
		padding: 8px;
		border-radius: 18px;
		color: #f5f5f7;
		background: rgb(26 26 30 / 90%);
		-webkit-backdrop-filter: blur(40px) saturate(180%);
		backdrop-filter: blur(40px) saturate(180%);
		box-shadow: 0 24px 60px rgb(0 0 0 / 45%), inset 0 1px 0 rgb(255 255 255 / 8%), inset 0 0 0 0.5px rgb(255 255 255 / 8%);
		animation: rise var(--dur, 0.3s) var(--ease-spring, cubic-bezier(0.32, 1.28, 0.5, 1)) both;
	}
	@keyframes rise { from { opacity: 0; transform: translateY(8px) scale(0.98); } }
	@media (prefers-reduced-motion: reduce) { .panel { animation: none; } }
	.head { display: flex; align-items: center; justify-content: space-between; padding: 6px 8px 10px; font: var(--t-callout); font-weight: 600; }
	.clear { font: var(--t-footnote); font-weight: 600; color: rgb(245 245 247 / 62%); padding: 4px 8px; border-radius: 999px; }
	.clear:hover { color: #f5f5f7; background: rgb(255 255 255 / 10%); }
	.rows { overflow-y: auto; display: flex; flex-direction: column; gap: 6px; padding: 2px; overscroll-behavior: contain; }
	/* Each alert is its own glass card, like the pills that drop in. */
	.row { display: flex; align-items: center; gap: 10px; padding: 10px 10px 10px 12px; border-radius: 16px; background: linear-gradient(180deg, rgb(255 255 255 / 9%), rgb(255 255 255 / 5%)); box-shadow: inset 0 1px 0 rgb(255 255 255 / 9%), inset 0 0 0 0.5px rgb(255 255 255 / 7%), 0 4px 14px rgb(0 0 0 / 22%); transition: background var(--dur-fast, 0.15s) var(--ease-out, ease); }
	.row:hover { background: linear-gradient(180deg, rgb(255 255 255 / 13%), rgb(255 255 255 / 8%)); }
	/* New since you last looked: a thin blue ring, matching the blue dot. */
	.row.fresh { box-shadow: inset 0 1px 0 rgb(255 255 255 / 10%), inset 0 0 0 1px rgb(100 181 255 / 28%), 0 4px 14px rgb(0 0 0 / 22%); }
	.tx { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.t { font: var(--t-subhead); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fresh .t::before { content: ''; display: inline-block; width: 6px; height: 6px; margin-right: 6px; border-radius: 50%; background: var(--info-on-art, #64b5ff); vertical-align: middle; }
	.b { font: var(--t-footnote); color: rgb(245 245 247 / 58%); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	time { flex: none; font: var(--t-footnote); color: rgb(245 245 247 / 45%); font-variant-numeric: tabular-nums; }
	.act { flex: none; padding: 5px 12px; border-radius: 999px; font: var(--t-footnote); font-weight: 600; color: #f5f5f7; background: rgb(255 255 255 / 14%); }
	.act:hover { background: rgb(255 255 255 / 22%); }
	.x { flex: none; width: 24px; height: 24px; display: grid; place-items: center; border-radius: 50%; color: rgb(245 245 247 / 55%); }
	.x:hover { color: #f5f5f7; background: rgb(255 255 255 / 12%); }
	.x:focus-visible, .act:focus-visible, .clear:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.empty { padding: 20px 8px; text-align: center; color: rgb(245 245 247 / 55%); font: var(--t-subhead); }
	/* Phone sheet: the app's own sheet, light or dark, so rows take the theme's colours. */
	.sheet-body { display: flex; flex-direction: column; gap: 4px; }
	.sheet-body .tools { display: flex; justify-content: flex-end; }
	.sheet-body .clear, .sheet-body .b, .sheet-body time, .sheet-body .x, .sheet-body .empty { color: var(--text-muted); }
	.sheet-body .act { color: var(--text); background: var(--surface-2); }
	/* Phone sheet: the same separate cards, in the theme's own surface. */
	.sheet-body .row, .sheet-body .row:hover { background: var(--surface-1); box-shadow: inset 0 0 0 0.5px var(--line), 0 2px 8px rgb(0 0 0 / 8%); }
	.sheet-body .row.fresh { box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--info, #0071e3) 35%, transparent), 0 2px 8px rgb(0 0 0 / 8%); }
	@media (pointer: coarse) { .x { width: 36px; height: 36px; margin-right: -6px; } }
</style>
