<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import Poster from '#lib/Poster.svelte';
	import Icon from '#lib/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import HistoryToolbar from './HistoryToolbar.svelte';
	import VerbMark from './VerbMark.svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import { createLatestOnly, dateRangeProblem, details, groupDownloads, groupLabel, pageLine, type HistoryFilter } from '#lib/history.ts';
	import { artOf, byWhom, eventsQuery, groupsOfTier, headVerb, settle, tierKinds, tiers, titleHref, verbOf, type Tier } from '#lib/history-views.ts';
	import type { EventPage, EventRecord, TitleView } from '#lib/types.ts';

	// 4 Important: the history in three piles with their counts. Important
	// (imported, failed, deleted, needs a decision, health) opens first;
	// Changes and Routine are one tap away. The kind menu offers the kinds of
	// the open pile.
	let { filter, titles, titleById, onfilter, phone }: { filter: HistoryFilter; titles: TitleView[]; titleById: Map<number, TitleView>; onfilter: (c: Partial<HistoryFilter>) => void; phone: boolean } = $props();

	const LIMIT = 50;
	let tier = $state<Tier>('important');
	let offset = $state(0);
	let events = $state<EventRecord[]>([]);
	let total = $state(0);
	let counts = $state<Record<Tier, number | null>>({ important: null, changes: null, routine: null });
	let loaded = $state(false);
	let error = $state('');
	let open = $state<string | null>(null);
	const latest = createLatestOnly();
	const latestCounts = createLatestOnly();

	const offered = (t: Tier) => groupsOfTier(t).filter((g) => !(t === 'changes' && g === 'import'));
	const kindsFor = (t: Tier) => tierKinds(t, filter.groups.filter((g) => offered(t).includes(g)));

	async function load() {
		const ok = latest();
		if (dateRangeProblem(filter)) { loaded = true; return; }
		const kinds = kindsFor(tier);
		if (!kinds.length) { events = []; total = 0; loaded = true; return; }
		try {
			const p = await api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit: LIMIT, offset }));
			if (!ok()) return;
			events = p.events;
			total = p.total;
			error = '';
		} catch (e) {
			if (ok()) error = message(e);
		} finally {
			if (ok()) loaded = true;
		}
	}
	async function count() {
		const ok = latestCounts();
		if (dateRangeProblem(filter)) return;
		const got = await Promise.all(tiers.map((t) => {
			const kinds = kindsFor(t.key);
			return kinds.length ? api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit: 1, offset: 0 })).then((p) => p.total).catch(() => null) : Promise.resolve(0);
		}));
		if (!ok()) return;
		counts = { important: got[0], changes: got[1], routine: got[2] };
	}
	$effect(() => {
		void [filter.title, filter.episode, filter.from, filter.to, filter.groups.join()];
		untrack(() => void count());
	});
	$effect(() => {
		void [filter.title, filter.episode, filter.from, filter.to, filter.groups.join(), tier];
		untrack(() => { offset = 0; open = null; void load(); });
	});
	const fresh = settle(() => { void count(); if (offset === 0) void load(); }, 2000);
	onMount(() => {
		const offs = [live.on('event', fresh.poke), live.on('resync', fresh.poke)];
		return () => { offs.forEach((o) => o()); fresh.stop(); };
	});
	function page(n: number) {
		offset = Math.max(0, n);
		open = null;
		void load();
		document.getElementById('hv-tiers')?.scrollIntoView({ block: 'nearest' });
	}
	function tabKey(e: KeyboardEvent, i: number) {
		const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
		if (!step) return;
		e.preventDefault();
		const next = tiers[(i + step + tiers.length) % tiers.length];
		tier = next.key;
		document.getElementById(`hv-tab-${next.key}`)?.focus();
	}
	const rows = $derived(groupDownloads(events));
	const filtered = $derived(!!(filter.title || filter.groups.length || filter.from || filter.to));
	const here = $derived(tiers.find((t) => t.key === tier)!);
</script>

