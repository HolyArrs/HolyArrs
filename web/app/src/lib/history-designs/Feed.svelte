<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import Poster from '#lib/Poster.svelte';
	import Icon from '#lib/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import HistoryToolbar from './HistoryToolbar.svelte';
	import EventSheet from './EventSheet.svelte';
	import VerbMark from './VerbMark.svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import { createLatestOnly, dateRangeProblem, type HistoryFilter, type HistoryRow } from '#lib/history.ts';
	import { artOf, clock, eventsQuery, feedItems, headVerb, settle, titleHref, verbOf } from '#lib/history-views.ts';
	import type { EventPage, EventRecord, TitleView } from '#lib/types.ts';

	// 1 Feed: the history as a phone's notification feed. A header per day;
	// routine runs (a refresh of 40 artists) fold into one line that opens;
	// results that matter (imported, failed, deleted, upgraded) are cards with
	// the title's art.
	let { filter, titles, titleById, onfilter, phone }: { filter: HistoryFilter; titles: TitleView[]; titleById: Map<number, TitleView>; onfilter: (c: Partial<HistoryFilter>) => void; phone: boolean } = $props();

	const STEP = 500;
	let events = $state<EventRecord[]>([]);
	let total = $state(0);
	let loaded = $state(false);
	let more = $state(false);
	let error = $state('');
	let sheet = $state<HistoryRow | null>(null);
	let opened = $state<Record<string, number>>({});
	const latest = createLatestOnly();

	const items = $derived(feedItems(events, titleById));

	async function load(append = false) {
		const ok = latest();
		if (dateRangeProblem(filter)) { loaded = true; return; }
		if (append) more = true;
		try {
			const p = await api.get<EventPage>('/events?' + eventsQuery(filter, { limit: STEP, offset: append ? events.length : 0 }));
			if (!ok()) return;
			events = append ? [...events, ...p.events] : p.events;
			total = p.total;
			error = '';
		} catch (e) {
			if (ok()) error = message(e);
		} finally {
			if (ok()) { loaded = true; more = false; }
		}
	}
	$effect(() => {
		void [filter.title, filter.episode, filter.from, filter.to, filter.groups.join()];
		untrack(() => { opened = {}; void load(); });
	});
	const fresh = settle(() => events.length <= STEP && load());
	onMount(() => {
		const offs = [live.on('event', fresh.poke), live.on('resync', fresh.poke)];
		return () => { offs.forEach((o) => o()); fresh.stop(); };
	});
	const toggle = (k: string) => (opened = { ...opened, [k]: opened[k] ? 0 : 40 });
	const showMore = (k: string) => (opened = { ...opened, [k]: (opened[k] ?? 0) + 200 });
	const filtered = $derived(!!(filter.title || filter.groups.length || filter.from || filter.to));
</script>

