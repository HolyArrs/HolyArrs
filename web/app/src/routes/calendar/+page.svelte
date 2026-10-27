<script lang="ts">
	import { sized } from '#lib/tmdb-image.ts';
	import { getBase } from '#lib/paths.ts';
	import { to } from '#lib/paths.ts';
	import { onMount, untrack } from 'svelte';
	import { page } from '$app/state';
	import { api, message, ApiError } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partOff, partOn } from '#lib/parts.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { date } from '#lib/format.ts';
	import { inLibraryWord } from '#lib/library.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import OpsCheckChip from '#lib/ui/OpsCheckChip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import VariantBar from '#lib/ui/VariantBar.svelte';
	import UpNext from '#lib/calendar/UpNext.svelte';
	import ComingSoon from '#lib/calendar/ComingSoon.svelte';
	import WeekStrip from '#lib/calendar/WeekStrip.svelte';
	import Shelves from '#lib/calendar/Shelves.svelte';
	import MonthGrid from '#lib/calendar/MonthGrid.svelte';
	import { calendarLooks } from '#lib/calendar-looks.ts';
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
		entryArt,
		entryIcon,
		albumHave,
		webcalLink,
		weekStart,
		weekRange,
		weekDays,
		shiftWeek,
		readView,
		itemDot,
		passes,
		readPresets,
		savePresets,
		stacked,
		stackedRange,
		weekLabel,
		ymd,
		type CalFilter,
		saveView,
		startView,
		type CalendarView,
		type TitlesShown
	} from '#lib/calendar.ts';
	import type { CalendarEntry, TitleView } from '#lib/types.ts';

	const now = new Date();
	// ?month=2026-10 opens that month (a link to a month can be shared).
	const asked = /^(\d{4})-(\d{2})$/.exec(page.url.searchParams.get('month') ?? '');
	let year = $state(asked ? Number(asked[1]) : now.getFullYear());
	let month = $state(asked && Number(asked[2]) >= 1 && Number(asked[2]) <= 12 ? Number(asked[2]) - 1 : now.getMonth());
	let view = $state<CalendarView>('week');
	// The layouts being compared (?view=1…5, kept for the session); 0 is the page as it is.
	let look = $state(0);
	// The week shown in Week view: this week, or the first week of a linked month.
	let week = $state(weekStart(asked ? new Date(Number(asked[1]), Number(asked[2]) - 1, 1) : now));
	let entries = $state<CalendarEntry[]>([]);
	let libraryEmpty = $state(false);
	let loaded = $state(false);
	let error = $state('');
	let kinds = $state({ cinema: true, digital: true, physical: true, episode: true, album: true });
	let keptOnly = $state(false);
	let haveOnly = $state(false);
	let presets = $state<CalFilter[]>([]);
	let panelOpen = $state(true);
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

	// Music turned off while picked falls back to Everything.
	$effect(() => {
		if (which === 'music' && partOff('music')) which = 'all';
	});
	const shown = $derived(entries.filter((e) => kinds[e.kind] && shows(e, which) && passes(e, { keptOnly, haveOnly })));
	const todayKey = ymd(now);
	const layout = $derived(stacked(week, shown, todayKey));
	const agenda = $derived(byDay(shown));
	// Month (5) loads and pages a whole month; the others the six weeks from the week shown.
	const range = $derived(look === 5 ? monthRange(year, month) : stackedRange(week));
	const rangeName = $derived(
		look === 5 ? new Date(year, month, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) :
		`${week.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} – ${new Date(week.getFullYear(), week.getMonth(), week.getDate() + 7 * 6 - 1).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })}`
	);
	// The mini month picker starts on the month of the first week shown, and can page on its own.
	let pickYear = $state(now.getFullYear());
	let pickMonth = $state(now.getMonth());
	const pickWeeks = $derived(monthGrid(pickYear, pickMonth, [], todayKey));
	const pickName = $derived(new Date(pickYear, pickMonth, 1).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }));
	const shownFrom = $derived(ymd(week));
	const shownTo = $derived(ymd(shiftWeek(week, 6)));
	const weekdays = ['M', 'T', 'W', 'T', 'F', 'S', 'S'];
	const dotWord = { have: 'In the Library', missing: 'Missing', upcoming: 'Coming up' } as const;

	let asks = 0;
	async function load() {
		const n = ++asks;
		try {
			const r = range;
			const [c, t] = await Promise.all([
				api.get<{ dates: CalendarEntry[] | null }>(`/calendar?from=${r.from}&to=${r.to}`),
				api.get<{ titles: TitleView[] }>('/titles')
			]);
			if (n !== asks) return;
			entries = c.dates ?? [];
			libraryEmpty = t.titles.length === 0;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	function jump(d: Date) {
		week = weekStart(d);
		year = d.getFullYear();
		month = d.getMonth();
		pickYear = week.getFullYear();
		pickMonth = week.getMonth();
	}
	const go = (n: number) => {
		if (look !== 5) return jump(shiftWeek(week, n));
		({ year, month } = shiftMonth(year, month, n));
		week = weekStart(new Date(year, month, 1));
	};
	// Each new range (the arrows, Today, a switch to or from Month) is loaded once.
	let started = $state(false);
	$effect(() => {
		void range.from;
		void range.to;
		if (started) untrack(() => void load());
	});
	const thisMonth = () => jump(new Date());
	function pickPage(n: number) {
		({ year: pickYear, month: pickMonth } = shiftMonth(pickYear, pickMonth, n));
	}

	// Weeks or List; the last pick is remembered on this browser.
	function pickView(v: CalendarView) {
		view = v;
		saveView(v);
	}

	// Saved filters live on this browser.
	const kindKeys = Object.keys(kinds) as (keyof typeof kinds)[];
	const current = (name: string): CalFilter => ({ name, which, off: kindKeys.filter((k) => !kinds[k]), keptOnly, haveOnly });
	const isOn = (p: CalFilter) => p.which === which && p.keptOnly === keptOnly && p.haveOnly === haveOnly && kindKeys.every((k) => kinds[k] === !p.off.includes(k));
	function applyPreset(p: CalFilter) {
		which = p.which;
		for (const k of kindKeys) kinds[k] = !p.off.includes(k);
		keptOnly = p.keptOnly;
		haveOnly = p.haveOnly;
	}
	function savePreset() {
		const name = prompt('Name these filters', 'My filters')?.trim();
		if (!name) return;
		presets = [...presets.filter((p) => p.name !== name), current(name)];
		savePresets(presets);
		toasts.push(`Saved the filters as ${name}.`, 'ok');
	}
	function dropPreset(p: CalFilter) {
		presets = presets.filter((x) => x.name !== p.name);
		savePresets(presets);
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
		const v = startView({ remembered: readView(), which, wide: !window.matchMedia('(max-width: 767px)').matches, monthLink: false });
		view = v === 'month' ? 'week' : v;
		presets = readPresets();
		pickYear = week.getFullYear();
		pickMonth = week.getMonth();
		panelOpen = !window.matchMedia('(max-width: 1023px)').matches;
		started = true;
		void loadFeed();
		const offs = [live.on('title', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
</script>

{#snippet item(e: CalendarEntry, compact: boolean)}
	{@const dot = itemDot(e, todayKey)}
	<li class="item k-{e.kind}" data-testid="cal-item">
		{@render art(e)}
		<div class="it">
			<a href={to(entryHref(e))}>{#if compact && entryTime(e)}<span class="t">{entryTime(e)}</span>{' '}{/if}{entryName(e)}</a>
			<span class="sub"
				><span class="sdot {dot}" title={dotWord[dot]} aria-hidden="true"></span><span class="sr-only">{dotWord[dot]}. </span>{kindWord[e.kind]}{#if !compact && entryTime(e)}{' at '}{entryTime(e)}{/if}{#if e.network}{' on '}{e.network}{/if}{inLibraryWord(e.state, e.versions)}{#if e.kind === 'album' && albumHave(e)}{' · '}{albumHave(e)}{/if}</span
			>
		</div>
	</li>
{/snippet}

{#snippet dayRow(d: { day: string; items: CalendarEntry[] })}
	{@const at = new Date(d.day + 'T00:00:00')}
	<li class="day" class:today={d.day === todayKey}>
		<div class="when" aria-hidden="true">
			<span class="dow">{at.toLocaleDateString(undefined, { weekday: 'short' })}</span>
			<b>{at.getDate()}</b>
		</div>
		<div class="items">
			<p class="sr-only">{date(d.day + 'T12:00:00')}</p>
			<ul>{#each d.items as e (entryKey(e))}{@render item(e, false)}{/each}</ul>
		</div>
	</li>
{/snippet}

{#snippet art(e: CalendarEntry)}
	<!-- A 64 px thumbnail: the row size from the artwork cache, not TMDB's full-size still. -->
	{@const src = entryArt(e) ? sized(entryArt(e), 'backdrop', 64) : null}
	<span class="thumb" aria-hidden="true">
		{#if src}<img {src} alt="" loading="lazy" decoding="async" />{:else}<Icon name={entryIcon(e.kind)} size={16} />{/if}
		<span class="dot"></span>
	</span>
{/snippet}

<svelte:head><title>Calendar · HolyArrs</title></svelte:head>

<PageHeader title="Calendar" subtitle="Cinema, digital and disc dates for your movies, when your episodes air in your own time zone, and the days your artists' albums come out.">
	{#snippet actions()}<Button variant="secondary" icon="calendar" onclick={thisMonth}>Today</Button>{/snippet}
</PageHeader>

<section class="tools" class:looked={look > 0} aria-label="Calendar controls">
	<div class="month-nav">
		<IconButton icon="chevronLeft" label={look === 5 ? 'Month before' : 'Week before'} variant="tinted" onclick={() => go(-1)} />
		<h2 class="month" aria-live="polite" data-testid="cal-month">{rangeName}</h2>
		<IconButton icon="chevronRight" label={look === 5 ? 'Month after' : 'Week after'} variant="tinted" onclick={() => go(1)} />
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
				{ value: 'shows', label: 'TV shows' },
				...(partOn('music') ? [{ value: 'music', label: 'Music' }] : [])
			]}
		/>
		{#if !look}
			<SegmentedControl label="View" testid="cal-view" value={view} onchange={(v) => pickView(v as CalendarView)} options={[{ value: 'week', label: 'Weeks' }, { value: 'list', label: 'List' }]} />
			<IconButton icon="sidebar" label={panelOpen ? 'Hide the month' : 'Show the month'} variant="tinted" aria-expanded={panelOpen} aria-controls="cal-side" onclick={() => (panelOpen = !panelOpen)} />
		{/if}
	</div>
	<div class="kinds" role="group" aria-label="Filters">
		{#if which === 'all' || which === 'movies'}
			{#each ['cinema', 'digital', 'physical'] as const as k (k)}
				<OpsCheckChip label={kindWord[k]} bind:checked={kinds[k]} dot={k === 'physical' ? 'var(--wait)' : 'var(--info)'} />
			{/each}
		{/if}
		<Chip label="Kept up to date" selected={keptOnly} onclick={() => (keptOnly = !keptOnly)} />
		<Chip label="In the Library" selected={haveOnly} onclick={() => (haveOnly = !haveOnly)} />
		{#each presets as p (p.name)}
			<span data-testid="cal-preset"><Chip label={p.name} icon="star" selected={isOn(p)} onclick={() => applyPreset(p)} onremove={() => dropPreset(p)} /></span>
		{/each}
		<Chip label="Save filters" icon="plus" onclick={savePreset} />
	</div>
</section>

{#if !loaded}
	<!-- Week-grid placeholder: seven day columns with a few event bars, the real grid's size. -->
	<div class="weeks-sk" aria-busy="true" aria-label="Loading the calendar" data-testid="calendar-skeleton">
	{#each [0, 1, 2] as w (w)}
	<div class="week week-sk">
		{#each Array.from({ length: 7 }, (_, i) => i) as d (d)}
			<div class="wday"><span class="sk sk-line" style="width: 40%; height: 10px"></span><span class="sk sk-line" style="width: 28px; height: 22px"></span>{#each Array.from({ length: (d * 5) % 3 + 1 }, (_, i) => i) as i (i)}<span class="sk" style="height: 26px; border-radius: 6px"></span>{/each}</div>
		{/each}
	</div>
	{/each}
	</div>
{:else if error && entries.length === 0}
	<LoadError hasData={false} message="Calendar could not be loaded." onretry={load} />
{:else if libraryEmpty}
	<EmptyState icon="calendar" title="Nothing coming up" text="Add movies, shows and artists to your Library and their dates, episodes and albums show up here." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
{:else}
	{#if error}<LoadError hasData message="Calendar could not be loaded." onretry={load} />{/if}
	{#if look === 1}
		<UpNext entries={shown} today={todayKey} from={range.from} to={range.to} onlater={() => go(6)} />
	{:else if look === 2}
		<ComingSoon entries={shown} today={todayKey} onlater={() => go(6)} />
	{:else if look === 3}
		<WeekStrip entries={shown} today={todayKey} start={week} days={7 * 6} onlater={() => go(6)} />
	{:else if look === 4}
		<Shelves entries={shown} today={todayKey} onlater={() => go(6)} />
	{:else if look === 5}
		<MonthGrid entries={shown} today={todayKey} {year} {month} />
	{:else}
	<div class="cal-body" class:with-side={panelOpen}>
		<div class="cal-main">
			{#if view === 'week'}
				<div class="stack" data-testid="cal-week">
					{#each layout.weeks as w (ymd(w.start))}
						<section class="wrow" aria-label={weekLabel(w.start, now)}>
							<h3 class="wtitle">{weekLabel(w.start, now)}</h3>
							<ol class="week">
								{#each w.days as d (d.day)}
									{@const at = new Date(d.day + 'T00:00:00')}
									<li class="wday" class:today={d.today} class:empty={!d.items.length} data-day={d.day}>
										<p class="wday-head"><span class="dow">{at.toLocaleDateString(undefined, { weekday: 'short' })}</span> <b class="num">{d.date}</b><span class="sr-only">, {date(d.day + 'T12:00:00')}</span></p>
										{#if d.items.length}
											<ul>{#each d.items as e (entryKey(e))}{@render item(e, true)}{/each}</ul>
										{:else}
											<p class="none">Nothing</p>
										{/if}
									</li>
								{/each}
							</ol>
						</section>
					{/each}
					<section class="wrow" aria-label="Later">
						<h3 class="wtitle">Later</h3>
						{#if layout.later.length}
							<ol class="days">
								{#each layout.later as d (d.day)}{@render dayRow(d)}{/each}
							</ol>
						{:else}
							<p class="none later-none">Nothing else in the next four weeks.</p>
						{/if}
					</section>
				</div>
			{:else if agenda.length === 0}
				<EmptyState compact icon="calendar" title="No dates from {rangeName}" />
			{:else}
				<ol class="days" data-testid="cal-list">
					{#each agenda as d (d.day)}{@render dayRow(d)}{/each}
				</ol>
			{/if}
		</div>
		{#if panelOpen}
			<aside class="side" id="cal-side" aria-label="Pick a week">
				<div class="pick-head">
					<IconButton icon="chevronLeft" size="sm" label="Month before" onclick={() => pickPage(-1)} />
					<b>{pickName}</b>
					<IconButton icon="chevronRight" size="sm" label="Month after" onclick={() => pickPage(1)} />
				</div>
				<div class="pick" data-testid="cal-picker">
					{#each weekdays as w, i (i)}<span class="pw" aria-hidden="true">{w}</span>{/each}
					{#each pickWeeks.flat() as d (d.day)}
						<button
							type="button"
							class="pd"
							class:out={!d.inMonth}
							class:today={d.today}
							class:shown={d.day >= shownFrom && d.day < shownTo}
							aria-label="Show the week of {date(d.day + 'T12:00:00')}"
							onclick={() => jump(new Date(d.day + 'T00:00:00'))}>{d.date}</button
						>
					{/each}
				</div>
				<Button variant="secondary" size="sm" full icon="calendarDay" onclick={thisMonth}>Today</Button>
			</aside>
		{/if}
	</div>
	{/if}
{/if}

{#if loaded && feedLoaded && feedError}
	<div class="feed-gap"><LoadError hasData message="Phone calendar could not be loaded." onretry={loadFeed} /></div>
{/if}

{#if loaded && feedLoaded && feedNeedsProof}
	<section class="feed" aria-labelledby="h-feed-proof">
		<h2 id="h-feed-proof">Confirm your password to manage the phone calendar</h2>
		<p class="muted">The feed status and its controls are protected. Confirm your HolyArrs password in My account, then return here.</p>
		<div><Button variant="secondary" href={to('/settings/account')}>Open My account</Button></div>
	</section>
{/if}

{#if loaded && feed}
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

<VariantBar options={calendarLooks} bind:value={look} param="view" storageKey="holyarrs.calendar-look" label="Calendar layout" />

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
	.ev { display: flex; align-items: center; gap: 6px; min-height: 26px; padding: 2px 6px; border-radius: 6px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; min-width: 0; transition: background var(--dur-fast) var(--ease-out); }
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
	.thumb { position: relative; flex: none; width: 64px; aspect-ratio: 16 / 9; border-radius: var(--r-sm); background: var(--surface-2); color: var(--text-muted); display: grid; place-items: center; overflow: hidden; }
	.thumb img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.thumb .dot { position: absolute; left: 4px; bottom: 4px; box-shadow: 0 0 0 2px var(--surface-1); }
	.it { display: flex; flex-direction: column; min-width: 0; }
	.it a { color: var(--text); font: var(--t-headline); font-size: 15px; overflow-wrap: anywhere; }
	.it a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sub { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; overflow-wrap: anywhere; }

	.week { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); margin: 0; padding: 0; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); outline-offset: -1px; box-shadow: var(--shadow-card); overflow: hidden; }
	.wday { min-width: 0; min-height: 220px; padding: 10px 8px; display: flex; flex-direction: column; gap: 6px; }
	.wday + .wday { border-left: 1px solid var(--line); }
	.wday-head { display: flex; align-items: center; gap: 6px; margin: 0; }
	.wday .num { min-width: 26px; height: 26px; }
	.wday ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.wday .it a { font-size: 14px; }
	.wday .t { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.none { color: var(--text-muted); font: var(--t-footnote); }
	.cal-body { display: grid; grid-template-columns: minmax(0, 1fr); gap: 20px; align-items: start; }
	.cal-body.with-side { grid-template-columns: minmax(0, 1fr) 248px; }
	.cal-main { min-width: 0; }
	.stack { display: flex; flex-direction: column; gap: 22px; }
	.wtitle { margin: 0 0 8px; font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	/* Empty days step back by colour, not opacity, so their text keeps its contrast. */
	.wday.empty .dow { color: var(--text-muted); }
	.wday.today { background: var(--accent-soft); }
	.wday.today .dow, .wday.today .none { color: var(--text); }
	.later-none { padding: 12px 0; }
	.day.today { outline-color: var(--accent-fill); }
	.sdot { display: inline-block; width: 7px; height: 7px; margin-right: 6px; border-radius: 50%; vertical-align: 1px; background: var(--text-faint); }
	.sdot.have { background: var(--ok); }
	.sdot.missing { background: var(--fail); }
	.sdot.upcoming { background: var(--info); }
	.side { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; gap: 10px; padding: 14px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.pick-head { display: flex; align-items: center; justify-content: space-between; gap: 6px; font: var(--t-subhead); }
	.pick { display: grid; grid-template-columns: repeat(7, 1fr); gap: 2px; text-align: center; }
	.pw { color: var(--text-muted); font: var(--t-caption); padding: 4px 0; }
	.pd { height: 30px; border-radius: 999px; font: var(--t-footnote); font-variant-numeric: tabular-nums; color: var(--text); }
	.pd.out { color: var(--text-muted); }
	.pd.shown { background: var(--surface-2); }
	.pd.today { background: var(--text); color: var(--bg); font-weight: 700; }
	.pd:hover { background: var(--surface-3); }
	.pd:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 1023px) {
		.cal-body.with-side { grid-template-columns: minmax(0, 1fr); }
		.side { position: static; order: -1; }
	}
	@media (max-width: 1023px) {
		.week { grid-template-columns: minmax(0, 1fr); }
		.wday { min-height: 0; flex-direction: row; align-items: flex-start; gap: 14px; }
		.wday + .wday { border-left: 0; border-top: 1px solid var(--line); }
		.wday-head { flex: none; width: 64px; }
		.wday ul, .none { flex: 1; min-width: 0; }
	}

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
		/* The layouts being compared: the filter chips are one sideways row on phones. */
		.looked .kinds { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; margin: 0 calc(-1 * var(--gutter)); padding: 2px var(--gutter); }
		.looked .kinds::-webkit-scrollbar { display: none; }
		.looked .kinds > :global(*) { flex: none; }
	}
	.weeks-sk { display: flex; flex-direction: column; gap: 16px; }
</style>
