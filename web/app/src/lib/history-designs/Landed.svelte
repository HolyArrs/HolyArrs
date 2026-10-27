<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import Poster from '#lib/Poster.svelte';
	import Icon from '#lib/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import type { StatusKind } from '#lib/ui/StatusPill.svelte';
	import HistoryToolbar from './HistoryToolbar.svelte';
	import EventSheet from './EventSheet.svelte';
	import VerbMark from './VerbMark.svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import { createLatestOnly, dateRangeProblem, groupDownloads, kindGroups, pageLine, type HistoryFilter, type HistoryRow } from '#lib/history.ts';
	import { artOf, eventsQuery, headVerb, notRoutineKinds, pickedKinds, routineKinds, settle, titleHref } from '#lib/history-views.ts';
	import type { EventPage, EventRecord, TitleView } from '#lib/types.ts';

	// 3 Recently added: the top is rows of posters of what landed (imported,
	// upgraded, failed, new in the Library), like a streaming home page; below
	// it, a compact log of everything else, routine scans left out until
	// asked for, with a quick find over the page.
	let { filter, titles, titleById, onfilter, phone }: { filter: HistoryFilter; titles: TitleView[]; titleById: Map<number, TitleView>; onfilter: (c: Partial<HistoryFilter>) => void; phone: boolean } = $props();

	const kindsOf = (...keys: string[]) => kindGroups.filter((g) => keys.includes(g.key)).flatMap((g) => g.kinds);
	const SHELVES: { key: string; title: string; kinds: string[]; status: StatusKind; word: string }[] = [
		{ key: 'imported', title: 'Recently imported', kinds: ['import_done', 'import_partly_done'], status: 'in_library', word: 'Imported' },
		{ key: 'upgraded', title: 'Upgraded to a better copy', kinds: ['file_replaced'], status: 'approved', word: 'Upgraded' },
		{ key: 'failed', title: 'Failed, worth a look', kinds: kindsOf('fail', 'decide'), status: 'failed', word: 'Failed' },
		{ key: 'added', title: 'New in the Library', kinds: ['title_added', 'adopted'], status: 'requested', word: 'Added' }
	];
	type Landing = { e: EventRecord; title: TitleView };
	let shelves = $state<Record<string, Landing[]>>({});
	let shelvesLoaded = $state(false);

	const LIMIT = 50;
	let offset = $state(0);
	let routine = $state(false);
	let find = $state('');
	let events = $state<EventRecord[]>([]);
	let total = $state(0);
	let routineCount = $state(0);
	let loaded = $state(false);
	let error = $state('');
	let sheet = $state<HistoryRow | null>(null);
	const latestLog = createLatestOnly();
	const latestShelves = createLatestOnly();
	const kinds = $derived(routine ? pickedKinds(filter.groups) : pickedKinds(filter.groups, notRoutineKinds));

	/** The shelves follow the title and dates; the kind menu is for the log. */
	async function loadShelves() {
		const ok = latestShelves();
		if (dateRangeProblem(filter)) { shelves = {}; shelvesLoaded = true; return; }
		const got = await Promise.all(SHELVES.map((s) => api.get<EventPage>('/events?' + eventsQuery({ ...filter, groups: [] }, { kinds: s.kinds, limit: 60, offset: 0 })).catch(() => null)));
		if (!ok()) return;
		const next: Record<string, Landing[]> = {};
		SHELVES.forEach((s, i) => {
			const seen = new Set<number>();
			next[s.key] = (got[i]?.events ?? []).flatMap((e) => {
				const title = titleById.get(e.TitleID ?? 0);
				if (!title || seen.has(title.id)) return [];
				seen.add(title.id);
				return [{ e, title }];
			}).slice(0, 20);
		});
		shelves = next;
		shelvesLoaded = true;
	}
	async function loadLog() {
		const ok = latestLog();
		if (dateRangeProblem(filter)) { loaded = true; return; }
		if (kinds && !kinds.length) { events = []; total = 0; loaded = true; return; }
		try {
			const [p, r] = await Promise.all([
				api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit: LIMIT, offset })),
				routine ? Promise.resolve(null) : api.get<EventPage>('/events?' + eventsQuery(filter, { kinds: pickedKinds(filter.groups, routineKinds), limit: 1, offset: 0 })).catch(() => null)
			]);
			if (!ok()) return;
			events = p.events;
			total = p.total;
			if (r) routineCount = r.total;
			error = '';
		} catch (e) {
			if (ok()) error = message(e);
		} finally {
			if (ok()) loaded = true;
		}
	}
	$effect(() => {
		void [filter.title, filter.episode, filter.from, filter.to, titles.length];
		untrack(() => void loadShelves());
	});
	$effect(() => {
		void [filter.title, filter.episode, filter.from, filter.to, filter.groups.join(), routine];
		untrack(() => { offset = 0; void loadLog(); });
	});
	const fresh = settle(() => { if (offset === 0) void loadLog(); void loadShelves(); }, 2500);
	onMount(() => {
		const offs = [live.on('event', fresh.poke), live.on('resync', fresh.poke)];
		return () => { offs.forEach((o) => o()); fresh.stop(); };
	});
	function page(to: number) {
		offset = Math.max(0, to);
		void loadLog();
	}

	const rows = $derived(groupDownloads(events));
	const words = $derived(find.trim().toLowerCase());
	const shown = $derived(words ? rows.filter((r) => r.events.some((e) => `${e.title_name ?? ''} ${e.episode ?? ''} ${e.Summary}`.toLowerCase().includes(words))) : rows);
	const titleHits = $derived(words.length > 1 ? titles.filter((t) => t.name.toLowerCase().includes(words)).slice(0, 4) : []);
	const anyShelf = $derived(SHELVES.some((s) => shelves[s.key]?.length));
	const filtered = $derived(!!(filter.title || filter.groups.length || filter.from || filter.to));
