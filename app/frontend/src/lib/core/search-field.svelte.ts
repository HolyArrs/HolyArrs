
import { DUR, EASE } from './transitions.ts';

type Hooks = { submit?: () => void; blur?: () => void; exit?: () => void };

export const searchField = $state({ term: '' });

export const dlScope = $state({ on: false, off: false, term: '' });

export const searchHooks: Hooks = {};

const fields = new Set<HTMLInputElement>();

export function registerField(el: HTMLInputElement): () => void {
	fields.add(el);
	return () => fields.delete(el);
}

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
export function rememberReturn() {
	if (armed || typeof window === 'undefined') return;
	armed = true;
	returnY = window.scrollY;
}
export function forgetReturn() {
	armed = false;
}
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