<div class="feed-wrap">
	<HistoryToolbar {filter} {titles} {onfilter} {phone} />

	{#if error && !events.length}
		<LoadError hasData={false} message="History could not be loaded." onretry={() => load()} />
	{:else if !loaded}
		<div class="feed" aria-busy="true">{#each Array(6) as _, i (i)}<Skeleton shape="block" height="64px" />{/each}</div>
	{:else if !events.length}
		<EmptyState icon="clock" title={filtered ? 'Nothing matches' : 'No history yet'} text={filtered ? 'No Event matches these filters. Try a wider date range or every kind.' : 'Every search, download, import, rename and delete writes an Event here.'} action={filtered ? { label: 'Show everything', onclick: () => onfilter({ title: undefined, groups: [], from: undefined, to: undefined }) } : undefined} />
	{:else}
		<ol class="feed" data-testid="hv-feed">
			{#each items as it (it.key)}
				{#if it.type === 'day'}
					<li class="day" data-testid="hv-day"><h2>{it.label}</h2><span>{it.count.toLocaleString()} {it.count === 1 ? 'Event' : 'Events'}</span></li>
				{:else if it.type === 'burst'}
					{@const n = it.rows.reduce((s, r) => s + r.events.length, 0)}
					{@const shown = opened[it.key] ?? 0}
					<li class="burst" class:important={it.tier !== 'routine'} data-testid="hv-burst">
						<button type="button" class="b-line" aria-expanded={shown > 0} onclick={() => toggle(it.key)}>
							<span class="stack" aria-hidden="true">
								{#each it.rows.slice(0, 3) as r (r.key)}<span class="s-art"><Poster src={artOf(r.head, titleById)} name={r.head.title_name ?? '·'} /></span>{/each}
							</span>
							<span class="b-text"><span class="b-what">{it.line}</span><span class="b-when">{clock(it.rows[it.rows.length - 1].head.At)}–{clock(it.rows[0].head.At)}</span></span>
							<span class="b-n">{n}</span>
							<Icon name="chevronDown" size={16} />
						</button>
						{#if shown}
							<ul class="b-list">
								{#each it.rows.slice(0, shown) as r (r.key)}
									{@const href = titleHref(r.head.TitleID, titleById)}
									<li>
										<VerbMark verb={headVerb(r)} size="sm" bare />
										<span class="bl-sum">{#if href}<a href={to(href)}>{r.head.title_name}</a>: {/if}{r.head.Summary}</span>
										<time datetime={r.head.At} title={dateTime(r.head.At)}>{clock(r.head.At)}</time>
									</li>
								{/each}
							</ul>
							{#if it.rows.length > shown}<button type="button" class="b-more" onclick={() => showMore(it.key)}>Show {Math.min(200, it.rows.length - shown)} more of {it.rows.length}</button>{/if}
						{/if}
					</li>
				{:else}
					{@const e = it.row.head}
					{@const verb = headVerb(it.row)}
					{@const href = titleHref(e.TitleID, titleById)}
					{@const t = titleById.get(e.TitleID ?? 0)}
					{#if it.tier === 'important' || verb.word === 'Upgraded'}
						<li class="hv-card {verb.tone}" data-testid="hv-card" data-kind={e.Kind}>
							{#if t?.backdrop && !phone}<span class="bd" aria-hidden="true"><img src={t.backdrop} alt="" loading="lazy" decoding="async" /></span>{/if}
							<span class="c-art" class:square={t?.kind === 'artist'}>{#if e.TitleID && e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{:else}<span class="c-icon"><Icon name={verb.icon} size={22} /></span>{/if}</span>
							<div class="c-body">
								<div class="c-top"><VerbMark {verb} /><time datetime={e.At} title={dateTime(e.At)}>{clock(e.At)}</time></div>
								<p class="c-title">{#if href}<a href={to(href)}>{e.title_name}</a>{:else}{e.title_name ?? 'HolyArrs'}{/if}{#if e.episode}<span class="c-ep"> {e.episode}</span>{/if}</p>
								<p class="c-sum">{e.Summary}</p>
								{#if it.row.steps.length}
									<ol class="c-steps" aria-label="Steps of this download">{#each it.row.steps as st (st.key)}<li class={st.state}>{st.label}</li>{/each}</ol>
								{/if}
								<button type="button" class="c-more" onclick={() => (sheet = it.row)}>Why and where<Icon name="chevronRight" size={14} /></button>
							</div>
						</li>
					{:else}
						<li class="line" data-testid="hv-line" data-kind={e.Kind}>
							<VerbMark {verb} size="sm" bare />
							<button type="button" class="l-sum" onclick={() => (sheet = it.row)}><span class="l-v">{verb.word}</span> {#if e.title_name}<b>{e.title_name}</b>{e.episode ? ` ${e.episode}` : ''}: {/if}{e.Summary}</button>
							<time datetime={e.At} title={dateTime(e.At)}>{clock(e.At)}</time>
						</li>
					{/if}
				{/if}
			{/each}
		</ol>
		<div class="foot">
			<p class="count" data-testid="history-count">{events.length.toLocaleString()} of {total.toLocaleString()} Events</p>
			{#if events.length < total}<Button variant="secondary" busy={more} onclick={() => load(true)}>Load older</Button>{/if}
		</div>
	{/if}
</div>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.feed-wrap { display: flex; flex-direction: column; gap: 20px; max-width: 820px; }
	.feed { display: flex; flex-direction: column; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.day { position: sticky; top: var(--topbar, 0px); z-index: 3; display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin: 14px -8px 2px; padding: 10px 8px 8px; background: color-mix(in srgb, var(--bg) 86%, transparent); -webkit-backdrop-filter: blur(16px); backdrop-filter: blur(16px); }
	.day:first-child { margin-top: 0; }
	.day h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.day span { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }

	.burst { border-radius: 16px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.b-line { display: flex; align-items: center; gap: 12px; width: 100%; min-height: 56px; padding: 10px 14px; border-radius: 16px; text-align: left; color: var(--text-muted); }
	.b-line:hover { background: var(--surface-2); }
	.b-line[aria-expanded='true'] :global(svg:last-child) { transform: rotate(180deg); }
	.stack { flex: none; display: flex; width: 52px; }
	.s-art { width: 28px; flex: none; --poster-ratio: 1; border-radius: 50%; overflow: hidden; box-shadow: 0 0 0 2px var(--surface-1); }
	.s-art + .s-art { margin-left: -16px; }
	.b-text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.b-what { color: var(--text); font: var(--t-callout); overflow-wrap: anywhere; }
	.b-when { font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.b-n { flex: none; min-width: 28px; padding: 2px 8px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-footnote); font-weight: 600; text-align: center; font-variant-numeric: tabular-nums; }
	.b-list { margin: 0; padding: 4px 14px 10px; list-style: none; }
	.b-list li { display: flex; align-items: center; gap: 10px; min-height: 34px; padding: 4px 0; box-shadow: 0 -1px 0 var(--line); font: var(--t-footnote); }
	.bl-sum { flex: 1; min-width: 0; overflow-wrap: anywhere; }
	.bl-sum a { color: var(--text); font-weight: 600; }
	.b-list time, .line time { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.b-more { margin: 0 14px 12px; color: var(--accent-text); font: var(--t-footnote); font-weight: 600; }

	.hv-card { position: relative; isolation: isolate; overflow: hidden; display: flex; gap: 14px; padding: 14px; border-radius: 18px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), var(--shadow-card); }
	.hv-card.fail { box-shadow: 0 0 0 1px color-mix(in srgb, var(--fail) 40%, var(--line)), var(--shadow-card); }
	.hv-card.ok { box-shadow: 0 0 0 1px color-mix(in srgb, var(--ok) 32%, var(--line)), var(--shadow-card); }
	.bd { position: absolute; z-index: -1; inset: 0 0 0 auto; width: 52%; -webkit-mask-image: linear-gradient(90deg, transparent, #000 70%); mask-image: linear-gradient(90deg, transparent, #000 70%); opacity: .35; }
	.bd img { width: 100%; height: 100%; object-fit: cover; }
	.c-art { flex: none; width: 64px; --poster-ratio: 2 / 3; align-self: flex-start; border-radius: 8px; overflow: hidden; }
	.c-art.square { --poster-ratio: 1; }
	.c-icon { display: grid; place-items: center; aspect-ratio: 1; border-radius: 50%; background: var(--surface-2); color: var(--text-muted); }
	.c-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
	.c-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	.c-top time { color: var(--text-muted); font: var(--t-footnote); }
	.c-title { font: var(--t-headline); overflow-wrap: anywhere; }
	.c-title a { color: var(--text); }
	.c-title a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.c-ep { color: var(--text-muted); font-weight: 400; }
	.c-sum { color: var(--text-muted); font: var(--t-callout); overflow-wrap: anywhere; }
	.c-steps { display: flex; flex-wrap: wrap; gap: 4px; margin: 4px 0 0; padding: 0; list-style: none; font: var(--t-caption); font-weight: 600; }
	.c-steps li { padding: 2px 8px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); }
	.c-steps li.done { background: color-mix(in srgb, var(--ok) 16%, transparent); color: var(--ok-text, var(--ok)); }
	.c-steps li.failed { background: color-mix(in srgb, var(--fail) 16%, transparent); color: var(--fail-text, var(--fail)); }
	.c-more { align-self: flex-start; display: inline-flex; align-items: center; gap: 2px; min-height: 32px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.c-more:hover { color: var(--text); }

	.line { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 4px 14px; border-radius: 14px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.l-sum { flex: 1; min-width: 0; padding: 6px 0; text-align: left; color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.l-sum b { color: var(--text); font-weight: 600; }
	.l-v { color: var(--text); font-weight: 600; }
	.l-sum:hover { color: var(--text); }

	.foot { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
	.count { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	@media (max-width: 767px) {
		.feed-wrap { gap: 14px; }
		.day h2 { font: var(--t-title3); }
		.hv-card { padding: 12px; gap: 12px; border-radius: 16px; }
		.c-art { width: 52px; }
		.b-line { padding: 10px 12px; gap: 10px; }
		.stack { width: 44px; }
		.s-art { width: 24px; }
		.foot > :global(*) { flex: 1 1 100%; }
		.foot > :global(button) { min-height: 44px; }
	}
	@media (prefers-reduced-motion: no-preference) { .b-line :global(svg:last-child) { transition: transform var(--dur-fast) var(--ease-out); } }
</style>
