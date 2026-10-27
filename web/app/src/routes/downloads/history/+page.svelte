<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import OpsCheckChip from '#lib/ui/OpsCheckChip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Poster from '#lib/Poster.svelte';
	import { createLatestOnly, dateRangeProblem, details, filterParams, groupDownloads, groupLabel, historyQuery, kindGroups, kindTone, pageLine, readFilter, type HistoryFilter } from '#lib/history.ts';
	import type { EventPage, EventRecord, TitleView } from '#lib/types.ts';
	import VariantBar from '#lib/ui/VariantBar.svelte';
	import Feed from '#lib/history-designs/Feed.svelte';
	import ByTitle from '#lib/history-designs/ByTitle.svelte';
	import Landed from '#lib/history-designs/Landed.svelte';
	import Importance from '#lib/history-designs/Importance.svelte';
	import EventTable from '#lib/history-designs/EventTable.svelte';

	// The History designs Luis is comparing (?view=1…5, kept for the session); 0 is today's page.
	const VIEWS = [
		{ value: 0, label: 'Now' },
		{ value: 1, label: '1 Feed' },
		{ value: 2, label: '2 By title' },
		{ value: 3, label: '3 Recently added' },
		{ value: 4, label: '4 Important' },
		{ value: 5, label: '5 Table' }
	];
	let view = $state(0);
	// Today's list loads only while it is the one shown (also when switched back to Now).
	$effect(() => {
		if (!view) untrack(() => void load());
	});

	const limit = 50;
	let filter = $state<HistoryFilter>(readFilter(page.url.searchParams, limit));
	let events = $state<EventRecord[]>([]);
	let total = $state(0);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let open = $state<string | null>(null);
	const rows = $derived(groupDownloads(events));
	const beginLoad = createLatestOnly();

	async function load() {
		if (view) return;
		const isLatest = beginLoad();
		if (dateRangeProblem(filter)) {
			error = '';
			loaded = true;
			return;
		}
		try {
			const p = await api.get<EventPage>('/events?' + historyQuery(filter));
			if (!isLatest()) return;
			events = p.events;
			total = p.total;
			episodeName = p.episode || 'one episode';
			error = '';
		} catch (e) {
			if (!isLatest()) return;
			error = message(e);
		} finally {
			if (isLatest()) loaded = true;
		}
	}

	// The filters live in the address, so a filtered view can be shared and Back works.
	function apply(change: Partial<HistoryFilter>) {
		filter = { ...filter, offset: 0, ...change };
		const params = filterParams(filter);
		const pick = page.url.searchParams.get('view');
		void goto(to('/downloads/history') + params + (pick ? `${params ? '&' : '?'}view=${pick}` : ''), { replace: true, reset: false });
		void load();
	}

	function toggleGroup(key: string, on: boolean) {
		apply({ groups: on ? [...filter.groups, key] : filter.groups.filter((g) => g !== key) });
	}

	function clear() {
		apply({ title: undefined, episode: undefined, groups: [], from: undefined, to: undefined });
	}

	const movieName = $derived(titles.find((t) => t.id === filter.title)?.name);
	const titleById = $derived(new Map(titles.map((title) => [title.id, title])));
	const filtered = $derived(!!(filter.title || filter.episode || filter.groups.length || filter.from || filter.to));
	const dateProblem = $derived(dateRangeProblem(filter));
	/** The episode the rows are filtered to, named from the rows themselves. */
	let episodeName = $state('');

	// On phones the filters live in a Sheet behind one button.
	let phone = $state(false);
	let filtersOpen = $state(false);
	const activeFilters = $derived((filter.title ? 1 : 0) + (filter.episode ? 1 : 0) + filter.groups.length + (filter.from ? 1 : 0) + (filter.to ? 1 : 0));
	function toneKind(t: string): StatusKind {
		return t === 'ok' ? 'in_library' : t === 'fail' ? 'failed' : t === 'wait' ? 'missing' : 'turned_down';
	}

	onMount(() => {
		const mq = window.matchMedia('(max-width: 767px)');
		const fit = () => (phone = mq.matches);
		fit();
		mq.addEventListener('change', fit);
		void api
			.get<{ titles: TitleView[] }>('/titles')
			.then((r) => (titles = r.titles))
			.catch(() => {});
		const offs = [live.on('event', () => !view && filter.offset === 0 && load()), live.on('resync', load)];
		return () => {
			offs.forEach((off) => off());
			mq.removeEventListener('change', fit);
		};
	});
