import { searchField } from './search-field.svelte.ts';

// The top bar hides while you scroll down and comes back when you scroll up.
// One listener for the whole app (installed by the root layout); pages only
// mark their scope pills with data-pin-pills so they pin in the freed space.
// The bar follows the scroll itself (--bar on <html>, 0 shown → 1 away, like
// Safari's toolbar), then settles to fully shown or away once scrolling stops.

export const HIDE_AFTER = 80; // px of downward scroll before the bar leaves
export const SHOW_AFTER = 24; // px of upward scroll before it comes back

export type BarState = { hidden: boolean; anchor: number };

/** nextBar is the pure direction logic: anchor is where the current run of scroll began. */
export function nextBar(s: BarState, y: number): BarState {
	if (y <= 0) return { hidden: false, anchor: 0 };
	if (s.hidden) {
		if (y > s.anchor) return { hidden: true, anchor: y }; // still going down: follow
		return s.anchor - y >= SHOW_AFTER ? { hidden: false, anchor: y } : s;
	}
	if (y < s.anchor) return { hidden: false, anchor: y }; // going up: follow
	return y - s.anchor >= HIDE_AFTER ? { hidden: true, anchor: y } : s;
}

export const BAR_TRAVEL = 64; // px of scroll that moves the bar all the way
export const SETTLE_MS = 360; // the settle once scrolling stops
const IDLE_MS = 120;

/** nextProgress moves the bar with the scroll: dy px moves it dy/BAR_TRAVEL of the way. */
export function nextProgress(p: number, dy: number, y: number): number {
	if (y <= 0) return 0;
	return Math.min(1, Math.max(0, p + dy / BAR_TRAVEL));
}

/** settleTarget is where a part-way bar rests: away only past halfway and clear of the top. */
export function settleTarget(p: number, y: number): 0 | 1 {
	return p > 0.5 && y > BAR_TRAVEL ? 1 : 0;
}

const easeOut = (x: number) => 1 - Math.pow(1 - x, 3);

export const topBar = $state({ hidden: false });

let st: BarState = { hidden: false, anchor: 0 };

function blocked(): boolean {
	const a = document.activeElement;
	if (a instanceof HTMLElement && a.closest('.ui-topbar')) return true;
	if (location.pathname.includes('/search') && searchField.term.trim()) return true; // typed words: results stay with the field
	return !!document.querySelector('dialog[open], [aria-modal="true"], .fullscreen, [data-fullscreen]') || !!document.fullscreenElement;
}

let prog = 0;
let settle = 0;

function apply(hidden: boolean) {
	if (topBar.hidden === hidden) return;
	topBar.hidden = hidden;
	document.documentElement.classList.toggle('bar-hidden', hidden);
}

function draw(p: number) {
	prog = p;
	document.documentElement.style.setProperty('--bar', p.toFixed(4));
	apply(p > 0.5);
}

/** glide eases the bar from where it is to `to` over SETTLE_MS. */
function glide(to: number) {
	cancelAnimationFrame(settle);
	const from = prog;
	if (from === to) return;
	const t0 = performance.now();
	const step = (now: number) => {
		const k = Math.min(1, (now - t0) / SETTLE_MS);
		draw(from + (to - from) * easeOut(k));
		if (k < 1) settle = requestAnimationFrame(step);
	};
	settle = requestAnimationFrame(step);
}

/** showBar brings the bar back at once (Ctrl K, focus) and restarts the run from here. */
export function showBar() {
	if (typeof window === 'undefined') return;
	st = { hidden: false, anchor: window.scrollY };
	glide(0);
}

/** installScrollBar starts the one passive listener; the returned function stops it. */
export function installScrollBar(): () => void {
	const still = matchMedia('(prefers-reduced-motion: reduce)');
	let queued = false;
	let lastY = window.scrollY;
	let idle = 0;
	const run = () => {
		queued = false;
		const y = window.scrollY;
		const dy = y - lastY;
		lastY = y;
		if (blocked()) {
			st = { hidden: false, anchor: y };
			glide(0);
			return;
		}
		if (still.matches) {
			// Reduced motion: no following, just shown or away.
			st = nextBar(st, y);
			cancelAnimationFrame(settle);
			draw(st.hidden ? 1 : 0);
			return;
		}
		cancelAnimationFrame(settle);
		draw(nextProgress(prog, dy, y));
		clearTimeout(idle);
		idle = window.setTimeout(() => glide(settleTarget(prog, window.scrollY)), IDLE_MS);
	};
	const onScroll = () => {
		if (!queued) (queued = true), requestAnimationFrame(run);
	};
	const onFocus = (e: FocusEvent) => e.target instanceof HTMLElement && e.target.closest('.ui-topbar') && showBar();
	window.addEventListener('scroll', onScroll, { passive: true });
	document.addEventListener('focusin', onFocus);
	return () => {
		window.removeEventListener('scroll', onScroll);
		document.removeEventListener('focusin', onFocus);
		clearTimeout(idle);
		cancelAnimationFrame(settle);
		draw(0);
	};
}
