<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { onMount, tick, untrack } from 'svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import AlertSymbol from './AlertSymbol.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { alertsPanel, toasts } from '../core/toasts.svelte.ts';
	import { pinned, type PinnedAlert } from './attention-alerts.ts';
	import { sections } from '../core/settings.ts';
	import { settingsNow } from './settings-status.svelte.ts';
	import { session } from '../core/session.svelte.ts';
	import { to } from '../core/paths.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { before = [], after = [] }: { before?: PinnedAlert[]; after?: PinnedAlert[] } = $props();

	let phone = $state(false);
	let left = $state(272);
	let now = $state(Date.now());
	let box = $state<HTMLElement>();
	let wasOpen = false;

	const fixes = $derived([
		...before,
		...(session.can('settings') ? pinned(settingsNow.answer?.items ?? [], sections) : []),
		...after
	]);

	onMount(() => {
		const mq = matchMedia('(max-width: 767px)');
		const set = () => (phone = mq.matches);
		set();
		mq.addEventListener('change', set);
		return () => mq.removeEventListener('change', set);
	});

	// untrack: alerts that arrive while open must keep their "new" ring.
	$effect(() => {
		if (alertsPanel.open) untrack(() => toasts.seeAll());
	});

	// Place the panel beside the side menu, then move focus in on open and back to the bell on close.
	$effect(() => {
		const open = alertsPanel.open && !phone;
		if (open && !wasOpen) {
			const menu = document.querySelector('.ui-sidebar');
			left = Math.round((menu?.getBoundingClientRect().right ?? 260) + 12);
			void tick().then(() => box?.focus());
		} else if (!open && wasOpen) {
			(document.querySelector('.alerts-bell') as HTMLElement | null)?.focus();
		}
		wasOpen = open;
	});

	$effect(() => {
		if (!alertsPanel.open) return;
		now = Date.now();
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});

	function close() {
		alertsPanel.open = false;
	}
	function outside(e: PointerEvent) {
		const el = e.target as Element;
		if (box && !box.contains(el) && !el.closest('[data-alerts-open]')) close();
	}
	function key(e: KeyboardEvent) {
		if (e.key !== 'Escape' || !alertsPanel.open || phone) return;
		e.stopPropagation();
		close();
	}
	function when(at: number) {
		const s = Math.max(0, Math.round((now - at) / 1000));
		if (s < 45) return t('alerts.time.now');
		if (s < 3600) return t('alerts.time.minutes', { n: Math.round(s / 60) });
		if (s < 86400) return t('alerts.time.hours', { n: Math.round(s / 3600) });
		return t('alerts.time.days', { n: Math.round(s / 86400) });
	}
	const live = (p: { action?: unknown; until?: number }) => !!p.action && (!p.until || now <= p.until);
</script>

<svelte:window onpointerdown={alertsPanel.open && !phone ? outside : undefined} onkeydown={key} />

