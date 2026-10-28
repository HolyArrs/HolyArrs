<script lang="ts">
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
	import { glideScroll, popIn, popOut } from '#lib/ui/transitions.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import ComingSoon from '#lib/calendar/ComingSoon.svelte';
	import MiniMonth from '#lib/calendar/MiniMonth.svelte';
	import FollowWho from '#lib/calendar/FollowWho.svelte';
	import CalBanner from '#lib/calendar/CalBanner.svelte';
	import { rowWords, DEFAULT_LOOK, type CaptionLook } from '#lib/calendar-captions.ts';
	import { setFollow } from '#lib/follow.ts';
	import {
		byDay,
		entryKey,
		readMonthHidden,
		saveMonthHidden,
		feedLink,
		shiftMonth,
		shows,
		entryHref,
		weekStart,
		webcalLink,
		addDays,
		entryDay,
		rangeChunks,
		titlesFromAddress,
		monthRange,
		itemDot,
		oneMovieDate,
		movieFacts,
		followsOk,
		withFollow,
		readFollows,
		saveFollows,
		startFollows,
		type Follows,
		type MovieFacts,
		ymd,
		readView,
		saveView,
		startView,
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
	// One menu ties the calendar to Follow: what you follow (asking for a title follows it),
	// what anyone on this server follows, or every date in the Library. Picked once the first
	// dates say how many titles you follow and how many people use the server; kept for the session.
	let follows = $state<Follows>(readFollows() ?? 'mine');
	let followsSet = false;
	let myFollows = $state(0);
	let people = $state(1);
	// Each movie's dates and copies.
	let movies = $state(new Map<number, MovieFacts>());
	// ?titles=shows opens the calendar on one kind of title.
	const askedTitles = page.url.searchParams.get('titles');
	let which = $state<TitlesShown>(titlesFromAddress(askedTitles));

	// The two caption lines under every card and row (?movie=1-6, ?music=1-5, ?channels=all|known).
	let look = $state<CaptionLook>({ ...DEFAULT_LOOK, region: 'global' });

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
	// A movie shows one date: the day it is out to watch, else its cinema date.
	const pool = $derived(oneMovieDate(entries, movies));
	const todayKey = ymd(now);
	const filterOk = (e: CalendarEntry) => followsOk(e, follows);
	const picked = (e: CalendarEntry) => shows(e, which) && filterOk(e);
	// The days in view for the type picked, before the filters (what the counts count).
	const base = $derived(pool.filter((e) => entryDay(e) >= firstDay && entryDay(e) < lastDay && shows(e, which)));
	const shown = $derived(base.filter(filterOk));
	const filteredOut = $derived(shown.length === 0 && base.length > 0);
	const agenda = $derived(byDay(shown));
	const range = $derived({ from: ymd(addDays(start, -1)), to: ymd(addDays(end, 1)) });
	const rangeName = $derived(
		`${start.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} – ${new Date(end.getFullYear(), end.getMonth(), end.getDate() - 1).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })}`
	);
	// The heading names the week in view (the day at the top of the page), not every day loaded.
	const viewWeek = $derived.by(() => {
		const s = weekStart(active ? new Date(active + 'T00:00:00') : start);
		const e = addDays(s, 6);
		const f = (d: Date, y: boolean) => d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', ...(y ? { year: 'numeric' } : {}) });
		return { long: `${f(s, false)} – ${f(e, true)}`, short: `${f(s, false)} – ${f(e, false)}` };
	});

	// The small month: starts on the first week's month, pages on its own, and follows the day in view.
	let pickYear = $state(untrack(() => start.getFullYear()));
	let pickMonth = $state(untrack(() => start.getMonth()));
	let active = $state<string | null>(null);
	let sheetOpen = $state(false);
	let filtersOpen = $state(false);

	const followsWords: Record<Follows, { label: string; text: string }> = {
		mine: { label: 'My follows', text: 'What you follow, including everything you asked for.' },
		everyone: { label: "Everyone's follows", text: 'What anyone on this server follows. Each row shows who.' },
		library: { label: 'Whole Library', text: 'Every date in the Library, followed or not.' }
	};
	// Everyone's follows only means something with more than one person on the server.
	const followsChoices = $derived<Follows[]>(people > 1 ? ['mine', 'everyone', 'library'] : ['mine', 'library']);
	// How many dates each choice shows in the days loaded (for the type picked).
	const followsCount = $derived({
		mine: base.filter((e) => followsOk(e, 'mine')).length,
		everyone: base.filter((e) => followsOk(e, 'everyone')).length,
		library: base.length
	});
	function pickFollows(f: Follows) {
		follows = f;
		saveFollows(f);
	}
	const clearFilters = () => pickFollows('library');
	// Following nothing yet: the calendar opens on the Whole Library with one line saying how to follow.
	const followNothing = $derived(loaded && myFollows === 0 && follows === 'library');
	// The slim bar shows once the controls above have scrolled away, and stays.
	let headEnd = $state<HTMLElement>();
	let pastHead = $state(false);
	let narrow = $state(false);
	// The small month beside the days can be hidden (wide screens; phones use the sheet), kept in this browser.
	let monthHidden = $state(readMonthHidden());
	function flipMonth() {
		monthHidden = !monthHidden;
		saveMonthHidden(monthHidden);
	}
	const barOn = $derived(pastHead);
	// Today's button only shows (in red) once today has left the screen.
	let away = $state(false);

	type CalendarAnswer = { dates: CalendarEntry[] | null; my_follows?: number; people?: number };
	let asks = 0;
	async function load() {
		const n = ++asks;
		fetching = true;
		try {
			const r = range;
			// The server takes up to a year a query; a longer stretch is asked in pieces.
			const [parts, t] = await Promise.all([
				Promise.all(rangeChunks(r.from, r.to).map((c) => api.get<CalendarAnswer>(`/calendar?from=${c.from}&to=${c.to}`))),
				api.get<{ titles: TitleView[] }>('/titles')
			]);
			if (n !== asks) return;
			movies = new Map(t.titles.filter((x) => x.kind === 'movie').map((x) => [x.id, movieFacts(x)]));
			const seen = new Set<string>();
			entries = parts.flatMap((c) => c.dates ?? []).filter((e) => !seen.has(entryKey(e)) && !!seen.add(entryKey(e)));
			if (parts[0]) {
				myFollows = parts[0].my_follows ?? 0;
				people = parts[0].people ?? 1;
				if (!followsSet) {
					followsSet = true;
					follows = startFollows(readFollows(), myFollows, people);
				}
			}
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
			api.get<CalendarAnswer>(`/calendar?from=${ymd(addDays(r.start, -1))}&to=${ymd(addDays(new Date(r.to + 'T00:00:00'), 1))}`)
				.then((c) => {
					if (monthAsk === key) monthDates = c.dates ?? [];
				})
				.catch(() => {});
		});
	});
	let monthAsk = '';
	const miniDates = $derived.by(() => {
		const keys = new Set(shown.map(entryKey));
		return [...shown, ...oneMovieDate(monthDates, movies).filter((e) => picked(e) && !keys.has(entryKey(e)))];
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

	// Follow or stop following a title from its row. The dates stay loaded; only who follows
	// changes, so the row updates in place (in My follows it leaves the list).
	let flipping = $state<number | null>(null);
	async function flipFollow(e: CalendarEntry, on: boolean) {
		if (flipping !== null) return;
		flipping = e.title_id;
		try {
			const now = await setFollow(api, e.title_id, on);
			const me = { name: session.s.name, avatar: session.s.avatar };
			const upd = (x: CalendarEntry) => (x.title_id === e.title_id ? withFollow(x, now, me) : x);
			const was = entries.some((x) => x.title_id === e.title_id && !!x.following);
			entries = entries.map(upd);
			monthDates = monthDates.map(upd);
			if (now !== was) myFollows = Math.max(0, myFollows + (now ? 1 : -1));
			toasts.push(now ? `You follow ${e.name}. Its dates are on your calendar.` : `You no longer follow ${e.name}.`, 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			flipping = null;
		}
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

{#snippet item(e: CalendarEntry, prevWhen = '')}
	{@const dot = itemDot(e, todayKey)}
	{@const w = rowWords({ key: entryKey(e), day: entryDay(e), lead: e, entries: [e] }, look, todayKey)}
	{@const lines = w.lines.length ? w.lines : ['']}
	<li class="item k-{e.kind}" data-testid="cal-item">
		<!-- The time column (TV-guide style): the air time, or Cinema, At home, Out now; a quiet All day without a time. -->
		<!-- A time repeated from the row above is left blank, TV-guide style, so the eye sees when it changes. -->
		<span class="tcol" class:quiet={w.quiet} class:same={w.when === prevWhen} data-testid="cal-when">{w.when}</span>
		<div class="it">
			<span class="when-phone" class:quiet={w.quiet} aria-hidden="true">{w.when}</span>
			<a href={to(entryHref(e))}>{w.title}</a>
			<!-- Each line only when it has real data; the status dot starts the first. -->
			{#each lines as l, i (i)}
				<span class="sub" class:first={i === 0} data-testid="cal-cap"
					>{#if i === 0}<span class="sdot {dot}" title={dotWord[dot]} aria-hidden="true"></span><span class="sr-only">{dotWord[dot]}. </span>{/if}{l}</span
				>
			{/each}
			{#if follows === 'everyone'}<span class="who"><FollowWho {e} /></span>{/if}
		</div>
		<!-- The bell follows or stops following the title; lit when you follow it. -->
		<span class="row-bell" class:on={e.following}>
			<IconButton icon={e.following ? 'bellOn' : 'bell'} size="sm" tipSide="below-end" label="{e.following ? 'Unfollow' : 'Follow'} {e.name}" aria-pressed={!!e.following} disabled={flipping === e.title_id} onclick={() => flipFollow(e, !e.following)} data-testid="cal-follow" />
		</span>
		<!-- The wide banner (title art with its logo) fills the row's right side, edge to edge. -->
		<span class="bwrap"><CalBanner {e} /></span>
	</li>
{/snippet}

{#snippet monthBtn(primary: boolean)}
	<!-- Hides or shows the small month; the days then take the full width. -->
	<button type="button" class="month-btn" class:sm={!primary} aria-pressed={!monthHidden} aria-label={monthHidden ? 'Show month' : 'Hide month'} title={monthHidden ? 'Show month' : 'Hide month'} onclick={flipMonth} data-testid={primary ? 'cal-month-toggle' : undefined}>
		<Icon name="calendar" size={16} /><span class="chev" class:off={monthHidden}><Icon name="chevronRight" size={14} /></span>
	</button>
{/snippet}

{#snippet mini()}
	<MiniMonth year={pickYear} month={pickMonth} entries={miniDates} today={todayKey} {active} onpick={pickDay} onpage={pickPage} />
{/snippet}

{#snippet rangeNav(primary: boolean, short: boolean)}
	<div class="range-nav">
		<IconButton icon="chevronLeft" label="Week before" variant="tinted" onclick={() => go(-1)} />
		{#if primary}
			<h2 class="range" aria-live="polite" data-testid="cal-month">{viewWeek.long}</h2>
		{:else if short && narrow}
			<!-- In the slim bar the dates also open the small month (on phones, where it is not beside the days). -->
			<button type="button" class="range range-btn" aria-label="Pick a day, {viewWeek.long}" aria-haspopup="dialog" onclick={() => (sheetOpen = true)}>{viewWeek.short}</button>
		{:else}
			<span class="range">{short ? viewWeek.short : viewWeek.long}</span>
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

{#snippet followsList()}
	<div class="showing" role="radiogroup" aria-label="Which dates">
		{#each followsChoices as k (k)}
			<button type="button" role="radio" aria-checked={follows === k} class:on={follows === k} onclick={() => pickFollows(k)} data-testid="cal-follows-{k}">
				<span class="tick" aria-hidden="true">{#if follows === k}<Icon name="check" size={16} />{/if}</span>
				<span class="sw"><b>{followsWords[k].label}</b><span>{followsWords[k].text}</span></span>
				<span class="n" aria-label="{followsCount[k]} {followsCount[k] === 1 ? 'date' : 'dates'} in view">{followsCount[k]}</span>
			</button>
		{/each}
	</div>
{/snippet}

{#snippet followsMenu()}
	<Popover label="Which dates: {followsWords[follows].label}" variant="button" icon={follows === 'library' ? 'bell' : 'bellOn'} text={followsWords[follows].label} width={320} align="end">
		{#snippet children()}<div class="pop-chips">{@render followsList()}</div>{/snippet}
	</Popover>
{/snippet}

{#snippet followsPhone()}
	<Button variant="secondary" size="sm" icon={follows === 'library' ? 'bell' : 'bellOn'} iconAfter="chevronDown" aria-haspopup="dialog" aria-label="Which dates: {followsWords[follows].label}" onclick={() => (filtersOpen = true)} data-testid="cal-follows-phone">{followsWords[follows].label}</Button>
{/snippet}


{#snippet viewSeg(primary: boolean)}
	<SegmentedControl
		label="Cards or list"
		testid={primary ? 'cal-view' : undefined}
		value={view}
		onchange={(v) => pickView(v as CalendarView)}
		options={[
			{ value: 'cards', label: 'Cards' },
			{ value: 'list', label: 'List' }
		]}
	/>
{/snippet}

<svelte:head><title>Calendar · HolyArrs</title></svelte:head>

<PageHeader title="Calendar">
	{#snippet actions()}
		{@render viewSeg(true)}
		<span class="wide-only">{@render todayBtn('md')}</span>
		<span class="phone-only"><IconButton icon="calendarDay" label="Pick a day" variant="tinted" aria-haspopup="dialog" onclick={() => (sheetOpen = true)} /></span>
	{/snippet}
</PageHeader>

<section class="tools" aria-label="Calendar controls">
		<div class="month-nav">{@render rangeNav(true, false)}</div>
		<div class="segs">{@render typeSeg(true)}</div>
		<span class="wide-only" data-testid="cal-follows">{@render followsMenu()}</span>
		<span class="wide-only">{@render monthBtn(true)}</span>
		<span class="phone-only">{@render followsPhone()}</span>
</section>
<span class="head-end" bind:this={headEnd} aria-hidden="true"></span>

<!-- The slim bar: under the app's top bar once the controls above have scrolled away
	and stays there; Today shows in it only once today has left the screen. -->
<div class="slim-wrap" class:on={barOn} inert={!barOn} data-testid="cal-slim">
	<div class="slim" role="toolbar" aria-label="Calendar controls">
		{@render rangeNav(false, true)}
		<!-- Cards or List stays at hand in the bar once the header has scrolled away. -->
		<span class="wide-only slim-view">{@render viewSeg(false)}</span>
		<span class="phone-only"><IconButton icon={view === 'cards' ? 'menu' : 'grid'} label={view === 'cards' ? 'Show as a list' : 'Show as cards'} variant="tinted" onclick={() => pickView(view === 'cards' ? 'list' : 'cards')} data-testid="cal-view-slim" /></span>
		<span class="wide-only slim-seg">{@render typeSeg(false)}</span>
		<span class="wide-only">{@render followsMenu()}</span>
		<span class="wide-only">{@render monthBtn(false)}</span>
		{@render todayBtn('sm')}
		<span class="phone-only">{@render followsPhone()}</span>
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
	<div class="cal-body" class:barred={barOn} class:month-off={monthHidden}>
		<div class="cal-main">
			{#if followNothing}<p class="follow-note" data-testid="cal-follow-none"><Icon name="bell" size={16} />You're not following anything yet. Press Follow on a title to see it here.</p>{/if}
			{#key view}
				<div class="view-in" data-view={view}>
					{#if filteredOut}
						<!-- The filters hide every date in these weeks: say so, and offer them all. -->
						<div class="filtered-out" data-testid="cal-filtered-out">
							<EmptyState compact icon="calendar" title={follows === 'mine' ? 'Nothing you follow here' : 'Nothing anyone follows here'} text="{base.length === 1 ? 'One date in the Library falls' : `${base.length} dates in the Library fall`} in {rangeName}." action={{ label: 'Show the Whole Library', onclick: clearFilters }} />
						</div>
					{:else if view === 'cards'}
						<ComingSoon entries={shown} today={todayKey} onlater={more} mode={follows} {look} onfollow={flipFollow} />
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
										<ul>{#each d.items as e, i (entryKey(e))}{@render item(e, i ? rowWords({ key: entryKey(d.items[i - 1]), day: entryDay(d.items[i - 1]), lead: d.items[i - 1], entries: [d.items[i - 1]] }, look, todayKey).when : '')}{/each}</ul>
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
		<aside class="side" aria-label="Pick a day" inert={monthHidden} aria-hidden={monthHidden}>{@render mini()}</aside>
	</div>
{/if}


<Sheet bind:open={sheetOpen} title="Pick a day" placement="bottom">
	<div class="sheet-mini">
		{@render rangeNav(false, false)}
		{@render mini()}
		<Button variant="secondary" full icon="calendar" onclick={() => { sheetOpen = false; toToday(); }}>Today</Button>
	</div>
</Sheet>

<Sheet bind:open={filtersOpen} title="Which dates" placement="bottom">
	<div class="sheet-filters">
		{@render typeSeg(false)}
		{@render followsList()}
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

	.cal-body { display: grid; grid-template-columns: minmax(0, 1fr) 260px; column-gap: 28px; align-items: start; transition: grid-template-columns var(--dur) var(--ease-out), column-gap var(--dur) var(--ease-out); }
	/* Month hidden: it slides off to the right and the days take its room. */
	@media (min-width: 1024px) { .cal-body.month-off { grid-template-columns: minmax(0, 1fr) 0px; column-gap: 0px; } }
	.cal-body .side { transition: top var(--dur) var(--ease-out), opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out); }
	.cal-body.month-off .side { opacity: 0; transform: translateX(24px); pointer-events: none; overflow: hidden; }
	.cal-body.month-off .days { max-width: none; }
	.month-btn { display: inline-flex; align-items: center; gap: 2px; height: 36px; padding: 0 10px; border: 0; border-radius: 999px; background: var(--surface-2); color: var(--text); cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.month-btn.sm { height: 32px; padding: 0 8px; }
	.month-btn:hover { background: var(--surface-3); }
	.month-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.month-btn .chev { display: inline-flex; color: var(--text-muted); transition: transform var(--dur) var(--ease-out); }
	.month-btn .chev.off { transform: rotate(180deg); }
	.cal-main { min-width: 0; }
	.side { position: sticky; top: calc(var(--topbar) + 16px); padding: 14px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.phone-only { display: none; }
	.sheet-mini, .sheet-filters { display: flex; flex-direction: column; gap: 16px; padding: 4px 0 12px; }
	.sheet-mini .range-nav { justify-content: space-between; }
	.sheet-mini .range { flex: 1; min-width: 0; font: var(--t-headline); }
	.sheet-filters > :global(.ui-seg) { width: 100%; }

	/* The slim bar under the app's top bar. */
	.slim-wrap { position: sticky; top: var(--topbar); z-index: 20; height: 0; margin-inline: calc(-1 * var(--gutter)); transition: transform var(--bar-glide); }
	:global(html.bar-hidden) .slim-wrap { transform: translateY(calc(-1 * var(--topbar))); }
	.slim { display: flex; align-items: center; gap: 12px; padding: 8px var(--gutter); background: color-mix(in srgb, var(--bg) 86%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); border-bottom: 1px solid var(--line);
		opacity: 0; visibility: hidden; transform: translateY(-8px); transition: opacity var(--dur-fast) var(--ease-in), transform var(--dur-fast) var(--ease-in), visibility 0s linear var(--dur-fast); }
	.slim-wrap.on .slim { opacity: 1; visibility: visible; transform: none; transition: opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out), visibility 0s; }
	.slim .slim-view { margin-right: auto; }
	.slim .range { min-width: 0; font: var(--t-headline); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.range-btn { padding: 4px 8px; border-radius: var(--r-sm); color: var(--text); transition: background var(--dur-fast) var(--ease-out); }
	.range-btn:hover { background: var(--surface-2); }
	.range-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pop-chips { padding: 4px; }
	.filtered-out { margin-bottom: 24px; }
	/* The Showing menu: three rows, one picked. */
	.showing { display: flex; flex-direction: column; gap: 2px; }
	.showing button { display: flex; align-items: flex-start; gap: 10px; width: 100%; padding: 10px; border: 0; border-radius: var(--r-sm); background: none; color: var(--text); text-align: left; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.showing button:hover, .showing button.on { background: var(--surface-2); }
	.showing button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.showing .tick { flex: none; width: 16px; height: 20px; display: grid; place-items: center; color: var(--accent-text); }
	.showing .sw { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.showing .sw b { font: var(--t-headline); font-size: 15px; }
	.showing .sw { flex: 1; }
	.showing .sw span { color: var(--text-muted); font: var(--t-footnote); }
	.showing .n { flex: none; min-width: 24px; padding: 1px 7px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; text-align: center; }
	.showing button.on .n { background: var(--surface-3); color: var(--text); }
	.follow-note { display: flex; align-items: center; gap: 8px; margin: 0 0 16px; color: var(--text-muted); font: var(--t-callout); animation: view-in var(--dur) var(--ease-out) both; }
	/* Today, once it shows: a soft ring pulses twice so the eye finds it. */
	.today-btn { display: inline-flex; border-radius: 999px; animation: today-ping 1.2s var(--ease-out) 2; }
	@keyframes today-ping { from { box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-fill) 55%, transparent); } to { box-shadow: 0 0 0 10px transparent; } }
	/* While the bar shows, the small month and picked days sit below it. */
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
	.item { display: flex; align-items: center; gap: 10px; padding: 8px 0; min-width: 0; }
	.item + .item { border-top: 1px solid var(--line); }
	/* List rows: the banner fills the row's right side, out to the card's edge (the card's corners round it). */
	.day { overflow: hidden; }
	.items { --bw: clamp(170px, 38%, 360px); }
	.item { position: relative; min-height: 66px; padding-right: calc(var(--bw) + 4px); }
	.bwrap { position: absolute; top: 0; bottom: 0; right: -12px; width: calc(var(--bw) + 12px); }
	.item:first-child .bwrap { top: -12px; }
	.item:last-child .bwrap { bottom: -12px; }
	.it, .tcol, .row-bell { position: relative; z-index: 1; }
	.it { display: flex; flex-direction: column; min-width: 0; flex: 1; }
	.who { display: flex; min-width: 0; margin-top: 3px; }
	.row-bell { flex: none; align-self: center; }
	.row-bell :global(.ui-ibtn) { color: var(--text-faint); transition: color var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.item:hover .row-bell :global(.ui-ibtn), .row-bell :global(.ui-ibtn:focus-visible) { color: var(--text-muted); }
	.row-bell.on :global(.ui-ibtn) { color: var(--accent-text); }
	.it a { color: var(--text); font: var(--t-headline); font-size: 15px; overflow-wrap: anywhere; }
	.it a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sub { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; min-height: 1lh; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sub.first { color: var(--text); }
	/* The time column: one width for every row, so times line up down the page. */
	/* One size for times and words (8 PM, In theaters, Streaming, Album), wide enough for the longest. */
	/* Set like the row's title (same face, weight and size) so it reads as part of the row. */
	.tcol { flex: none; width: 92px; font: var(--t-headline); font-size: 15px; font-variant-numeric: tabular-nums; white-space: nowrap; color: var(--text); align-self: center; text-align: center; }
	.tcol.quiet { color: var(--text-muted); }
	.tcol.same { visibility: hidden; }
	.when-phone { display: none; }
	/* Phones: the time sits above the title as a small bold line instead of a column. */
	@media (max-width: 640px) {
		.tcol { display: none; }
		/* A smaller picture leaves the words room on narrow phones. */
		/* Phones: the banner becomes a slim strip across the top of the row, the words below it. */
		.item { flex-wrap: wrap; padding-right: 0; min-height: 0; }
		.bwrap, .item:first-child .bwrap, .item:last-child .bwrap { position: relative; inset: auto; order: -1; flex: 0 0 100%; width: 100%; height: 56px; border-radius: var(--r-sm); overflow: hidden; }
		.bwrap :global(.cal-banner) { -webkit-mask-image: none; mask-image: none; }
		.item { gap: 8px; }
		.when-phone { display: block; font: var(--t-footnote); font-weight: 700; font-variant-numeric: tabular-nums; color: var(--text); }
		.when-phone.quiet { color: var(--text-muted); font-weight: 500; }
	}
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
		.view-in, .today-btn, .follow-note { animation: none; }
		.cal-body, .cal-body .side, .month-btn .chev { transition: none; }
		.row-bell :global(.ui-ibtn) { transition: none; }
		.showing button { transition: none; }
		.slim, .slim-wrap.on .slim, .slim-wrap { transition: none; }
	}
</style>
