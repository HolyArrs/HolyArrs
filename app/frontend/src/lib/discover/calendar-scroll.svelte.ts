// The Calendar's moving around: jumping to a day, the arrows, endless scrolling both ways and keeping the place on screen.
import { rebaseBar } from './scroll-bar.svelte.ts';
import { entryKey, shiftMonth, weekStart, addDays, monthRange, oneMovieDate, ymd, saveView, type CalendarView } from './calendar.ts';
import { tick, untrack } from 'svelte';
import { page } from '$app/state';
import { afterNavigate, replaceState } from '$app/navigation';
import { to } from '#lib/core/paths.ts';
import { api } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { glideScroll } from '#lib/core/transitions.ts';
import type { CalendarEntry } from '#lib/core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type CalendarAnswer = { dates: CalendarEntry[] | null; my_follows?: number; people?: number };

export function createCalendarScroll(ctx: any) {
	function jump(d: Date) {
		stepAgain = null;
		unhold();
		ctx.start = addDays(d, 0);
		ctx.end = addDays(ctx.start, 7 * ctx.REACH);
		anchor = null;
		ctx.pickYear = ctx.start.getFullYear();
		ctx.pickMonth = ctx.start.getMonth();
	}
	// The arrows move a week from the week in view (the one the heading names). They land on the first row of the next week that has dates (back over empty weeks: the dates before them), and load further when that week is not loaded yet.
	let stepAgain: { n: number; from: string; far?: boolean } | null = null;
	// The latest day before `day` that a Library movie shows on (one date per movie, as the cards).
	function shownBefore(day: string): string {
		if (ctx.which !== 'all' && ctx.which !== 'movies') return '';
		let best = '';
		for (const f of ctx.movies.values()) {
			const d = [f.digital, f.physical].filter((x): x is string => !!x).sort()[0] ?? f.cinema;
			if (d && d < day && d > best) best = d;
		}
		return best;
	}
	// While an arrow's scroll runs, follow() keeps the week the arrow picked; otherwise the frames in between, or a page that cannot scroll that far, would put the old week back.
	let held: string | null = null;
	let unhold = () => {};
	const userInput = ['wheel', 'touchstart', 'pointerdown', 'keydown'] as const;
	function hold(day: string) {
		unhold();
		held = day;
		// An instant scroll ends in the same frame it starts: only a later scrollend frees the hold.
		let armed = false;
		requestAnimationFrame(() => requestAnimationFrame(() => (armed = true)));
		// Released after this frame's callbacks, so a follow() queued by the last scroll frame still sees the hold.
		const onEnd = () => armed && setTimeout(() => unhold(), 0);
		const timer = setTimeout(() => unhold(), 1500);
		unhold = () => {
			held = null;
			clearTimeout(timer);
			removeEventListener('scrollend', onEnd);
			for (const t of userInput) removeEventListener(t, unhold);
			unhold = () => {};
		};
		addEventListener('scrollend', onEnd);
		// The person scrolling or pressing anything takes over at once.
		for (const t of userInput) addEventListener(t, unhold, { passive: true, once: true });
	}
	function land(day: string) {
		ctx.active = day;
		const d = new Date(day + 'T00:00:00');
		ctx.pickYear = d.getFullYear();
		ctx.pickMonth = d.getMonth();
		hold(day);
	}
	function go(n: number, from0?: string, far = false) {
		if (!ctx.placed) return;
		// The latest press wins: a step or a small-month pick still waiting for a load is dropped (the step is set again below if this one waits too).
		stepAgain = null;
		goTo = null;
		const from = from0 ?? ctx.active ?? ymd(weekStart(ctx.askedMonth ?? ctx.now));
		const fromWeek = ymd(weekStart(new Date(from + 'T00:00:00')));
		const target = addDays(new Date(fromWeek + 'T00:00:00'), 7 * n);
		// How far back a step may load: a year from today, or for a linked month further back than that, down to the nearest earlier movie date in the Library (its day out to watch, else its cinema day).
		let lo = ctx.start < ctx.earliest ? ctx.start : ctx.earliest;
		// Only when the week asked for is before the days loaded: steps inside them never load.
		if (n < 0 && ctx.start < ctx.earliest && !far && ymd(target) < ctx.firstDay) {
			// Before what is loaded already, so a movie hidden by the filters does not stop the search.
			const before = shownBefore(fromWeek < ctx.firstDay ? fromWeek : ctx.firstDay);
			if (before) {
				const w = weekStart(new Date(before + 'T00:00:00'));
				if (w < lo) lo = w;
			}
		}
		const edge = () => toasts.push(n > 0 ? t('discover.calendarScroll.noLater') : t('discover.calendarScroll.noEarlier'), 'ok');
		if (target >= ctx.latest || addDays(target, 7) <= lo) return edge();
		const tw = ymd(target);
		const rows = [...document.querySelectorAll<HTMLElement>('.cal-main [data-day]')];
		const dayOfRow = (r: HTMLElement) => r.dataset.day ?? '';
		let row = n > 0 ? rows.find((r) => dayOfRow(r) >= tw) : (rows.find((r) => dayOfRow(r) >= tw && dayOfRow(r) < fromWeek) ?? rows.filter((r) => dayOfRow(r) < fromWeek).at(-1));
		if (!row) {
			// Not loaded yet: load the rest of that side once (or wait for the load running), then step again.
			if (ctx.fetching) return void (stepAgain = { n, from });
			if (n > 0 && ctx.end < ctx.latest) {
				stepAgain = { n, from };
				ctx.end = ctx.latest;
				return;
			}
			if (n < 0 && ctx.start > lo) {
				// Once per press: a far-back search that still finds no row names the week or says so.
				stepAgain = { n, from, far: ctx.start < ctx.earliest };
				earlier(lo);
				return;
			}
			// Nothing earlier, but the days loaded start before this week: name the week before at the top.
			if (n < 0 && tw >= ctx.firstDay) {
				land(tw);
				window.scrollTo({ top: 0, behavior: 'instant' });
				return;
			}
			return edge();
		}
		// The first row of that row's week, so the heading names it from its start.
		const rowWeek = ymd(weekStart(new Date(dayOfRow(row) + 'T00:00:00')));
		row = rows.find((r) => dayOfRow(r) >= rowWeek) ?? row;
		land(dayOfRow(row));
		goRow(row);
	}
	// Today's row just below the top bar and the date bar, at once.
	function atToday(): boolean {
		if (!todayRow()) return false;
		ctx.landOn(ctx.todayKey, ctx.narrow ? 112 : 136);
		return true;
	}
	// Calendar picked again (side menu) while it already shows: start over at today and the top.
	afterNavigate((nav) => {
		if (nav.type !== 'link' || nav.from?.route.id !== nav.to?.route.id) return;
		stepAgain = null;
		unhold();
		const m = /^(\d{4})-(\d{2})$/.exec(nav.to?.url.searchParams.get('month') ?? '');
		if (m && Number(m[2]) >= 1 && Number(m[2]) <= 12) {
			ctx.toTodayOnce = false;
			ctx.placed = true;
			const at = new Date(Number(m[1]), Number(m[2]) - 1, 1);
			jump(at < ctx.earliest ? ctx.earliest : at >= ctx.latest ? addDays(ctx.latest, -7 * ctx.REACH) : at);
			window.scrollTo({ top: 0, behavior: 'instant' });
			return;
		}
		if (ctx.start.getTime() === ctx.earliest.getTime() && ctx.end.getTime() === addDays(ctx.today0, 7 * ctx.REACH).getTime()) return void atToday();
		ctx.start = ctx.earliest;
		ctx.end = addDays(ctx.today0, 7 * ctx.REACH);
		anchor = null;
		ctx.toTodayOnce = true;
		ctx.placed = false;
	});
	// Each new range (the arrows, Today, more weeks, the small month) is loaded once.
	let started = $state(false);
	$effect(() => {
		void ctx.range.from;
		void ctx.range.to;
		if (started) untrack(() => void ctx.load());
	});
	// Today glides to today's row when it is loaded, else loads the usual stretch and opens there.
	function toToday() {
		stepAgain = null;
		unhold();
		const row = ctx.firstDay <= ctx.todayKey ? todayRow() : undefined;
		if (row) goRow(row);
		else {
			// Today is not loaded (a linked month): load the past year to six weeks ahead, then go to today.
			if (ctx.start.getTime() === ctx.earliest.getTime() && ctx.end.getTime() === addDays(ctx.today0, 7 * ctx.REACH).getTime()) return void atToday();
			ctx.start = ctx.earliest;
			ctx.end = addDays(ctx.today0, 7 * ctx.REACH);
			anchor = null;
			ctx.toTodayOnce = true;
			ctx.placed = false;
		}
	}
	const todayRow = () => [...document.querySelectorAll<HTMLElement>('.cal-main [data-day]')].find((r) => (r.dataset.day ?? '') >= ctx.todayKey);

	// The small month's arrows; a month outside what is loaded gets loaded (within a year).
	function pickPage(n: number) {
		({ year: ctx.pickYear, month: ctx.pickMonth } = shiftMonth(ctx.pickYear, ctx.pickMonth, n));
	}
	// The small month shows its whole month: dates outside the days loaded are asked for it alone.
	let monthDates = $state<CalendarEntry[]>([]);
	$effect(() => {
		const r = monthRange(ctx.pickYear, ctx.pickMonth);
		if (!started || (r.from >= ctx.firstDay && r.to <= ctx.lastDay)) {
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
		const keys = new Set(ctx.shown.map(entryKey));
		return [...ctx.shown, ...oneMovieDate(monthDates, ctx.movies).filter((e) => ctx.picked(e) && !keys.has(entryKey(e)))];
	});
	// A day not loaded yet (from the small month) loads first, then the page glides to it.
	let goTo: string | null = null;

	// When days go in above the ones on screen (a far day picked in the small month, a live update), the row at the top of the screen is noted before and put back where it was after (scrollTop moves by what was added), so nothing jumps.
	let anchor: { day: string; top: number } | null = null;
	function notePlace() {
		// The first row that starts below the top bar and the date bar (rows differ in height between views, so a row half hidden under the bars is a poor mark); else the one under them.
		const line = ctx.narrow ? 112 : 136;
		const rows = [...document.querySelectorAll<HTMLElement>('.cal-main [data-day]')];
		const r = rows.find((x) => x.getBoundingClientRect().top >= line - 8) ?? rows.find((x) => x.getBoundingClientRect().bottom > line);
		anchor = r ? { day: r.dataset.day ?? '', top: r.getBoundingClientRect().top } : null;
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
			// The page moved, not the person: the top bar starts counting again from here.
			rebaseBar();
		}
	}
	function earlier(to: Date) {
		if (to >= ctx.start) return;
		notePlace();
		ctx.start = to;
	}
	// Glide to a row; a long way (more than two screens) goes at once, since a long glide stalls.
	function goRow(row: HTMLElement) {
		const far = Math.abs(row.getBoundingClientRect().top) > 2 * innerHeight;
		row.scrollIntoView({ block: 'start', behavior: far ? 'instant' : glideScroll() });
		if (far) rebaseBar();
	}
	// A day picked in the small month: glide to its row (instant under reduced motion).
	async function pickDay(day: string) {
		stepAgain = null;
		const fromSheet = ctx.sheetOpen;
		ctx.sheetOpen = false;
		if (fromSheet) await tick();
		const at = new Date(day + 'T00:00:00');
		if (day < ctx.firstDay) {
			const to = at < ctx.earliest ? ctx.earliest : at;
			if (to >= ctx.start) return;
			goTo = day;
			earlier(to);
			return;
		}
		if (day >= ctx.lastDay) {
			const to = addDays(at, 7) > ctx.latest ? ctx.latest : addDays(at, 7);
			if (to <= ctx.end) return;
			goTo = day;
			ctx.end = to;
			return;
		}
		const row = document.querySelector<HTMLElement>(`.cal-main [data-day="${day}"]`);
		if (!row) return;
		ctx.active = day;
		goRow(row);
	}
	// The day at the top of the page, for the small month (which pages along when the month changes).
	function follow() {
		// Not on today yet: the small month and the heading stay on today, never the first day loaded.
		if (!ctx.placed || held) return;
		const rows = document.querySelectorAll<HTMLElement>('.cal-main [data-day]');
		const line = 140;
		let top: string | null = null;
		for (const r of rows) {
			if (r.getBoundingClientRect().top <= line) top = r.dataset.day ?? null;
			else break;
		}
		if (!top) return;
		// Today has nothing on it: the first row after today stands for today's week, not its own.
		if (top > ctx.todayKey && top === todayRow()?.dataset.day && ctx.firstDay <= ctx.todayKey) top = ctx.todayKey;
		if (top === ctx.active) return;
		ctx.active = top;
		if (top) {
			const d = new Date(top + 'T00:00:00');
			ctx.pickYear = d.getFullYear();
			ctx.pickMonth = d.getMonth();
		}
	}
	// Today is on screen when its row (or, on a day with nothing, the next day's row) shows below the bars. Days loaded from after today, or none loaded yet, count as away.
	function checkToday() {
		if (!ctx.loaded || ctx.firstDay > ctx.todayKey) {
			ctx.away = ctx.loaded;
			return;
		}
		const row = todayRow();
		if (!row) {
			ctx.away = !!document.querySelector('.cal-main [data-day]');
			return;
		}
		const b = row.getBoundingClientRect();
		ctx.away = b.bottom < (ctx.narrow ? 96 : 120) || b.top > innerHeight;
	}
	let ticking = false;
	function onScroll() {
		if (ctx.headEnd) ctx.pastHead = ctx.headEnd.getBoundingClientRect().top < (ctx.narrow ? 44 : 56);
		if (ticking) return;
		ticking = true;
		requestAnimationFrame(() => {
			ticking = false;
			follow();
			checkToday();
		});
	}

	// Cards or List: in the address and kept for the session. Switching keeps the same day under the bars: the height is held while the views swap, so the page cannot shrink and scroll up, which would bring the top bar back.
	function pickView(v: CalendarView) {
		if (v === ctx.view) return;
		// Not placed yet (the first load is still on its way): switch only; the load puts the new view on today.
		if (!ctx.placed) {
			ctx.view = v;
			saveView(v);
			const u0 = new URL(page.url.href);
			u0.searchParams.set('view', v);
			u0.searchParams.delete('span');
			replaceState(u0, page.state);
			return;
		}
		const main = document.querySelector<HTMLElement>('.cal-main');
		if (main) main.style.minHeight = `${main.offsetHeight}px`;
		notePlace();
		const was = anchor;
		anchor = null;
		// The same full date goes back where it was in the new view; at the very top, the day in view.
		const day = was && window.scrollY >= 4 ? was.day : (ctx.active ?? (ctx.askedMonth ? null : ctx.todayKey));
		const at = was && window.scrollY >= 4 ? was.top : ctx.narrow ? 112 : 136;
		void tick().then(() => {
			if (!day) {
				if (main) main.style.minHeight = '';
				return;
			}
			ctx.landOn(day, at, () => {
				if (main) main.style.minHeight = '';
			});
		});
		ctx.view = v;
		saveView(v);
		const u = new URL(page.url.href);
		u.searchParams.set('view', v);
		u.searchParams.delete('span');
		replaceState(u, page.state);
	}

	// Near the end, the next six weeks load.
	function more() {
		if (ctx.fetching || ctx.end >= ctx.latest) return;
		// Nothing shows under the filters: the end stays in view, so six weeks at a time would load again and again (and the count under the empty page would keep changing). Ask once to the end.
		if (ctx.filteredOut) return void (ctx.end = ctx.latest);
		const to = addDays(ctx.end, 7 * ctx.REACH);
		ctx.end = to > ctx.latest ? ctx.latest : to;
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

	return {
		todayRow,
		follow,
		checkToday,
		holdPlace,
		get goTo() { return goTo; },
		set goTo(v) { goTo = v; },
		pickDay,
		get stepAgain() { return stepAgain; },
		set stepAgain(v) { stepAgain = v; },
		go,
		atToday,
		get monthDates() { return monthDates; },
		set monthDates(v) { monthDates = v; },
		get started() { return started; },
		set started(v) { started = v; },
		notePlace,
		onScroll,
		get miniDates() { return miniDates; },
		pickPage,
		toToday,
		pickView,
		more,
		endless,
	};
}
