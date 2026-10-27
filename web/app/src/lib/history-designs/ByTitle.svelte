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
	import { artOf, eventsQuery, notRoutineKinds, pickedKinds, settle, stories, titleHref, type Story, type StoryStep } from '#lib/history-views.ts';
	import type { EventPage, EventRecord, TitleView } from '#lib/types.ts';

	// 2 By title: one card per movie, show or artist, its Events told as a
	// small vertical timeline, oldest at the top (asked, approved, searched,
	// chosen, downloading, imported, renamed). Routine upkeep is left out
	// unless switched on; with it on, titles that only had upkeep sit in a
	// compact strip under the stories.
	let { filter, titles, titleById, onfilter, phone }: { filter: HistoryFilter; titles: TitleView[]; titleById: Map<number, TitleView>; onfilter: (c: Partial<HistoryFilter>) => void; phone: boolean } = $props();

	const STEP = 500;
	const SHOWN = 6;
	let routine = $state(false);
	let events = $state<EventRecord[]>([]);
	let total = $state(0);
	let loaded = $state(false);
	let more = $state(false);
	let error = $state('');
	let sheet = $state<HistoryRow | null>(null);
	let whole = $state<Record<string, boolean>>({});
	const latest = createLatestOnly();
	/** One title picked: its whole story, upkeep included. */
	const withRoutine = $derived(routine || !!filter.title);
	const kinds = $derived(withRoutine ? pickedKinds(filter.groups) : pickedKinds(filter.groups, notRoutineKinds));

	const all = $derived(stories(events));
	/** A story worth a card: two steps or more that are not upkeep, or a failure. One title picked: always. */
	const full = (s: Story) => !!filter.title || s.steps.filter((st) => st.tier !== 'routine').length >= 2 || s.steps.some((st) => st.verb.tone === 'fail');
	const told = $derived(all.filter((s) => !s.quiet && full(s)));
	/** Titles where one thing happened (added, deleted, asked): a compact list, so 300 one-line stories do not bury the real ones. */
	const brief = $derived(all.filter((s) => !s.quiet && !full(s)));
	let briefAll = $state(false);
	const quiet = $derived(all.filter((s) => s.quiet));

	async function load(append = false) {
		const ok = latest();
		if (dateRangeProblem(filter)) { loaded = true; return; }
		if (kinds && !kinds.length) { events = []; total = 0; loaded = true; return; }
		if (append) more = true;
		try {
			const p = await api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit: STEP, offset: append ? events.length : 0 }));
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
		void [filter.title, filter.episode, filter.from, filter.to, filter.groups.join(), withRoutine];
		untrack(() => { whole = {}; void load(); });
	});
	const fresh = settle(() => events.length <= STEP && load());
	onMount(() => {
		const offs = [live.on('event', fresh.poke), live.on('resync', fresh.poke)];
		return () => { offs.forEach((o) => o()); fresh.stop(); };
	});

	function rowOf(st: StoryStep): HistoryRow {
		return { key: st.key, head: st.last, events: [...st.events].reverse(), steps: [] };
	}
	const filtered = $derived(!!(filter.title || filter.groups.length || filter.from || filter.to));
	const kindWord = (t?: TitleView) => (t?.kind === 'artist' ? 'Artist' : t?.kind === 'show' ? 'Show' : t?.kind === 'movie' ? 'Movie' : '');
	/** The step a story ends on, for its card's status line. */
	const ending = (s: Story) => s.steps[s.steps.length - 1];
</script>

