<script lang="ts">
	import { getBase } from '#lib/paths.ts';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { date } from '#lib/format.ts';
	import { inLibraryWord } from '#lib/library.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
	import {
		byDay,
		entryDay,
		entryKey,
		entryName,
		entryTime,
		feedLink,
		kindWord,
		monthGrid,
		monthRange,
		shiftMonth,
		shows,
		entryHref,
		albumHave,
		webcalLink,
		type TitlesShown
	} from '#lib/calendar.ts';
	import type { CalendarEntry, TitleView } from '#lib/types.ts';

	const now = new Date();
	// ?month=2026-10 opens that month (a link to a month can be shared).
	const asked = /^(\d{4})-(\d{2})$/.exec(page.url.searchParams.get('month') ?? '');
	let year = $state(asked ? Number(asked[1]) : now.getFullYear());
	let month = $state(asked && Number(asked[2]) >= 1 && Number(asked[2]) <= 12 ? Number(asked[2]) - 1 : now.getMonth());
	let view = $state<'month' | 'list'>('month');
	let entries = $state<CalendarEntry[]>([]);
	let libraryEmpty = $state(false);
	let loaded = $state(false);
	let error = $state('');
	let kinds = $state({ cinema: true, digital: true, physical: true, episode: true, album: true });
	// ?titles=shows opens the calendar on one kind of title.
	const askedTitles = page.url.searchParams.get('titles');
	let which = $state<TitlesShown>(askedTitles === 'movies' || askedTitles === 'shows' || askedTitles === 'music' ? askedTitles : 'all');

	// The phone calendar feed.
	let feed = $state<{ set: boolean; created_at?: string } | null>(null);
	let newLink = $state('');
	let feedBusy = $state(false);

	const shown = $derived(entries.filter((e) => kinds[e.kind] && shows(e, which)));
	const weeks = $derived(monthGrid(year, month, shown));
	const monthKey = $derived(`${year}-${String(month + 1).padStart(2, '0')}`);
	const list = $derived(byDay(shown.filter((e) => entryDay(e).startsWith(monthKey))));
	const monthName = $derived(new Date(year, month, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }));
	const weekdays = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

	async function load() {
		try {
			const r = monthRange(year, month);
			const [c, t] = await Promise.all([
				api.get<{ dates: CalendarEntry[] | null }>(`/calendar?from=${r.from}&to=${r.to}`),
				api.get<{ titles: TitleView[] }>('/titles')
			]);
			entries = c.dates ?? [];
			libraryEmpty = t.titles.length === 0;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	function go(n: number) {
		({ year, month } = shiftMonth(year, month, n));
		void load();
	}

	function thisMonth() {
		year = now.getFullYear();
		month = now.getMonth();
		void load();
	}

	async function loadFeed() {
		try {
			feed = await api.get<{ set: boolean; created_at?: string }>('/calendar/feed-key');
		} catch {
			feed = null;
		}
	}

	async function makeKey() {
		if (feed?.set && !confirm('Make a new feed link? The old link stops working on every phone that uses it.')) return;
		feedBusy = true;
		try {
			const r = await api.post<{ path: string }>('/calendar/feed-key');
			newLink = feedLink(location.origin + getBase(), r.path);
			await loadFeed();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			feedBusy = false;
		}
	}

	async function turnOff() {
		if (!confirm('Turn the phone calendar feed off? Every phone that uses the link stops getting dates.')) return;
		try {
			await api.del('/calendar/feed-key');
			newLink = '';
			await loadFeed();
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}

	async function copy() {
		try {
			await navigator.clipboard.writeText(newLink);
			toasts.push('Feed link copied.', 'ok');
		} catch {
			toasts.push('Copy did not work; select the link and copy it by hand.', 'fail');
		}
	}

	onMount(() => {
		void load();
		void loadFeed();
		const offs = [live.on('title', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
</script>

<svelte:head><title>Calendar · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Calendar</h1>
		<p>
			Cinema, digital and disc dates for the movies in your Library, when the episodes of your shows air, in your own time zone, and the days your artists'
			albums come out.
		</p>
	</div>
</header>

<div class="bar-row">
	<div class="row nav-month">
		<button class="btn btn-ghost icon-btn" type="button" onclick={() => go(-1)} aria-label="Month before"><Icon name="back" size={18} /></button>
		<h2 class="h2 month" aria-live="polite" data-testid="cal-month">{monthName}</h2>
		<button class="btn btn-ghost icon-btn flip" type="button" onclick={() => go(1)} aria-label="Month after"><Icon name="back" size={18} /></button>
		<button class="btn btn-quiet" type="button" onclick={thisMonth}>Today</button>
	</div>
	<div class="row seg" role="group" aria-label="View">
		<button class="btn {view === 'month' ? 'btn-ghost on' : 'btn-quiet'}" type="button" aria-pressed={view === 'month'} onclick={() => (view = 'month')}>Month</button>
		<button class="btn {view === 'list' ? 'btn-ghost on' : 'btn-quiet'}" type="button" aria-pressed={view === 'list'} onclick={() => (view = 'list')}>List</button>
	</div>
</div>
<div class="row filters-row">
	<div class="row seg" role="group" aria-label="Movies, shows or music" data-testid="cal-titles">
		{#each [['all', 'Everything'], ['movies', 'Movies'], ['shows', 'Shows'], ['music', 'Music']] as [v, l] (v)}
			<button class="btn {which === v ? 'btn-ghost on' : 'btn-quiet'}" type="button" aria-pressed={which === v} onclick={() => (which = v as TitlesShown)}>{l}</button>
		{/each}
	</div>
	{#if which === 'all' || which === 'movies'}
		<fieldset class="kinds">
			<legend class="sr-only">Which movie dates</legend>
			{#each ['cinema', 'digital', 'physical'] as const as k (k)}
				<label class="pick k-{k}"><input type="checkbox" bind:checked={kinds[k]} /><span class="dot" aria-hidden="true"></span>{kindWord[k]}</label>
			{/each}
		</fieldset>
	{/if}
</div>

{#if !loaded}
	<div class="sk" style="height: 320px"></div>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if libraryEmpty}
	<EmptyState icon="calendar" title="Nothing coming up">
		<p>Add movies, shows and artists to your Library and their dates, episodes and albums show up here.</p>
		{#snippet actions()}<a class="btn btn-red" href={to(`/search`)}>Add a movie or show</a>{/snippet}
	</EmptyState>
{:else if view === 'month'}
	<div class="grid card" data-testid="cal-grid">
		<div class="wk head" aria-hidden="true">
			{#each weekdays as w (w)}<span class="xs muted mono">{w}</span>{/each}
		</div>
		{#each weeks as week, i (i)}
			<div class="wk">
				{#each week as d (d.day)}
					<div class="cell" class:out={!d.inMonth} class:today={d.today} data-day={d.day}>
						<span class="num mono" aria-label={date(d.day + 'T12:00:00')}>{d.date}</span>
						{#each d.items as e (entryKey(e))}
							<a class="ev k-{e.kind}" href={to(entryHref(e))} data-testid="cal-entry" title="{entryName(e)}: {kindWord[e.kind]}{entryTime(e) ? ` at ${entryTime(e)}` : ''}">
								<span class="dot" aria-hidden="true"></span><span class="ev-name">{#if entryTime(e)}<span class="mono">{entryTime(e)}</span> {/if}{entryName(e)}</span><span class="sr-only">, {kindWord[e.kind]}</span>
							</a>
						{/each}
					</div>
				{/each}
			</div>
		{/each}
	</div>
{:else if list.length === 0}
	<p class="card small muted">No dates in {monthName}.</p>
{:else}
	<ol class="days" data-testid="cal-list">
		{#each list as d (d.day)}
			{@const at = new Date(d.day + 'T00:00:00')}
			<li class="day card">
				<div class="when">
					<span class="dow mono">{at.toLocaleDateString(undefined, { weekday: 'short' })}</span>
					<b>{at.getDate()}</b>
					<span class="mon mono">{at.toLocaleDateString(undefined, { month: 'short' })}</span>
				</div>
				<ul class="grow">
					{#each d.items as e (entryKey(e))}
						<li class="item" data-testid="cal-item">
							<a href={to(entryHref(e))}>{entryName(e)}</a>
							<span class="xs muted k-{e.kind}"
								><span class="dot" aria-hidden="true"></span>{kindWord[e.kind]}{#if entryTime(e)}{' at '}{entryTime(e)}{/if}{#if e.network}{' on '}{e.network}{/if}{inLibraryWord(e.state, e.versions)}{#if e.kind === 'album' && albumHave(e)}{' · '}{albumHave(e)}{/if}</span
							>
						</li>
					{/each}
				</ul>
			</li>
		{/each}
	</ol>
{/if}

{#if feed}
	<section class="card phone" aria-labelledby="h-feed" data-testid="cal-feed">
		<h2 id="h-feed" class="h2">Phone calendar</h2>
		<p class="small muted">
			Add these dates and episodes to your phone's calendar with a feed link. The link has its own key: it can only read this calendar, not sign in.
			Your phone asks for the last week and the next four weeks, and shows each episode at its air time in your time zone.
		</p>
		{#if newLink}
			<div class="field">
				<label for="feed-link">Your feed link (shown this once)</label>
				<input id="feed-link" class="input mono" readonly value={newLink} onfocus={(e) => e.currentTarget.select()} />
				<span class="hint">Copy it now: HolyArrs keeps only a fingerprint of the key and cannot show it again. Making a new link ends this one.</span>
			</div>
			<div class="row actions">
				<button class="btn btn-red" type="button" onclick={copy}><Icon name="copy" size={16} /> Copy link</button>
				<a class="btn btn-ghost" href={webcalLink(newLink)}>Open in a calendar app</a>
			</div>
		{:else if feed.set}
			<p class="small" data-testid="feed-state">A feed link is on{feed.created_at ? `, made ${date(feed.created_at)}` : ''}.</p>
		{:else}
			<p class="small" data-testid="feed-state">The feed is off.</p>
		{/if}
		{#if session.can('settings')}
			<div class="row actions">
				<button class="btn {feed.set ? 'btn-ghost' : 'btn-red'}" type="button" onclick={makeKey} disabled={feedBusy} aria-busy={feedBusy}>
					{feed.set ? 'Make a new link' : 'Make a feed link'}
				</button>
				{#if feed.set}<button class="btn btn-quiet" type="button" onclick={turnOff}>Turn the feed off</button>{/if}
			</div>
		{/if}
	</section>
{/if}

<style>
	.bar-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-3);
		flex-wrap: wrap;
		margin-bottom: var(--s-3);
	}
	.nav-month {
		align-items: center;
		gap: var(--s-2);
	}
	.month {
		min-width: 10ch;
		text-align: center;
		text-transform: capitalize;
	}
	.icon-btn {
		width: 40px;
		padding: 0;
		justify-content: center;
	}
	.flip :global(.icon) {
		transform: scaleX(-1);
	}
	.seg .on {
		background: var(--surface-3);
	}
	.kinds {
		border: 0;
		padding: 0;
		margin: 0 0 var(--s-4);
		display: flex;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.pick {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-height: 36px;
		padding: 0 var(--s-3);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-pill);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex: none;
		display: inline-block;
	}
	.k-cinema .dot {
		background: var(--ha-red);
	}
	.k-digital .dot {
		background: var(--info);
	}
	.k-physical .dot {
		background: var(--wait);
	}
	.k-episode .dot {
		background: var(--ok);
	}
	.k-album .dot {
		background: var(--text-muted);
	}
	.filters-row {
		gap: var(--s-3);
		flex-wrap: wrap;
		align-items: center;
		margin-bottom: var(--s-4);
	}
	.filters-row .kinds {
		margin: 0;
	}
	.grid {
		padding: 0;
		overflow: hidden;
	}
	.wk {
		display: grid;
		grid-template-columns: repeat(7, minmax(0, 1fr));
	}
	.wk.head span {
		padding: var(--s-2);
		text-transform: uppercase;
	}
	.cell {
		min-height: 96px;
		padding: var(--s-2);
		border-top: 1px solid var(--line);
		border-left: 1px solid var(--line);
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.cell:first-child {
		border-left: 0;
	}
	.cell.out {
		background: var(--bg);
	}
	.cell.out .num {
		color: var(--text-muted);
		font-style: italic;
	}
	.num {
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.cell.today .num {
		color: var(--text);
		font-weight: var(--weight-semibold);
		text-decoration: underline;
		text-decoration-color: var(--ha-red);
		text-underline-offset: 3px;
	}
	.ev {
		display: flex;
		align-items: center;
		gap: 4px;
		min-height: 24px;
		font-size: var(--text-xs);
		border-radius: var(--r-sm);
		min-width: 0;
	}
	.ev:hover {
		color: var(--ha-red);
	}
	.ev-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.days {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
		max-width: 720px;
	}
	.day {
		display: flex;
		gap: var(--s-5);
		align-items: flex-start;
	}
	.day.card + .day.card {
		margin-top: 0;
	}
	.when {
		width: 52px;
		flex: none;
		display: flex;
		flex-direction: column;
		align-items: center;
		text-transform: uppercase;
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.when b {
		font: var(--weight-semibold) var(--text-xl) / 1.1 var(--font-display);
		color: var(--text);
	}
	.item {
		display: flex;
		flex-direction: column;
		padding: var(--s-1) 0;
	}
	.item a {
		font-weight: var(--weight-semibold);
	}
	.item a:hover {
		color: var(--ha-red);
	}
	.item .xs {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.phone {
		margin-top: var(--s-6);
		max-width: 720px;
	}
	.actions {
		margin-top: var(--s-3);
		flex-wrap: wrap;
	}
	@media (max-width: 700px) {
		.cell {
			min-height: 64px;
			padding: 2px;
		}
		/* Phones show a dot per date; the name stays for screen readers. */
		.ev-name {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip: rect(0 0 0 0);
			white-space: nowrap;
		}
		.ev {
			min-height: 24px;
			min-width: 24px;
			justify-content: center;
		}
		.ev .dot {
			width: 10px;
			height: 10px;
		}
	}
</style>
