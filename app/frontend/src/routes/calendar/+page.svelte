<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import { createCalendarScroll, type CalendarAnswer } from '#lib/discover/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import { setFollow } from '#lib/ui/follow.ts';
	import { rebaseBar, ComingSoon, MiniMonth, FollowWho, CalBanner, CalListRows, rowWords, DEFAULT_LOOK, type CaptionLook, byDay, entryKey, readMonthHidden, saveMonthHidden, shows, entryHref, weekStart, addDays, entryDay, rangeChunks, titlesFromAddress, itemDot, oneMovieDate, movieFacts, followsOk, withFollow, readFollows, saveFollows, startFollows, type Follows, type MovieFacts, ymd, readView, saveView, startView, type CalendarView, type TitlesShown } from '#lib/discover/index.ts';
	import { libraryBrief, partOff, partOn } from '#lib/mediafiles/index.ts';
	import { onMount, tick } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { date } from '#lib/core/format.ts';
	import { popIn, popOut } from '#lib/core/transitions.ts';
	import type { CalendarEntry } from '#lib/core/types.ts';

	const now = new Date();
	// ?month=October 2026 opens on that month, so a link to a month can be shared.
	const asked = /^(\d{4})-(\d{2})$/.exec(page.url.searchParams.get('month') ?? '');
	const askedMonth = asked && Number(asked[2]) >= 1 && Number(asked[2]) <= 12 ? new Date(Number(asked[1]), Number(asked[2]) - 1, 1) : null;
	// Cards (big pictures, day by day) or List (compact rows): ?view=cards|list, kept for the session. Old layout links (?view=2&span=...) land on Cards.
	let view = $state<CalendarView>(startView(page.url.searchParams.get('view'), readView()));
	// The days shown run from `start` (a year back, or the first of a linked month) to the day before `end`. Six more weeks load as the end comes near. Both stop a year from today.
	const today0 = addDays(now, 0);
	const REACH = 6;
	const earliest = addDays(today0, -365);
	const latest = addDays(today0, 366);
	// The whole past year loads with the first answer (one quick ask), and the page opens at today with that year above it, so scrolling up glides like scrolling down and nothing is ever slipped in above the person. A linked month starts at that month.
	let start = $state(askedMonth ?? earliest);
	let toTodayOnce = !askedMonth;
	// False until the page has been put on today (or on the linked month). Until then nothing may read the scroll position as the day in view: the rows at the top are a year back.
	let placed = !!askedMonth;
	let end = $state(addDays(askedMonth ?? today0, 7 * REACH));
	let entries = $state<CalendarEntry[]>([]);
	let libraryEmpty = $state(false);
	let loaded = $state(false);
	let fetching = $state(false);
	let error = $state('');
	// One menu ties the calendar to Follow: what you follow (asking for a title follows it), what anyone on this server follows, or every date in the Library. Picked once the first dates say how many titles you follow and how many people use the server.
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

	// Music turned off while picked falls back to All.
	$effect(() => {
		if (which === 'music' && partOff('music')) which = 'all';
	});
	const firstDay = $derived(ymd(start));
	const lastDay = $derived(ymd(end));
	// The query asks a day either side (an episode's local day can differ from the day it is stored under); those extra days never show as days of their own. A movie shows one date: the day it is out to watch, else its cinema date.
	const pool = $derived(oneMovieDate(entries, movies));
	const todayKey = ymd(now);
	const filterOk = (e: CalendarEntry) => followsOk(e, follows);
	const picked = (e: CalendarEntry) => shows(e, which) && filterOk(e);
	// The days in view for the type picked, before the filters (what the counts count).
	const base = $derived(pool.filter((e) => entryDay(e) >= firstDay && entryDay(e) < lastDay && shows(e, which)));
	const shown = $derived(base.filter(filterOk));
	const filteredOut = $derived(shown.length === 0 && base.length > 0);
	// Dates from today on under the other filters, for the empty page's one line (said once the year has loaded).
	const soonCount = $derived(base.filter((e) => entryDay(e) >= todayKey).length);
	const agenda = $derived(byDay(shown));
	const range = $derived({ from: ymd(addDays(start, -1)), to: ymd(addDays(end, 1)) });
	const rangeName = $derived(
		`${start.toLocaleDateString(appLocale(), { day: 'numeric', month: 'short' })} – ${new Date(end.getFullYear(), end.getMonth(), end.getDate() - 1).toLocaleDateString(appLocale(), { day: 'numeric', month: 'short', year: 'numeric' })}`
	);
	// The heading names the week in view (the day at the top of the page), not every day loaded.
	const viewWeek = $derived.by(() => {
		const s = weekStart(active ? new Date(active + 'T00:00:00') : (askedMonth ?? now));
		const e = addDays(s, 6);
		const f = (d: Date, y: boolean) => d.toLocaleDateString(appLocale(), { day: 'numeric', month: 'short', ...(y ? { year: 'numeric' } : {}) });
		return { from: ymd(s), long: `${f(s, false)} – ${f(e, true)}`, short: `${f(s, false)} – ${f(e, false)}` };
	});

	// The small month starts on the first week's month, pages on its own, and follows the day in view. It is anchored to today (or the linked month), never to the year back where loading starts.
	let pickYear = $state((askedMonth ?? now).getFullYear());
	let pickMonth = $state((askedMonth ?? now).getMonth());
	let active = $state<string | null>(null);
	let sheetOpen = $state(false);
	let filtersOpen = $state(false);

	const followsWords = $derived<Record<Follows, { label: string; text: string }>>({
		mine: { label: t('discover.calendar.myFollows'), text: t('discover.calendar.myFollowsText') },
		everyone: { label: t('discover.calendar.everyoneFollows'), text: t('discover.calendar.everyoneFollowsText') },
		library: { label: t('discover.calendar.wholeLibrary'), text: t('discover.calendar.wholeLibraryText') }
	});
	// Everyone's follows only means something with more than one person on the server.
	const followsChoices = $derived<Follows[]>(people > 1 ? ['mine', 'everyone', 'library'] : ['mine', 'library']);
	// How many dates each choice shows in the days loaded (for the type picked).
	const followsCount = $derived({
		mine: base.filter((e) => followsOk(e, 'mine')).length,
		everyone: base.filter((e) => followsOk(e, 'everyone')).length,
		library: base.length
	});
	function pickFollows(f: Follows) {
		if (f !== follows) keepDay();
		follows = f;
		saveFollows(f);
	}
	const clearFilters = () => pickFollows('library');
	// A filter change keeps the date in view. The day at the top is noted, and after the list changes that day (or the next one with something on it, else the last before it) goes back under the bars. The page never jumps to today.
	function keepDay() {
		const line = narrow ? 112 : 136;
		const rows = [...document.querySelectorAll<HTMLElement>('.cal-main [data-day]')];
		// Near the top (a closed card may have left the page there) the day in view is the one the heading names, else today; never the first row loaded, which is a year back.
		let was = window.scrollY < 4 || !placed ? (active ?? todayKey) : rows.find((x) => x.getBoundingClientRect().bottom > line + 8)?.dataset.day;
		if (!was) return;
		// The row standing in for today (today has nothing on it) means today: the new list may have today.
		if (was > todayKey && was === scroller.todayRow()?.dataset.day && firstDay <= todayKey) was = todayKey;
		const main = document.querySelector<HTMLElement>('.cal-main');
		if (main) main.style.minHeight = `${main.offsetHeight}px`;
		void tick().then(() => landOn(was, line, () => {
			if (main) main.style.minHeight = '';
		}));
	}
	// Puts the row for this full date (YYYY-MM-DD, or the next one after it) at `at` px from the top, and keeps putting it back for a few frames while the new rows settle and grow.
	function landOn(day: string, at: number, done?: () => void) {
		let frames = 0;
		let still = 0;
		const step = () => {
			const views = document.querySelectorAll<HTMLElement>('.cal-main .view-in');
			const scope = views[views.length - 1] ?? document;
			const rows = [...scope.querySelectorAll<HTMLElement>('[data-day]')];
			const row = rows.find((x) => (x.dataset.day ?? '') >= day) ?? rows.at(-1);
			if (row) {
				const off = row.getBoundingClientRect().top - at;
				if (Math.abs(off) > 1) {
					window.scrollTo({ top: window.scrollY + off, behavior: 'instant' });
					still = 0;
				} else still++;
			}
			if ((views.length > 1 || still < 4) && frames++ < 60) return void requestAnimationFrame(step);
			rebaseBar();
			done?.();
			scroller.follow();
			scroller.checkToday();
		};
		step();
	}
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

	let asks = 0;
	async function load() {
		const n = ++asks;
		fetching = true;
		try {
			const r = range;
			// The server takes up to a year per query; a longer stretch is asked in pieces.
			const [parts, t] = await Promise.all([
				Promise.all(rangeChunks(r.from, r.to).map((c) => api.get<CalendarAnswer>(`/calendar?from=${c.from}&to=${c.to}`))),
				libraryBrief()
			]);
			if (n !== asks) return;
			movies = new Map(t.filter((x) => x.kind === 'movie').map((x) => [x.id, movieFacts(x)]));
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
			libraryEmpty = t.length === 0;
			error = '';
			await tick();
			scroller.holdPlace();
			if (scroller.goTo) {
				const day = scroller.goTo;
				scroller.goTo = null;
				void scroller.pickDay(day);
			}
		} catch (e) {
			if (n !== asks) return;
			error = message(e);
			scroller.goTo = null;
			scroller.stepAgain = null;
		} finally {
			loaded = true;
			if (n === asks) {
				fetching = false;
				// A step that waited for this load runs once the rows are drawn.
				if (scroller.stepAgain && !error) {
					const s = scroller.stepAgain;
					scroller.stepAgain = null;
					void tick().then(() => scroller.go(s.n, s.from, s.far));
				}
				// The rows draw once loaded is set; then the page goes to today, below the bars.
				if (toTodayOnce && !error) {
					// One go: with nothing on or after today the page simply stays where it is.
					toTodayOnce = false;
					// Wait while an old view is still fading out (Cards or List clicked during the load).
					let tries = 0;
					const place = () => {
						if (document.querySelectorAll('.cal-main .view-in').length > 1 && tries++ < 40) return void requestAnimationFrame(place);
						scroller.atToday();
						placed = true;
						scroller.follow();
						scroller.checkToday();
					};
					void tick().then(place);
				}
			}
		}
	}

	const scrollerCtx = {
		get start() { return start; },
		set start(v) { start = v; },
		get end() { return end; },
		set end(v) { end = v; },
		get REACH() { return REACH; },
		get pickYear() { return pickYear; },
		set pickYear(v) { pickYear = v; },
		get pickMonth() { return pickMonth; },
		set pickMonth(v) { pickMonth = v; },
		get which() { return which; },
		get movies() { return movies; },
		get active() { return active; },
		set active(v) { active = v; },
		get placed() { return placed; },
		set placed(v) { placed = v; },
		get askedMonth() { return askedMonth; },
		get now() { return now; },
		get earliest() { return earliest; },
		get firstDay() { return firstDay; },
		get latest() { return latest; },
		get fetching() { return fetching; },
		landOn,
		get todayKey() { return todayKey; },
		get narrow() { return narrow; },
		get toTodayOnce() { return toTodayOnce; },
		set toTodayOnce(v) { toTodayOnce = v; },
		get today0() { return today0; },
		get range() { return range; },
		load,
		get lastDay() { return lastDay; },
		get shown() { return shown; },
		picked,
		get sheetOpen() { return sheetOpen; },
		set sheetOpen(v) { sheetOpen = v; },
		get loaded() { return loaded; },
		get away() { return away; },
		set away(v) { away = v; },
		get headEnd() { return headEnd; },
		get pastHead() { return pastHead; },
		set pastHead(v) { pastHead = v; },
		get view() { return view; },
		set view(v) { view = v; },
		get filteredOut() { return filteredOut; },
	};
	const scroller = createCalendarScroll(scrollerCtx);

	// Follow or stop following a title from its row. The dates stay loaded; only who follows changes, so the row updates in place (in My follows it leaves the list).
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
			scroller.monthDates = scroller.monthDates.map(upd);
			if (now !== was) myFollows = Math.max(0, myFollows + (now ? 1 : -1));
			toasts.push(now ? t('discover.calendar.followed', { name: e.name }) : t('discover.calendar.unfollowed', { name: e.name }), 'ok', now ? { body: t('discover.calendar.datesAdded') } : {});
		} catch (err) {
			toasts.push(t('discover.calendar.couldntUpdateFollow'), 'fail', { body: message(err) });
		} finally {
			flipping = null;
		}
	}

	const dotWord = $derived({ have: t('discover.calendar.inTheLibrary'), missing: t('discover.calendar.missing'), upcoming: t('discover.calendar.comingUp') });

	onMount(() => {
		const mq = matchMedia('(max-width: 1023px)');
		const fit = () => (narrow = mq.matches);
		fit();
		mq.addEventListener('change', fit);
		scroller.started = true;
		saveView(view);
		// The address always names the view; old layout parameters go.
		const u = new URL(page.url.href);
		if (u.searchParams.get('view') !== view || u.searchParams.has('span')) {
			u.searchParams.set('view', view);
			u.searchParams.delete('span');
			replaceState(u, page.state);
		}
		const again = () => (scroller.notePlace(), void load());
		const offs = [live.on('title', again), live.on('resync', again)];
		return () => {
			offs.forEach((off) => off());
			mq.removeEventListener('change', fit);
		};
	});
	// Rows change as dates load or filters change: the small month follows the day in view.
	$effect(() => {
		void shown;
		void view;
		if (loaded) void tick().then(() => (scroller.follow(), scroller.checkToday()));
	});