<div class="wrap">
	<HistoryToolbar {filter} {titles} {onfilter} {phone} groups={offered(tier)} />

	<div class="tiers" role="tablist" aria-label="How much it matters" id="hv-tiers" data-testid="hv-tiers">
		{#each tiers as t, i (t.key)}
			<button type="button" role="tab" id="hv-tab-{t.key}" aria-selected={tier === t.key} aria-controls="hv-pile" tabindex={tier === t.key ? 0 : -1} class="tab {t.key}" onclick={() => (tier = t.key)} onkeydown={(e) => tabKey(e, i)}>
				<span class="t-label">{t.label}</span>
				<span class="t-count" data-testid="hv-count-{t.key}">{counts[t.key] === null ? '–' : counts[t.key]!.toLocaleString()}</span>
				<span class="t-hint">{t.hint}</span>
			</button>
		{/each}
	</div>

	<div id="hv-pile" role="tabpanel" aria-labelledby="hv-tab-{tier}" class="pile {tier}">
		{#if error && !events.length}
			<LoadError hasData={false} message="History could not be loaded." onretry={load} />
		{:else if !loaded}
			<div class="list">{#each Array(6) as _, i (i)}<Skeleton height="56px" />{/each}</div>
		{:else if !events.length}
			<EmptyState icon={tier === 'important' ? 'check' : 'clock'} title={filtered ? 'Nothing matches' : tier === 'important' ? 'Nothing needs a look' : `No ${here.label.toLowerCase()} yet`} text={filtered ? `No ${here.label} Event matches these filters.` : here.hint + '.'} action={filtered ? { label: 'Show everything', onclick: () => onfilter({ title: undefined, groups: [], from: undefined, to: undefined }) } : undefined} />
		{:else}
			<p class="count" data-testid="history-count">{pageLine(total, offset, events.length)} in {here.label}</p>
			<ol class="list" data-testid="hv-pile-list">
				{#each rows as r (r.key)}
					{@const e = r.head}
					{@const verb = headVerb(r)}
					{@const href = titleHref(e.TitleID, titleById)}
					{@const t = titleById.get(e.TitleID ?? 0)}
					{@const why = r.steps.length ? [] : details(e)}
					{@const more = r.events.length > 1 || why.length > 0}
					<li class="hv-row {verb.tone}" data-kind={e.Kind}>
						<div class="r-line">
							<span class="r-art" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="r-ic"><Icon name={verb.icon} size={16} /></span>{/if}</span>
							<div class="r-main">
								<p class="r-title">{#if e.title_name}{#if href}<a href={to(href)}>{e.title_name}</a>{:else}{e.title_name}{/if}{#if e.episode}<span class="r-ep"> {e.episode}</span>{/if}{:else}{groupLabel(e.Kind)}{/if}</p>
								<p class="r-sum">{e.Summary}</p>
								<p class="r-meta"><VerbMark {verb} size="sm" /><span>·</span><time datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time><span>·</span><span>{byWhom(e)}</span></p>
							</div>
							{#if more}<button type="button" class="r-more" aria-expanded={open === r.key} aria-label="Why and where" onclick={() => (open = open === r.key ? null : r.key)}><Icon name="chevronDown" size={16} /></button>{/if}
						</div>
						{#if open === r.key}
							<div class="why">
								{#if r.events.length > 1}
									<ol class="steps">{#each r.events as s (s.ID)}<li><VerbMark verb={verbOf(s.Kind)} size="sm" /><span class="s-sum">{s.Summary}</span><time datetime={s.At}>{ago(s.At)}</time></li>{/each}</ol>
								{/if}
								{#if why.length}<dl>{#each why as [k, v], i (i)}<dt>{k}</dt><dd>{v}</dd>{/each}</dl>{/if}
							</div>
						{/if}
					</li>
				{/each}
			</ol>
			<nav class="pages" aria-label="Pages">
				<Button variant="secondary" icon="chevronLeft" disabled={offset === 0} onclick={() => page(offset - LIMIT)}>Newer</Button>
				<Button variant="secondary" iconAfter="chevronRight" disabled={offset + events.length >= total} onclick={() => page(offset + LIMIT)}>Older</Button>
			</nav>
		{/if}
	</div>
</div>

<style>
	.wrap { display: flex; flex-direction: column; gap: 18px; min-width: 0; }
	.tiers { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; scroll-margin-top: calc(var(--topbar, 0px) + 12px); }
	.tab { position: relative; display: grid; grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'label count' 'hint hint'; gap: 2px 10px; padding: 14px 16px; border-radius: 16px; background: var(--surface-1); text-align: left; box-shadow: 0 0 0 1px var(--line); transition: box-shadow var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.tab:hover { background: var(--surface-2); }
	.tab[aria-selected='true'] { box-shadow: 0 0 0 2px var(--text), var(--shadow-card); background: var(--surface-1); }
	.tab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.t-label { grid-area: label; font: var(--t-headline); }
	.t-count { grid-area: count; font: var(--t-title2); font-variant-numeric: tabular-nums; letter-spacing: -.01em; }
	.important .t-count { color: var(--accent-text); }
	.routine .t-count, .routine .t-label { color: var(--text-muted); }
	.t-hint { grid-area: hint; color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.count { margin-bottom: 8px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.list { display: flex; flex-direction: column; margin: 0; padding: 6px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), var(--shadow-card); }
	.hv-row { position: relative; padding: 10px; border-radius: var(--r-btn); }
	.hv-row + .hv-row { box-shadow: 0 -1px 0 var(--line); }
	.pile.important .hv-row.fail::before, .pile.important .hv-row.ok::before, .pile.important .hv-row.wait::before { content: ''; position: absolute; left: 0; top: 12px; bottom: 12px; width: 3px; border-radius: 2px; background: var(--ok); }
	.pile.important .hv-row.fail::before { background: var(--fail); }
	.pile.important .hv-row.wait::before { background: var(--wait); }
	.r-line { display: flex; align-items: flex-start; gap: 12px; }
	.r-art { flex: none; width: 40px; --poster-ratio: 2 / 3; border-radius: 6px; overflow: hidden; }
	.r-art.square { --poster-ratio: 1; }
	.r-ic { display: grid; place-items: center; aspect-ratio: 1; border-radius: 50%; background: var(--surface-2); color: var(--text-muted); }
	.r-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
	.r-title { font: var(--t-callout); font-weight: 600; overflow-wrap: anywhere; }
	.r-title a { color: var(--text); }
	.r-title a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.r-ep { color: var(--text-muted); font-weight: 400; }
	.r-sum { color: var(--text); font: var(--t-callout); overflow-wrap: anywhere; }
	.r-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px; color: var(--text-muted); font: var(--t-footnote); }
	.r-more { flex: none; display: grid; place-items: center; width: 36px; height: 36px; border-radius: 50%; color: var(--text-muted); }
	.r-more:hover { background: var(--surface-2); color: var(--text); }
	.r-more[aria-expanded='true'] { transform: rotate(180deg); }
	.pile.routine .hv-row { padding: 6px 10px; }
	.pile.routine .r-art { width: 24px; }
	.pile.routine .r-sum { color: var(--text-muted); font: var(--t-footnote); }
	.why { margin: 10px 0 2px 52px; padding: 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-footnote); }
	.steps { display: flex; flex-direction: column; gap: 8px; margin: 0 0 8px; padding: 0; list-style: none; }
	.steps li { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 10px; }
	.s-sum { flex: 1 1 200px; min-width: 0; overflow-wrap: anywhere; }
	.steps time { color: var(--text-muted); }
	.why dl { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 6px 14px; margin: 0; }
	.why dt { color: var(--text-muted); }
	.why dd { margin: 0; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.pages { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
	@media (max-width: 767px) {
		.tiers { position: sticky; top: var(--topbar, 0px); z-index: 4; gap: 4px; margin: 0 -4px; padding: 4px; border-radius: 14px; background: color-mix(in srgb, var(--surface-2) 86%, transparent); -webkit-backdrop-filter: blur(16px); backdrop-filter: blur(16px); }
		.tab { grid-template-columns: minmax(0, 1fr); grid-template-areas: 'count' 'label'; justify-items: center; padding: 8px 4px; border-radius: 10px; background: none; box-shadow: none; text-align: center; }
		.tab[aria-selected='true'] { background: var(--surface-1); box-shadow: var(--shadow-card); }
		.t-count { font: var(--t-headline); }
		.t-label { font: var(--t-footnote); font-weight: 600; }
		.t-hint { display: none; }
		.list { padding: 2px; }
		.why { margin-left: 0; }
		.why dl { grid-template-columns: minmax(0, 1fr); gap: 2px; }
		.why dd + dt { margin-top: 6px; }
		.r-more { width: 44px; height: 44px; }
		.pages > :global(*) { flex: 1; min-height: 44px; }
	}
</style>
