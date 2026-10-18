<script lang="ts">
	import { getBase } from '#lib/paths.ts';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message, ApiError } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { date } from '#lib/format.ts';
	import { inLibraryWord } from '#lib/library.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import OpsCheckChip from '#lib/ui/OpsCheckChip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
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
	let feedLoaded = $state(false);
	let feedError = $state('');
	let feedNeedsProof = $state(false);
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
		if (!session.can('manage_library')) {
			feedLoaded = true;
			return;
		}
		if (session.s.via !== 'api_key' && !session.s.localRecent) {
			feed = null;
			feedError = '';
			feedNeedsProof = true;
			feedLoaded = true;
			return;
		}
		try {
			feed = await api.get<{ set: boolean; created_at?: string }>('/calendar/feed-key');
			feedError = '';
			feedNeedsProof = false;
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) {
				feed = null;
				feedError = '';
				feedNeedsProof = true;
			} else feedError = message(e);
		} finally {
			feedLoaded = true;
		}
	}

	async function makeKey() {
		if (session.s.via !== 'api_key' && !session.s.localRecent) {
			feedNeedsProof = true;
			feed = null;
			return;
		}
		if (feed?.set && !confirm('Make a new feed link? The old link stops working on every phone that uses it.')) return;
		feedBusy = true;
		try {
			const r = await api.post<{ path: string }>('/calendar/feed-key');
			newLink = feedLink(location.origin + getBase(), r.path);
			await loadFeed();
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) {
				feedNeedsProof = true;
				feed = null;
			} else toasts.push(message(e), 'fail');
		} finally {
			feedBusy = false;
		}
	}

	async function turnOff() {
		if (session.s.via !== 'api_key' && !session.s.localRecent) {
			feedNeedsProof = true;
			feed = null;
			return;
		}
		if (!confirm('Turn the phone calendar feed off? Every phone that uses the link stops getting dates.')) return;
		try {
			await api.del('/calendar/feed-key');
			newLink = '';
			await loadFeed();
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) {
				feedNeedsProof = true;
				feed = null;
			} else toasts.push(message(e), 'fail');
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
		if (window.matchMedia('(max-width: 767px)').matches) view = 'list';
		void load();
		void loadFeed();
		const offs = [live.on('title', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
</script>

<svelte:head><title>Calendar · HolyArrs</title></svelte:head>

<PageHeader title="Calendar" subtitle="Cinema, digital and disc dates for your movies, when your episodes air in your own time zone, and the days your artists' albums come out.">
	{#snippet actions()}<Button variant="secondary" icon="calendar" onclick={thisMonth}>Today</Button>{/snippet}
</PageHeader>

<section class="tools" aria-label="Calendar controls">
	<div class="month-nav">
		<IconButton icon="chevronLeft" label="Month before" variant="tinted" onclick={() => go(-1)} />
		<h2 class="month" aria-live="polite" data-testid="cal-month">{monthName}</h2>
		<IconButton icon="chevronRight" label="Month after" variant="tinted" onclick={() => go(1)} />
	</div>
	<div class="segs">
		<SegmentedControl
			label="Movies, shows or music"
			testid="cal-titles"
			value={which}
			onchange={(v) => (which = v as TitlesShown)}
			options={[
				{ value: 'all', label: 'Everything' },
				{ value: 'movies', label: 'Movies' },
				{ value: 'shows', label: 'Shows' },
				{ value: 'music', label: 'Music' }
			]}
		/>
		<SegmentedControl label="View" value={view} onchange={(v) => (view = v as 'month' | 'list')} options={[{ value: 'month', label: 'Month' }, { value: 'list', label: 'List' }]} />
	</div>
	{#if which === 'all' || which === 'movies'}
		<fieldset class="kinds">
			<legend class="sr-only">Which movie dates</legend>
			{#each ['cinema', 'digital', 'physical'] as const as k (k)}
				<OpsCheckChip label={kindWord[k]} bind:checked={kinds[k]} dot={k === 'physical' ? 'var(--wait)' : 'var(--info)'} />
			{/each}
		</fieldset>
	{/if}
</section>

{#if !loaded}
	<Skeleton shape="block" height="420px" />
{:else if error && entries.length === 0}
	<LoadError hasData={false} message="Calendar could not be loaded." onretry={load} />
{:else if libraryEmpty}
	<EmptyState icon="calendar" title="Nothing coming up" text="Add movies, shows and artists to your Library and their dates, episodes and albums show up here." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
{:else}
	{#if error}<LoadError hasData message="Calendar could not be loaded." onretry={load} />{/if}
	{#if view === 'month'}
		<!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users need to focus and scroll this region) -->
		<div class="month-scroll" role="region" tabindex="0" aria-label="{monthName} month calendar; scroll horizontally to see every day">
			<div class="grid" data-testid="cal-grid">
				<div class="wk head" aria-hidden="true">
					{#each weekdays as w (w)}<span>{w}</span>{/each}
				</div>
				{#each weeks as week, i (i)}
					<div class="wk">
						{#each week as d (d.day)}
							<div class="cell" class:out={!d.inMonth} class:today={d.today} data-day={d.day}>
								<span class="num"><span aria-hidden="true">{d.date}</span><span class="sr-only">{date(d.day + 'T12:00:00')}. </span></span>
								{#each d.items as e (entryKey(e))}
									<a class="ev k-{e.kind}" href={to(entryHref(e))} data-testid="cal-entry" title="{entryName(e)}: {kindWord[e.kind]}{entryTime(e) ? ` at ${entryTime(e)}` : ''}">
										<span class="dot" aria-hidden="true"></span><span class="ev-name">{#if entryTime(e)}<span class="t">{entryTime(e)}</span>{' '}{/if}{entryName(e)}</span><span class="sr-only">, {kindWord[e.kind]}</span>
									</a>
								{/each}
							</div>
						{/each}
					</div>
				{/each}
			</div>
		</div>
	{:else if list.length === 0}
		<EmptyState compact icon="calendar" title="No dates in {monthName}" />
	{:else}
		<ol class="days" data-testid="cal-list">
			{#each list as d (d.day)}
				{@const at = new Date(d.day + 'T00:00:00')}
				<li class="day">
					<div class="when" aria-hidden="true">
						<span class="dow">{at.toLocaleDateString(undefined, { weekday: 'short' })}</span>
						<b>{at.getDate()}</b>
					</div>
					<div class="items">
						<p class="sr-only">{date(d.day + 'T12:00:00')}</p>
						<ul>
							{#each d.items as e (entryKey(e))}
								<li class="item k-{e.kind}" data-testid="cal-item">
									<span class="dot" aria-hidden="true"></span>
									<div class="it">
										<a href={to(entryHref(e))}>{entryName(e)}</a>
										<span class="sub">{kindWord[e.kind]}{#if entryTime(e)}{' at '}{entryTime(e)}{/if}{#if e.network}{' on '}{e.network}{/if}{inLibraryWord(e.state, e.versions)}{#if e.kind === 'album' && albumHave(e)}{' · '}{albumHave(e)}{/if}</span>
									</div>
								</li>
							{/each}
						</ul>
					</div>
				</li>
			{/each}
		</ol>
	{/if}
{/if}

{#if feedLoaded && feedError}
	<div class="feed-gap"><LoadError hasData message="Phone calendar could not be loaded." onretry={loadFeed} /></div>
{/if}

{#if feedLoaded && feedNeedsProof}
	<section class="feed" aria-labelledby="h-feed-proof">
		<h2 id="h-feed-proof">Confirm your password to manage the phone calendar</h2>
		<p class="muted">The feed status and its controls are protected. Confirm your HolyArrs password in My account, then return here.</p>
		<div><Button variant="secondary" href={to('/settings/account')}>Open My account</Button></div>
	</section>
{/if}

{#if feed}
	<section class="feed" aria-labelledby="h-feed" data-testid="cal-feed">
		<h2 id="h-feed">Phone calendar</h2>
		<p class="muted">
			Add these dates and episodes to your phone's calendar with a feed link. The link has its own key: it can only read this calendar, not sign in. Your phone asks for the
			last week and the next four weeks, and shows each episode at its air time in your time zone.
		</p>
		{#if newLink}
			<div class="field">
				<label for="feed-link">Your feed link (shown this once)</label>
				<input id="feed-link" class="input mono" readonly value={newLink} onfocus={(e) => e.currentTarget.select()} />
				<span class="hint">Copy it now: HolyArrs keeps only a fingerprint of the key and cannot show it again. Making a new link ends this one.</span>
			</div>
			<div class="acts">
				<Button variant="primary" icon="copy" onclick={copy}>Copy link</Button>
				<Button variant="secondary" href={webcalLink(newLink)}>Open in a calendar app</Button>
			</div>
		{:else if feed.set}
			<p data-testid="feed-state">A feed link is on{feed.created_at ? `, made ${date(feed.created_at)}` : ''}.</p>
		{:else}
			<p data-testid="feed-state">The feed is off.</p>
		{/if}
		{#if session.can('settings')}
			<div class="acts">
				<Button variant={feed.set ? 'secondary' : 'primary'} onclick={makeKey} disabled={feedBusy} busy={feedBusy}>{feed.set ? 'Make a new link' : 'Make a feed link'}</Button>
				{#if feed.set}<Button variant="plain" onclick={turnOff}>Turn the feed off</Button>{/if}
			</div>
		{/if}
	</section>
{/if}

<style>
	.tools { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 20px; margin-bottom: 18px; }
	.month-nav { display: flex; align-items: center; gap: 8px; margin-right: auto; }
	.month { min-width: 9.5ch; font: var(--t-title2); letter-spacing: var(--t-title2-track); text-align: center; }
	.segs { display: flex; flex-wrap: wrap; gap: 10px; max-width: 100%; }
	.kinds { display: flex; flex-wrap: wrap; gap: 8px; border: 0; margin: 0; padding: 0; min-width: 0; flex-basis: 100%; }
	.dot { width: 8px; height: 8px; border-radius: 50%; flex: none; display: inline-block; }
	.k-cinema .dot, .k-digital .dot { background: var(--info); }
	.k-physical .dot { background: var(--wait); }
	.k-episode .dot { background: var(--ok); }
	.k-album .dot { background: var(--text-muted); }

	.month-scroll { overflow-x: auto; border-radius: var(--r-panel); }
	.month-scroll:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.grid { min-width: 760px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); outline-offset: -1px; box-shadow: var(--shadow-card); overflow: hidden; }
	.wk { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); }
	.wk + .wk { border-top: 1px solid var(--line); }
	.head span { padding: 10px 10px 8px; color: var(--text-muted); font: var(--t-footnote); }
	.cell { min-height: 112px; padding: 6px; display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.cell + .cell { border-left: 1px solid var(--line); }
	.cell.out { background: transparent; }
	.cell.out .num { color: var(--text-muted); }
	.num { align-self: flex-start; display: inline-grid; place-items: center; min-width: 26px; height: 26px; padding: 0 4px; border-radius: 999px; color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.today .num { background: var(--text); color: var(--bg); font-weight: 700; }
	.ev { display: flex; align-items: center; gap: 6px; min-height: 26px; padding: 2px 6px; border-radius: 6px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; min-width: 0; transition: background var(--dur-fast); }
	.ev:hover { background: var(--surface-3); }
	.ev:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ev-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ev .t { color: var(--text-muted); font-variant-numeric: tabular-nums; }

	.days { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; max-width: 860px; }
	.day { display: grid; grid-template-columns: 56px minmax(0, 1fr); gap: 14px; padding: 12px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.when { display: flex; flex-direction: column; align-items: center; padding-top: 2px; }
	.dow { color: var(--accent-text); font: var(--t-caption); text-transform: none; }
	.when b { font: var(--t-title2); font-variant-numeric: tabular-nums; }
	.items ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.item { display: flex; align-items: flex-start; gap: 10px; padding: 8px 0; min-width: 0; }
	.item + .item { border-top: 1px solid var(--line); }
	.item .dot { margin-top: 7px; }
	.it { display: flex; flex-direction: column; min-width: 0; }
	.it a { color: var(--text); font: var(--t-headline); font-size: 15px; overflow-wrap: anywhere; }
	.it a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sub { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; overflow-wrap: anywhere; }

	.feed-gap { margin-top: 24px; }
	.feed { margin-top: 28px; max-width: 860px; padding: 18px; display: flex; flex-direction: column; gap: 12px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.feed h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.feed p { font: var(--t-callout); }
	.muted { color: var(--text-muted); }
	.field { display: flex; flex-direction: column; gap: 6px; }
	.field label { font: var(--t-subhead); color: var(--text-muted); }
	.hint { color: var(--text-muted); font: var(--t-footnote); }
	.acts { display: flex; flex-wrap: wrap; gap: 8px; }
	@media (max-width: 640px) {
		.month-nav { width: 100%; justify-content: space-between; margin-right: 0; }
		.segs, .segs > :global(.ui-seg) { width: 100%; }
		.acts > :global(*) { flex: 1 1 100%; }
	}
</style>
