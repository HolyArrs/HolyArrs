<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Poster from '#lib/Poster.svelte';
	import { details, filterParams, groupLabel, historyQuery, kindGroups, kindTone, pageLine, readFilter, type HistoryFilter } from '#lib/history.ts';
	import type { EventPage, EventRecord, TitleView } from '#lib/types.ts';

	const limit = 50;
	let filter = $state<HistoryFilter>(readFilter(page.url.searchParams, limit));
	let events = $state<EventRecord[]>([]);
	let total = $state(0);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let open = $state<number | null>(null);

	async function load() {
		try {
			const p = await api.get<EventPage>('/events?' + historyQuery(filter));
			events = p.events;
			total = p.total;
			episodeName = p.episode || 'one episode';
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	// The filters live in the address, so a filtered view can be shared and Back works.
	function apply(change: Partial<HistoryFilter>) {
		filter = { ...filter, offset: 0, ...change };
		void goto(to('/downloads/history') + filterParams(filter), { replace: true, reset: false });
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
	/** The episode the rows are filtered to, named from the rows themselves. */
	let episodeName = $state('');

	onMount(() => {
		void load();
		void api
			.get<{ titles: TitleView[] }>('/titles')
			.then((r) => (titles = r.titles))
			.catch(() => {});
		const offs = [live.on('event', () => filter.offset === 0 && load()), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
</script>

<svelte:head><title>History · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<p class="eyebrow">Operations</p>
		<h1>History</h1>
		<p>Everything HolyArrs did, newest first: what it added, searched, held, chose, sent, imported, renamed and deleted, and why.</p>
	</div>
	<a class="btn btn-ghost" href={to(`/downloads`)}>Back to downloads</a>
</header>

<div class="history-workspace">
<form class="card filters" aria-label="Filter the history" onsubmit={(e) => e.preventDefault()}>
	<div class="row-fields">
		<div class="field">
			<label for="h-movie">Movie or show</label>
			<select id="h-movie" class="input" value={filter.title ?? ''} onchange={(e) => apply({ title: Number(e.currentTarget.value) || undefined, episode: undefined })}>
				<option value="">Every movie and show</option>
				{#each titles as t (t.id)}<option value={t.id}>{t.name}{t.year ? ` (${t.year})` : ''}</option>{/each}
			</select>
		</div>
		<div class="field">
			<label for="h-from">From</label>
			<input id="h-from" class="input" type="date" value={filter.from ?? ''} onchange={(e) => apply({ from: e.currentTarget.value || undefined })} />
		</div>
		<div class="field">
			<label for="h-to">To</label>
			<input id="h-to" class="input" type="date" value={filter.to ?? ''} onchange={(e) => apply({ to: e.currentTarget.value || undefined })} />
		</div>
	</div>
	<fieldset class="kinds">
		<legend class="label xs muted">What happened</legend>
		<div class="chips">
			{#each kindGroups as g (g.key)}
				<label class="pick">
					<input type="checkbox" checked={filter.groups.includes(g.key)} onchange={(e) => toggleGroup(g.key, e.currentTarget.checked)} />
					<span>{g.label}</span>
				</label>
			{/each}
		</div>
	</fieldset>
	{#if filter.episode}
		<p class="small episode-filter" data-testid="episode-filter">
			Only {episodeName}{movieName ? ` of ${movieName}` : ''}.
			<button class="btn btn-quiet" type="button" onclick={() => apply({ episode: undefined })}>All of the show</button>
		</p>
	{/if}
	{#if filtered}<button class="btn btn-quiet" type="button" onclick={clear}>Show everything</button>{/if}
</form>

<div class="history-results">
{#if error}
	<div class="notice bad load-error" role="alert">
		<div><b>History could not be loaded.</b><br /><span class="small">{error}</span></div>
		<button class="btn btn-ghost" type="button" onclick={load}>Try again</button>
	</div>
{:else if !loaded}
	<div class="sk" style="height: 240px"></div>
{:else if events.length === 0}
	<EmptyState icon="calendar" title={filtered ? 'Nothing matches' : 'No history yet'}>
		<p>{filtered ? 'No Event matches these filters.' : 'Every search, download, import, rename and delete writes a row here.'}</p>
	</EmptyState>
{:else}
	<section class="results-surface" aria-labelledby="history-results-title">
	<div class="results-head">
		<div><p class="eyebrow">Event log</p><h2 id="history-results-title" class="h2">Activity</h2></div>
		<p class="small muted count" aria-live="polite" data-testid="history-count">
			{pageLine(total, filter.offset, events.length)}{movieName ? ` for ${movieName}` : ''}
		</p>
	</div>
	<ol class="rows events" data-testid="history">
		{#each events as e (e.ID)}
			{@const extra = details(e)}
			{@const eventTitle = titleById.get(e.TitleID ?? 0)}
			<li class="event" data-testid="history-row" data-kind={e.Kind}>
				<div class="line">
					{#if eventTitle}<span class="event-art" class:square={eventTitle.kind === 'artist'} aria-hidden="true"><Poster src={e.artwork ?? eventTitle.artwork[0]} name={eventTitle.name} /></span>{/if}
					<span class="chip {kindTone(e.Kind)}">{groupLabel(e.Kind)}</span>
					<span class="grow small">
						{#if e.TitleID && e.title_name}<a href={to(`/library/${e.TitleID}`)}>{e.title_name}</a>{#if e.episode}{' '}<span class="mono xs ep" data-testid="history-episode">{e.episode}</span>{/if}{#if e.version}{' '}<span class="chip" data-testid="history-version">{e.version}</span>{/if}: {/if}{e.Summary}
					</span>
					<time class="mono xs muted" datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
				</div>
				{#if extra.length}
					<button class="btn btn-quiet more" type="button" aria-expanded={open === e.ID} onclick={() => (open = open === e.ID ? null : e.ID)}>
						{open === e.ID ? 'Hide why' : 'Why and where'}
					</button>
					{#if open === e.ID}
						<dl class="facts why">
							{#each extra as [k, v], i (i)}<dt>{k}</dt><dd class="mono xs">{v}</dd>{/each}
						</dl>
					{/if}
				{/if}
			</li>
		{/each}
	</ol>
	<nav class="row pages" aria-label="Pages">
		<button class="btn btn-ghost" type="button" disabled={filter.offset === 0} onclick={() => apply({ offset: Math.max(0, filter.offset - limit) })}>Newer</button>
		<button class="btn btn-ghost" type="button" disabled={filter.offset + events.length >= total} onclick={() => apply({ offset: filter.offset + limit })}>Older</button>
	</nav>
	</section>
{/if}
</div>
</div>

<style>
	.eyebrow {
		margin: 0 0 var(--s-2);
		font-size: 11px;
		font-weight: var(--weight-semibold);
		letter-spacing: 0.09em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.history-workspace {
		display: grid;
		grid-template-columns: 280px minmax(0, 1fr);
		gap: var(--s-4);
		align-items: start;
	}
	.history-results {
		min-width: 0;
	}
	.filters {
		position: sticky;
		top: calc(var(--topbar) + var(--s-4));
		margin: 0;
		padding: var(--s-4);
		box-shadow: none;
	}
	.episode-filter {
		margin-top: var(--s-3);
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--s-2);
	}
	.ep {
		color: var(--text-muted);
	}
	.row-fields {
		display: grid;
		grid-template-columns: 1fr;
		gap: var(--s-3);
	}
	.row-fields .field + .field {
		margin-top: 0;
	}
	.kinds {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.label {
		text-transform: uppercase;
		letter-spacing: 0.06em;
		margin-bottom: var(--s-2);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
	}
	.pick {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-height: 40px;
		padding: 0 var(--s-3);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-sm);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.pick:has(input:checked) {
		background: var(--text);
		border-color: var(--text);
		color: var(--surface-1);
	}
	.results-surface {
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
		overflow: hidden;
	}
	.results-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-4);
		min-height: 64px;
		padding: var(--s-3) var(--s-4);
		border-bottom: 1px solid var(--line);
	}
	.results-head .eyebrow {
		margin-bottom: 2px;
	}
	.count {
		margin: 0;
		text-align: right;
	}
	.results-surface .rows {
		padding: 0;
		border: 0;
		border-radius: 0;
		box-shadow: none;
	}
	.event {
		flex-direction: column;
		align-items: stretch;
		padding: var(--s-3) var(--s-4);
	}
	.line {
		display: flex;
		align-items: baseline;
		gap: var(--s-3);
		min-width: 0;
	}
	.event-art {
		flex: 0 0 36px;
		width: 36px;
		align-self: flex-start;
		--poster-ratio: 2 / 3;
	}
	.event-art.square {
		--poster-ratio: 1;
	}
	.line .grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.line a {
		font-weight: var(--weight-semibold);
	}
	.line a:hover {
		color: var(--info);
	}
	.more {
		align-self: flex-start;
		min-height: 32px;
		padding: 0 var(--s-2);
		font-size: var(--text-xs);
	}
	.why dd {
		overflow-wrap: anywhere;
	}
	.pages {
		margin: 0;
		padding: var(--s-3) var(--s-4);
		border-top: 1px solid var(--line);
		justify-content: flex-end;
	}
	.load-error {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-4);
	}
	@media (max-width: 960px) {
		.history-workspace {
			grid-template-columns: 1fr;
		}
		.filters {
			position: static;
		}
		.row-fields {
			grid-template-columns: minmax(0, 2fr) minmax(0, 1fr) minmax(0, 1fr);
		}
	}
	@media (max-width: 700px) {
		.page-head > :global(.btn) {
			width: 100%;
			min-height: 44px;
		}
		.row-fields {
			grid-template-columns: 1fr;
		}
		.row-fields .field + .field {
			margin-top: var(--s-3);
		}
		.line {
			flex-wrap: wrap;
		}
		.results-head {
			align-items: flex-start;
			flex-direction: column;
		}
		.count {
			text-align: left;
		}
		.pick,
		.pages :global(.btn) {
			min-height: 44px;
		}
		.load-error {
			align-items: stretch;
			flex-direction: column;
		}
	}
</style>
