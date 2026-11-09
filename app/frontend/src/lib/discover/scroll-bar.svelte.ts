import { searchField } from '../core/search-field.svelte.ts';

// The top bar hides while you scroll down and comes back when you scroll up. One listener for the whole app (installed by the root layout); pages only mark their scope pills with data-pin-pills so they pin in the freed space.

export const HIDE_AFTER = 80; // Px of downward scroll before the bar leaves.
export const SHOW_AFTER = 24; // Px of upward scroll before it comes back.

export type BarState = { hidden: boolean; anchor: number };

/** The pure direction logic; `anchor` is where the current run of scroll began. */
export function nextBar(s: BarState, y: number): BarState {
	if (y <= 0) return { hidden: false, anchor: 0 };
	if (s.hidden) {
		if (y > s.anchor) return { hidden: true, anchor: y }; // Still going down: follow.
		return s.anchor - y >= SHOW_AFTER ? { hidden: false, anchor: y } : s;
	}
	if (y < s.anchor) return { hidden: false, anchor: y }; // Going up: follow.
	return y - s.anchor >= HIDE_AFTER ? { hidden: true, anchor: y } : s;
}

export const topBar = $state({ hidden: false });

let st: BarState = { hidden: false, anchor: 0 };

function blocked(): boolean {
	const a = document.activeElement;
	if (a instanceof HTMLElement && a.closest('.ui-topbar')) return true;
	if (location.pathname.includes('/search') && searchField.term.trim()) return true; // Typed words: results stay with the field.
	return !!document.querySelector('dialog[open], [aria-modal="true"], .fullscreen, [data-fullscreen]') || !!document.fullscreenElement;
}

function apply(hidden: boolean) {
	if (topBar.hidden === hidden) return;
	topBar.hidden = hidden;
	document.documentElement.classList.toggle('bar-hidden', hidden);
}

/** Whether a page hero ([data-hero]) still shows under the bar; then the bar drops its glass and line. */
export function overHero(heroBottom: number | null, barBottom: number): boolean {
	return heroBottom !== null && heroBottom > barBottom + 24;
}

/** Sets html.over-hero from the page's hero; the layout calls it after each navigation too. */
export function checkHero() {
	if (typeof document === 'undefined') return;
	const h = document.querySelector('[data-hero]');
	const bar = document.querySelector('.ui-topbar');
	const over = overHero(h ? h.getBoundingClientRect().bottom : null, bar ? bar.getBoundingClientRect().height : 0);
	document.documentElement.classList.toggle('over-hero', over);
}

/** Keeps the bar as it is and counts from the current place after the page itself moved (content added above). */
export function rebaseBar() {
	if (typeof window === 'undefined') return;
	st = { hidden: topBar.hidden, anchor: window.scrollY };
}

/** Brings the bar back at once (Ctrl K, focus) and restarts the run from here. */
export function showBar() {
	if (typeof window === 'undefined') return;
	st = { hidden: false, anchor: window.scrollY };
	apply(false);
}

/** Starts the one passive listener; the returned function stops it. */
export function installScrollBar(): () => void {
	let queued = false;
	// Which way the person last asked to go (wheel, swipe, keys). While it is up, the page moving down by itself (days added above, pictures loading) never hides the bar. The wish counts for a short while only.
	let goingUp = false;
	let wishAt = 0;
	const wish = (up: boolean) => ((goingUp = up), (wishAt = performance.now()));
	let touchY = 0;
	const onWheel = (e: WheelEvent) => e.deltaY && wish(e.deltaY < 0);
	const onTouchStart = (e: TouchEvent) => (touchY = e.touches[0]?.clientY ?? 0);
	const onTouchMove = (e: TouchEvent) => {
		const y = e.touches[0]?.clientY ?? 0;
		if (Math.abs(y - touchY) > 4) wish(y > touchY), (touchY = y);
	};
	const onKey = (e: KeyboardEvent) => {
		const t = e.target;
		if (e.defaultPrevented || (t instanceof HTMLElement && t.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])'))) return;
		if (['ArrowUp', 'PageUp', 'Home'].includes(e.key) || (e.key === ' ' && e.shiftKey)) wish(true);
		else if (['ArrowDown', 'PageDown', 'End', ' '].includes(e.key)) wish(false);
	};
	const run = () => {
		queued = false;
		let next = nextBar(st, window.scrollY);
		if (goingUp && performance.now() - wishAt < 600 && next.hidden && (!st.hidden || window.scrollY < st.anchor)) next = { hidden: false, anchor: window.scrollY };
		st = next.hidden && blocked() ? { hidden: false, anchor: window.scrollY } : next;
		apply(st.hidden);
		checkHero();
	};
	const onScroll = () => {
		if (!queued) (queued = true), requestAnimationFrame(run);
	};
	const onFocus = (e: FocusEvent) => e.target instanceof HTMLElement && e.target.closest('.ui-topbar') && showBar();
	window.addEventListener('scroll', onScroll, { passive: true });
	window.addEventListener('wheel', onWheel, { passive: true });
	window.addEventListener('touchstart', onTouchStart, { passive: true });
	window.addEventListener('touchmove', onTouchMove, { passive: true });
	window.addEventListener('keydown', onKey);
	document.addEventListener('focusin', onFocus);
	checkHero();
	return () => {
		window.removeEventListener('scroll', onScroll);
		window.removeEventListener('wheel', onWheel);
		window.removeEventListener('touchstart', onTouchStart);
		window.removeEventListener('touchmove', onTouchMove);
		window.removeEventListener('keydown', onKey);
		document.removeEventListener('focusin', onFocus);
		apply(false);
	};
}
