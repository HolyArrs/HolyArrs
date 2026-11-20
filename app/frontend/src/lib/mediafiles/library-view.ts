// What the Library prints for a title, and how its lists move.
import { fade, slide } from 'svelte/transition';
import { ago, bytes, date } from '../core/format.ts';
import { reducedMotion } from '../core/motion.ts';
import { peopleNames, shownPerson } from '../core/plain.ts';
import { DUR, easeOut } from '../core/transitions.ts';
import type { TitleView } from '../core/types.ts';
import { quality } from '#lib/ui/title-cards.ts';
import { t as tr } from '../core/i18n/i18n.svelte.ts';
import { codecLabel, languageName } from './copies.ts';
import { libraryState } from './library-state.ts';
import { shownScore } from '../core/rating.ts';
import { scoreName, scoreText as outsideText, type OutsideScore } from '#lib/ui/outside-ratings.ts';
import { progressOf, releaseDate, type Column, type FilterState } from './table.ts';

export type View = 'rows' | 'table' | 'posters' | 'overview';

export type Entry = { type: 'head'; key: string; id: number; name: string; count: number } | { type: 'row'; key: string; t: TitleView };

/** How many poster columns fit the width, as the grid's auto-fill counts them. */
export const posterCols = (width: number, min: number, gap: number) => Math.max(1, Math.floor((width + gap) / (min + gap)));

/** A poster row's height before it is drawn: the tallest art, the caption line (14 px at 1.35), any 12 px lines under it, and the gap below. */
export function posterRowGuess(cards: { square: boolean; lines: number }[], colW: number, gap: number): number {
	let h = 0;
	for (const c of cards) h = Math.max(h, colW * (c.square ? 1 : 1.5) + 6 + 3 + 18.9 + c.lines * (3 + 15.6));
	return h + gap;
}

/** The first line whose bottom is below `y`, from the running tops (`offsets[i]` is line i's top, the last is the end). */
export function lineAt(offsets: number[], y: number): number {
	let lo = 0;
	let hi = offsets.length - 1;
	while (lo < hi) {
		const mid = (lo + hi) >> 1;
		if (offsets[mid + 1] <= y) lo = mid + 1;
		else hi = mid;
	}
	return lo;
}

/** The lines to draw with the list scrolled to `top`: the screen and a margin above and below it. */
export function drawRange(offsets: number[], top: number, viewH: number, margin = 800): { start: number; end: number } {
	const n = offsets.length - 1;
	return { start: Math.max(0, lineAt(offsets, top - margin)), end: Math.min(n, lineAt(offsets, top + viewH + margin) + 1) };
}

/** A measured poster row's key: the same row redrawn with other columns, details or width starts again from its guess. */
export const posterRowKey = (cols: number, detail: string, guess: number, firstId: number) => `p${cols}:${detail}:${Math.round(guess)}:${firstId}`;

type FrameHost = Pick<typeof globalThis, 'requestAnimationFrame' | 'cancelAnimationFrame' | 'addEventListener' | 'removeEventListener'>;
const STOP_ON = ['wheel', 'touchstart', 'keydown', 'pointerdown'];
// A modifier alone or a held key's repeat is not the person moving on (Alt is often still held from Alt+Left, going back).
const QUIET = new Set(['Shift', 'Control', 'Alt', 'AltGraph', 'Meta', 'CapsLock']);
const SIDEWAYS = new Set(['ArrowLeft', 'ArrowRight']);
/** The browser's own back and forward: Alt with an arrow, Cmd with an arrow or a bracket, the mouse's back and forward buttons. They leave the page rather than move it. */
const goesBackOrForward = (e: Event) => {
	const k = e as KeyboardEvent;
	if (e.type === 'pointerdown') return (e as PointerEvent).button > 2;
	return e.type === 'keydown' && ((k.altKey && SIDEWAYS.has(k.key)) || (k.metaKey && (SIDEWAYS.has(k.key) || k.key === '[' || k.key === ']')));
};

