// The card-to-page zoom (tablet and desktop). One flight at a time; a new
// move, Back mid-way or a second click turns the current one around or
// clears it, so nothing is ever left half-drawn. Transform and opacity only.

import { EASE, MS, flip, phone, reduced, shift, type Box, type Move } from './motion.ts';

/** What the next Settings screen should play, set before it mounts. */
export const settingsNav: { move: Move; group: string | null } = { move: null, group: null };

interface Flight {
	group: string;
	dir: 'open' | 'back';
	ghost: HTMLElement;
	art: HTMLElement | null;
	anims: Animation[];
	grown: boolean;
	arrived: boolean;
	hidden: HTMLElement | null;
}

let flight: Flight | null = null;

const boxOf = (el: Element): Box => {
	const r = el.getBoundingClientRect();
	return { left: r.left, top: r.top, width: r.width, height: r.height };
};

/** stage is the content column below the top bar, inside the screen. */
function stage(): Box {
	const col = document.querySelector('.ui-shell > .content') ?? document.body;
	const r = col.getBoundingClientRect();
	const bar = document.querySelector('.ui-topbar');
	const top = Math.max(0, bar ? bar.getBoundingClientRect().bottom : 0);
	return { left: r.left, top, width: Math.min(r.width, innerWidth - r.left), height: innerHeight - top };
}

function place(el: HTMLElement, b: Box) {
	el.style.left = `${b.left}px`;
	el.style.top = `${b.top}px`;
	el.style.width = `${b.width}px`;
	el.style.height = `${b.height}px`;
}

function make(cls: string, b: Box): HTMLElement {
	const el = document.createElement('div');
	el.className = cls;
	el.setAttribute('aria-hidden', 'true');
	place(el, b);
	document.body.appendChild(el);
	return el;
}

function artClone(card: HTMLElement, b: Box): HTMLElement | null {
	const art = card.querySelector('.sx-art');
	if (!art) return null;
	const el = make('sx-flyart', b);
	const copy = art.cloneNode(true) as HTMLElement;
	// Keep the hover scale it has right now, so the picture does not jump.
	const now = getComputedStyle(art).transform;
	if (now && now !== 'none') copy.style.transform = now;
	copy.style.transition = 'none';
	el.appendChild(copy);
	return el;
}

/** clear ends any flight at once and puts every card back as it was. */
export function clearFlight() {
	if (!flight) return;
	for (const a of flight.anims) a.cancel();
	flight.ghost.remove();
	flight.art?.remove();
	if (flight.hidden) flight.hidden.style.visibility = '';
	flight = null;
}

function others(card: HTMLElement): HTMLElement[] {
	return [...document.querySelectorAll<HTMLElement>('.sx-card .sx-face, .sx-head, .sx-attention')].filter((el) => !card.contains(el));
}

/** zoomOpen grows the card into the page. Returns false where it does not zoom (phones, less motion). */
export function zoomOpen(card: HTMLElement, group: string): boolean {
	clearFlight();
	if (reduced() || phone()) return false;
	const face = (card.querySelector('.sx-face') as HTMLElement | null) ?? card;
	const from = boxOf(face);
	const to = stage();
	const ghost = make('sx-ghost', to);
	const art = artClone(card, from);
	const g = ghost.animate([{ transform: flip(from, to), borderRadius: '16px' }, { transform: 'none', borderRadius: '0px' }], { duration: MS.zoom, easing: EASE, fill: 'both' });
	const anims: Animation[] = [g];
	if (art) anims.push(art.animate([{ transform: 'none', opacity: 1 }, { transform: shift(to, from), opacity: 0 }], { duration: MS.zoom, easing: EASE, fill: 'both' }));
	for (const el of others(card)) anims.push(el.animate([{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.97)' }], { duration: MS.recede, easing: EASE, fill: 'both' }));
	face.style.visibility = 'hidden';
	const f: Flight = { group, dir: 'open', ghost, art, anims, grown: false, arrived: false, hidden: face };
	flight = f;
	g.finished.then(() => {
		if (flight !== f) return;
		f.grown = true;
		if (f.arrived) settle(f);
	}, () => {});
	return true;
}

/** arrive is called by the page once it is on screen: the grown card clears to show it. */
export function arrive() {
	const f = flight;
	if (!f || f.dir !== 'open') return;
	f.arrived = true;
	f.hidden = null; // that card is gone with the Settings home
	if (f.grown) settle(f);
}

function settle(f: Flight) {
	const fade = f.ghost.animate([{ opacity: 1 }, { opacity: 0 }], { duration: MS.reveal, easing: EASE, fill: 'both' });
	f.anims.push(fade);
	fade.finished.then(() => {
		if (flight === f) clearFlight();
	}, () => {});
}

/**
 * zoomBack shrinks the page into its card on the Settings home. Mid-flight
 * (Back during the open) it turns the running zoom around from where it is.
 */
export function zoomBack(card: HTMLElement, group: string): void {
	const face = (card.querySelector('.sx-face') as HTMLElement | null) ?? card;
	const rest = others(card);
	if (reduced() || phone()) {
		clearFlight();
		return;
	}
	const intro = rest.map((el) => el.animate([{ opacity: 0, transform: 'scale(.97)' }, { opacity: 1, transform: 'none' }], { duration: MS.recede + 120, easing: EASE, fill: 'backwards' }));
	const running = flight && flight.dir === 'open' && flight.group === group && !flight.grown ? flight : null;
	if (running) {
		// Turn the same flight around: the ghost shrinks back from where it is now.
		for (const a of running.anims.slice(running.art ? 2 : 1)) a.cancel();
		running.anims = running.anims.slice(0, running.art ? 2 : 1);
		running.dir = 'back';
		running.hidden = face;
		face.style.visibility = 'hidden';
		for (const a of running.anims) a.reverse();
		running.anims.push(...intro);
		running.anims[0].finished.then(() => {
			if (flight === running) clearFlight();
		}, () => {});
		return;
	}
	clearFlight();
	const to = boxOf(face);
	const from = stage();
	const ghost = make('sx-ghost', from);
	const art = artClone(card, to);
	const g = ghost.animate([{ transform: 'none', borderRadius: '0px' }, { transform: flip(to, from), borderRadius: '16px' }], { duration: MS.zoomBack, easing: EASE, fill: 'both' });
	const anims: Animation[] = [g];
	if (art) anims.push(art.animate([{ transform: shift(from, to), opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: MS.zoomBack, easing: EASE, fill: 'both' }));
	anims.push(...intro);
	face.style.visibility = 'hidden';
	const f: Flight = { group, dir: 'back', ghost, art, anims, grown: true, arrived: true, hidden: face };
	flight = f;
	g.finished.then(() => {
		if (flight === f) clearFlight();
	}, () => {});
}

/** push brings a screen in on a phone: a fade with a slight settle (never past the screen edge). */
export function push(el: HTMLElement, back: boolean): Animation | null {
	if (reduced() || !phone()) return null;
	return back
		? el.animate([{ transform: 'scale(0.985)', opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: MS.pop, easing: EASE })
		: // A fade with a slight settle: nothing ever sits past the screen edge, even mid-motion.
		  el.animate([{ transform: 'scale(0.985)', opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: MS.push, easing: EASE });
}

/** swap is the quiet change between two pages of one group. */
export function swap(el: HTMLElement): Animation | null {
	if (reduced()) return null;
	return el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: MS.swap, easing: EASE });
}