{#snippet rows()}
	{#if fixes.length}
		<h3 class="sub">{t('alerts.panel.needsAttention')}</h3>
		<ul class="rows pins">
			{#each fixes as f (f.key)}
				<li class="row pin">
					<AlertSymbol tone={f.tone} size={24} />
					<span class="tx">
						<span class="t">{serverWords(f.title)}</span>
						{#if f.body}<span class="b">{serverWords(f.body)}</span>{/if}
						<a class="act" href={to(f.href)} onclick={() => (alertsPanel.open = false)}>{f.label} ›</a>
					</span>
				</li>
			{/each}
		</ul>
		{#if toasts.past.length}<h3 class="sub">{t('alerts.panel.recent')}</h3>{/if}
	{/if}
	{#if toasts.past.length}
		<ul class="rows">
			{#each toasts.past as p (p.id)}
				<!-- One pattern for every row: symbol | text column (title, second line, Undo) | time over ×. -->
				<li class="row" class:fresh={p.fresh}>
					<AlertSymbol tone={p.tone} size={24} />
					<span class="tx">
						<span class="t">{p.text}</span>
						{#if p.body}<span class="b">{p.body}</span>{/if}
						{#if live(p)}<button type="button" class="act" onclick={() => toasts.runPast(p.id)}>{p.action?.label}</button>{/if}
					</span>
					<span class="side">
						<time datetime={new Date(p.at).toISOString()}>{when(p.at)}</time>
						<button type="button" class="x" aria-label={t('alerts.panel.remove', { text: p.text })} onclick={() => toasts.forget(p.id)}><Icon name="x" size={11} stroke={2.4} /></button>
					</span>
				</li>
			{/each}
		</ul>
	{:else if !fixes.length}
		<p class="empty">{t('alerts.panel.empty')}</p>
	{/if}
{/snippet}

{#if phone}
	<Sheet bind:open={alertsPanel.open} title={t('alerts.panel.title')} size="sm">
		<div class="sheet-body">
			{#if alertsPanel.open && toasts.past.length}<div class="tools"><button type="button" class="clear" onclick={() => toasts.forgetAll()}>{t('alerts.panel.clearAll')}</button></div>{/if}
			<!-- Rows only while open, so a closed sheet adds no second copy of each alert's text. -->
			{#if alertsPanel.open}{@render rows()}{/if}
		</div>
	</Sheet>
{:else if alertsPanel.open}
	<div class="panel" role="dialog" aria-label={t('alerts.panel.title')} tabindex="-1" style:left="{left}px" bind:this={box}>
		<div class="head">
			<strong>{t('alerts.panel.title')}</strong>
			{#if toasts.past.length}<button type="button" class="clear" onclick={() => toasts.forgetAll()}>{t('alerts.panel.clearAll')}</button>{/if}
		</div>
		{@render rows()}
	</div>
{/if}

<style>
	.panel:focus { outline: none; }
	.panel {
		position: fixed;
		z-index: 91;
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
	.row { display: flex; align-items: flex-start; gap: 10px; padding: 10px 10px 10px 12px; border-radius: 16px; background: linear-gradient(180deg, rgb(255 255 255 / 9%), rgb(255 255 255 / 5%)); box-shadow: inset 0 1px 0 rgb(255 255 255 / 9%), inset 0 0 0 0.5px rgb(255 255 255 / 7%), 0 4px 14px rgb(0 0 0 / 22%); transition: background var(--dur-fast, 0.15s) var(--ease-out, ease); }
	.row:hover { background: linear-gradient(180deg, rgb(255 255 255 / 13%), rgb(255 255 255 / 8%)); }
	.row.fresh { box-shadow: inset 0 1px 0 rgb(255 255 255 / 10%), inset 0 0 0 1px rgb(100 181 255 / 28%), 0 4px 14px rgb(0 0 0 / 22%); }
	.tx { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.t { font: var(--t-subhead); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fresh .t::before { content: ''; display: inline-block; width: 6px; height: 6px; margin-right: 6px; border-radius: 50%; background: var(--info-on-art, #64b5ff); vertical-align: middle; }
	.b { font: var(--t-footnote); color: rgb(245 245 247 / 58%); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.row :global(.ui-alert-sym) { margin-top: 1px; }
	.side { flex: none; width: 34px; display: flex; flex-direction: column; align-items: flex-end; gap: 2px; }
	time { flex: none; line-height: 20px; font: var(--t-footnote); color: rgb(245 245 247 / 58%); font-variant-numeric: tabular-nums; }
	.act { align-self: flex-start; margin-top: 6px; padding: 4px 12px; border-radius: 999px; font: var(--t-footnote); font-weight: 600; color: #f5f5f7; background: rgb(255 255 255 / 14%); }
	.act:hover { background: rgb(255 255 255 / 22%); }
	.x { flex: none; width: 22px; height: 22px; margin-right: -4px; display: grid; place-items: center; border-radius: 50%; color: rgb(245 245 247 / 55%); }
	.x:hover { color: #f5f5f7; background: rgb(255 255 255 / 12%); }
	.x:focus-visible, .act:focus-visible, .clear:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sub { margin: 4px 8px 6px; font: var(--t-footnote); font-weight: 600; color: rgb(245 245 247 / 58%); }
	.pins { flex: none; margin-bottom: 8px; }
	a.act { text-decoration: none; }
	.empty { padding: 20px 8px; text-align: center; color: rgb(245 245 247 / 55%); font: var(--t-subhead); }
	.sheet-body { display: flex; flex-direction: column; gap: 4px; }
	.sheet-body .tools { display: flex; justify-content: flex-end; }
	.sheet-body .sub, .sheet-body .clear, .sheet-body .b, .sheet-body time, .sheet-body .x, .sheet-body .empty { color: var(--text-muted); }
	.sheet-body .act { color: var(--text); background: var(--surface-2); }
	.sheet-body .row, .sheet-body .row:hover { background: var(--surface-1); box-shadow: inset 0 0 0 0.5px var(--line), 0 2px 8px rgb(0 0 0 / 8%); }
	.sheet-body .row.fresh { box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--info, #0071e3) 35%, transparent), 0 2px 8px rgb(0 0 0 / 8%); }
	.x { position: relative; }
	@media (pointer: coarse) { .x::after { content: ''; position: absolute; inset: -10px; } }
</style>