/** Calls `stop` at the first scroll, tap or key that moves the page on. Returns the unlisten. */
function onMove(win: Pick<FrameHost, 'addEventListener' | 'removeEventListener'>, stop: () => void): () => void {
	const f = (e: Event) => {
		const k = e as KeyboardEvent;
		if (!(e.type === 'keydown' && (k.repeat || QUIET.has(k.key))) && !goesBackOrForward(e)) stop();
	};
	for (const e of STOP_ON) win.addEventListener(e, f, { capture: true, passive: true });
	return () => {
		for (const e of STOP_ON) win.removeEventListener(e, f, true);
	};
}

/** Runs `step` now and then once a frame until it says it is done, the time is up, or the person scrolls, taps or types. Returns a stop. */
export function untilSettled(step: () => boolean, ms: number, onEnd = () => {}, win: FrameHost = globalThis): () => void {
	const until = performance.now() + ms;
	let raf = 0;
	let over = false;
	const stop = () => {
		if (over) return;
		over = true;
		win.cancelAnimationFrame(raf);
		off();
		onEnd();
	};
	const off = onMove(win, stop);
	const run = () => {
		if (over) return;
		if (step() || performance.now() >= until) stop();
		else raf = win.requestAnimationFrame(run);
	};
	run();
	return stop;
}

/** A restored place has landed: within a pixel of it (`goal`, when the place moves as rows are measured) on a page that kept its height since the last frame (a list that just appeared is laid out again once its width is measured), or at the end of a page that has not grown for `calm` ms (`room` is how far the page scrolls; a slow read can still lengthen a short list). */
export function lander(want: number, calm = 500, now = () => performance.now()): (at: number, room: number, goal?: number) => boolean {
	let lastRoom = -1;
	let since = 0;
	return (at, room, goal = want) => {
		const same = room === lastRoom;
		if (!same) [lastRoom, since] = [room, now()];
		return (same && Math.abs(at - goal) <= 1) || (at >= room - 1 && now() - since >= calm);
	};
}

export type Anchor = { id: number; top: number };

/** Where a saved place goes back to: the same spot at the list width it was saved at. At another width (a turned phone, a resized window, the sidebar shown or hidden) rows hold other numbers of titles, so the row of the title kept with it (`a`), as high on screen as that title was (no lower than the middle of a screen `viewH` tall, which may be shorter now), once the list knows where that row is. */
export function placeTop(p: { y: number; w?: number; a?: Anchor }, width: number, rowTop: (id: number) => number | null | undefined, viewH = Infinity): number {
	const top = p.w !== undefined && p.a && p.w !== width ? rowTop(p.a.id) : null;
	return top == null || !p.a ? p.y : Math.max(0, top - Math.min(p.a.top, viewH / 2));
}

/** The title a place is kept by, with its top on screen: the one being opened when it is drawn, else the first on screen. */
export function anchorOf(cards: Element[], viewH: number, opened: number | null): Anchor | null {
	const el = cards.find((c) => opened !== null && (c as HTMLElement).dataset.titleId === String(opened));
	if (el) return { id: Number(opened), top: el.getBoundingClientRect().top };
	const first = firstOnScreen(cards, viewH);
	return first && { id: first.id, top: first.top };
}

type Waits = { subs: boolean; sets: boolean };

/** The list is not whole yet: a filter or grouping needs data that is still on its way (titles that want subtitles, the movie series). Posters are never grouped. */
export const listWaits = (f: Pick<FilterState, 'subs' | 'series'>, grouped: boolean, view: View, wait: Waits) => (f.subs && wait.subs) || ((!!f.series || (grouped && view !== 'posters')) && wait.sets);

/** Marks a read as on its way until it answers or fails. */
export function reading<T>(wait: Waits, which: keyof Waits, p: Promise<T>): Promise<T> {
	wait[which] = true;
	return p.finally(() => (wait[which] = false));
}

