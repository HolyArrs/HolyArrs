import { searchField } from './search-field.svelte.ts';

// The top bar hides while you scroll down and comes back when you scroll up.
// One listener for the whole app (installed by the root layout); pages only
// mark their scope pills with data-pin-pills so they pin in the freed space.
// The change is one class on <html>; the bar and pills glide on the
// compositor (transform only, CSS transitions), so a mouse wheel's 100 px
// steps can't make it jump and no style is recomputed per frame.

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

export const topBar = $state({ hidden: false });

let st: BarState = { hidden: false, anchor: 0 };

function blocked(): boolean {
	const a = document.activeElement;
	if (a instanceof HTMLElement && a.closest('.ui-topbar')) return true;
	if (location.pathname.includes('/search') && searchField.term.trim()) return true; // typed words: results stay with the field
	return !!document.querySelector('dialog[open], [aria-modal="true"], .fullscreen, [data-fullscreen]') || !!document.fullscreenElement;
}

function apply(hidden: boolean) {
	if (topBar.hidden === hidden) return;
	topBar.hidden = hidden;
	document.documentElement.classList.toggle('bar-hidden', hidden);
}

/** overHero is whether a page hero ([data-hero]) still shows under the bar: then the bar drops its glass and line. */
export function overHero(heroBottom: number | null, barBottom: number): boolean {
	return heroBottom !== null && heroBottom > barBottom + 24;
}

/** checkHero sets html.over-hero from the page's hero; the layout calls it after each navigation too. */
export function checkHero() {
	if (typeof document === 'undefined') return;
	const h = document.querySelector('[data-hero]');
	const bar = document.querySelector('.ui-topbar');
	const over = overHero(h ? h.getBoundingClientRect().bottom : null, bar ? bar.getBoundingClientRect().height : 0);
	document.documentElement.classList.toggle('over-hero', over);
}

/** rebaseBar keeps the bar as it is and counts from the current place, after the page itself moved (content added above). */
export function rebaseBar() {
	if (typeof window === 'undefined') return;
	st = { hidden: topBar.hidden, anchor: window.scrollY };
}

/** showBar brings the bar back at once (Ctrl K, focus) and restarts the run from here. */
export function showBar() {
	if (typeof window === 'undefined') return;
	st = { hidden: false, anchor: window.scrollY };
	apply(false);
}

/** installScrollBar starts the one passive listener; the returned function stops it. */
export function installScrollBar(): () => void {
	let queued = false;
	const run = () => {
		queued = false;
		const next = nextBar(st, window.scrollY);
		st = next.hidden && blocked() ? { hidden: false, anchor: window.scrollY } : next;
		apply(st.hidden);
		checkHero();
	};
	const onScroll = () => {
		if (!queued) (queued = true), requestAnimationFrame(run);
	};
	const onFocus = (e: FocusEvent) => e.target instanceof HTMLElement && e.target.closest('.ui-topbar') && showBar();
	window.addEventListener('scroll', onScroll, { passive: true });
	document.addEventListener('focusin', onFocus);
	checkHero();
	return () => {
		window.removeEventListener('scroll', onScroll);
		document.removeEventListener('focusin', onFocus);
		apply(false);
	};
}