{#snippet routineSwitch()}
	<label class="hv-switch" title={filter.title ? 'One title shows its whole story' : undefined}>
		<input type="checkbox" role="switch" checked={withRoutine} disabled={!!filter.title} onchange={(e) => (routine = e.currentTarget.checked)} data-testid="hv-routine" />
		<span>Routine scans</span>
	</label>
{/snippet}

<div class="wrap">
	<HistoryToolbar {filter} {titles} {onfilter} {phone} extra={routineSwitch} />

	{#if error && !events.length}
		<LoadError hasData={false} message="History could not be loaded." onretry={() => load()} />
	{:else if !loaded}
		<div class="grid" aria-busy="true">{#each Array(phone ? 2 : 6) as _, i (i)}<Skeleton shape="block" height="260px" />{/each}</div>
	{:else if !events.length}
		<EmptyState icon="clock" title={filtered ? 'Nothing matches' : 'No stories yet'} text={filtered ? 'No Event matches these filters.' : withRoutine ? 'Every search, download, import, rename and delete writes an Event here.' : 'Only routine scans so far. Switch on Routine scans to see them.'} action={filtered ? { label: 'Show everything', onclick: () => onfilter({ title: undefined, groups: [], from: undefined, to: undefined }) } : undefined} />
	{:else}
		<p class="count" data-testid="history-count">{told.length} {told.length === 1 ? 'story' : 'stories'}{brief.length ? ` and ${brief.length} single ${brief.length === 1 ? 'Event' : 'Events'}` : ''} from the newest {events.length.toLocaleString()} of {total.toLocaleString()} Events{withRoutine ? '' : ', routine scans left out'}</p>
		<div class="grid" data-testid="hv-stories">
			{#each told as s (s.key)}
				{@const t = titleById.get(s.titleId ?? 0)}
				{@const href = titleHref(s.titleId, titleById)}
				{@const end = ending(s)}
				{@const open = whole[s.key] || s.steps.length <= SHOWN}
				{@const steps = open ? s.steps : s.steps.slice(-SHOWN)}
				<article class="story" data-testid="hv-story" aria-label={s.name}>
					<header class="s-head">
						{#if t?.backdrop && !phone}<span class="bd" aria-hidden="true"><img src={t.backdrop} alt="" loading="lazy" decoding="async" /></span>{/if}
						<span class="s-art" class:square={t?.kind === 'artist'}>{#if s.titleId}<Poster src={artOf(s.events[0], titleById)} name={s.name} />{:else}<span class="s-icon"><Icon name="info" size={24} /></span>{/if}</span>
						<div class="s-name">
							<h2>{#if href}<a href={to(href)}>{s.name}</a>{:else}{s.titleId ? s.name : 'Not tied to a title'}{/if}</h2>
							<p class="s-kind">{[kindWord(t), t?.year].filter(Boolean).join(' · ') || (s.titleId ? 'No longer in the Library' : 'Health, asks and settings')}</p>
							<p class="s-now"><VerbMark verb={end.verb} size="sm" /><span>{ago(end.last.At)}</span></p>
						</div>
					</header>
					{#if !open}<button type="button" class="earlier" onclick={() => (whole = { ...whole, [s.key]: true })}><Icon name="chevronUpDown" size={14} />{s.steps.length - SHOWN} earlier {s.steps.length - SHOWN === 1 ? 'step' : 'steps'}</button>{/if}
					<ol class="tl">
						{#each steps as st, i (st.key)}
							<li class="st {st.verb.tone} {st.tier}" class:last={i === steps.length - 1}>
								<span class="pin" aria-hidden="true"><Icon name={st.verb.icon} size={12} /></span>
								<button type="button" class="st-body" onclick={() => (sheet = rowOf(st))}>
									<span class="st-top"><b>{st.verb.word}</b>{#if st.count > 1}<span class="x">×{st.count}</span>{/if}<time datetime={st.last.At} title={dateTime(st.last.At)}>{ago(st.last.At)}</time></span>
									<span class="st-sum">{st.last.episode ? `${st.last.episode}: ` : ''}{st.last.Summary}</span>
								</button>
							</li>
						{/each}
					</ol>
				</article>
			{/each}
		</div>
		{#if !told.length && !brief.length}<EmptyState compact icon="clock" title="Only upkeep here" text="These Events are refreshes, scans and searches; the titles they touched are listed below." />{/if}
		{#if brief.length}
			<section class="brief" aria-labelledby="hv-brief">
				<h2 id="hv-brief">One thing happened <span>{brief.length}</span></h2>
				<ul data-testid="hv-brief">
					{#each briefAll ? brief : brief.slice(0, 24) as s (s.key)}
						{@const href = titleHref(s.titleId, titleById)}
						{@const t = titleById.get(s.titleId ?? 0)}
						{@const st = s.steps.find((x) => x.tier !== 'routine') ?? s.steps[0]}
						<li>
							<span class="q-art" class:square={t?.kind === 'artist'}>{#if s.titleId}<Poster src={artOf(s.events[0], titleById)} name={s.name} />{/if}</span>
							<span class="q-t">{#if href}<a href={to(href)}>{s.name}</a>{:else}<span class="gone">{s.titleId ? s.name : 'Not tied to a title'}</span>{/if}<small>{ago(st.last.At)}{s.titleId && !href ? ' · no longer in the Library' : ''}</small></span>
							<button type="button" class="b-verb" onclick={() => (sheet = rowOf(st))} aria-label="{st.verb.word}: {st.last.Summary}"><VerbMark verb={st.verb} size="sm" />{#if st.count > 1}<span class="x">×{st.count}</span>{/if}</button>
						</li>
					{/each}
				</ul>
				{#if brief.length > 24 && !briefAll}<button type="button" class="all" onclick={() => (briefAll = true)}>Show all {brief.length}</button>{/if}
			</section>
		{/if}
		{#if quiet.length}
			<section class="quiet" aria-labelledby="hv-quiet">
				<h2 id="hv-quiet">Only kept up to date <span>{quiet.length}</span></h2>
				<ul>
					{#each quiet as s (s.key)}
						{@const href = titleHref(s.titleId, titleById)}
						{@const t = titleById.get(s.titleId ?? 0)}
						<li>
							<span class="q-art" class:square={t?.kind === 'artist'}><Poster src={artOf(s.events[0], titleById)} name={s.name} /></span>
							<span class="q-t">{#if href}<a href={to(href)}>{s.name}</a>{:else}{s.name}{/if}<small>{s.events.length} {s.events.length === 1 ? 'refresh or scan' : 'refreshes and scans'} · {ago(s.latest)}</small></span>
						</li>
					{/each}
				</ul>
			</section>
		{/if}
		<div class="foot">
			{#if events.length < total}<Button variant="secondary" busy={more} onclick={() => load(true)}>Load older</Button>{/if}
		</div>
	{/if}
</div>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.wrap { display: flex; flex-direction: column; gap: 18px; }
	.hv-switch { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 0 12px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 600; cursor: pointer; }
	.hv-switch input { width: 16px; height: 16px; accent-color: var(--accent); }
	.hv-switch:has(input:disabled) { opacity: .6; cursor: default; }
	.count { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr)); gap: 16px; align-items: start; }
	.story { overflow: hidden; border-radius: 20px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line), var(--shadow-card); }
	.s-head { position: relative; isolation: isolate; display: flex; gap: 14px; padding: 16px 16px 12px; }
	.bd { position: absolute; z-index: -1; inset: 0; opacity: .22; -webkit-mask-image: linear-gradient(180deg, #000, transparent); mask-image: linear-gradient(180deg, #000, transparent); }
	.bd img { width: 100%; height: 100%; object-fit: cover; }
	.s-art { flex: none; width: 76px; --poster-ratio: 2 / 3; border-radius: 10px; overflow: hidden; box-shadow: var(--shadow-card); }
	.s-art.square { --poster-ratio: 1; }
	.s-icon { display: grid; place-items: center; aspect-ratio: 2 / 3; background: var(--surface-2); color: var(--text-muted); }
	.s-name { min-width: 0; display: flex; flex-direction: column; gap: 4px; padding-top: 2px; }
	.s-name h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); overflow-wrap: anywhere; }
	.s-name h2 a { color: var(--text); }
	.s-name h2 a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.s-kind { color: var(--text-muted); font: var(--t-footnote); }
	.s-now { display: flex; align-items: center; gap: 8px; margin-top: 4px; color: var(--text-muted); font: var(--t-footnote); }
	.earlier { display: flex; align-items: center; gap: 6px; margin: 0 16px 2px 44px; min-height: 32px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.earlier:hover { color: var(--text); }
	.tl { margin: 0; padding: 4px 16px 16px; list-style: none; }
	.st { position: relative; display: flex; gap: 12px; }
	.st::before { content: ''; position: absolute; left: 11px; top: 26px; bottom: -2px; width: 2px; border-radius: 1px; background: var(--line-strong); }
	.st.last::before { display: none; }
	.pin { position: relative; flex: none; display: grid; place-items: center; width: 24px; height: 24px; margin-top: 6px; border-radius: 50%; background: var(--surface-2); color: var(--text-muted); box-shadow: 0 0 0 3px var(--surface-1); }
	.st.ok .pin { background: var(--ok); color: #fff; }
	.st.fail .pin { background: var(--fail); color: #fff; }
	.st.wait .pin { background: var(--wait); color: #fff; }
	.st.info .pin { background: color-mix(in srgb, var(--info) 22%, var(--surface-1)); color: var(--info); }
	.st-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; padding: 6px 8px 10px; margin-left: -4px; border-radius: var(--r-btn); text-align: left; }
	.st-body:hover { background: var(--surface-2); }
	.st-top { display: flex; align-items: baseline; gap: 6px; font: var(--t-callout); }
	.st.routine .st-top b { color: var(--text-muted); font-weight: 500; }
	.x { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.st-top time { margin-left: auto; flex: none; color: var(--text-muted); font: var(--t-footnote); }
	.st-sum { color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.st.last .st-top b { font-weight: 700; }
	.brief { padding: 16px; border-radius: 20px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.brief h2 { display: flex; align-items: baseline; gap: 8px; margin-bottom: 10px; font: var(--t-headline); }
	.brief h2 span { color: var(--text-muted); font: var(--t-footnote); }
	.brief ul { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr)); gap: 2px 18px; margin: 0; padding: 0; list-style: none; }
	.brief li { display: flex; align-items: center; gap: 10px; min-width: 0; min-height: 44px; box-shadow: 0 -1px 0 var(--line); }
	.brief .q-t { flex: 1; }
	.gone { color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.b-verb { flex: none; display: inline-flex; align-items: center; gap: 4px; min-height: 36px; padding: 0 8px; border-radius: var(--r-pill); }
	.b-verb:hover { background: var(--surface-2); }
	.all { margin-top: 10px; color: var(--accent-text); font: var(--t-footnote); font-weight: 600; }
	.quiet { padding: 16px; border-radius: 20px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.quiet h2 { display: flex; align-items: baseline; gap: 8px; margin-bottom: 10px; font: var(--t-headline); }
	.quiet h2 span { color: var(--text-muted); font: var(--t-footnote); }
	.quiet ul { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 220px), 1fr)); gap: 6px 14px; margin: 0; padding: 0; list-style: none; }
	.quiet li { display: flex; align-items: center; gap: 10px; min-width: 0; padding: 4px 0; }
	.q-art { flex: none; width: 28px; --poster-ratio: 2 / 3; border-radius: 4px; overflow: hidden; }
	.q-art.square { --poster-ratio: 1; border-radius: 50%; }
	.q-t { min-width: 0; display: flex; flex-direction: column; font: var(--t-footnote); font-weight: 600; }
	.q-t a { color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.q-t small { color: var(--text-muted); font-weight: 400; }
	.foot { display: flex; justify-content: center; }
	@media (max-width: 767px) {
		.grid { gap: 12px; }
		.story { border-radius: 16px; }
		.s-head { padding: 14px 14px 10px; gap: 12px; }
		.s-art { width: 60px; }
		.tl { padding: 2px 12px 12px; }
		.earlier { margin-left: 40px; }
		.foot > :global(button) { width: 100%; min-height: 44px; }
	}
</style>
