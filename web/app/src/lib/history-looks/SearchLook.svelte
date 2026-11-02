<script lang="ts">
	// Looks E and F (?look=pill|plain): no tabs, a search bar, a live dot and
	// one glass Filter (steps, type, who, dates). Each filter that is on shows
	// as a removable chip under the bar. Look E adds a fixed "Failed n" pill
	// for problems of the last 24 hours that toggles the Failed step. Routine
	// work is left out; the list is the same day-grouped timeline.
	import { onMount, tick } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { cubicIn, cubicOut } from 'svelte/easing';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import Icon from '../Icon.svelte';
	import LookDays from './LookDays.svelte';
	import RowActions from './RowActions.svelte';
	import { api } from '../api.ts';
	import { live } from '../live.svelte.ts';
	import { dateRangeProblem, type HistoryFilter } from '../history.ts';
	import { eventsQuery, settle } from '../history-views.ts';
	import { tabKinds } from '../history-looks.ts';
	import { reducedMotion } from '../motion.ts';
	import { clearChips, dirQuery, readSearch, searchChips, searchSteps, stepKinds, textMatches, toggleStep, typeLabels, typeMatches, whoLabels, whoMatches, type MediaType, type Who } from './dirs.ts';
	import type { EventPage, EventRecord, TitleView } from '../types.ts';

	let { titleById, people, me, pill = false }: { titleById: Map<number, TitleView>; people: Map<number, string>; me?: number; pill?: boolean } = $props();

	const st = $derived(readSearch(page.url.searchParams));
	const chips = $derived(searchChips(st));
	const kinds = $derived(stepKinds(st.steps));
	// The server filters by title id: text that names one Library title exactly goes there.
	const exact = $derived.by(() => {
		const t = st.q.trim().toLowerCase();
		if (!t) return undefined;
		const hits = [...titleById.values()].filter((x) => x.name.toLowerCase() === t);
		return hits.length === 1 ? hits[0].id : undefined;
	});
	const filter = $derived<HistoryFilter>({ title: exact, from: st.from, to: st.to, groups: [], limit: 200, offset: 0 });
	const keep = $derived.by(() => {
		const { q, type, who } = st;
		if (!q.trim() && !type && !who) return undefined;
		return (e: EventRecord) => (exact ? true : textMatches(e, q, people)) && typeMatches(e.TitleID ? titleById.get(e.TitleID)?.kind : undefined, type) && whoMatches(e, who, me);
	});
	const empty = $derived({ title: chips.length || st.q ? 'Nothing matches' : 'No history yet.', text: 'Every download step, newest first. Routine work is left out.' });
	const problem = $derived(dateRangeProblem(st));
	const ms = (n: number) => (reducedMotion() ? 0 : n);

	function set(change: Record<string, string | undefined>) {
		void goto(page.url.pathname + dirQuery(page.url.searchParams, change), { replace: true, reset: false });
	}

	// The search box writes ?q= a moment after typing stops.
	let text = $state(page.url.searchParams.get('q') ?? '');
	let timer: ReturnType<typeof setTimeout> | undefined;
	function typed(v: string) {
		text = v;
		clearTimeout(timer);
		timer = setTimeout(() => set({ q: v.trim() || undefined }), 250);
	}

	// Look E: problems of the last 24 hours.
	let failed = $state(0);
	const failedOn = $derived(st.steps.includes('failed'));
	async function countFailed() {
		const since = Date.now() - 86400000;
		try {
			const p = await api.get<EventPage>('/events?' + eventsQuery({ from: new Date(since).toISOString().slice(0, 10), groups: [], limit: 200, offset: 0 }, { kinds: tabKinds('problems') }));
			failed = p.events.filter((e) => Date.parse(e.At) >= since).length;
		} catch {
			/* keep the last count */
		}
	}
	onMount(() => {
		if (!pill) return;
		void countFailed();
		const s = settle(() => void countFailed(), 2000);
		const offs = [live.on('event', s.poke), live.on('resync', s.poke)];
		return () => {
			offs.forEach((o) => o());
			s.stop();
		};
	});

	let wide = $state(false);
	let phone = $state(false);
	let panel = $state(true);
	onMount(() => {
		const w = window.matchMedia('(min-width: 1024px)');
		const p = window.matchMedia('(max-width: 767px)');
		const fit = () => ((wide = w.matches), (phone = p.matches));
		fit();
		w.addEventListener('change', fit);
		p.addEventListener('change', fit);
		return () => {
			w.removeEventListener('change', fit);
			p.removeEventListener('change', fit);
		};
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
	<div class="fl"><span class="fh">Step</span>
		<div class="steps">
			{#each searchSteps as s (s.key)}
				<button type="button" class="step" class:fail={s.key === 'failed'} aria-pressed={st.steps.includes(s.key)} onclick={() => set({ step: toggleStep(st.steps, s.key) })}>{s.label}</button>
			{/each}
		</div>
	</div>
	<label class="fl"><span class="fh">Type</span>
		<select class="fc" value={st.type} onchange={(e) => set({ type: e.currentTarget.value })}>
			<option value="">Every type</option>
			{#each Object.entries(typeLabels) as [k, v] (k)}<option value={k as MediaType}>{v}</option>{/each}
		</select>
	</label>
	<label class="fl"><span class="fh">Who</span>
		<select class="fc" value={st.who} onchange={(e) => set({ who: e.currentTarget.value })}>
			{#each Object.entries(whoLabels) as [k, v] (k)}<option value={k as Who}>{v}</option>{/each}
		</select>
	</label>
	<div class="two">
		<label class="fl"><span class="fh">From</span><input class="fc" type="date" value={st.from ?? ''} max={st.to} onchange={(e) => set({ from: e.currentTarget.value })} /></label>
		<label class="fl"><span class="fh">To</span><input class="fc" type="date" value={st.to ?? ''} min={st.from} onchange={(e) => set({ to: e.currentTarget.value })} /></label>
	</div>
	{#if problem}<p class="bad" role="alert">{problem}</p>{/if}
	{#if chips.length}<button type="button" class="clear" onclick={() => set(clearChips)}>Clear filters</button>{/if}
{/snippet}

<h1 class="sr-only">History</h1>
<div class="toolbar" data-testid="history-search-bar">
	<label class="search">
		<Icon name="search" size={16} />
		<span class="sr-only">Search</span>
		<input type="search" placeholder="Search titles, people, sources…" value={text} oninput={(e) => typed(e.currentTarget.value)} />
		<span class="live" class:off={live.status.state !== 'live'} title={live.status.state === 'live' ? 'Updates as they happen' : 'Reconnecting'}><span class="dot" aria-hidden="true"></span>Live</span>
	</label>
	<div class="right">
		<div class="anchor">
			<button type="button" class="sf-btn" bind:this={btn} aria-haspopup="dialog" aria-expanded={open} aria-label="Filter{chips.length ? `, ${chips.length} on` : ''}" onclick={toggle}>
				<svg class="funnel" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M2.5 3.5h11l-4.2 5v4l-2.6 1.2V8.5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" /></svg>
				<span>Filter</span>
				{#if chips.length}<span class="badge" aria-hidden="true">{chips.length}</span>{/if}
			</button>
			{#if open && !phone}
				<div class="menu" role="dialog" aria-label="Filter" tabindex="-1" bind:this={pop} onkeydown={keys} in:fly={{ y: -8, duration: ms(280), easing: cubicOut }} out:fade={{ duration: ms(180) }}>
					{@render fields()}
				</div>
			{/if}
		</div>
		{#if pill}
			<button type="button" class="failed" class:hot={failed > 0} aria-pressed={failedOn} aria-label="Failed in the last 24 hours: {failed}" data-testid="history-failed-pill" onclick={() => set({ step: toggleStep(st.steps, 'failed') })}>Failed <b>{failed}</b></button>
		{/if}
	</div>
</div>
{#if chips.length}
	<div class="chips" data-testid="history-chips">
		{#each chips as c (c.key)}
			<button type="button" class="chip" aria-label="Remove {c.label}" onclick={() => set(c.change)}>{c.label}<Icon name="x" size={12} /></button>
		{/each}
		<button type="button" class="all" onclick={() => set(clearChips)}>Clear all</button>
	</div>
{/if}
{#if open && phone}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="scrim" aria-hidden="true" in:fade={{ duration: ms(300) }} out:fade={{ duration: ms(200) }} onclick={() => close()}></div>
	<div class="sheet" role="dialog" aria-modal="true" aria-label="Filter" tabindex="-1" bind:this={pop} onkeydown={keys} in:fly={{ y: 80, duration: ms(420), easing: cubicOut }} out:fly={{ y: 80, duration: ms(200), easing: cubicIn }}>
		<div class="sh"><span class="sht">Filter</span><button type="button" class="done" onclick={() => close()}><Icon name="check" size={16} />Done</button></div>
		{@render fields()}
	</div>
{/if}

<LookDays {filter} {kinds} {titleById} {empty} size="m" {wide} bind:panel panelId="history-search-details" {people} {me} {keep}>
	{#snippet actions(row)}<RowActions {row} />{/snippet}
</LookDays>

<style>
	.toolbar { display: flex; align-items: center; gap: 10px; margin: 0 0 12px; }
	.search { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; height: 40px; box-sizing: border-box; padding: 0 12px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text-muted); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.search:focus-within { box-shadow: var(--focus-ring); }
	.search input { flex: 1; min-width: 0; height: 100%; padding: 0; border: 0; background: none; color: var(--text); font: 500 14px/1 var(--font-ui); outline: none; }
	.search input::placeholder { color: var(--text-muted); }
	.live { flex: none; display: inline-flex; align-items: center; gap: 5px; color: var(--text-muted); font: 500 12px/1 var(--font-ui); }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); }
	.live.off .dot { background: var(--text-muted); }
	.right { flex: none; display: flex; align-items: center; gap: 8px; }
	.anchor { position: relative; }
	.sf-btn, .failed { display: inline-flex; align-items: center; gap: 7px; height: 40px; padding: 0 14px; box-sizing: border-box; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: background 160ms var(--ease-out); }
	.sf-btn:hover, .sf-btn[aria-expanded='true'] { background: var(--surface-2); }
	.failed { color: var(--text-muted); opacity: 0.7; }
	.failed b { font-weight: 600; font-variant-numeric: tabular-nums; }
	.failed.hot { opacity: 1; color: var(--fail-text); border-color: color-mix(in srgb, var(--fail) 40%, var(--line)); background: color-mix(in srgb, var(--fail) 10%, var(--surface-1)); }
	.failed[aria-pressed='true'] { opacity: 1; box-shadow: 0 0 0 1px var(--fail); }
	.sf-btn:focus-visible, .failed:focus-visible, .clear:focus-visible, .done:focus-visible, .chip:focus-visible, .all:focus-visible, .step:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.funnel { color: var(--text-muted); }
	.badge { min-width: 18px; height: 18px; padding: 0 5px; box-sizing: border-box; border-radius: 9px; background: var(--info); color: var(--bg); font: 600 11px/18px var(--font-ui); text-align: center; }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin: 0 0 14px; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 10px 0 12px; border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-1); color: var(--text); font: 500 12.5px/1 var(--font-ui); cursor: pointer; }
	.chip:hover { background: var(--surface-2); }
	.all { height: 30px; padding: 0 10px; border: 0; background: none; color: var(--text-muted); font: 500 12.5px/1 var(--font-ui); cursor: pointer; }
	.all:hover { color: var(--text); }
	.menu { position: absolute; right: 0; top: calc(100% + 6px); z-index: 20; width: 320px; max-height: min(560px, 75vh); overflow-y: auto; display: flex; flex-direction: column; gap: 10px; padding: 12px; border: 1px solid var(--line-strong); border-radius: 12px; background: var(--surface-2); box-shadow: 0 18px 40px -12px rgb(0 0 0 / 0.55); }
	.menu:focus, .sheet:focus { outline: none; }
	.fl { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
	.fh { color: var(--text-muted); font: 600 11px/1 var(--font-ui); letter-spacing: 0.06em; text-transform: uppercase; }
	.steps { display: flex; flex-wrap: wrap; gap: 5px; }
	.step { height: 30px; padding: 0 10px; border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-1); color: var(--text); font: 500 12.5px/1 var(--font-ui); cursor: pointer; }
	.step[aria-pressed='true'] { background: var(--seg-thumb); border-color: var(--line-strong); box-shadow: 0 0 0 1px var(--line-strong); }
	.step.fail[aria-pressed='true'] { color: var(--fail-text); box-shadow: 0 0 0 1px var(--fail); }
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
		.search, .sf-btn, .failed, .fc, .clear, .chip, .all, .step { height: 44px; }
		.search input, .fc { font-size: 16px; }
	}
	@media (max-width: 767px) {
		.toolbar { flex-wrap: wrap; }
		.search { flex-basis: 100%; }
		.right { width: 100%; }
		.anchor { flex: 1; }
		.sf-btn { width: 100%; }
		.badge { margin-left: auto; }
	}
	@media (prefers-reduced-motion: reduce) { .sf-btn, .failed { transition: none; } }
</style>
