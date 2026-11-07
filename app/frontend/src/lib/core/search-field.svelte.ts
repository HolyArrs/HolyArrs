// The one search field lives in the top bar (a phone-only copy sits on the
// Search page, where the top bar is hidden). Its words are shared here so the
// Search page can follow them as you type on any page.

import { DUR, EASE } from './transitions.ts';

type Hooks = { submit?: () => void; blur?: () => void; exit?: () => void };

export const searchField = $state({ term: '' });

/** On Downloads the one search field filters that list: `on` while the page is open, `off` once the scope chip is removed. */
export const dlScope = $state({ on: false, off: false, term: '' });

/** The Search page's own reactions (Enter, leaving the field); set while it is open. */
export const searchHooks: Hooks = {};

const fields = new Set<HTMLInputElement>();

/** registerField records a field element; the returned function forgets it. */
export function registerField(el: HTMLInputElement): () => void {
	fields.add(el);
	return () => fields.delete(el);
}

/** focusField focuses (and selects) the field that is on screen; false when none is. */
export function focusField(select = false): boolean {
	for (const el of fields) {
		if (el.offsetParent === null && el.getClientRects().length === 0) continue;
		el.focus();
		if (select) el.select();
		return true;
	}
	return false;
}

let returnY = 0;
let armed = false;
/** rememberReturn notes (once per visit) the scroll of the page Search was opened from (call before leaving it). */
export function rememberReturn() {
	if (armed || typeof window === 'undefined') return;
	armed = true;
	returnY = window.scrollY;
}
/** forgetReturn ends a Search visit (the page closing), so the next visit notes its own spot. */
export function forgetReturn() {
	armed = false;
}
/** restoreReturn puts that scroll back once the page is long enough (its rows load late). */
export function restoreReturn(fromPath: string) {
	const y = returnY;
	const t0 = performance.now();
	const step = () => {
		if (location.pathname === fromPath) return void requestAnimationFrame(step);
		const room = document.documentElement.scrollHeight - innerHeight;
		if (room >= y || performance.now() - t0 > 3000) return void scrollTo({ top: Math.min(y, room), behavior: 'instant' });
		requestAnimationFrame(step);
	};
	requestAnimationFrame(step);
}

/** playFieldEntry brings the field in first on entering Search: a fade and a .985 → 1 scale, then a faint glow that fades. */
export function playFieldEntry(calm: boolean) {
	for (const el of fields) {
		const form = el.closest('form');
		if (!form || el.getClientRects().length === 0) continue;
		form.style.willChange = 'transform, opacity';
		const a = form.animate([{ opacity: 0.2, transform: calm ? 'none' : 'scale(0.985)' }, { opacity: 1, transform: 'none' }], { duration: DUR.slow, easing: EASE.out });
		a.onfinish = a.oncancel = () => (form.style.willChange = '');
		form.querySelector('.glow')?.animate([{ opacity: 0 }, { opacity: 1, offset: 0.35 }, { opacity: 0 }], { duration: 1400, easing: 'ease-out' });
	}
}
