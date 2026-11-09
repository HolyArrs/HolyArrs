// Roving tab stop: one card in a row is tabbable and arrow keys move between cards.

// Elements inside a card that can take focus.
const FOCUSABLE = 'a[href], button:not([disabled]), input, select, textarea, [tabindex]';

// Re-applies on DOM changes so cards added later join the row.
export function rove(node: HTMLElement) {
	let active = 0;
	const cards = () => [...node.children].filter((c) => stops(c).length) as HTMLElement[];
	function apply() {
		const list = cards();
		if (active >= list.length) active = Math.max(0, list.length - 1);
		list.forEach((c, i) => stops(c).forEach((el) => (el.tabIndex = i === active ? 0 : -1)));
	}
	const cardOf = (t: EventTarget | null) => cards().findIndex((c) => c.contains(t as Node));
	function focusin(e: FocusEvent) {
		const i = cardOf(e.target);
		if (i >= 0 && i !== active) {
			active = i;
			apply();
		}
	}
	function keydown(e: KeyboardEvent) {
		if ((e.target as HTMLElement).matches?.('input, select, textarea')) return;
		const list = cards();
		const i = cardOf(e.target);
		if (i < 0) return;
		const to = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : e.key === 'Home' ? 0 : e.key === 'End' ? list.length - 1 : -2;
		if (to === -2) return;
		e.preventDefault();
		const c = list[Math.max(0, Math.min(list.length - 1, to))];
		if (!c) return;
		active = list.indexOf(c);
		apply();
		stops(c)[0]?.focus();
	}
	let queued = false;
	const mo = new MutationObserver(() => {
		if (queued) return;
		queued = true;
		queueMicrotask(() => {
			queued = false;
			apply();
		});
	});
	mo.observe(node, { childList: true, subtree: true });
	node.addEventListener('focusin', focusin);
	node.addEventListener('keydown', keydown);
	apply();
	return {
		destroy() {
			mo.disconnect();
			node.removeEventListener('focusin', focusin);
			node.removeEventListener('keydown', keydown);
		}
	};
}

// data-rove remembers each element's own tabindex choice, so a naturally skipped one stays skipped.
function stops(card: Element): HTMLElement[] {
	return [...card.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => {
		if (el.closest('[inert]')) return false;
		if (!el.dataset.rove) el.dataset.rove = el.getAttribute('tabindex') === '-1' ? 'skip' : 'on';
		return el.dataset.rove === 'on';
	});
}
