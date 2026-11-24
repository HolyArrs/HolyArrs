// DOM actions for the Plugins page: sideways wheel on the tab chips, one-line card chips, card name room and arrow keys between tiles.
import { moveIndex } from './view.ts';

// A mouse wheel scrolls the chip row sideways, so chips past the edge can be reached without a trackpad.
export function wheelSideways(node: HTMLElement) {
	const on = (e: WheelEvent) => {
		if (node.scrollWidth <= node.clientWidth || Math.abs(e.deltaX) >= Math.abs(e.deltaY)) return;
		e.preventDefault();
		node.scrollLeft += e.deltaY;
	};
	node.addEventListener('wheel', on, { passive: false });
	return { destroy: () => node.removeEventListener('wheel', on) };
}
// Card footer chips stay on one line and the version always shows whole. When room runs short:
// drop the downloads chip, then show the date in its numeric form, then tighten chip padding and font.
export function fitChips(node: HTMLElement) {
	const fit = () => {
		const drops = [...node.querySelectorAll<HTMLElement>('[data-drop]')];
		for (const d of drops) d.hidden = false;
		node.classList.remove('sdate', 'tight', 'tighter');
		const over = () => node.scrollWidth > node.clientWidth + 1;
		const steps = [() => drops.forEach((d) => (d.hidden = true)), () => node.classList.add('sdate'), () => node.classList.add('tight'), () => node.classList.add('tighter')];
		for (const step of steps) {
			if (!over()) break;
			step();
		}
	};
	const ro = new ResizeObserver(fit);
	ro.observe(node);
	const mo = new MutationObserver(fit);
	mo.observe(node, { childList: true, characterData: true, subtree: true });
	void document.fonts?.ready.then(fit);
	fit();
	return { destroy: () => (ro.disconnect(), mo.disconnect()) };
}
// A card name keeps its two full lines: when the name and its New or Trending tag don't fit, the tag moves
// under the name and the words below give up one line, so every card keeps the same shape.
export function tagRoom(node: HTMLElement) {
	const card = node.closest('li');
	const fit = () => {
		if (!card) return;
		card.classList.remove('tagdown', 'longname', 'longername');
		const over = () => node.scrollHeight > node.clientHeight + 1;
		if (over() && node.querySelector('.tin .tag')) card.classList.add('tagdown');
		// A name that alone needs a third line steps its font down a size instead of being cut.
		if (over()) card.classList.add('longname');
		if (over()) card.classList.add('longername');
	};
	const ro = new ResizeObserver(fit);
	ro.observe(node);
	void document.fonts?.ready.then(fit);
	fit();
	return { destroy: () => ro.disconnect() };
}
// Arrow keys move between the tiles or rows; Enter opens one (the tile is a button).
export function arrowKeys(node: HTMLElement) {
	node.addEventListener('keydown', arrows);
	return { destroy: () => node.removeEventListener('keydown', arrows) };
}
function arrows(e: KeyboardEvent) {
	if (!['ArrowRight', 'ArrowLeft', 'ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return;
	const box = e.currentTarget as HTMLElement;
	const opens = [...box.querySelectorAll<HTMLElement>('[data-testid="plugin-row"] .open')];
	const from = opens.indexOf(document.activeElement as HTMLElement);
	if (from < 0) return;
	const to = moveIndex(opens.map((o) => o.closest('li')!.getBoundingClientRect()), from, e.key);
	if (to >= 0 && to !== from) {
		e.preventDefault();
		opens[to].focus();
	}
}