</script>

<svelte:window onscroll={scroller.onScroll} />

{#snippet item(e: CalendarEntry, prevWhen = '')}
	{@const dot = itemDot(e, todayKey)}
	{@const w = rowWords({ key: entryKey(e), day: entryDay(e), lead: e, entries: [e] }, look, todayKey)}
	{@const lines = w.lines.length ? w.lines : ['']}
	<li class="item k-{e.kind}" data-testid="cal-item">
		<!-- The time column: the air time, or Cinema, At home, Out now; a quiet All day without a time. -->
		<!-- A time repeated from the row above is left blank, so the eye sees when it changes. -->
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
			<IconButton icon={e.following ? 'bellOn' : 'bell'} size="sm" tipSide="below-end" label={e.following ? t('discover.calendar.unfollow', { name: e.name }) : t('discover.calendar.follow', { name: e.name })} aria-pressed={!!e.following} disabled={flipping === e.title_id} onclick={() => flipFollow(e, !e.following)} data-testid="cal-follow" />
		</span>
		<!-- The wide banner (title art with its logo) fills the row's right side, edge to edge. -->
		<span class="bwrap"><CalBanner {e} /></span>
	</li>
{/snippet}

{#snippet monthBtn(primary: boolean)}
	<!-- Hides or shows the small month; the days then take the full width. -->
	<button type="button" class="month-btn" class:sm={!primary} aria-pressed={!monthHidden} aria-label={monthHidden ? t('discover.calendar.showMonth') : t('discover.calendar.hideMonth')} title={monthHidden ? t('discover.calendar.showMonth') : t('discover.calendar.hideMonth')} onclick={flipMonth} data-testid={primary ? 'cal-month-toggle' : undefined}>
		<Icon name="calendar" size={16} /><span class="chev" class:off={monthHidden}><Icon name="chevronRight" size={14} /></span>
	</button>
{/snippet}

{#snippet mini()}
	<MiniMonth year={pickYear} month={pickMonth} entries={scroller.miniDates} today={todayKey} {active} onpick={scroller.pickDay} onpage={scroller.pickPage} />
{/snippet}

{#snippet rangeNav(primary: boolean, short: boolean)}
	<div class="range-nav">
		<IconButton icon="chevronLeft" label={t('discover.calendar.weekBefore')} variant="tinted" onclick={() => scroller.go(-1)} />
		{#if primary}
			<h2 class="range" aria-live="polite" data-testid="cal-month" data-week={viewWeek.from}>{viewWeek.long}</h2>
		{:else if short && narrow}
			<!-- In the slim bar the dates also open the small month (on phones, where it is not beside the days). -->
			<button type="button" class="range range-btn" aria-label={t('discover.calendar.pickADay', { long: viewWeek.long })} aria-haspopup="dialog" onclick={() => (sheetOpen = true)}>{viewWeek.short}</button>
		{:else}
			<span class="range">{short ? viewWeek.short : viewWeek.long}</span>
		{/if}
		<IconButton icon="chevronRight" label={t('discover.calendar.weekAfter')} variant="tinted" onclick={() => scroller.go(1)} />
	</div>
{/snippet}

{#snippet todayBtn(size: 'sm' | 'md')}
	<!-- Hidden while today is on screen; once it is not, it pops in red to bring you back. -->
	{#if away}
		<span class="today-btn" in:popIn out:popOut data-testid="cal-today">
			<Button variant="primary" {size} icon="calendar" onclick={scroller.toToday} aria-label={t('discover.calendar.backToToday')}>{t('discover.calendar.today')}</Button>
		</span>
	{/if}
{/snippet}

{#snippet typeSeg(primary: boolean)}
	<SegmentedControl
		label={t('discover.calendar.moviesShowsOrMusic')}
		testid={primary ? 'cal-titles' : undefined}
		value={which}
		onchange={(v) => (keepDay(), (which = v as TitlesShown))}
		options={[
			{ value: 'all', label: t('discover.calendar.all') },
			{ value: 'movies', label: t('discover.calendar.movies') },
			{ value: 'shows', label: t('discover.calendar.tvShows') },
			...(partOn('music') ? [{ value: 'music', label: t('discover.calendar.music') }] : [])
		]}
	/>
{/snippet}

{#snippet followsList()}
	<div class="showing" role="radiogroup" aria-label={t('discover.calendar.whichDates')}>
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
	<Popover label={t('discover.calendar.whichDates2', { label: followsWords[follows].label })} variant="button" icon={follows === 'library' ? 'bell' : 'bellOn'} text={followsWords[follows].label} width={320} align="end">
		{#snippet children()}<div class="pop-chips">{@render followsList()}</div>{/snippet}
	</Popover>
{/snippet}

{#snippet followsPhone()}
	<Button variant="secondary" size="sm" icon={follows === 'library' ? 'bell' : 'bellOn'} iconAfter="chevronDown" aria-haspopup="dialog" aria-label={t('discover.calendar.whichDates2', { label: followsWords[follows].label })} onclick={() => (filtersOpen = true)} data-testid="cal-follows-phone">{followsWords[follows].label}</Button>
{/snippet}

{#snippet viewSeg(primary: boolean)}
	<SegmentedControl
		label={t('discover.calendar.cardsOrList')}
		testid={primary ? 'cal-view' : undefined}
		value={view}
		onchange={(v) => scroller.pickView(v as CalendarView)}
		options={[
			{ value: 'cards', label: t('discover.calendar.cards') },
			{ value: 'list', label: t('discover.calendar.list') }
		]}
	/>
{/snippet}

<svelte:head><title>{t('discover.calendar.calendarHolyArrs')}</title></svelte:head>

<PageHeader title={t('discover.calendar.calendar')}>
	{#snippet actions()}
		{@render viewSeg(true)}
		<span class="wide-only">{@render todayBtn('md')}</span>
		<span class="phone-only"><IconButton icon="calendarDay" label={t('discover.calendar.pickADay2')} variant="tinted" aria-haspopup="dialog" onclick={() => (sheetOpen = true)} /></span>
	{/snippet}
</PageHeader>

<section class="tools" aria-label={t('discover.calendar.calendarControls')}>
		<div class="month-nav">{@render rangeNav(true, false)}</div>
		<div class="segs">{@render typeSeg(true)}</div>
		<span class="wide-only" data-testid="cal-follows">{@render followsMenu()}</span>
		<span class="wide-only">{@render monthBtn(true)}</span>
		<span class="phone-only">{@render followsPhone()}</span>
</section>
<span class="head-end" bind:this={headEnd} aria-hidden="true"></span>

<!-- The slim bar sits under the app's top bar once the controls above have scrolled away and stays there; Today shows in it only once today has left the screen. -->
<div class="slim-wrap" class:on={barOn} inert={!barOn} data-testid="cal-slim">
	<div class="slim" role="toolbar" aria-label={t('discover.calendar.calendarControls')}>
		{@render rangeNav(false, true)}
		<!-- Cards or List stays at hand in the bar once the header has scrolled away. -->
		<span class="wide-only slim-view">{@render viewSeg(false)}</span>
		<span class="phone-only"><IconButton icon={view === 'cards' ? 'menu' : 'grid'} label={view === 'cards' ? t('discover.calendar.showAsList') : t('discover.calendar.showAsCards')} variant="tinted" onclick={() => scroller.pickView(view === 'cards' ? 'list' : 'cards')} data-testid="cal-view-slim" /></span>
		<span class="wide-only slim-seg">{@render typeSeg(false)}</span>
		<span class="wide-only">{@render followsMenu()}</span>
		<span class="wide-only">{@render monthBtn(false)}</span>
		{@render todayBtn('sm')}
		<span class="phone-only">{@render followsPhone()}</span>
	</div>
</div>

{#if !loaded}
	<!-- Placeholder: a few day rows at the real rows' size. -->
	<div class="days-sk" aria-busy="true" aria-label={t('discover.calendar.loadingTheCalendar')} data-testid="calendar-skeleton">
		{#each [0, 1, 2] as d (d)}
			<div class="day-sk"><span class="sk" style="width: 56px; height: 64px; border-radius: 10px"></span><span class="sk" style="flex: 1; aspect-ratio: 16 / 7; border-radius: var(--r-panel)"></span></div>
		{/each}
	</div>
{:else if error && entries.length === 0}
	<LoadError hasData={false} message={t('discover.calendar.calendarCouldNotBe')} onretry={load} />
{:else if libraryEmpty}
	<EmptyState icon="calendar" title={t('discover.calendar.nothingComingUp')} text={t('discover.calendar.datesForYourMovies')} action={{ label: t('discover.calendar.addMedia'), href: to('/search'), icon: 'search' }} />
{:else}
	{#if error}<LoadError hasData message={t('discover.calendar.calendarCouldNotBe')} onretry={load} />{/if}
	<div class="cal-body" class:barred={barOn} class:month-off={monthHidden}>
		<div class="cal-main">
			{#key view}
				<div class="view-in" data-view={view}>
					{#if filteredOut}
						<!-- The filters hide every date in these weeks: say so, and offer them all. -->
						<div class="filtered-out" data-testid="cal-filtered-out">
							<EmptyState compact icon="calendar" title={follows === 'mine' ? t('discover.calendar.nothingYouFollow') : t('discover.calendar.nothingAnyoneFollows')} text={end < latest || fetching ? t('discover.calendar.lookingFurther') : soonCount === 1 ? t('discover.calendar.soonOne') : soonCount ? t('discover.calendar.soonOther', { n: soonCount }) : t('discover.calendar.soonNone')} action={{ label: t('discover.calendar.showWholeLibrary'), onclick: clearFilters }} />
						</div>
										{:else if view === 'cards'}
						<ComingSoon entries={shown} today={todayKey} onlater={scroller.more} mode={follows} {look} onfollow={flipFollow} />
					{:else if agenda.length === 0}
						<EmptyState compact icon="calendar" title={t('discover.calendar.noDatesFrom', { rangeName })} />
					{:else}
						<CalListRows days={agenda} layout={1} today={todayKey} {look} {follows} {flipping} onfollow={flipFollow} />
					{/if}
				</div>
			{/key}
			<!-- Reaching the end loads the next weeks, up to a year. -->
			<div class="endless" use:scroller.endless={fetching} aria-hidden="true"></div>
			{#if fetching}<p class="more-note" role="status">{t('discover.calendar.loadingTheNextWeeks')}</p>{/if}
		</div>
		<aside class="side" aria-label={t('discover.calendar.pickADay2')} inert={monthHidden} aria-hidden={monthHidden}>{@render mini()}</aside>
	</div>
{/if}

<Sheet bind:open={sheetOpen} title={t('discover.calendar.pickADay2')} placement="bottom">
	<div class="sheet-mini">
		{@render rangeNav(false, false)}
		{@render mini()}
		<Button variant="secondary" full icon="calendar" onclick={() => { sheetOpen = false; scroller.toToday(); }}>{t('discover.calendar.today')}</Button>
	</div>
</Sheet>

<Sheet bind:open={filtersOpen} title={t('discover.calendar.whichDates')} placement="bottom">
	<div class="sheet-filters">
		{@render typeSeg(false)}
		{@render followsList()}
	</div>
</Sheet>

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

	.cal-body { overflow-x: clip; overflow-clip-margin: 24px; display: grid; grid-template-columns: minmax(0, 1fr) 260px; column-gap: 28px; align-items: start; transition: grid-template-columns var(--dur) var(--ease-out), column-gap var(--dur) var(--ease-out); }
	/* Month hidden: it slides off to the right and the days take its room. */
	@media (min-width: 1024px) { .cal-body.month-off { grid-template-columns: minmax(0, 1fr) 0px; column-gap: 0px; } }
	.cal-body .side { transition: top var(--dur) var(--ease-out), opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out); }
	.cal-body.month-off .side { opacity: 0; transform: translateX(24px); pointer-events: none; overflow: hidden; visibility: hidden; width: 0; padding: 0; outline: 0; box-shadow: none; }
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

	/* The top bar and the date bar are one solid band, so nothing shows through either. */
	:global(html:has(.slim-wrap.on) .ui-topbar) { background-color: var(--bg); border-bottom-color: transparent; -webkit-backdrop-filter: none; backdrop-filter: none; }
	/* Over the poster wall it stays the app's light glass, like every other page. */
	:global(html:has([data-poster-wall])) .slim { background: color-mix(in srgb, var(--bg) 62%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); border-bottom-color: color-mix(in srgb, var(--text) 8%, transparent); }
	:global(html:has([data-poster-wall]):has(.slim-wrap.on) .ui-topbar) { background-color: color-mix(in srgb, var(--bg) 62%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); }
	.slim-wrap { position: sticky; top: var(--topbar); z-index: 20; height: 0; margin-inline: calc(-1 * var(--gutter)); transition: transform var(--bar-glide); }
	:global(html.bar-hidden) .slim-wrap { transform: translateY(calc(-1 * var(--topbar))); }
	.slim { display: flex; align-items: center; gap: 12px; padding: 8px var(--gutter); background: var(--bg); border-bottom: 1px solid var(--line); box-shadow: 0 10px 24px rgb(0 0 0 / .25);
		opacity: 0; visibility: hidden; transform: translateY(-8px); transition: opacity var(--dur-fast) var(--ease-in), transform var(--dur-fast) var(--ease-in), visibility 0s linear var(--dur-fast); }
	.slim-wrap.on .slim { opacity: 1; visibility: visible; transform: none; transition: opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out), visibility 0s; }
	.slim .slim-view { margin-right: auto; }
	/* One line, never squeezed: each control keeps its size; on narrower screens the extras step out (Cards or List first, then the kinds). */
	@media (min-width: 1024px) { .slim > :global(*) { flex: none; } .slim .range { overflow: visible; } }
	@media (max-width: 1320px) { .slim .slim-view { display: none; } .slim .slim-seg { margin-right: auto; } }
	@media (min-width: 1024px) and (max-width: 1080px) { .slim .slim-seg { display: none; } .slim :global(.range-nav) { margin-right: auto; } }
	.slim .range { min-width: 0; font: var(--t-headline); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.range-btn { padding: 4px 8px; border-radius: var(--r-sm); color: var(--text); transition: background var(--dur-fast) var(--ease-out); }
	.range-btn:hover { background: var(--surface-2); }
	.range-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) and (pointer: coarse) { .range-btn { min-height: 44px; } }
	.pop-chips { padding: 4px; }
	.filtered-out { margin-bottom: 24px; }
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
	.view-in { animation: view-in var(--dur-slow) var(--ease-out) both; }
	@keyframes view-in { from { opacity: 0; transform: translateY(8px); } }
	.endless { height: 1px; }
	.more-note { margin-top: 18px; color: var(--text-muted); font: var(--t-callout); }

	.days { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
	.day { display: grid; grid-template-columns: 56px minmax(0, 1fr); gap: 14px; padding: 12px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); scroll-margin-top: 8px; }
	/* Today is told by a small tag under the date, not by colour. */
	.today-tag { margin-top: 6px; display: inline-flex; }
	.when { display: flex; flex-direction: column; align-items: center; padding-top: 2px; }
	.dow { color: var(--accent-text); font: var(--t-caption); text-transform: none; }
	.when b { font: var(--t-title2); font-variant-numeric: tabular-nums; }
	.item { display: flex; align-items: center; gap: 10px; padding: 8px 0; min-width: 0; }
	.item + .item { border-top: 1px solid var(--line); }
	/* List rows: the banner fills the row's right side, out to the card's edge (the card's corners round it). */
	.day { overflow: hidden; }
	/* The picture fills the whole row (time, words and all); a dark wash from the left keeps the words readable. Each row has the picture's own wide shape, so every row is the same size; it grows a little on hover. */
	.item { position: relative; min-height: 84px; padding-right: 12px; transition: min-height var(--dur) var(--ease-out); }
	@media (hover: hover) { .item:hover { min-height: 96px; } }
	.bwrap { position: absolute; top: 0; bottom: 0; left: -12px; right: -12px; }
	/* Each row is its own soft card: rounded picture, a small gap between rows (no fades into the page, which turned white in light mode). */
	@media (min-width: 641px) {
		.bwrap { border-radius: 12px; overflow: hidden; }
		.item + .item { border-top: 0; margin-top: 6px; }
	}
	.bwrap :global(.cal-banner) { -webkit-mask-image: none; mask-image: none; }
	/* Behind the words: a frosted, darkened patch (blur, dim and a little grain of colour) that feathers softly into the picture, plus a faint wash over the rest so the right side keeps its colour. */
	.bwrap::before { content: ''; position: absolute; z-index: 1; top: 0; bottom: 0; left: 0; width: min(520px, 62%); pointer-events: none;
		background: linear-gradient(90deg, rgb(10 10 12 / .72), rgb(10 10 12 / .55) 60%, rgb(10 10 12 / .35));
		-webkit-backdrop-filter: blur(16px) saturate(1.25) brightness(.8); backdrop-filter: blur(16px) saturate(1.25) brightness(.8);
		-webkit-mask-image: linear-gradient(90deg, #000 0%, #000 68%, rgb(0 0 0 / .6) 84%, transparent 100%); mask-image: linear-gradient(90deg, #000 0%, #000 68%, rgb(0 0 0 / .6) 84%, transparent 100%); }
	.bwrap::after { content: ''; position: absolute; z-index: 1; inset: 0; pointer-events: none;
		background: linear-gradient(90deg, rgb(8 8 10 / .25) 0%, rgb(8 8 10 / 0) 70%), linear-gradient(0deg, rgb(8 8 10 / .25), rgb(8 8 10 / 0) 45%); }
	/* Over the picture the words are always light, in both themes. */
	@media (min-width: 641px) {
		.item .it a, .item .tcol { color: #fff; }
		.item .sub, .item .sub.first, .item .tcol.quiet { color: rgb(255 255 255 / .82); }
		.item .sub.first { color: #fff; }
	}
	.it, .tcol, .row-bell { position: relative; z-index: 2; }
	/* The words have one fixed width, so the bell sits at the same spot on every row. */
	.it { display: flex; flex-direction: column; min-width: 0; flex: 0 1 260px; }
	.who { display: flex; min-width: 0; margin-top: 3px; }
	.row-bell { flex: none; align-self: center; margin-left: 4px; }
	/* A round glass behind the bell keeps it visible over any picture. */
	.row-bell :global(.ui-ibtn) { background: rgb(0 0 0 / .45); -webkit-backdrop-filter: blur(8px); backdrop-filter: blur(8px); border-radius: 50%; }
	:global(html:not([data-theme='dark'])) .row-bell :global(.ui-ibtn) { background: rgb(255 255 255 / .7); }
	.row-bell :global(.ui-ibtn) { color: var(--text-faint); transition: color var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.item:hover .row-bell :global(.ui-ibtn), .row-bell :global(.ui-ibtn:focus-visible) { color: var(--text-muted); }
	.row-bell.on :global(.ui-ibtn) { color: var(--accent-text); }
	.it a { color: var(--text); font: var(--t-headline); font-size: 15px; overflow-wrap: anywhere; }
	.it a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sub { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; min-height: 1lh; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sub.first { color: var(--text); }
	/* One size for times and words (8 PM, In theaters, Streaming, Album), wide enough for the longest. */
	/* Set like the row's title (same face, weight and size) so it reads as part of the row. */
	.tcol { flex: none; width: 92px; font: var(--t-headline); font-size: 15px; font-variant-numeric: tabular-nums; white-space: nowrap; color: var(--text); align-self: center; text-align: center; }
	.tcol.quiet { color: var(--text-muted); }
	.tcol.same { visibility: hidden; }
	.when-phone { display: none; }
	/* Phones: the time sits above the title as a small bold line instead of a column. */
	@media (max-width: 640px) {
		.tcol { display: none; }
		/* Phones: the banner becomes a slim strip across the top of the row, the words below it; a smaller picture leaves the words room on narrow phones. */
		.item { flex-wrap: wrap; padding-right: 0; min-height: 0; }
		.bwrap, .item:first-child .bwrap, .item:last-child .bwrap { position: relative; inset: auto; order: -1; flex: 0 0 100%; width: 100%; height: 56px; border-radius: var(--r-sm); overflow: hidden; }
		.bwrap :global(.cal-banner) { -webkit-mask-image: none; mask-image: none; }
		.bwrap::before, .bwrap::after { display: none; }
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
		/* No room for the which-dates button beside the dates and Today; it stays in the controls above. */
		.slim > .phone-only:last-child { display: none; }
		.slim .range-nav { flex: 1; }
		.slim .range { flex: 1; text-align: center; }
	}

	.muted { color: var(--text-muted); }
	.field { display: flex; flex-direction: column; gap: 6px; }
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
