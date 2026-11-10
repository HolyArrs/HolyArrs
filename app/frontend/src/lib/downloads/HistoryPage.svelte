<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// History page: a search bar, a live dot and one Filter (steps, type, who, dates, Source). Active filters show as removable chips. Routine work is left out. Every filter lives in the address, so Back and links show the same list. Old ?look= and ?acts= links are ignored.
	import Icon from '#lib/ui/Icon.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LookDays from './HistoryDays.svelte';
	import DetailActions from './HistoryActions.svelte';
	import { dateRangeProblem, readFilter, type HistoryFilter } from './history.ts';
	import { fineCheck } from './history-tabs.ts';
	import { clearChips, dirQuery, readSearch, searchChips, searchSteps, stepBreak, stepKinds, textMatches, toggleStep, typeLabels, typeMatches, whenFrom, whenOf, whenSpans, whoLabels, whoMatches, type MediaType, type Who } from './history-search.ts';
	import { dlTop } from './dl-top.svelte.ts';
	import { searchShortcut } from './keys.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { cubicIn, cubicOut } from 'svelte/easing';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '../core/api.ts';
	import { live } from '../core/live.svelte.ts';
	import { reducedMotion } from '../core/motion.ts';
	import type { EventRecord, TitleView } from '../core/types.ts';
	import { session } from '../core/session.svelte.ts';

	let { titleById, people, me }: { titleById: Map<number, TitleView>; people: Map<number, string>; me?: number } = $props();

	const st = $derived(readSearch(page.url.searchParams));
	// Owners and admins can narrow to one Source's grabs and pauses; feed checks stay hidden.
	const srcId = $derived(Number(page.url.searchParams.get('source')) || undefined);
	let sources = $state<{ id: number; name: string }[]>([]);
	const srcName = $derived(sources.find((x) => x.id === srcId)?.name ?? t('downloads.hpage.oneSource'));
	// Links from a title or episode page keep the list to that title (?title=, ?episode=).
	const link = $derived(readFilter(page.url.searchParams));
	let episodeName = $state('');
	const chips = $derived([
		...(link.title ? [{ key: 'title', label: titleById.get(link.title)?.name ?? t('downloads.hpage.oneTitle'), change: { title: undefined, episode: undefined } }] : []),
		...(link.episode ? [{ key: 'episode', label: episodeName || t('downloads.hpage.oneEpisode'), change: { episode: undefined } }] : []),
		...searchChips(st),
		...(srcId ? [{ key: 'source', label: srcName, change: { source: undefined } }] : [])
	]);
	const kinds = $derived(stepKinds(st.steps));
	const typeOptions = $derived<{ value: MediaType; label: string }[]>([{ value: '', label: t('downloads.hpage.all') }, ...(Object.entries(typeLabels) as [MediaType, string][]).map(([value, label]) => ({ value, label }))]);
	const when = $derived(whenOf(st.from, st.to));
	let customOpen = $state(false);
	const showDates = $derived(customOpen || when === 'custom');
	function pickWhen(days: number) {
		customOpen = false;
		set({ from: days ? whenFrom(days) : undefined, to: undefined });
	}
	// The server filters by title id, so text naming one Library title exactly is sent as that id.
	const exact = $derived.by(() => {
		const t = st.q.trim().toLowerCase();
		if (!t) return undefined;
		const hits = [...titleById.values()].filter((x) => x.name.toLowerCase() === t);
		return hits.length === 1 ? hits[0].id : undefined;
	});
	const filter = $derived<HistoryFilter>({ title: link.title ?? exact, episode: link.episode, from: st.from, to: st.to, source: srcId, hideFeed: true, groups: [], limit: 200, offset: 0 });
	const keep = $derived.by(() => {
		const { q, type, who } = st;
		// "Failed" asks for every Library folder check; checks that found the folder fine stay out.
		const fails = st.steps.includes('failed');
		if (!q.trim() && !type && !who && !fails) return undefined;
		return (e: EventRecord) => !(fails && fineCheck(e)) && (exact && !link.title ? true : textMatches(e, q, people)) && typeMatches(e.TitleID ? titleById.get(e.TitleID)?.kind : undefined, type) && whoMatches(e, who, me);
	});
	const problem = $derived(dateRangeProblem(st) ? t('downloads.hpage.dateProblem') : '');
	const filtered = $derived(chips.length > 0 || !!st.q.trim());
	const empty = $derived(
		problem
			? { title: problem, text: '' }
			: filtered
				? { title: t('downloads.hpage.nothingMatches'), text: '' }
				: { title: t('downloads.hpage.noEvents'), text: '' }
	);
	const clearAll = () => {
		text = '';
		set({ ...clearChips, q: undefined, title: undefined, episode: undefined });
	};
	const ms = (n: number) => (reducedMotion() ? 0 : n);

	function set(change: Record<string, string | undefined>) {
		void goto(page.url.pathname + dirQuery(page.url.searchParams, change), { replace: true, reset: false });
	}

	// The search box writes ?q= shortly after typing stops, and follows the address (Back, links).
	let text = $state(page.url.searchParams.get('q') ?? '');
	$effect(() => {
		const q = page.url.searchParams.get('q') ?? '';
		if (untrack(() => text.trim()) !== q) text = q;
	});
	let timer: ReturnType<typeof setTimeout> | undefined;
	function typed(v: string) {
		text = v;
		clearTimeout(timer);
		timer = setTimeout(() => set({ q: v.trim() || undefined }), 250);
	}
	// Ctrl K and / focus this page's own search, as on Downloads and Library; History has no shared top bar.
	let findEl = $state<HTMLInputElement>();
	function findKey(e: KeyboardEvent) {
		const t = e.target as HTMLElement | null;
		const inField = !!t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
		const inDialog = !!t?.closest?.('[role="dialog"], dialog, [popover]:popover-open') || !!document.querySelector('dialog[open]');
		// In the Filter or a sheet the shortcut does nothing, so it never leaves History for the app-wide search.
		if (inDialog && searchShortcut(e, inField, false)) {
			e.preventDefault();
			e.stopImmediatePropagation();
			return;
		}
		if (!searchShortcut(e, inField, inDialog)) return;
		e.preventDefault();
		e.stopImmediatePropagation();
		findEl?.focus();
		findEl?.select();
	}
	// At rest the top row sits on the page; once the list scrolls under it, it gets a frosted background and a hairline.
	let stuck = $state(false);
	$effect(() => {
		const on = () => (stuck = window.scrollY > 4);
		on();
		window.addEventListener('scroll', on, { passive: true });
		return () => window.removeEventListener('scroll', on);
	});
	$effect(() => {
		window.addEventListener('keydown', findKey, true);
		return () => window.removeEventListener('keydown', findKey, true);
	});

	onMount(() => {
		if (session.can('settings'))
			void api
				.get<{ sources: { id: number; name: string }[] }>('/sources')
				.then((r) => (sources = r.sources.map((x) => ({ id: x.id, name: x.name }))))
				.catch(() => {});
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
		// Runs after the sheet's inert is lifted.
		if (back) void tick().then(() => btn?.focus());
	}
	function keys(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		e.preventDefault();
		e.stopPropagation();
		close();
	}
	// The phone Filter sheet is modal: everything behind it is inert while it is open.
	$effect(() => {
		if (!open || !phone || !pop) return;
		const off: Element[] = [];
		for (let n: Element | null = pop; n && n !== document.body; n = n.parentElement)
			for (const sib of n.parentElement?.children ?? [])
				if (sib !== n && !sib.classList.contains('scrim') && !sib.hasAttribute('inert') && !/^(SCRIPT|STYLE)$/.test(sib.tagName)) {
					sib.setAttribute('inert', '');
					off.push(sib);
				}
		return () => off.forEach((x) => x.removeAttribute('inert'));
	});
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
	<div class="fl"><span class="fh" id="hf-step">{t('downloads.hpage.step')}</span>
		<div class="steps" role="group" aria-labelledby="hf-step">
			{#each searchSteps as s, i (s.key)}
				{#if i === stepBreak}<span class="brk" aria-hidden="true"></span>{/if}
				<button type="button" class="step" class:fail={s.key === 'failed'} aria-pressed={st.steps.includes(s.key)} onclick={() => set({ step: toggleStep(st.steps, s.key) })}>{s.label}</button>
			{/each}
		</div>
	</div>
	<div class="fl"><span class="fh">{t('downloads.hpage.type')}</span>
		<SegmentedControl size="sm" label={t('downloads.hpage.type')} options={typeOptions} value={st.type} onchange={(v) => set({ type: v })} testid="history-type" />
	</div>
	<div class="fl"><span class="fh" id="hf-who">{t('downloads.hpage.who')}</span>
		<div class="steps" role="group" aria-labelledby="hf-who">
			{#each Object.entries(whoLabels) as [k, v] (k)}<button type="button" class="step" aria-pressed={st.who === k} onclick={() => set({ who: k })}>{v}</button>{/each}
		</div>
	</div>
	<div class="fl"><span class="fh" id="hf-when">{t('downloads.hpage.when')}</span>
		<div class="steps" role="group" aria-labelledby="hf-when">
			{#each whenSpans as w (w.days)}<button type="button" class="step" aria-pressed={!showDates && when === w.days} onclick={() => pickWhen(w.days)}>{w.label}</button>{/each}
			<button type="button" class="step" aria-pressed={showDates} onclick={() => (customOpen = true)}>{t('downloads.hpage.custom')}</button>
		</div>
		{#if showDates}
			<div class="two">
				<label class="fl"><span class="sr-only">{t('downloads.hpage.from')}</span><input class="fc" type="date" aria-label={t('downloads.hpage.from')} value={st.from ?? ''} max={st.to} onchange={(e) => set({ from: e.currentTarget.value })} /></label>
				<label class="fl"><span class="sr-only">{t('downloads.hpage.to')}</span><input class="fc" type="date" aria-label={t('downloads.hpage.to')} value={st.to ?? ''} min={st.from} onchange={(e) => set({ to: e.currentTarget.value })} /></label>
			</div>
		{/if}
	</div>
	{#if sources.length}
		<label class="fl"><span class="fh">{t('downloads.hpage.source')}</span>
			<select class="fc" value={srcId ?? ''} onchange={(e) => set({ source: e.currentTarget.value || undefined })}>
				<option value="">{t('downloads.hpage.all')}</option>
				{#each sources as x (x.id)}<option value={x.id}>{x.name}</option>{/each}
			</select>
		</label>
	{/if}
	{#if problem}<p class="bad" role="alert">{problem}</p>{/if}
	{#if chips.length}<button type="button" class="clear" onclick={() => clearAll()}>{t('downloads.hpage.clear')}</button>{/if}
{/snippet}

<h1 class="sr-only">{t('downloads.hpage.history')}</h1>
<div class="toolbar" class:stuck data-testid="history-search-bar">
	{#if dlTop.collapsed && !phone}<span class="cap"><IconButton icon="sidebar" label={t('downloads.bar.showSidebar')} onclick={() => dlTop.toggle()} data-testid="history-side-btn" /></span>{/if}
	<label class="search">
		<span class="mag" aria-hidden="true"><Icon name="search" size={16} /></span>
		<span class="sr-only">{t('downloads.hpage.search')}</span>
		<input bind:this={findEl} type="search" placeholder={t('downloads.hpage.searchPh')} value={text} oninput={(e) => typed(e.currentTarget.value)} />
		<span class="live" class:off={live.status.state !== 'live'} title={live.status.state === 'live' ? t('downloads.hpage.updates') : t('downloads.hpage.reconnecting')}><span class="dot" aria-hidden="true"></span>{t('downloads.hpage.live')}</span>
		{#if !phone}<kbd class="kb" aria-hidden="true">{dlTop.searchKey}</kbd>{/if}
	</label>
	<div class="right">
		<div class="anchor">
			<button type="button" class="sf-btn" bind:this={btn} aria-haspopup="dialog" aria-expanded={open} aria-label={chips.length ? t('downloads.hpage.filterOn', { count: chips.length }) : t('downloads.hpage.filter')} onclick={toggle}>
				<svg class="funnel" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M2.5 3.5h11l-4.2 5v4l-2.6 1.2V8.5z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" /></svg>
				<span>{t('downloads.hpage.filter')}</span>
				{#if chips.length}<span class="badge" aria-hidden="true">{chips.length}</span>{/if}
			</button>
			{#if open && !phone}
				<div class="menu" role="dialog" aria-label={t('downloads.hpage.filter')} tabindex="-1" bind:this={pop} onkeydown={keys} in:fly={{ y: -8, duration: ms(280), easing: cubicOut }} out:fade={{ duration: ms(180) }}>
					{@render fields()}
				</div>
			{/if}
		</div>
	</div>
</div>
{#if chips.length}
	<div class="chips" data-testid="history-chips">
		{#each chips as c (c.key)}
			<button type="button" class="chip" aria-label={t('alerts.panel.remove', { text: c.label })} onclick={() => set(c.change)}>{c.label}<Icon name="x" size={12} /></button>
		{/each}
		<button type="button" class="all" onclick={clearAll}>{t('downloads.bar.clear')}</button>
	</div>
{/if}
{#if open && phone}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="scrim" aria-hidden="true" in:fade={{ duration: ms(300) }} out:fade={{ duration: ms(200) }} onclick={() => close()}></div>
	<div class="sheet" role="dialog" aria-modal="true" aria-label={t('downloads.hpage.filter')} tabindex="-1" bind:this={pop} onkeydown={keys} in:fly={{ y: 80, duration: ms(420), easing: cubicOut }} out:fly={{ y: 80, duration: ms(200), easing: cubicIn }}>
		<div class="sh"><span class="sht">{t('downloads.hpage.filter')}</span><button type="button" class="done" onclick={() => close()}><Icon name="check" size={16} />{t('downloads.hpage.done')}</button></div>
		{@render fields()}
	</div>
{/if}

<LookDays {filter} {kinds} {titleById} {empty} onall={filtered ? clearAll : undefined} size="m" {wide} bind:panel panelId="history-search-details" {people} {me} {keep} bind:episodeName compact>
	{#snippet actions(row)}<DetailActions {row} {titleById} />{/snippet}
</LookDays>

<style>
	.toolbar { position: sticky; top: 0; z-index: 6; display: flex; align-items: center; gap: 8px; height: 72px; box-sizing: border-box; margin: calc(-1 * var(--s-6, 24px)) calc(-1 * var(--gutter, 0px)) 4px; padding: 10px var(--gutter, 0px); background: transparent; border-bottom: 1px solid transparent; transition: background var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out); }
	.toolbar.stuck { background: color-mix(in srgb, var(--bg) 80%, transparent); -webkit-backdrop-filter: blur(20px) saturate(1.4); backdrop-filter: blur(20px) saturate(1.4); border-bottom-color: var(--line); }
	.cap { display: flex; flex: none; align-items: center; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.cap :global(.ui-ibtn) { width: 44px; height: 44px; border-radius: 10px; }
	.search { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; height: 52px; box-sizing: border-box; padding: 0 12px 0 15px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text-muted); -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); transition: border-color 220ms var(--ease-out), background 220ms var(--ease-out); }
	.search:hover { border-color: color-mix(in srgb, var(--line-strong) 60%, var(--line)); }
	.search:focus-within { border-color: var(--line-strong); background: color-mix(in srgb, var(--surface-1) 86%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.08), 0 0 0 3px color-mix(in srgb, var(--text) 6%, transparent), 0 8px 24px -12px rgb(0 0 0 / 0.4); }
	.mag { display: grid; place-items: center; flex: none; }
	.search:focus-within .mag { color: var(--text); }
	.kb { flex: none; padding: 2px 7px; border: 1px solid var(--line); border-radius: 6px; color: var(--text-muted); font: 500 11px/1.4 var(--font-ui); white-space: nowrap; }
	.search input { flex: 1; min-width: 0; height: 100%; padding: 0; border: 0; background: none; color: var(--text); font: var(--t-callout); outline: none; }
	.search input::placeholder { color: var(--text-muted); }
	.live { flex: none; display: inline-flex; align-items: center; gap: 5px; color: var(--text-muted); font: 500 12px/1 var(--font-ui); }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--ok); }
	.live.off .dot { background: var(--text-muted); }
	.right { flex: none; display: flex; align-items: center; gap: 8px; }
	.anchor { position: relative; }
	.sf-btn { display: inline-flex; align-items: center; gap: 7px; height: 52px; padding: 0 18px; box-sizing: border-box; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); white-space: nowrap; cursor: pointer; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: background 160ms var(--ease-out); }
	.sf-btn:hover, .sf-btn[aria-expanded='true'] { background: var(--surface-2); }
	.sf-btn:focus-visible, .clear:focus-visible, .done:focus-visible, .chip:focus-visible, .all:focus-visible, .step:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.funnel { color: var(--text-muted); }
	.badge { min-width: 18px; height: 18px; padding: 0 5px; box-sizing: border-box; border-radius: 9px; background: var(--info); color: var(--bg); font: 600 11px/18px var(--font-ui); text-align: center; }
	.chips { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin: 0 0 14px; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 10px 0 12px; border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-1); color: var(--text); font: 500 12.5px/1 var(--font-ui); cursor: pointer; }
	.chip:hover { background: var(--surface-2); }
	.all { height: 30px; padding: 0 10px; border: 0; background: none; color: var(--text-muted); font: 500 12.5px/1 var(--font-ui); cursor: pointer; }
	.all:hover { color: var(--text); }
	.menu, .sheet { --sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f6f7fa, #17171a); background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); }
	.menu { position: absolute; right: 0; top: calc(100% + 8px); z-index: 20; width: 320px; max-height: min(600px, 78vh); overflow-y: auto; display: flex; flex-direction: column; gap: 14px; padding: 14px; box-sizing: border-box; border: 0; border-radius: 14px; box-shadow: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 7%)), 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 24px 48px -16px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%)); }
	.menu:focus, .sheet:focus { outline: none; }
	.fl { display: flex; flex-direction: column; gap: 7px; min-width: 0; }
	.fh { color: var(--text-muted); font: 600 12px/1 var(--font-ui); }
	.brk { flex-basis: 100%; height: 4px; }
	.steps { display: flex; flex-wrap: wrap; gap: 5px; }
	.step { height: 30px; padding: 0 11px; border: 1px solid var(--line); border-radius: var(--r-pill); background: var(--surface-1); color: var(--text); font: 500 12.5px/1 var(--font-ui); cursor: pointer; }
	.step[aria-pressed='true'] { background: var(--seg-thumb); border-color: var(--line-strong); box-shadow: 0 0 0 1px var(--line-strong); }
	.step.fail[aria-pressed='true'] { color: var(--fail-text); box-shadow: 0 0 0 1px var(--fail); }
	.fc { width: 100%; min-width: 0; height: 30px; box-sizing: border-box; padding: 0 10px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface-1); color: var(--text); font: 500 13px/1 var(--font-ui); color-scheme: light dark; }
	.fc:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.two { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
	.bad { margin: 0; color: var(--fail-text); font: var(--t-footnote); }
	.clear { align-self: flex-start; height: 34px; padding: 0 12px; border: 0; border-radius: var(--r-pill); background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); cursor: pointer; }
	.scrim { position: fixed; inset: 0; z-index: 96; background: color-mix(in srgb, var(--bg) 55%, transparent); }
	.sheet { position: fixed; left: 8px; right: 8px; bottom: calc(var(--tabbar) + 14px + env(safe-area-inset-bottom, 0px)); z-index: 97; max-height: calc(100dvh - var(--tabbar) - env(safe-area-inset-bottom, 0px) - 62px); overflow-y: auto; display: flex; flex-direction: column; gap: 14px; box-sizing: border-box; padding: 10px 16px 16px; border: 0; border-radius: 18px; box-shadow: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 7%)), 0 -20px 50px light-dark(rgb(20 24 40 / 22%), rgb(0 0 0 / 55%)); }
	.sh { display: flex; align-items: center; justify-content: space-between; }
	.sht { color: var(--text); font: 600 15px/1.2 var(--font-ui); }
	.done { display: inline-flex; align-items: center; gap: 6px; min-height: 44px; padding: 0 14px; border: 0; border-radius: var(--r-pill); background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	@media (pointer: coarse) {
		.fc, .clear, .chip, .all, .step { height: 44px; }
		.search input, .fc { font-size: 16px; }
	}
	@media (max-width: 767px) {
		.toolbar { position: static; height: auto; flex-wrap: wrap; margin: 0 0 12px; padding: 0; background: none; }
		.toolbar.stuck { background: none; -webkit-backdrop-filter: none; backdrop-filter: none; border-bottom-color: transparent; }
		.search, .sf-btn { height: 44px; }
		.search { flex-basis: 100%; }
		.right { width: 100%; }
		.anchor { flex: 1; }
		.sf-btn { width: 100%; }
		.badge { margin-left: auto; }
	}
	@media (prefers-reduced-motion: reduce) { .sf-btn { transition: none; } }
</style>
