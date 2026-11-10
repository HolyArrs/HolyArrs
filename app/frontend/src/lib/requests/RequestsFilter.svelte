<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import StatusIcon, { statusKey } from '#lib/ui/StatusIcon.svelte';
	import type { Life } from './lifecycle.ts';
	import type { BoardCtx } from './board.ts';
	import { tick } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { cubicIn, cubicOut } from 'svelte/easing';

	// The Filter button of the Requests page: Status for this tab, then Type. On phones it opens as a bottom sheet.
	let { ctx, types, subs, hasSubs, subsTotal, filterCount, status, toneOf, setStatus, ms, open = $bindable(false), btn = $bindable() }: {
		ctx: BoardCtx;
		types: { k: 'all' | 'movie' | 'show' | 'artist'; label: string; icon: IconName }[];
		subs: { word: string; key: string; count: number; tone: Life['tone'] }[];
		hasSubs: boolean;
		subsTotal: number;
		filterCount: number;
		status: string;
		toneOf: (t: Life['tone']) => string;
		setStatus: (word: string) => void;
		ms: (d: number) => number;
		open?: boolean;
		btn?: HTMLButtonElement;
	} = $props();
	let phone = $state(false);
	$effect(() => {
		const mq = window.matchMedia('(max-width: 767px)');
		phone = mq.matches;
		const on = () => (phone = mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	});
		let sfPop = $state<HTMLElement>();
	const sfItems = () => [...(sfPop?.querySelectorAll<HTMLElement>('[role^="menuitem"]') ?? [])];
	function sfToggle() {
		open = !open;
		if (open) void tick().then(() => (sfPop?.querySelector<HTMLElement>('[aria-checked="true"]') ?? sfItems()[0])?.focus());
	}
	function sfClose(back = true) {
		if (!open) return;
		open = false;
		if (back) btn?.focus();
	}
	function sfKeys(e: KeyboardEvent) {
		if (e.key === 'Escape' || e.key === 'Tab') {
			if (e.key === 'Escape') {
				e.preventDefault();
				e.stopPropagation();
			}
			// Tab leaves the menu from the Filter button, so the order stays page order.
			sfClose();
			return;
		}
		if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return;
		const items = sfItems();
		const i = items.indexOf(document.activeElement as HTMLElement);
		const n = e.key === 'Home' ? 0 : e.key === 'End' ? items.length - 1 : (i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
		e.preventDefault();
		items[n]?.focus();
	}
	$effect(() => {
		if (!open) return;
		const out = (e: PointerEvent) => {
			const node = e.target as Node;
			// The phone sheet closes on its scrim's own click instead, so the tap cannot land on a card below.
			if (!phone && !sfPop?.contains(node) && !btn?.contains(node)) sfClose(false);
		};
		document.addEventListener('pointerdown', out, true);
		return () => document.removeEventListener('pointerdown', out, true);
	});
</script>

{#snippet statusItems(close: boolean)}
	<button type="button" role="menuitemradio" aria-checked={!status} class="sf-mi" onclick={() => { setStatus(''); if (close) sfClose(); }}><span class="sf-tick" aria-hidden="true"><Icon name="check" size={14} /></span><span class="sf-d" aria-hidden="true"><Icon name="requests" size={14} /></span><span class="sf-w">{t('requests.filter.allStatuses')}</span><span class="sf-c">{subsTotal}</span></button>
	{#each subs as x (x.word)}
		<button type="button" role="menuitemradio" aria-checked={status === x.word} class="sf-mi sf-t-{toneOf(x.tone)}" onclick={() => { setStatus(x.word); if (close) sfClose(); }}><span class="sf-tick" aria-hidden="true"><Icon name="check" size={14} /></span><span class="sf-d" aria-hidden="true"><StatusIcon status={statusKey(x.key)} size={14} motion={false} /></span><span class="sf-w">{x.word}</span><span class="sf-c">{x.count}</span></button>
	{/each}
{/snippet}
{#snippet filterItems()}
	{#if hasSubs}
			<div role="group" aria-labelledby="sf-mh-status">
			<div class="sf-mh" id="sf-mh-status">{t('requests.filter.status')}</div>
			{@render statusItems(false)}
			</div>
			<div class="sf-sep" role="separator"></div>
		{/if}
		<div role="group" aria-labelledby="sf-mh-type">
		<div class="sf-mh" id="sf-mh-type">{t('requests.filter.type')}</div>
		{#each types as ty (ty.k)}
			<button type="button" role="menuitemradio" aria-checked={ctx.kind === ty.k} class="sf-mi" onclick={() => ctx.setKind(ty.k)}><span class="sf-tick" aria-hidden="true"><Icon name="check" size={14} /></span><span class="sf-ti" aria-hidden="true"><Icon name={ty.icon} size={14} /></span><span class="sf-w">{ty.label}</span></button>
		{/each}
		</div>
{/snippet}
<div class="sf-anchor">
		<button type="button" class="sf-btn" bind:this={btn} aria-haspopup={phone ? 'dialog' : 'menu'} aria-expanded={open} aria-label={filterCount ? t('requests.filter.labelOn', { n: filterCount }) : t('requests.filter.label')} onclick={sfToggle}>
			<svg class="sf-funnel" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M2.5 3.5h11l-4.2 5v4l-2.6 1.2V8.5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" /></svg>
			<span class="sf-v">{t('requests.filter.label')}</span>
			{#if filterCount}<span class="sf-badge" aria-hidden="true">{filterCount}</span>{/if}
		</button>
		{#if open && !phone}
			<div class="sf-menu" in:fly={{ y: -8, duration: ms(280), easing: cubicOut }} out:fade={{ duration: ms(180) }} role="menu" aria-label={t('requests.filter.label')} tabindex="-1" bind:this={sfPop} onkeydown={sfKeys}>
				{@render filterItems()}
			</div>
		{/if}
	</div>
{#if open && phone}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="sf-scrim" aria-hidden="true" in:fade={{ duration: ms(300) }} out:fade={{ duration: ms(200) }} onclick={(e) => { e.preventDefault(); sfClose(true); }}></div>
	<div class="sf-sheet" role="dialog" aria-modal="true" aria-label={t('requests.filter.label')} tabindex="-1" bind:this={sfPop} onkeydown={sfKeys} in:fly={{ y: 80, duration: ms(420), easing: cubicOut }} out:fly={{ y: 80, duration: ms(200), easing: cubicIn }}>
		<div class="sf-head"><span class="sf-ht">{t('requests.filter.label')}</span><button type="button" class="sf-done" onclick={() => sfClose(true)}>{t('requests.filter.done')}</button></div>
		<div role="menu" aria-label={t('requests.filter.options')}>{@render filterItems()}</div>
	</div>
{/if}

<style>
	/* Tones: failed red, live steps blue, ready green, waiting amber, the rest muted. */
	.sf-t-fail { --sc: var(--fail, #ff6961); --st: var(--fail-text, var(--fail, #ff6961)); }
	.sf-t-info { --sc: var(--info, #409cff); --st: var(--info-text, var(--info, #409cff)); }
	.sf-t-ok { --sc: var(--ok, #30d158); --st: var(--ok-text, var(--ok, #30d158)); }
	.sf-t-wait { --sc: var(--wait, #ffb340); --st: var(--wait-text, var(--wait, #ffb340)); }
	.sf-t-off { --sc: var(--text-muted); --st: var(--text); }
	.sf-d, .sf-ti { flex: none; display: grid; place-items: center; width: 16px; height: 16px; color: var(--st, var(--text-muted)); }
	.sf-anchor { position: relative; }
	.sf-btn { position: relative; display: inline-flex; align-items: center; gap: 7px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: background 160ms var(--ease-out); }
	.sf-btn:hover, .sf-btn[aria-expanded='true'] { background: var(--surface-2); }
	.sf-funnel { color: var(--text-muted); }
	.sf-badge { min-width: 18px; height: 18px; padding: 0 5px; box-sizing: border-box; border-radius: 9px; background: light-dark(var(--info, #0071e3), #1d5fd6); color: #fff; font: 600 11px/18px var(--font-ui); text-align: center; }
	.sf-menu { position: absolute; right: 0; top: calc(100% + 6px); z-index: 20; min-width: 230px; max-height: min(420px, 70vh); overflow-y: auto; padding: 4px; border: 1px solid var(--line-strong); border-radius: 12px; background: var(--surface-2); box-shadow: 0 18px 40px -12px rgb(0 0 0 / 0.55); }
	.sf-menu:focus { outline: none; }
	.sf-mh { padding: 8px 10px 4px; color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: 0.06em; text-transform: uppercase; }
	.sf-sep { height: 1px; margin: 4px 6px; background: var(--line); }
	.sf-mi { display: flex; align-items: center; gap: 10px; width: 100%; height: 34px; padding: 0 10px; border: 0; border-radius: 8px; background: none; color: var(--text); font: 500 13px/1 var(--font-ui); text-align: left; white-space: nowrap; cursor: pointer; }
	/* Only the row under the pointer is shaded; the picked row is marked by its tick and weight, so the two never merge. */
	.sf-mi + .sf-mi { margin-top: 2px; }
	.sf-mi { transition: background-color 120ms ease; }
	.sf-mi:hover, .sf-mi:focus-visible { background: color-mix(in srgb, var(--text) 8%, transparent); }
	.sf-mi[aria-checked='true'] { font-weight: 650; }
	.sf-mi:focus-visible { box-shadow: var(--focus-ring); }
	.sf-mi.sf-t-fail .sf-w { color: var(--st); }
	.sf-tick { display: grid; place-items: center; width: 14px; color: var(--info-text, var(--info, #409cff)); visibility: hidden; }
	.sf-mi[aria-checked='true'] .sf-tick { visibility: visible; }
	.sf-ti { display: grid; place-items: center; color: var(--text-muted); }
	.sf-c { margin-left: auto; padding-left: 16px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.sf-scrim { position: fixed; inset: 0; z-index: 96; background: rgb(0 0 0 / 0.45); }
	.sf-sheet { position: fixed; left: 8px; right: 8px; bottom: calc(var(--tabbar, 64px) + 14px + env(safe-area-inset-bottom, 0px)); z-index: 97; max-height: calc(100dvh - var(--tabbar, 64px) - env(safe-area-inset-bottom, 0px) - 62px); overflow-y: auto; box-sizing: border-box; padding: 10px max(16px, env(safe-area-inset-right, 0px)) 12px max(16px, env(safe-area-inset-left, 0px)); border: 1px solid var(--line-strong); border-radius: 18px; background: var(--surface-2); box-shadow: 0 -20px 50px rgb(0 0 0 / 0.45); }
	.sf-sheet:focus { outline: none; }
	.sf-sheet .sf-mi { height: 48px; font-size: 15px; }
	.sf-head { display: flex; align-items: center; justify-content: space-between; margin: 0 0 6px; padding: 0 4px; }
	.sf-ht { font: 600 15px/1.2 var(--font-ui); color: var(--text); }
	.sf-done { min-height: 44px; padding: 0 14px; border: 0; border-radius: var(--r-pill, 999px); background: var(--surface-2); color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.sf-done:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .sf-mi { height: 44px; } }
	@media (max-width: 767px) {
		.sf-btn { width: 100%; justify-content: flex-start; padding-inline: 10px; gap: 5px; }
		.sf-badge { margin-left: auto; }
		.sf-menu { left: 0; right: 0; }
	}
</style>
