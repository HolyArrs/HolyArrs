<script lang="ts">
	// The top bar of the preview looks, the same pattern as Requests: status
	// tabs (40px, 44px on touch, sideways on phones) and one glass Filter
	// button. On wide screens Filter opens a menu; on phones a sheet. It holds
	// kind, a date range, who did it and the title; everything lives in the
	// address so Back and shared links work.
	import { tick } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { cubicIn, cubicOut } from 'svelte/easing';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import Icon from '../Icon.svelte';
	import { ALL, lookTabs } from '../history-looks.ts';
	import { dateRangeProblem } from '../history.ts';
	import { reducedMotion } from '../motion.ts';
	import { dirQuery, readDir, showChange, whoLabels, type Who } from './dirs.ts';
	import type { TitleView } from '../types.ts';

	let { counts, titles, label = 'History' }: { counts: Record<string, number>; titles: TitleView[]; label?: string } = $props();

	const st = $derived(readDir(page.url.searchParams));
	const ms = (n: number) => (reducedMotion() ? 0 : n);
	const problem = $derived(dateRangeProblem(st.filter));
	const filterCount = $derived((st.sub ? 1 : 0) + (st.filter.from ? 1 : 0) + (st.filter.to ? 1 : 0) + (st.who ? 1 : 0) + (st.filter.title ? 1 : 0));
	const sortedTitles = $derived([...titles].sort((a, b) => a.name.localeCompare(b.name)));

	function set(change: Record<string, string | number | undefined>) {
		void goto(page.url.pathname + dirQuery(page.url.searchParams, change), { replace: true, reset: false });
	}
	const pickTab = (key: string) => set(showChange(key));
	function pickKind(v: string) {
		const [show = ALL, sub = ''] = v.split('.');
		set(showChange(show, sub));
	}

	let phone = $state(false);
	$effect(() => {
		const mq = window.matchMedia('(max-width: 767px)');
		phone = mq.matches;
		const on = () => (phone = mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	});
	let open = $state(false);
	let btn = $state<HTMLButtonElement>();
	let pop = $state<HTMLElement>();
	function toggle() {
		open = !open;
		if (open) void tick().then(() => pop?.querySelector<HTMLElement>('select, input, button')?.focus());
	}
	function close(back = true) {
		if (!open) return;
		open = false;
		if (back) btn?.focus();
	}
	function keys(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		e.preventDefault();
		e.stopPropagation();
		close();
	}
	$effect(() => {
		if (!open || phone) return;
		const out = (e: PointerEvent) => {
			const t = e.target as Node;
			if (!pop?.contains(t) && !btn?.contains(t)) close(false);
		};
		document.addEventListener('pointerdown', out, true);
		return () => document.removeEventListener('pointerdown', out, true);
	});
</script>

{#snippet fields()}
	<label class="fl"><span class="fh">Kind</span>
		<select class="fc" value={st.sub ? `${st.show}.${st.sub}` : ''} onchange={(e) => pickKind(e.currentTarget.value || st.show)}>
			<option value="">Every kind</option>
			{#each lookTabs.filter((t) => t.subs.length) as t (t.key)}
				<optgroup label={t.label}>{#each t.subs as s (s.key)}<option value="{t.key}.{s.key}">{s.label}</option>{/each}</optgroup>
			{/each}
		</select>
	</label>
	<div class="two">
		<label class="fl"><span class="fh">From</span><input class="fc" type="date" value={st.filter.from ?? ''} max={st.filter.to} onchange={(e) => set({ from: e.currentTarget.value })} /></label>
		<label class="fl"><span class="fh">To</span><input class="fc" type="date" value={st.filter.to ?? ''} min={st.filter.from} onchange={(e) => set({ to: e.currentTarget.value })} /></label>
	</div>
	{#if problem}<p class="bad" role="alert">{problem}</p>{/if}
	<label class="fl"><span class="fh">Who</span>
		<select class="fc" value={st.who} onchange={(e) => set({ who: e.currentTarget.value })}>
			{#each Object.entries(whoLabels) as [k, v] (k)}<option value={k as Who}>{v}</option>{/each}
		</select>
	</label>
	<label class="fl"><span class="fh">Title</span>
		<select class="fc" value={st.filter.title ?? ''} onchange={(e) => set({ title: e.currentTarget.value, episode: undefined })}>
			<option value="">Every title</option>
			{#each sortedTitles as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
		</select>
	</label>
	{#if filterCount}<button type="button" class="clear" onclick={() => set({ show: st.show === ALL ? undefined : st.show, from: undefined, to: undefined, who: undefined, title: undefined, episode: undefined })}>Clear filters</button>{/if}
{/snippet}

<h1 class="sr-only">{label}</h1>
<div class="toolbar" data-testid="history-dir-bar">
	<div class="tabs" role="group" aria-label="Status">
		{#each lookTabs as t (t.key)}
			{@const n = counts[t.key]}
			{@const here = st.show === t.key}
			<button type="button" class="tab" class:on={here} class:zero={n === 0} aria-pressed={here} onclick={() => pickTab(t.key)}>{t.label}{#if n != null}<span class="n" class:fail={t.key === 'problems' && n > 0}>{n}</span>{/if}</button>
		{/each}
	</div>
	<div class="right">
		<div class="anchor">
			<button type="button" class="sf-btn" bind:this={btn} aria-haspopup="dialog" aria-expanded={open} aria-label="Filter{filterCount ? `, ${filterCount} on` : ''}" onclick={toggle}>
				<svg class="funnel" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M2.5 3.5h11l-4.2 5v4l-2.6 1.2V8.5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" /></svg>
				<span>Filter</span>
				{#if filterCount}<span class="badge" aria-hidden="true">{filterCount}</span>{/if}
			</button>
			{#if open && !phone}
				<div class="menu" role="dialog" aria-label="Filter" tabindex="-1" bind:this={pop} onkeydown={keys} in:fly={{ y: -8, duration: ms(280), easing: cubicOut }} out:fade={{ duration: ms(180) }}>
					{@render fields()}
				</div>
			{/if}
		</div>
	</div>
</div>
{#if open && phone}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="scrim" aria-hidden="true" in:fade={{ duration: ms(300) }} out:fade={{ duration: ms(200) }} onclick={() => close()}></div>
	<div class="sheet" role="dialog" aria-modal="true" aria-label="Filter" tabindex="-1" bind:this={pop} onkeydown={keys} in:fly={{ y: 80, duration: ms(420), easing: cubicOut }} out:fly={{ y: 80, duration: ms(200), easing: cubicIn }}>
		<div class="sh"><span class="sht">Filter</span><button type="button" class="done" onclick={() => close()}><Icon name="check" size={16} />Done</button></div>
		{@render fields()}
	</div>
{/if}

<style>
	.toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 0 0 16px; }
	.tabs { display: flex; align-items: center; gap: 2px; min-width: 0; height: 40px; box-sizing: border-box; padding: 3px; overflow-x: auto; scrollbar-width: none; overscroll-behavior-x: contain; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.tabs::-webkit-scrollbar { display: none; }
	.tab { flex: none; display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 13px; border: 0; border-radius: 9px; background: none; color: color-mix(in srgb, var(--text) 88%, var(--bg)); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.tab:hover:not(.on) { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.tab.on { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	.tab.zero:not(.on):not(:hover) { color: var(--text-muted); }
	.tab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.n { font-weight: 600; font-variant-numeric: tabular-nums; color: var(--text-muted); }
	.n.fail { color: var(--fail-text); }
	.right { flex: none; display: flex; align-items: center; gap: 10px; margin-left: auto; }
	.anchor { position: relative; }
	.sf-btn { display: inline-flex; align-items: center; gap: 7px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: background 160ms var(--ease-out); }
	.sf-btn:hover, .sf-btn[aria-expanded='true'] { background: var(--surface-2); }
	.sf-btn:focus-visible, .clear:focus-visible, .done:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.funnel { color: var(--text-muted); }
	.badge { min-width: 18px; height: 18px; padding: 0 5px; box-sizing: border-box; border-radius: 9px; background: var(--info); color: var(--bg); font: 600 11px/18px var(--font-ui); text-align: center; }
	.menu { position: absolute; right: 0; top: calc(100% + 6px); z-index: 20; width: 300px; max-height: min(520px, 75vh); overflow-y: auto; display: flex; flex-direction: column; gap: 10px; padding: 12px; border: 1px solid var(--line-strong); border-radius: 12px; background: var(--surface-2); box-shadow: 0 18px 40px -12px rgb(0 0 0 / 0.55); }
	.menu:focus, .sheet:focus { outline: none; }
	.fl { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
	.fh { color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: 0.06em; text-transform: uppercase; }
	.fc { width: 100%; min-width: 0; height: 36px; box-sizing: border-box; padding: 0 10px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface-1); color: var(--text); font: 500 13px/1 var(--font-ui); color-scheme: light dark; }
	.fc:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.two { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
	.bad { margin: 0; color: var(--fail-text); font: var(--t-footnote); }
	.clear { align-self: flex-start; height: 34px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); cursor: pointer; }
	.scrim { position: fixed; inset: 0; z-index: 96; background: color-mix(in srgb, var(--bg) 55%, transparent); }
	.sheet { position: fixed; left: 8px; right: 8px; bottom: calc(var(--tabbar) + 14px + env(safe-area-inset-bottom, 0px)); z-index: 97; max-height: calc(100dvh - var(--tabbar) - env(safe-area-inset-bottom, 0px) - 62px); overflow-y: auto; display: flex; flex-direction: column; gap: 12px; box-sizing: border-box; padding: 10px 16px 16px; border: 1px solid var(--line-strong); border-radius: 18px; background: var(--surface-2); box-shadow: 0 -20px 50px rgb(0 0 0 / 0.45); }
	.sh { display: flex; align-items: center; justify-content: space-between; }
	.sht { color: var(--text); font: 600 15px/1.2 var(--font-ui); }
	.done { display: inline-flex; align-items: center; gap: 6px; min-height: 44px; padding: 0 14px; border: 0; border-radius: var(--r-pill); background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	@media (pointer: coarse) {
		.tabs, .sf-btn { height: 44px; }
		.tab { height: 36px; }
		.fc, .clear { height: 44px; font-size: 15px; }
	}
	@media (max-width: 767px) {
		.toolbar { flex-direction: column; align-items: stretch; gap: 10px; }
		.tabs { width: 100%; }
		.tab { padding: 0 12px; }
		.right, .anchor, .sf-btn { width: 100%; margin-left: 0; }
		.badge { margin-left: auto; }
	}
	@media (prefers-reduced-motion: reduce) { .tab, .sf-btn { transition: none; } }
</style>