</script>

<svelte:head><title>History · HolyArrs</title></svelte:head>

<PageHeader title="History" subtitle="Everything HolyArrs did, newest first: what it added, searched, chose, sent, imported, renamed and deleted, and why." />

{#if view}
	{#if view === 1}<Feed {filter} {titles} {titleById} onfilter={apply} {phone} />
	{:else if view === 2}<ByTitle {filter} {titles} {titleById} onfilter={apply} {phone} />
	{:else if view === 3}<Landed {filter} {titles} {titleById} onfilter={apply} {phone} />
	{:else if view === 4}<Importance {filter} {titles} {titleById} onfilter={apply} {phone} />
	{:else}<EventTable {filter} {titles} {titleById} onfilter={apply} {phone} />{/if}
{:else}
{#snippet filterForm()}
	<form class="filters" aria-label="Filter the history" onsubmit={(e) => e.preventDefault()}>
		<div class="fields">
			<div class="field grow">
				<label for="h-movie">Movie, show or artist</label>
				<select id="h-movie" class="input" value={filter.title ?? ''} onchange={(e) => apply({ title: Number(e.currentTarget.value) || undefined, episode: undefined })}>
					<option value="">Every movie, show and artist</option>
					{#each titles as t (t.id)}<option value={t.id}>{t.name}{t.year ? ` (${t.year})` : ''}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="h-from">From</label>
				<input id="h-from" class="input" type="date" value={filter.from ?? ''} aria-invalid={dateProblem ? 'true' : undefined} aria-describedby={dateProblem ? 'history-date-error' : undefined} onchange={(e) => apply({ from: e.currentTarget.value || undefined })} />
			</div>
			<div class="field">
				<label for="h-to">To</label>
				<input id="h-to" class="input" type="date" value={filter.to ?? ''} aria-invalid={dateProblem ? 'true' : undefined} aria-describedby={dateProblem ? 'history-date-error' : undefined} onchange={(e) => apply({ to: e.currentTarget.value || undefined })} />
			</div>
		</div>
		{#if dateProblem}<p id="history-date-error" class="field-error" role="alert">{dateProblem}</p>{/if}
		<fieldset class="kinds">
			<legend>What happened</legend>
			<div class="chips">
				{#each kindGroups as g (g.key)}
					<OpsCheckChip label={g.label} checked={filter.groups.includes(g.key)} onchange={(on) => toggleGroup(g.key, on)} />
				{/each}
			</div>
		</fieldset>
		{#if filter.episode}
			<p class="episode-filter" data-testid="episode-filter">
				Only {episodeName}{movieName ? ` of ${movieName}` : ''}.
				<Button variant="plain" size="sm" onclick={() => apply({ episode: undefined })}>All of the show</Button>
			</p>
		{/if}
		{#if filtered}<div><Button variant="plain" icon="x" onclick={clear}>Show everything</Button></div>{/if}
	</form>
{/snippet}

{#if phone}
	<div class="phone-bar">
		<Button variant="secondary" icon="grid" onclick={() => (filtersOpen = true)}>Filters{activeFilters ? ` (${activeFilters})` : ''}</Button>
		{#if filtered}<Button variant="plain" onclick={clear}>Show everything</Button>{/if}
	</div>
	<Sheet bind:open={filtersOpen} title="Filter the history" size="md">
		{@render filterForm()}
		{#snippet footer()}<Button variant="primary" onclick={() => (filtersOpen = false)}>Done</Button>{/snippet}
	</Sheet>
{:else}
	<div class="filterbar">{@render filterForm()}</div>
{/if}

{#if !dateProblem}
	{#if error && events.length === 0}
		<LoadError hasData={false} message="History could not be loaded." onretry={load} />
	{:else}
	<PageLoader loading={!loaded} label="Loading history">
	{#if events.length === 0}
		<EmptyState icon="clock" title={filtered ? 'Nothing matches' : 'No history yet'} text={filtered ? 'No Event matches these filters.' : 'Every search, download, import, rename and delete writes a row here.'} />
	{:else}
		{#if error}<LoadError hasData message="History could not be loaded." onretry={load} />{/if}
		<section aria-labelledby="history-results-title">
			<header class="sec">
				<h2 id="history-results-title">Events</h2>
				<p class="count" aria-live="polite" data-testid="history-count">{pageLine(total, filter.offset, events.length)}{movieName ? ` for ${movieName}` : ''}</p>
			</header>
			<ol class="panel events" data-testid="history">
				{#each rows as row (row.key)}
					{@const e = row.head}
					{@const extra = row.steps.length ? [] : details(e)}
					{@const eventTitle = titleById.get(e.TitleID ?? 0)}
					{@const expandable = row.events.length > 1 || extra.length > 0 || (row.steps.length > 0 && details(e).length > 0)}
					<li class="event" data-testid="history-row" data-kind={e.Kind} data-steps={row.steps.length ? row.events.length : undefined}>
						<div class="line">
							{#if eventTitle}<span class="art" class:square={eventTitle.kind === 'artist'} aria-hidden="true"><Poster src={e.artwork ?? eventTitle.artwork[0]} name={eventTitle.name} /></span>{:else}<span class="art blank" aria-hidden="true"></span>{/if}
							<div class="what">
								<p class="sum">
									{#if e.TitleID && e.title_name}<a href={to(`/library/${e.TitleID}`)}>{e.title_name}</a>{#if e.episode}{' '}<span class="ep" data-testid="history-episode">{e.episode}</span>{/if}{#if e.version}{' '}<span class="ver" data-testid="history-version">{e.version}</span>{/if}{': '}{/if}{e.Summary}
								</p>
								<div class="sub">
									{#if row.steps.length}
										<ol class="steps" aria-label="Steps of this download" data-testid="history-steps">
											{#each row.steps as st (st.key)}
												<li class="step {st.state}" data-state={st.state}><span class="pip" aria-hidden="true"></span>{st.label}<span class="sr-only">: {st.state === 'done' ? 'done' : st.state === 'failed' ? 'failed' : 'not yet'}</span></li>
											{/each}
										</ol>
									{:else}
										<StatusPill kind={toneKind(kindTone(e.Kind))} label={groupLabel(e.Kind)} />
									{/if}
									<time datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
								</div>
							</div>
							{#if expandable}
								<Button variant="plain" size="md" class="more" aria-expanded={open === row.key} onclick={() => (open = open === row.key ? null : row.key)}>{row.steps.length ? (open === row.key ? 'Hide steps' : `Every step (${row.events.length})`) : open === row.key ? 'Hide why' : 'Why and where'}</Button>
							{/if}
						</div>
						{#if open === row.key}
							{#if row.steps.length}
								<ol class="trail" data-testid="history-trail">
									{#each row.events as step (step.ID)}
										{@const why = details(step)}
										<li data-kind={step.Kind}>
											<div class="trail-line">
												<StatusPill kind={toneKind(kindTone(step.Kind))} label={groupLabel(step.Kind)} />
												<span class="trail-sum">{step.Summary}</span>
												<time datetime={step.At} title={dateTime(step.At)}>{ago(step.At)}</time>
											</div>
											{#if why.length}<dl class="why">{#each why as [k, v], i (i)}<dt>{k}</dt><dd>{v}</dd>{/each}</dl>{/if}
										</li>
									{/each}
								</ol>
							{:else if extra.length}
								<dl class="why">
									{#each extra as [k, v], i (i)}<dt>{k}</dt><dd>{v}</dd>{/each}
								</dl>
							{/if}
						{/if}
					</li>
				{/each}
			</ol>
			<nav class="pages" aria-label="Pages">
				<Button variant="secondary" icon="chevronLeft" disabled={filter.offset === 0} onclick={() => apply({ offset: Math.max(0, filter.offset - limit) })}>Newer</Button>
				<Button variant="secondary" iconAfter="chevronRight" disabled={filter.offset + events.length >= total} onclick={() => apply({ offset: filter.offset + limit })}>Older</Button>
			</nav>
		</section>
	{/if}
	</PageLoader>
	{/if}
{/if}
{/if}

<VariantBar options={VIEWS} bind:value={view} storageKey="holyarrs.history-view" label="History design" />

<style>
	.filterbar { margin-bottom: 24px; padding: 16px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.phone-bar { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 20px; }
	.filters { display: flex; flex-direction: column; gap: 14px; }
	.fields { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 12px; }
	.field { display: flex; flex-direction: column; gap: 6px; min-width: 0; flex: 0 1 180px; }
	.field.grow { flex: 1 1 260px; }
	.field label, legend { color: var(--text-muted); font: var(--t-subhead); }
	.field-error { color: var(--fail); font: var(--t-callout); }
	.kinds { border: 0; margin: 0; padding: 0; min-width: 0; }
	legend { padding: 0; margin-bottom: 8px; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.episode-filter { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; font: var(--t-callout); }
	.loading { display: grid; gap: 0; }
	.sec { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 4px 12px; margin-bottom: 8px; }
	.sec h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.count { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.panel { margin: 0; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.events { list-style: none; padding: 6px; }
	.event { padding: 10px; border-radius: var(--r-btn); }
	.event + .event { box-shadow: 0 -1px 0 var(--line); }
	.line { display: flex; align-items: center; gap: 12px; min-width: 0; }
	.art { flex: none; width: 32px; --poster-ratio: 2 / 3; }
	.art.square { --poster-ratio: 1; }
	.art.blank { height: 1px; }
	.what { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
	.sum { font: var(--t-callout); overflow-wrap: anywhere; }
	.sum a { color: var(--text); font-weight: 600; }
	.sum a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.ep { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.ver { display: inline-block; padding: 0 6px; border-radius: var(--r-sm); background: var(--surface-2); font: var(--t-footnote); }
	.sub { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.why { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 6px 14px; margin: 10px 0 2px 44px; padding: 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-footnote); }
	.why dt { color: var(--text-muted); }
	.why dd { margin: 0; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.line > :global(.more) { color: var(--text-muted); font-weight: 500; }
	.line > :global(.more:hover) { color: var(--text); }
	.steps { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 0; margin: 0; padding: 0; list-style: none; font: var(--t-footnote); }
	.step { display: inline-flex; align-items: center; gap: 5px; color: var(--text-muted); }
	.step + .step::before { content: ''; width: 14px; height: 1px; margin: 0 6px; background: var(--line-strong, var(--line)); }
	.pip { width: 8px; height: 8px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--text-muted); }
	.step.done { color: var(--text); }
	.step.done .pip { background: var(--ok); box-shadow: none; }
	.step.failed { color: var(--fail); font-weight: 600; }
	.step.failed .pip { background: var(--fail); box-shadow: none; }
	.trail { display: flex; flex-direction: column; gap: 10px; margin: 10px 0 2px 44px; padding: 12px; list-style: none; border-radius: var(--r-btn); background: var(--surface-2); }
	.trail > li + li { padding-top: 10px; box-shadow: 0 -1px 0 var(--line); }
	.trail-line { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 10px; font: var(--t-footnote); }
	.trail-sum { flex: 1 1 200px; min-width: 0; overflow-wrap: anywhere; }
	.trail-line time { color: var(--text-muted); }
	.trail .why { margin: 8px 0 0; padding: 0; background: none; }
	.pages { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
	@media (max-width: 640px) {
		.line { flex-wrap: wrap; }
		.line > :global(.more) { margin-left: 44px; }
		.why { margin-left: 0; grid-template-columns: minmax(0, 1fr); }
		.trail { margin-left: 0; }
		.pages > :global(*) { flex: 1; min-height: 44px; }
	}
</style>