/** A saved place that waits for the list to be whole. The first scroll, tap or key drops it, and so do a list other than the one it was saved on (`key`), the page moved half a screen some other way (a scrollbar drag, find in page), and 8 s of waiting once the titles are here (`loaded`; a big library may take longer to load). `take` gives it once nothing is on its way; `peek` shows it while it waits; `stop` drops it. */
export function placeGate<P>(place: P, key: string, win: FrameHost & Pick<typeof globalThis, 'scrollY' | 'innerHeight'> = globalThis) {
	let left: P | null = place;
	// Where the page sits a frame after it is shown (the browser may have put it somewhere first).
	let from: number | null = null;
	const raf = win.requestAnimationFrame(() => (from = win.scrollY));
	let late: ReturnType<typeof setTimeout> | undefined;
	const stop = () => {
		left = null;
		win.cancelAnimationFrame(raf);
		clearTimeout(late);
		off();
	};
	const off = onMove(win, stop);
	const moved = () => from !== null && Math.abs(win.scrollY - from) > win.innerHeight / 2;
	return {
		take(now: string, waits: boolean, loaded = true): P | null {
			if (now !== key || moved()) stop();
			if (left && loaded && late === undefined) late = setTimeout(stop, 8000);
			if (!left || waits) return null;
			const p = left;
			stop();
			return p;
		},
		peek(): P | null {
			if (moved()) stop();
			return left;
		},
		stop
	};
}

export type Place = { id: number; at: number; top: number };

/** The first card at least half on screen: its title, its place in the list and its top on screen. */
export function firstOnScreen(cards: Iterable<Element>, viewH: number): Place | null {
	for (const el of cards) {
		const r = el.getBoundingClientRect();
		if (r.top + r.height / 2 < 0) continue;
		return r.top < viewH ? { id: Number((el as HTMLElement).dataset.titleId), at: Number(el.getAttribute('aria-posinset')) - 1, top: r.top } : null;
	}
	return null;
}

/** One frame of keeping a title at `want` on screen, given where it is (null while not drawn: then `guess`, its row's place). True once it stayed put two frames, or when someone else moved the page or it cannot move further. */
export function holder(want: number, page: { y: () => number; by: (dy: number) => void }): (top: number | null, guess: number) => boolean {
	let at = page.y();
	let still = 0;
	return (top, guess) => {
		if (Math.abs(page.y() - at) > 1) return true;
		const d = (top ?? guess) - want;
		// Settled only once the title itself is drawn in place; a guessed row alone may still be off.
		if (Math.abs(d) < 1) return top !== null && ++still >= 2;
		still = 0;
		page.by(d);
		const moved = Math.abs(page.y() - at) >= 0.5;
		at = page.y();
		// Stuck at an end of the page: done once the title is drawn; a guess for an undrawn one may have overshot, so keep going.
		return !moved && top !== null;
	};
}

/** A focused card about to leave the drawn rows hands focus to its grid, so focus never drops to the page. */
export function holdFocus(grid: HTMLElement | undefined, drawn: { id: number }[]) {
	const li = grid?.contains(document.activeElement) ? document.activeElement?.closest<HTMLElement>('li') : null;
	if (li && !drawn.some((t) => String(t.id) === li.dataset.titleId)) grid?.focus({ preventScroll: true });
}

