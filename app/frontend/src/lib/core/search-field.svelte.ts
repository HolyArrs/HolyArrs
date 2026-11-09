// Shared search box state and the page-return scroll memory.

import { DUR, EASE } from './transitions.ts';

type Hooks = { submit?: () => void; blur?: () => void; exit?: () => void };

// Text in the main search box.
export const searchField = $state({ term: '' });

// Download-page scope toggle and its own search text.
export const dlScope = $state({ on: false, off: false, term: '' });

// Lets the active page react to submit, blur and exit of the box.
export const searchHooks: Hooks = {};

// Search inputs currently mounted, since layouts may render more than one.
const fields = new Set<HTMLInputElement>();

// Returns an undo function for cleanup when the input unmounts.
export function registerField(el: HTMLInputElement): () => void {
	fields.add(el);
	return () => fields.delete(el);
}

// Focuses the first visible input; hidden copies are skipped.
export function focusField(select = false): boolean {
	for (const el of fields) {
		if (el.offsetParent === null && el.getClientRects().length === 0) continue;
		el.focus();
		if (select) el.select();
		return true;
	}
	return false;
}

// Scroll spot saved when leaving a list, so Back lands in the same place.
let returnY = 0;
let armed = false;
// Only the first call counts until forgotten, so later scrolls do not overwrite it.
export function rememberReturn() {
	if (armed || typeof window === 'undefined') return;
	armed = true;
	returnY = window.scrollY;
}
export function forgetReturn() {
	armed = false;
}
// Waits for the old page to leave and the new one to grow tall enough, then scrolls; gives up after 3s.
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

// Fades the search form in; calm mode skips the scale.
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
