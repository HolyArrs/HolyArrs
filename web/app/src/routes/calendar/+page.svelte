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
	import { glideScroll, popIn, popOut } from '#lib/ui/transitions.ts';
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
	import Popover from '#lib/ui/Popover.svelte';
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
		addDays,
		entryDay,
		rangeChunks,
		titlesFromAddress,
		monthRange,
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
	// The days shown run from `start` (today, or the first of a linked month) to the day before `end`.
	// Six more weeks load as the end comes near; scrolling up past the first day loads a week
	// before it. Both stop a year from today.
	const today0 = addDays(now, 0);
	const REACH = 6;
	const earliest = addDays(today0, -365);
	const latest = addDays(today0, 366);
	let start = $state(askedMonth ?? today0);
	let end = $state(addDays(askedMonth ?? today0, 7 * REACH));
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
	let which = $state<TitlesShown>(titlesFromAddress(askedTitles));

	// The phone calendar feed.
	let feed = $state<{ set: boolean; created_at?: string } | null>(null);
	let feedLoaded = $state(false);
	let feedError = $state('');
	let feedNeedsProof = $state(false);
	let newLink = $state('');
	let feedBusy = $state(false);

	// Music turned off while picked falls back to All.
	$effect(() => {
		if (which === 'music' && partOff('music')) which = 'all';
	});
	const firstDay = $derived(ymd(start));
	const lastDay = $derived(ymd(end));
	// The query asks a day either side (an episode's local day can differ from the day it is
	// stored under); those extra days never show as days of their own.
	const picked = (e: CalendarEntry) => kinds[e.kind] && shows(e, which) && passes(e, { keptOnly, haveOnly });
	const shown = $derived(entries.filter((e) => entryDay(e) >= firstDay && entryDay(e) < lastDay && picked(e)));
	const todayKey = ymd(now);
	const agenda = $derived(byDay(shown));
	const range = $derived({ from: ymd(addDays(start, -1)), to: ymd(addDays(end, 1)) });
	const rangeName = $derived(
		`${start.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} – ${new Date(end.getFullYear(), end.getMonth(), end.getDate() - 1).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })}`
	);

	// The small month: starts on the first week's month, pages on its own, and follows the day in view.
	let pickYear = $state(untrack(() => start.getFullYear()));
	let pickMonth = $state(untrack(() => start.getMonth()));
	let active = $state<string | null>(null);
	let sheetOpen = $state(false);
	let filtersOpen = $state(false);

	const filtersOn = $derived((['cinema', 'digital', 'physical'] as const).filter((k) => !kinds[k]).length + (keptOnly ? 1 : 0) + (haveOnly ? 1 : 0));
	const filtersWord = $derived(filtersOn ? `Filters · ${filtersOn}` : 'Filters');
	const rangeShort = $derived(
		`${start.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} – ${addDays(end, -1).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}`
	);
	// The slim bar shows once the controls above have scrolled away, and stays.
	let headEnd = $state<HTMLElement>();
	let pastHead = $state(false);
	let narrow = $state(false);
	const barOn = $derived(pastHead);
	// Today's button only shows (in red) once today has left the screen.
	let away = $state(false);

	let asks = 0;
	async function load() {
		const n = ++asks;
		fetching = true;
		try {
			const r = range;
			// The server takes up to a year a query; a longer stretch is asked in pieces.
			const [parts, t] = await Promise.all([
				Promise.all(rangeChunks(r.from, r.to).map((c) => api.get<{ dates: CalendarEntry[] | null }>(`/calendar?from=${c.from}&to=${c.to}`))),
				api.get<{ titles: TitleView[] }>('/titles')
			]);
			if (n !== asks) return;
			const seen = new Set<string>();
			entries = parts.flatMap((c) => c.dates ?? []).filter((e) => !seen.has(entryKey(e)) && !!seen.add(entryKey(e)));
			libraryEmpty = t.titles.length === 0;
			error = '';
			await tick();
			holdPlace();
			if (goTo) {
				const day = goTo;
				goTo = null;
				void pickDay(day);
			}
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
			if (n === asks) fetching = false;
		}
	}

	function jump(d: Date) {
		start = addDays(d, 0);
		end = addDays(start, 7 * REACH);
		anchor = null;
		pickYear = start.getFullYear();
		pickMonth = start.getMonth();
	}
	const go = (n: number) => jump(addDays(start, 7 * n));
	// Each new range (the arrows, Today, more weeks, the small month) is loaded once.
	let started = $state(false);
	$effect(() => {
		void range.from;
		void range.to;
		if (started) untrack(() => void load());
	});
	// Today glides to today's row when it is loaded, else reloads from today. Either way no
	// older weeks load while the page moves, or they would push today away again.
	function toToday() {
		guard();
		const row = firstDay <= todayKey ? todayRow() : undefined;
		if (row) row.scrollIntoView({ block: 'start', behavior: glideScroll() });
		else {
			jump(new Date());
			window.scrollTo({ top: 0, behavior: glideScroll() });
		}
	}
	const todayRow = () => [...document.querySelectorAll<HTMLElement>('.cal-main [data-day]')].find((r) => (r.dataset.day ?? '') >= todayKey);

	// The small month's arrows; a month outside what is loaded gets loaded (within a year).
	function pickPage(n: number) {
		({ year: pickYear, month: pickMonth } = shiftMonth(pickYear, pickMonth, n));
	}
	// The small month shows its whole month: dates outside the days loaded are asked for it alone.
	let monthDates = $state<CalendarEntry[]>([]);
	$effect(() => {
		const r = monthRange(pickYear, pickMonth);
		if (!started || (r.from >= firstDay && r.to <= lastDay)) {
			monthDates = [];
			return;
		}
		const key = `${r.from}:${r.to}`;
		untrack(() => {
			monthAsk = key;
			api.get<{ dates: CalendarEntry[] | null }>(`/calendar?from=${ymd(addDays(r.start, -1))}&to=${ymd(addDays(new Date(r.to + 'T00:00:00'), 1))}`)
				.then((c) => {
					if (monthAsk === key) monthDates = c.dates ?? [];
				})
				.catch(() => {});
		});
	});
	let monthAsk = '';
	const miniDates = $derived.by(() => {
		const keys = new Set(shown.map(entryKey));
		return [...shown, ...monthDates.filter((e) => picked(e) && !keys.has(entryKey(e)))];
	});
	// A day not loaded yet (from the small month) loads first, then the page glides to it.
	let goTo: string | null = null;

	// Older days go in above the ones on screen. The row at the top of the screen is noted
	// before they load and put back where it was after (scrollTop moves by what was added),
	// so nothing jumps.
	let anchor: { day: string; top: number } | null = null;
	function notePlace() {
		const rows = document.querySelectorAll<HTMLElement>('.cal-main [data-day]');
		for (const r of rows) {
			const b = r.getBoundingClientRect();
			if (b.bottom > 0) {
				anchor = { day: r.dataset.day ?? '', top: b.top };
				return;
			}
		}
		anchor = null;
	}
	function holdPlace() {
		if (!anchor) return;
		const a = anchor;
		anchor = null;
		const row = document.querySelector<HTMLElement>(`.cal-main [data-day="${a.day}"]`);
		if (!row) return;
		const moved = row.getBoundingClientRect().top - a.top;
		if (Math.abs(moved) > 0.5) {
			window.scrollBy({ top: moved, behavior: 'instant' });
		}
	}
	function earlier(to: Date) {
		if (to >= start) return;
		notePlace();
		start = to;
	}
	// gliding stays on until the page has stopped moving for a moment.
	let gliding = false;
	let glideTimer: ReturnType<typeof setTimeout> | undefined;
	function guard() {
		gliding = true;
		clearTimeout(glideTimer);
		glideTimer = setTimeout(() => (gliding = false), 600);
	}
	// Scrolling up near the first day loads the week before it.
	function back() {
		if (fetching || gliding || start <= earliest) return;
		const to = addDays(start, -7);
		earlier(to < earliest ? earliest : to);
	}
	const nearFirst = () => {
		const first = document.querySelector<HTMLElement>('.cal-main [data-day]');
		return !!first && first.getBoundingClientRect().top > -2 * innerHeight;
	};
	// At the very top the page cannot scroll, so an upward wheel or swipe asks instead.
	function onWheel(e: WheelEvent) {
		if (e.deltaY < 0 && window.scrollY <= 2 && loaded) back();
	}
	let touchY = 0;
	const onTouchStart = (e: TouchEvent) => (touchY = e.touches[0]?.clientY ?? 0);
	function onTouchMove(e: TouchEvent) {
		if (window.scrollY <= 2 && (e.touches[0]?.clientY ?? 0) - touchY > 40 && loaded) back();
	}
	// A day picked in the small month: glide to its row (instant under reduced motion).
	async function pickDay(day: string) {
		const fromSheet = sheetOpen;
		sheetOpen = false;
		if (fromSheet) await tick();
		const at = new Date(day + 'T00:00:00');
		if (day < firstDay || day >= lastDay) {
			goTo = day;
			if (day < firstDay) earlier(at < earliest ? earliest : at);
			else {
				const to = addDays(at, 7);
				end = to > latest ? latest : to;
			}
			return;
		}
		const row = document.querySelector<HTMLElement>(`.cal-main [data-day="${day}"]`);
		if (!row) return;
		active = day;
		// No loading above while the page glides to the day, or the rows would move under it.
		guard();
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
	// Today is on screen when its row (or, on a day with nothing, the next day's row) shows
	// below the bars. Days loaded from after today, or none loaded yet, count as away.
	function checkToday() {
		if (!loaded || firstDay > todayKey) {
			away = loaded;
			return;
		}
		const row = todayRow();
		if (!row) {
			away = !!document.querySelector('.cal-main [data-day]');
			return;
		}
		const b = row.getBoundingClientRect();
		away = b.bottom < (narrow ? 96 : 120) || b.top > innerHeight;
	}
	let ticking = false;
	let lastY = 0;
	function onScroll() {
		const y = window.scrollY;
		const up = y < lastY;
		lastY = y;
		if (gliding) guard();
		if (headEnd) pastHead = headEnd.getBoundingClientRect().top < (narrow ? 44 : 56);
		if (ticking) return;
		ticking = true;
		requestAnimationFrame(() => {
			ticking = false;
			if (up && loaded && nearFirst()) back();
			follow();
			checkToday();
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
		if (fetching || end >= latest) return;
		const to = addDays(end, 7 * REACH);
		end = to > latest ? latest : to;
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
		const mq = matchMedia('(max-width: 1023px)');
		const fit = () => (narrow = mq.matches);
		fit();
		mq.addEventListener('change', fit);
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
		return () => {
			offs.forEach((off) => off());
			mq.removeEventListener('change', fit);
		};
	});
	// Rows change as dates load or filters change: the small month follows the day in view.
	$effect(() => {
		void shown;
		void view;
		if (loaded) void tick().then(() => (follow(), checkToday()));
	});
</script>

<svelte:window onscroll={onScroll} onwheel={onWheel} ontouchstart={onTouchStart} ontouchmove={onTouchMove} />

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
	<MiniMonth year={pickYear} month={pickMonth} entries={miniDates} today={todayKey} {active} onpick={pickDay} onpage={pickPage} />
{/snippet}

{#snippet rangeNav(primary: boolean, short: boolean)}
	<div class="range-nav">
		<IconButton icon="chevronLeft" label="Week before" variant="tinted" onclick={() => go(-1)} />
		{#if primary}
			<h2 class="range" aria-live="polite" data-testid="cal-month">{rangeName}</h2>
		{:else if short && narrow}
			<!-- In the slim bar the dates also open the small month (on phones, where it is not beside the days). -->
			<button type="button" class="range range-btn" aria-label="Pick a day, {rangeName}" aria-haspopup="dialog" onclick={() => (sheetOpen = true)}>{rangeShort}</button>
		{:else}
			<span class="range">{short ? rangeShort : rangeName}</span>
		{/if}
		<IconButton icon="chevronRight" label="Week after" variant="tinted" onclick={() => go(1)} />
	</div>
{/snippet}

{#snippet todayBtn(size: 'sm' | 'md')}
	<!-- Hidden while today is on screen; once it is not, it pops in red to bring you back. -->
	{#if away}
		<span class="today-btn" in:popIn out:popOut data-testid="cal-today">
			<Button variant="primary" {size} icon="calendar" onclick={toToday} aria-label="Back to today">Today</Button>
		</span>
	{/if}
{/snippet}

{#snippet typeSeg(primary: boolean)}
	<SegmentedControl
		label="Movies, shows or music"
		testid={primary ? 'cal-titles' : undefined}
		value={which}
		onchange={(v) => (which = v as TitlesShown)}
		options={[
			{ value: 'all', label: 'All' },
			{ value: 'movies', label: 'Movies' },
			{ value: 'shows', label: 'TV shows' },
			...(partOn('music') ? [{ value: 'music', label: 'Music' }] : [])
		]}
	/>
{/snippet}

{#snippet chipList(stacked: boolean)}
	<div class="kinds" class:stacked role="group" aria-label="Filters">
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
{/snippet}

<svelte:head><title>Calendar · HolyArrs</title></svelte:head>

<PageHeader title="Calendar" subtitle={"Cinema, digital and disc dates for your movies, when your episodes air in your own time zone, and the days your artists' albums come out."}>
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
		<span class="wide-only">{@render todayBtn('md')}</span>
		<span class="phone-only"><IconButton icon="calendarDay" label="Pick a day" variant="tinted" aria-haspopup="dialog" onclick={() => (sheetOpen = true)} /></span>
	{/snippet}
</PageHeader>

<section class="tools" aria-label="Calendar controls">
		<div class="month-nav">{@render rangeNav(true, false)}</div>
		<div class="segs">{@render typeSeg(true)}</div>
		{@render chipList(false)}
</section>
<span class="head-end" bind:this={headEnd} aria-hidden="true"></span>

<!-- The slim bar: under the app's top bar once the controls above have scrolled away
	and stays there; Today shows in it only once today has left the screen. -->
<div class="slim-wrap" class:on={barOn} inert={!barOn} data-testid="cal-slim">
	<div class="slim" role="toolbar" aria-label="Calendar controls">
		{@render rangeNav(false, true)}
		<span class="wide-only slim-seg">{@render typeSeg(false)}</span>
		<span class="wide-only">
			<Popover label="Filters" variant="button" text={filtersWord} width={260} align="end">
				{#snippet children()}<div class="pop-chips">{@render chipList(true)}</div>{/snippet}
			</Popover>
		</span>
		{@render todayBtn('sm')}
		<span class="phone-only"><Button variant="secondary" size="sm" aria-haspopup="dialog" onclick={() => (filtersOpen = true)}>{filtersWord}</Button></span>
	</div>
</div>

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
	<div class="cal-body" class:barred={barOn}>
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
	<div class="sheet-mini">
		{@render rangeNav(false, false)}
		{@render mini()}
		<Button variant="secondary" full icon="calendar" onclick={() => { sheetOpen = false; toToday(); }}>Today</Button>
	</div>
</Sheet>

<Sheet bind:open={filtersOpen} title="Filters" placement="bottom">
	<div class="sheet-filters">
		{@render typeSeg(false)}
		{@render chipList(false)}
	</div>
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
	.month-nav { margin-right: auto; }
	.range-nav { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.range { min-width: 9.5ch; font: var(--t-title2); letter-spacing: var(--t-title2-track); text-align: center; }
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
	.phone-only { display: none; }
	.sheet-mini, .sheet-filters { display: flex; flex-direction: column; gap: 16px; padding: 4px 0 12px; }
	.sheet-mini .range-nav { justify-content: space-between; }
	.sheet-mini .range { flex: 1; min-width: 0; font: var(--t-headline); }
	.sheet-filters > :global(.ui-seg) { width: 100%; }
	.kinds.stacked { flex-direction: column; align-items: flex-start; flex-wrap: nowrap; }

	/* The slim bar under the app's top bar. */
	.slim-wrap { position: sticky; top: var(--topbar); z-index: 20; height: 0; margin-inline: calc(-1 * var(--gutter)); transition: transform var(--bar-glide); }
	:global(html.bar-hidden) .slim-wrap { transform: translateY(calc(-1 * var(--topbar))); }
	.slim { display: flex; align-items: center; gap: 12px; padding: 8px var(--gutter); background: color-mix(in srgb, var(--bg) 86%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line);
		opacity: 0; visibility: hidden; transform: translateY(-8px); transition: opacity var(--dur-fast) var(--ease-in), transform var(--dur-fast) var(--ease-in), visibility 0s linear var(--dur-fast); }
	.slim-wrap.on .slim { opacity: 1; visibility: visible; transform: none; transition: opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out), visibility 0s; }
	.slim .range-nav { margin-right: auto; }
	.slim .range { min-width: 0; font: var(--t-headline); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.range-btn { padding: 4px 8px; border-radius: var(--r-sm); color: var(--text); transition: background var(--dur-fast) var(--ease-out); }
	.range-btn:hover { background: var(--surface-2); }
	.range-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pop-chips { padding: 4px; }
	/* Today, once it shows: a soft ring pulses twice so the eye finds it. */
	.today-btn { display: inline-flex; border-radius: 999px; animation: today-ping 1.2s var(--ease-out) 2; }
	@keyframes today-ping { from { box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-fill) 55%, transparent); } to { box-shadow: 0 0 0 10px transparent; } }
	/* While the bar shows, the small month and picked days sit below it. */
	.side { transition: top var(--dur) var(--ease-out); }
	.cal-body.barred .side { top: calc(var(--topbar) + 76px); }
	:global(html:has(.slim-wrap.on)) { scroll-padding-top: calc(var(--topbar) + 68px); }
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
		.phone-only { display: inline-flex; }
		.wide-only { display: none; }
		.slim { gap: 8px; }
		.slim .range-nav { flex: 1; }
		.slim .range { flex: 1; text-align: center; }
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
		.month-nav { width: 100%; margin-right: 0; }
		.month-nav .range-nav { justify-content: space-between; }
		.slim-wrap { top: calc(44px + env(safe-area-inset-top)); }
		:global(html.bar-hidden) .slim-wrap { transform: none; }
		.segs, .segs > :global(.ui-seg) { width: 100%; }
		.acts > :global(*) { flex: 1 1 100%; }
		/* The filter chips are one sideways row on phones. */
		.tools .kinds { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; margin: 0 calc(-1 * var(--gutter)); padding: 2px var(--gutter); }
		.tools .kinds::-webkit-scrollbar { display: none; }
		.tools .kinds > :global(*) { flex: none; }
	}
	@media (prefers-reduced-motion: reduce) {
		.view-in, .today-btn { animation: none; }
		.slim, .slim-wrap.on .slim, .slim-wrap { transition: none; }
	}
</style>