/** Keeps the first title on screen where it was while the poster rows change shape (columns, size, details, a live update), as the browser does for a plain page. `rowTop` is where a title's row now is on screen, or null when it left the list or moved far. */
export function placeKeeper(grid: () => HTMLElement | undefined, rowTop: (p: Place) => number | null) {
	let hold: Place | null = null;
	let last: Place | null = null;
	let stop = () => {};
	const seen = () => {
		const g = grid();
		return g ? firstOnScreen(g.children, innerHeight) : null;
	};
	let lastW: number | undefined;
	const mark = (force?: unknown) => {
		if (hold) return;
		// While the grid is another width than when the place was seen, a resize is under way (and the browser may scroll meanwhile): keep the place seen before it.
		const w = grid()?.clientWidth;
		if (force !== true && last && w !== lastW) return;
		last = seen();
		lastW = w;
	};
	// A remembered place counts while its title is still drawn within the drawn rows' reach of where it was seen: a resize moves it at most that far, a jump much further.
	const near = (p: Place | null) => {
		const r = p && grid()?.querySelector(`[data-title-id="${p.id}"]`)?.getBoundingClientRect();
		return !!p && !!r && Math.abs(r.top - p.top) < innerHeight + 1600;
	};
	let notes = 0;
	let kept = 0;
	let restarting = false;
	// A pointer held down (a scrollbar drag, a finger on the screen) is the person moving the page: no hold starts or restarts until it lets go.
	let pressed = false;
	const down = () => (pressed = true);
	// A move with no button down also clears it, should a release be missed.
	const up = (e?: PointerEvent) => {
		if (!(e?.type === 'pointermove' && e.buttons)) pressed = false;
	};
	const PRESS = [['pointerdown', down], ['pointerup', up], ['pointercancel', up], ['pointermove', up]] as const;
	for (const [e, f] of PRESS) globalThis.addEventListener?.(e, f as EventListener, { capture: true, passive: true });
	/** A finished hold notes the held title while it sits where it was held; the first card on screen would creep to the start of its row with every change. */
	const settle = (h: Place) => {
		const el = grid()?.querySelector(`[data-title-id="${h.id}"]`);
		const top = el?.getBoundingClientRect().top;
		if (!el || top === undefined || Math.abs(top - h.top) >= 1) return mark(true);
		last = { id: h.id, at: Number(el.getAttribute('aria-posinset')) - 1, top };
		lastW = grid()?.clientWidth;
	};
	/** After: bring it back to the same height, until it stays put (a second at most). */
	const keep = () => {
		kept = notes;
		const h = hold;
		restarting = true;
		stop();
		restarting = false;
		if (!h || pressed || rowTop(h) === null) return void (hold = null);
		hold = h;
		const step = holder(h.top, { y: () => scrollY, by: (top) => scrollBy({ top, behavior: 'instant' }) });
		stop = untilSettled(() => step(grid()?.querySelector(`[data-title-id="${h.id}"]`)?.getBoundingClientRect().top ?? null, rowTop(h) ?? h.top), 1000, () => {
			// Restarting for the next change keeps the place; only a finished hold notes the new one.
			if (restarting) return;
			hold = null;
			settle(h);
		});
	};
	return {
		/** While scrolling and once rows are measured: the place to go back to, since a resize moves the cards before the rows change. */
		mark,
		/** Before the rows change. */
		note() {
			if (!hold) hold = (near(last) ? last : null) ?? seen();
			// The new width is taken in hand: scrolling notes the place again from here on.
			lastW = grid()?.clientWidth;
			// Svelte can run the after-step of an update before its before-step: keep right after the update then, so a noted place is never left waiting for a later change.
			const n = ++notes;
			queueMicrotask(() => void (kept < n && keep()));
		},
		keep,
		/** The list is gone: stop holding and stop listening. */
		stop: () => {
			stop();
			for (const [e, f] of PRESS) globalThis.removeEventListener?.(e, f as EventListener, true);
		}
	};
}

// Rows of chips and folds grow in; a fade under reduced motion.
export const grow = (n: Element) => (reducedMotion() ? fade(n, { duration: DUR.fast }) : slide(n, { duration: DUR.base, easing: easeOut }));
export const shrink = (n: Element) => (reducedMotion() ? fade(n, { duration: DUR.fast }) : slide(n, { duration: DUR.fast, easing: easeOut }));

export function linkedPeople(t: TitleView) {
	const people = new Map<number, { id: number; name: string }>();
	for (const ask of t.asks ?? []) {
		if (ask.title_id !== t.id) continue;
		for (const person of [ask.asked_by, ...ask.people]) if (person.name.trim()) people.set(person.id, { ...person, name: shownPerson(person.name) });
	}
	return [...people.values()];
}

