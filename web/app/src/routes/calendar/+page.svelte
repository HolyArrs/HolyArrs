<script lang="ts">
	import { sized } from '#lib/tmdb-image.ts';
	import { getBase } from '#lib/paths.ts';
	import { to } from '#lib/paths.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { api, message, ApiError } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partOff, partOn } from '#lib/parts.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { date } from '#lib/format.ts';
	import { inLibraryWord } from '#lib/library.ts';
	import { glideScroll } from '#lib/ui/transitions.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import OpsCheckChip from '#lib/ui/OpsCheckChip.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import ComingSoon from '#lib/calendar/ComingSoon.svelte';
	import MiniMonth from '#lib/calendar/MiniMonth.svelte';
	import {
		byDay,
		entryKey,
		entryName,
		entryTime,
		feedLink,
		kindWord,
		shiftMonth,
		shows,
		entryHref,
		entryArt,
		entryIcon,
		albumHave,
		webcalLink,
		weekStart,
		shiftWeek,
		itemDot,
		passes,
		readPresets,
		savePresets,
		ymd,
		readView,
		saveView,
		startView,
		type CalFilter,
		type CalendarView,
		type TitlesShown
	} from '#lib/calendar.ts';
	import type { CalendarEntry, TitleView } from '#lib/types.ts';

	const now = new Date();
	// ?month=2026-10 opens on that month (a link to a month can be shared).
	const asked = /^(\d{4})-(\d{2})$/.exec(page.url.searchParams.get('month') ?? '');
	const askedMonth = asked && Number(asked[2]) >= 1 && Number(asked[2]) <= 12 ? new Date(Number(asked[1]), Number(asked[2]) - 1, 1) : null;
	// Cards (big pictures, day by day) or List (compact rows): ?view=cards|list, kept for the
	// session; the old layout links (?view=2&span=…) land on Cards.
	let view = $state<CalendarView>(startView(page.url.searchParams.get('view'), readView()));
	// The first week shown: this week, or the first week of a linked month.
	let week = $state(weekStart(askedMonth ?? now));
	// How many weeks are loaded; six more each time the end comes near (a year at most).
	const REACH = 6;
	const MAX_WEEKS = 52;
	let reach = $state(REACH);
	let entries = $state<CalendarEntry[]>([]);
	let libraryEmpty = $state(false);
	let loaded = $state(false);
	let fetching = $state(false);
	let error = $state('');
	let kinds = $state({ cinema: true, digital: true, physical: true, episode: true, album: true });
	let keptOnly = $state(false);
	let haveOnly = $state(false);
	let presets = $state<CalFilter[]>([]);
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
	const agenda = $derived(byDay(shown));
	const end = $derived(shiftWeek(week, reach));
	const range = $derived({ from: ymd(week), to: ymd(end) });
	const rangeName = $derived(
		`${week.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} – ${new Date(end.getFullYear(), end.getMonth(), end.getDate() - 1).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })}`
	);

	// The small month: starts on the first week's month, pages on its own, and follows the day in view.
	let pickYear = $state(untrack(() => week.getFullYear()));
	let pickMonth = $state(untrack(() => week.getMonth()));
	let active = $state<string | null>(null);
	let sheetOpen = $state(false);

	let asks = 0;
	async function load() {
		const n = ++asks;
		fetching = true;
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
			if (n === asks) fetching = false;
		}
	}

	function jump(d: Date) {
		week = weekStart(d);
		reach = REACH;
		pickYear = week.getFullYear();
		pickMonth = week.getMonth();
	}
	const go = (n: number) => jump(shiftWeek(week, n));
	// Each new range (the arrows, Today, more weeks, the small month) is loaded once.
	let started = $state(false);
	$effect(() => {
		void range.from;
		void range.to;
		if (started) untrack(() => void load());
	});
	function toToday() {
		jump(new Date());
		window.scrollTo({ top: 0, behavior: glideScroll() });
	}

	// The small month's arrows; a month outside what is loaded gets loaded (within a year).
	const DAY = 86400000;
	function pickPage(n: number) {
		({ year: pickYear, month: pickMonth } = shiftMonth(pickYear, pickMonth, n));
		const first = new Date(pickYear, pickMonth, 1);
		const after = new Date(pickYear, pickMonth + 1, 1);
		if (first < week) {
			const from = weekStart(first);
			const weeks = Math.ceil((end.getTime() - from.getTime()) / (7 * DAY));
			week = from;
			reach = Math.min(MAX_WEEKS, Math.max(REACH, weeks));
		} else if (after > end) {
			reach = Math.min(MAX_WEEKS, Math.ceil((after.getTime() - week.getTime()) / (7 * DAY)));
		}
	}
	// A day picked in the small month: glide to its row (instant under reduced motion).
	async function pickDay(day: string) {
		const fromSheet = sheetOpen;
		sheetOpen = false;
		if (fromSheet) await tick();
		const row = document.querySelector<HTMLElement>(`.cal-main [data-day="${day}"]`);
		if (!row) return;
		active = day;
		row.scrollIntoView({ block: 'start', behavior: glideScroll() });
	}
	// The day at the top of the page, for the small month (which pages along when the month changes).
	function follow() {
		const rows = document.querySelectorAll<HTMLElement>('.cal-main [data-day]');
		const line = 140;
		let top: string | null = null;
		for (const r of rows) {
			if (r.getBoundingClientRect().top <= line) top = r.dataset.day ?? null;
			else break;
		}
		top ??= rows[0]?.dataset.day ?? null;
		if (top === active) return;
		active = top;
		if (top) {
			const d = new Date(top + 'T00:00:00');
			pickYear = d.getFullYear();
			pickMonth = d.getMonth();
		}
	}
	let ticking = false;
	function onScroll() {
		if (ticking) return;
		ticking = true;
		requestAnimationFrame(() => {
			ticking = false;
			follow();
		});
	}

	// Cards or List: in the address and kept for the session.
	function pickView(v: CalendarView) {
		view = v;
		saveView(v);
		const u = new URL(page.url.href);
		u.searchParams.set('view', v);
		u.searchParams.delete('span');
		replaceState(u, page.state);
		active = null;
		void tick().then(follow);
	}

	// Near the end, the next six weeks load.
	function more() {
		if (fetching || reach >= MAX_WEEKS) return;
		reach = Math.min(MAX_WEEKS, reach + REACH);
	}
	function endless(node: HTMLElement, busy: boolean) {
		const near = () => node.getBoundingClientRect().top < innerHeight + 900;
		const io = new IntersectionObserver((es) => es.some((e) => e.isIntersecting) && more(), { rootMargin: '0px 0px 900px 0px' });
		io.observe(node);
		return {
			// When a load ends with the end still near, ask for the next weeks.
			update(now: boolean) {
				if (!now && busy && near()) void tick().then(more);
				busy = now;
			},
			destroy: () => io.disconnect()
		};
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

	const dotWord = { have: 'In the Library', missing: 'Missing', upcoming: 'Coming up' } as const;

	onMount(() => {
		presets = readPresets();
		started = true;
		saveView(view);
		// The address always names the view; old layout parameters go.
		const u = new URL(page.url.href);
		if (u.searchParams.get('view') !== view || u.searchParams.has('span')) {
			u.searchParams.set('view', view);
			u.searchParams.delete('span');
			replaceState(u, page.state);
		}
		void loadFeed();
		const offs = [live.on('title', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});
	// Rows change as dates load or filters change: the small month follows the day in view.
	$effect(() => {
		void shown;
		void view;
		if (loaded) void tick().then(follow);
	});
</script>

<svelte:window onscroll={onScroll} />

{#snippet item(e: CalendarEntry)}
	{@const dot = itemDot(e, todayKey)}
	<li class="item k-{e.kind}" data-testid="cal-item">
		{@render art(e)}
		<div class="it">
			<a href={to(entryHref(e))}>{entryName(e)}</a>
			<span class="sub"
				><span class="sdot {dot}" title={dotWord[dot]} aria-hidden="true"></span><span class="sr-only">{dotWord[dot]}. </span>{kindWord[e.kind]}{#if entryTime(e)}{' at '}{entryTime(e)}{/if}{#if e.network}{' on '}{e.network}{/if}{inLibraryWord(e.state, e.versions)}{#if e.kind === 'album' && albumHave(e)}{' · '}{albumHave(e)}{/if}</span
			>
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

{#snippet mini()}
	<MiniMonth year={pickYear} month={pickMonth} entries={shown} today={todayKey} {active} onpick={pickDay} onpage={pickPage} />
{/snippet}

<svelte:head><title>Calendar · HolyArrs</title></svelte:head>

<PageHeader title="Calendar" subtitle="Cinema, digital and disc dates for your movies, when your episodes air in your own time zone, and the days your artists' albums come out.">
	{#snippet actions()}
		<SegmentedControl
			label="Cards or list"
			testid="cal-view"
			value={view}
			onchange={(v) => pickView(v as CalendarView)}
			options={[
				{ value: 'cards', label: 'Cards' },
				{ value: 'list', label: 'List' }
			]}
		/>
		<Button variant="secondary" icon="calendar" onclick={toToday}>Today</Button>
		<span class="pick-btn"><IconButton icon="calendarDay" label="Pick a day" variant="tinted" aria-haspopup="dialog" onclick={() => (sheetOpen = true)} /></span>
	{/snippet}
</PageHeader>

<section class="tools" aria-label="Calendar controls">
	<div class="month-nav">
		<IconButton icon="chevronLeft" label="Week before" variant="tinted" onclick={() => go(-1)} />
		<h2 class="month" aria-live="polite" data-testid="cal-month">{rangeName}</h2>
		<IconButton icon="chevronRight" label="Week after" variant="tinted" onclick={() => go(1)} />
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
	<!-- Placeholder: a few day rows at the real rows' size. -->
	<div class="days-sk" aria-busy="true" aria-label="Loading the calendar" data-testid="calendar-skeleton">
		{#each [0, 1, 2] as d (d)}
			<div class="day-sk"><span class="sk" style="width: 56px; height: 64px; border-radius: 10px"></span><span class="sk" style="flex: 1; aspect-ratio: 16 / 7; border-radius: var(--r-panel)"></span></div>
		{/each}
	</div>
{:else if error && entries.length === 0}
	<LoadError hasData={false} message="Calendar could not be loaded." onretry={load} />
{:else if libraryEmpty}
	<EmptyState icon="calendar" title="Nothing coming up" text="Add movies, shows and artists to your Library and their dates, episodes and albums show up here." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
{:else}
	{#if error}<LoadError hasData message="Calendar could not be loaded." onretry={load} />{/if}
	<div class="cal-body">
		<div class="cal-main">
			{#key view}
				<div class="view-in" data-view={view}>
					{#if view === 'cards'}
						<ComingSoon entries={shown} today={todayKey} onlater={more} />
					{:else if agenda.length === 0}
						<EmptyState compact icon="calendar" title="No dates from {rangeName}" />
					{:else}
						<ol class="days" data-testid="cal-list">
							{#each agenda as d (d.day)}
								{@const at = new Date(d.day + 'T00:00:00')}
								<li class="day" class:today={d.day === todayKey} data-day={d.day}>
									<div class="when" aria-hidden="true">
										<span class="dow">{at.toLocaleDateString(undefined, { weekday: 'short' })}</span>
										<b>{at.getDate()}</b>
										{#if d.day === todayKey}<span class="today-tag"><Badge label="Today" /></span>{/if}
									</div>
									<div class="items">
										<p class="sr-only">{date(d.day + 'T12:00:00')}{d.day === todayKey ? ', today' : ''}</p>
										<ul>{#each d.items as e (entryKey(e))}{@render item(e)}{/each}</ul>
									</div>
								</li>
							{/each}
						</ol>
					{/if}
				</div>
			{/key}
			<!-- Reaching the end loads the next weeks, up to a year. -->
			<div class="endless" use:endless={fetching} aria-hidden="true"></div>
			{#if fetching}<p class="more-note" role="status">Loading the next weeks…</p>{/if}
		</div>
		<aside class="side" aria-label="Pick a day">{@render mini()}</aside>
	</div>
{/if}

<Sheet bind:open={sheetOpen} title="Pick a day" placement="bottom">
	<div class="sheet-mini">{@render mini()}</div>
</Sheet>

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

	.cal-body { display: grid; grid-template-columns: minmax(0, 1fr) 260px; gap: 28px; align-items: start; }
	.cal-main { min-width: 0; }
	.side { position: sticky; top: calc(var(--topbar) + 16px); padding: 14px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.pick-btn { display: none; }
	.sheet-mini { padding: 4px 0 12px; }
	/* Cards and List fade in when switched. */
	.view-in { animation: view-in var(--dur-slow) var(--ease-out) both; }
	@keyframes view-in { from { opacity: 0; transform: translateY(8px); } }
	.endless { height: 1px; }
	.more-note { margin-top: 18px; color: var(--text-muted); font: var(--t-callout); }

	.days { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; max-width: 860px; }
	.day { display: grid; grid-template-columns: 56px minmax(0, 1fr); gap: 14px; padding: 12px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); scroll-margin-top: 8px; }
	/* Today is told by a small tag under the date, not by colour. */
	.today-tag { margin-top: 6px; display: inline-flex; }
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
	.sdot { display: inline-block; width: 7px; height: 7px; margin-right: 6px; border-radius: 50%; vertical-align: 1px; background: var(--text-faint); }
	.sdot.have { background: var(--ok); }
	.sdot.missing { background: var(--fail); }
	.sdot.upcoming { background: var(--info); }
	.days-sk { display: flex; flex-direction: column; gap: 22px; max-width: 1120px; }
	.day-sk { display: flex; gap: 20px; align-items: flex-start; }

	/* Narrower than the small month needs beside the days: it opens as a sheet from the header. */
	@media (max-width: 1023px) {
		.cal-body { grid-template-columns: minmax(0, 1fr); }
		.side { display: none; }
		.pick-btn { display: inline-flex; }
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
		/* The filter chips are one sideways row on phones. */
		.kinds { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; margin: 0 calc(-1 * var(--gutter)); padding: 2px var(--gutter); }
		.kinds::-webkit-scrollbar { display: none; }
		.kinds > :global(*) { flex: none; }
	}
	@media (prefers-reduced-motion: reduce) {
		.view-in { animation: none; }
	}
</style>