</script>

{#snippet routineSwitch()}
	<label class="hv-switch"><input type="checkbox" role="switch" bind:checked={routine} data-testid="hv-routine" /><span>Routine scans{!routine && routineCount ? ` (${routineCount.toLocaleString()})` : ''}</span></label>
{/snippet}

<div class="wrap">
	<HistoryToolbar {filter} {titles} {onfilter} {phone} />

	{#if !shelvesLoaded}
		<div class="sk-row" aria-busy="true">{#each Array(phone ? 2 : 6) as _, i (i)}<Skeleton shape="poster" />{/each}</div>
	{:else if anyShelf}
		<div class="shelves" data-testid="hv-shelves">
			{#each SHELVES as s (s.key)}
				{@const list = shelves[s.key] ?? []}
				{#if list.length}
					<Shelf title={s.title} subtitle="{list.length} {list.length === 1 ? 'title' : 'titles'}{list[0] ? `, newest ${ago(list[0].e.At)}` : ''}" item={phone ? '42vw' : '156px'}>
						{#each list as l (l.e.ID)}
							<PosterCard title={l.title.name} href={to(`/library/${l.title.id}`)} image={artOf(l.e, titleById)} needsArt={l.title.kind === 'artist'} shape={l.title.kind === 'artist' ? 'square' : 'poster'} subtitle="{headVerb({ key: '', head: l.e, events: [l.e], steps: [] }).word} {ago(l.e.At)}" status={s.status} statusLabel={s.word} kind={l.title.kind === 'artist' ? 'artist' : l.title.kind === 'show' ? 'show' : 'movie'} />
						{/each}
					</Shelf>
				{/if}
			{/each}
		</div>
	{/if}

	<section class="log" aria-labelledby="hv-log-title">
		<header class="log-head">
			<div>
				<h2 id="hv-log-title">Everything else</h2>
				<p class="count" data-testid="history-count">{total ? `${pageLine(total, offset, events.length)} Events` : 'No Events'}{routine ? '' : ', routine scans left out'}</p>
			</div>
			<div class="log-tools">
				<label class="lfind"><Icon name="search" size={15} /><input type="search" placeholder="Find on this page" aria-label="Find on this page" bind:value={find} data-testid="hv-find" /></label>
				{@render routineSwitch()}
			</div>
		</header>
		{#if titleHits.length && !filter.title}
			<p class="hits">Only one title: {#each titleHits as t (t.id)}<button type="button" onclick={() => { find = ''; onfilter({ title: t.id }); }}>{t.name}</button>{/each}</p>
		{/if}
		{#if error && !events.length}
			<LoadError hasData={false} message="History could not be loaded." onretry={loadLog} />
		{:else if !loaded}
			<div class="list">{#each Array(8) as _, i (i)}<Skeleton height="40px" />{/each}</div>
		{:else if !shown.length}
			<EmptyState compact icon="clock" title="Nothing matches" text={words ? `No Event on this page mentions “${find.trim()}”.` : filtered ? 'No Event matches these filters.' : 'No Events yet.'} action={filtered ? { label: 'Show everything', onclick: () => onfilter({ title: undefined, groups: [], from: undefined, to: undefined }) } : undefined} />
		{:else}
			<ol class="list" data-testid="hv-log">
				{#each shown as r (r.key)}
					{@const e = r.head}
					{@const href = titleHref(e.TitleID, titleById)}
					{@const t = titleById.get(e.TitleID ?? 0)}
					<li class="hv-row" data-kind={e.Kind}>
						<span class="r-art" class:square={t?.kind === 'artist'}>{#if e.title_name}<Poster src={artOf(e, titleById)} name={e.title_name} />{/if}</span>
						<span class="r-verb"><VerbMark verb={headVerb(r)} size="sm" /></span>
						<span class="r-text">{#if e.title_name}{#if href}<a href={to(href)}>{e.title_name}</a>{:else}<b>{e.title_name}</b>{/if}{e.episode ? ` ${e.episode}` : ''}<span class="sep" aria-hidden="true"> · </span>{/if}<span class="r-sum">{e.Summary}</span></span>
						<time datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
						<button type="button" class="r-open" aria-label="Why and where: {e.title_name ?? ''} {e.Summary}" onclick={() => (sheet = r)}><Icon name="chevronRight" size={16} /></button>
					</li>
				{/each}
			</ol>
			<nav class="pages" aria-label="Pages">
				<Button variant="secondary" icon="chevronLeft" disabled={offset === 0} onclick={() => page(offset - LIMIT)}>Newer</Button>
				<Button variant="secondary" iconAfter="chevronRight" disabled={offset + events.length >= total} onclick={() => page(offset + LIMIT)}>Older</Button>
			</nav>
		{/if}
	</section>
</div>

<EventSheet bind:row={sheet} {titleById} />

<style>
	.wrap { display: flex; flex-direction: column; gap: 26px; min-width: 0; }
	.shelves { display: flex; flex-direction: column; gap: 28px; min-width: 0; }
	.sk-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 16px; }
	.log { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
	.log-head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 10px 16px; }
	.log-head h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.count { margin-top: 2px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.log-tools { display: flex; flex-wrap: wrap; gap: 8px; min-width: 0; }
	.lfind { display: flex; align-items: center; gap: 8px; height: 36px; padding: 0 12px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); box-shadow: inset 0 0 0 1px var(--line); }
	.lfind input { width: 180px; min-width: 0; border: 0; background: none; color: var(--text); font: var(--t-callout); outline: none; }
	.hv-switch { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 0 12px; border-radius: var(--r-pill); background: var(--surface-2); font: var(--t-footnote); font-weight: 600; cursor: pointer; }
	.hv-switch input { width: 16px; height: 16px; accent-color: var(--accent); }
	.hits { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; color: var(--text-muted); font: var(--t-footnote); }
	.hits button { padding: 4px 10px; border-radius: var(--r-pill); background: var(--accent-soft); color: var(--accent-text); font-weight: 600; }
	.list { display: flex; flex-direction: column; margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.hv-row { display: grid; grid-template-columns: 22px 138px minmax(0, 1fr) auto 32px; align-items: center; gap: 10px; min-height: 40px; padding: 2px 6px 2px 8px; border-radius: var(--r-btn); font: var(--t-footnote); }
	.hv-row + .hv-row { box-shadow: 0 -1px 0 var(--line); }
	.hv-row:hover { background: var(--surface-2); }
	.r-art { width: 22px; --poster-ratio: 2 / 3; border-radius: 3px; overflow: hidden; }
	.r-art.square { --poster-ratio: 1; border-radius: 50%; }
	.r-verb { min-width: 0; overflow: hidden; }
	.r-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); }
	.r-text a, .r-text b { color: var(--text); font-weight: 600; }
	.r-text a:hover { text-decoration: underline; }
	.hv-row time { color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.r-open { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; color: var(--text-muted); }
	.r-open:hover { background: var(--surface-3); color: var(--text); }
	.pages { display: flex; justify-content: flex-end; gap: 8px; }
	@media (max-width: 767px) {
		.wrap { gap: 22px; }
		.log-tools, .lfind { width: 100%; }
		.lfind { height: 44px; }
		.lfind input { flex: 1; width: auto; }
		.hv-switch { min-height: 44px; }
		.hv-row { grid-template-columns: 28px minmax(0, 1fr) 32px; grid-template-areas: 'art verb open' 'art text open' 'art time open'; gap: 2px 10px; padding: 8px 4px 8px 8px; }
		.r-art { grid-area: art; width: 28px; align-self: start; }
		.r-verb { grid-area: verb; }
		.r-text { grid-area: text; white-space: normal; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
		.hv-row time { grid-area: time; }
		.r-open { grid-area: open; width: 44px; height: 44px; }
		.pages > :global(*) { flex: 1; min-height: 44px; }
	}
</style>