export function requestedBy(t: TitleView): string {
	return peopleNames(linkedPeople(t).map((p) => p.name));
}

export const isProgressCol = (c: Column) => c.id === 'episodes' || c.id === 'tracks';

/** A column's width for the Rows grid (a stretching column takes its floor). */
export const colPx = (c: Column) => Math.max(84, parseInt(c.width.replace(/^minmax\((\d+)px.*$/, '$1'), 10) || 150);

/** A fact for the Rows view: counts for progress columns, "14 d ago" for dates that are recent by nature. */
export function rowCell(t: TitleView, c: Column, p: { have: number; total: number }, series: string): string {
	if (isProgressCol(c)) return p.total ? `${p.have}/${p.total}` : '';
	if (c.id === 'release' && t.kind === 'movie' && t.digital_date && Date.parse(t.digital_date) <= Date.now()) return date(t.digital_date);
	if (c.id === 'added') return ago(t.added_at);
	if (c.id === 'activity') return ago(t.last_activity);
	return cell(t, c, series);
}

/** Bar colour: all in, some missing, nothing yet, or only future ones. */
export function tone(t: TitleView): 'full' | 'gap' | 'none' | 'later' {
	const p = progressOf(t);
	const s = t.kind === 'show' ? t.show : t.kind === 'artist' ? t.artist : undefined;
	if (p.total === 0) return 'later';
	if (p.have >= p.total) return 'full';
	if (!t.keep_up_to_date && p.have === 0) return 'none';
	return s && s.missing === 0 && p.have < p.total ? 'later' : 'gap';
}

function movieOut(t: TitleView): string {
	const now = Date.now();
	if (t.digital_date) return Date.parse(t.digital_date) <= now ? date(t.digital_date) : tr('mediafiles.view.homeRelease', { date: date(t.digital_date) });
	if (t.cinema_date) return Date.parse(t.cinema_date) <= now ? tr('mediafiles.view.inCinemas', { date: date(t.cinema_date) }) : tr('mediafiles.view.cinemas', { date: date(t.cinema_date) });
	return date(releaseDate(t)) || tr('mediafiles.view.notKnown');
}

/** A title's score to one decimal, or '' when it has none or too few votes. */
export function scoreText(t: TitleView): string {
	return shownScore(t.score, t.votes)?.toFixed(1) ?? '';
}

/** A title's IMDb, Rotten Tomatoes and Metacritic scores (from OMDb), worded as the title page words them; [] when none is known. */
export function outsideScores(t: TitleView): OutsideScore[] {
	const out: OutsideScore[] = [];
	if (t.imdb_rating) out.push({ source: 'imdb', value: t.imdb_rating, max: 10, votes: t.imdb_votes ?? 0 });
	if (t.rotten_tomatoes) out.push({ source: 'rt_critics', value: t.rotten_tomatoes, max: 100, votes: 0 });
	if (t.metacritic) out.push({ source: 'metacritic', value: t.metacritic, max: 100, votes: 0 });
	return out;
}

/** The hover text of a cell: the full path, or the score's outside scores beside it. */
export function cellTitle(t: TitleView, c: Column, series: string): string | undefined {
	if (c.id === 'path') return cell(t, c, series);
	if (c.id !== 'score') return undefined;
	const words = outsideScores(t).map((s) => `${scoreName(s.source)} ${outsideText(s)}`);
	return words.length ? [scoreText(t) ? `TMDB ${scoreText(t)}` : '', ...words].filter(Boolean).join(' · ') : undefined;
}

export function cell(t: TitleView, c: Column, series: string): string {
	switch (c.id) {
		case 'network': return t.show?.network ?? '';
		case 'rules': return t.pick_rules_name ?? '';
		case 'next': return t.show?.next_air_date ? date(t.show.next_air_date) : t.show?.status === 'ended' ? tr('mediafiles.view.ended') : '';
		case 'last': return date(t.show?.last_air_date);
		case 'seasons': return t.show?.season_count ? String(t.show.season_count) : '';
		case 'added': return date(t.added_at);
		case 'year': return t.year ? String(t.year) : '';
		case 'size': return t.size_bytes ? bytes(t.size_bytes) : '';
		case 'codec': return t.copy && t.copy.codec !== 'unknown' ? codecLabel(t.copy.codec) : '';
		case 'audio': return (t.copy?.audio_languages ?? []).map(languageName).join(', ');
		case 'genres': return (t.genres ?? []).slice(0, 3).join(', ');
		case 'rating': return t.rating ?? '';
		case 'score': return scoreText(t) ? `★ ${scoreText(t)}` : '';
		case 'path': return t.folder_path ?? '';
		case 'release': return t.kind === 'movie' ? movieOut(t) : '';
		case 'runtime': return t.runtime_minutes ? tr('mediafiles.view.runtime', { h: Math.floor(t.runtime_minutes / 60), m: t.runtime_minutes % 60 }) : '';
		case 'series': return series;
		case 'albums': return t.artist ? `${t.artist.included}/${t.artist.albums}` : '';
		case 'latest': return date(t.artist?.next_release);
		case 'status': return libraryState(t)?.label ?? '';
		case 'requested': return requestedBy(t);
		case 'quality': {
			const have = quality(t);
			const want = t.versions?.[0]?.label;
			return have || want ? `${have ?? '—'} → ${want ?? '—'}` : '';
		}
		case 'kept': return t.keep_up_to_date ? tr('mediafiles.view.yes') : tr('mediafiles.view.no');
		case 'showstate': return t.show?.status === 'ended' ? tr('mediafiles.view.ended') : t.show?.status ? tr('mediafiles.view.continuing') : '';
		case 'activity': return date(t.last_activity);
		default: return '';
	}
}

/** The "Everything" line under a poster: next airing for shows, else rules and quality. */
export function posterExtra(t: TitleView): string {
	if (t.kind === 'show' && t.show?.next_air_date) return tr('mediafiles.view.next', { date: date(t.show.next_air_date) });
	return [t.pick_rules_name, quality(t)].filter(Boolean).join(' · ');
}

/** The hover panel: quality and pick rules, next airing, added, who asked. */
export function posterFacts(t: TitleView): string[] {
	const asked = requestedBy(t);
	return [
		[quality(t), t.pick_rules_name].filter(Boolean).join(' · '),
		t.kind === 'show' && t.show?.next_air_date ? tr('mediafiles.view.next', { date: date(t.show.next_air_date) }) : '',
		t.added_at ? tr('mediafiles.view.added', { date: date(t.added_at) }) : '',
		asked ? tr('mediafiles.view.askedBy', { name: asked }) : ''
	].filter(Boolean);
}

/** The Library's rows: each movie, show or artist, with a heading before every series when grouped. */
export function buildEntries(shown: TitleView[], grouped: boolean, seriesOfTitle: Map<number, number>, seriesName: (id: number) => string): Entry[] {
	if (!grouped) return shown.map((t) => ({ type: 'row', key: `r${t.id}`, t }));
	const bySet = new Map<number, TitleView[]>();
	const loose: TitleView[] = [];
	for (const t of shown) {
		const s = seriesOfTitle.get(t.id);
		if (s) bySet.set(s, [...(bySet.get(s) ?? []), t]);
		else loose.push(t);
	}
	const out: Entry[] = [];
	for (const [id, ts] of [...bySet].sort((a, b) => seriesName(a[0]).localeCompare(seriesName(b[0])))) {
		out.push({ type: 'head', key: `h${id}`, id, name: seriesName(id), count: ts.length });
		for (const t of ts) out.push({ type: 'row', key: `r${t.id}`, t });
	}
	if (loose.length) {
		out.push({ type: 'head', key: 'h0', id: 0, name: tr('mediafiles.view.noCollection'), count: loose.length });
		for (const t of loose) out.push({ type: 'row', key: `r${t.id}`, t });
	}
	return out;
}
