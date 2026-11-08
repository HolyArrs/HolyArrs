// The card-to-page zoom (tablet and desktop). One flight at a time; a new move, Back mid-way or a second click turns the current one around or clears it, so nothing is ever left half-drawn. Transform and opacity only.

import { EASE, MS, flip, phone, radiusFor, reduced, shift, type Box, type Move } from '#lib/core/motion.ts';

interface Flight {
	group: string;
	dir: 'open' | 'back' | 'cover';
	ghost: HTMLElement;
	art: HTMLElement | null;
	anims: Animation[];
	grown: boolean;
	arrived: boolean;
	hidden: HTMLElement | null;
	/** Resolves once the sheet fills the page (open) or covers it (Back). */
	landed: Promise<boolean>;
}

/** What the next Settings screen should play, set before it mounts. */
export const settingsNav: { move: Move; group: string | null } = { move: null, group: null };

let flight: Flight | null = null;

const boxOf = (el: Element): Box => {
	const r = el.getBoundingClientRect();
	return { left: r.left, top: r.top, width: r.width, height: r.height };
};

/** Ends any flight at once and puts every card back as it was. */
export function clearFlight() {
	if (!flight) return;
	for (const a of flight.anims) a.cancel();
	flight.ghost.remove();
	flight.art?.remove();
	if (flight.hidden) flight.hidden.style.visibility = '';
	flight = null;
}

const done = (a: Animation) => a.finished.then(() => true, () => false);

/** Grows the card into an opaque sheet the size of the page and resolves true once it has landed, so the caller can then load the page under it. Resolves false if the flight was turned around or replaced. Returns null where it does not zoom (phones, less motion). */
export function zoomOpen(card: HTMLElement, group: string): Promise<boolean> | null {
	clearFlight();
	if (reduced() || phone()) return null;
	const face = (card.querySelector('.sx-face') as HTMLElement | null) ?? card;
	const from = boxOf(face);
	const to = stage();
	const ghost = sheet(to, radiusFor(from, to));
	const art = artClone(card, from);
	// Transform and opacity only: the compositor plays every frame even while the main thread is busy.
	const g = ghost.animate([{ transform: flip(from, to) }, { transform: 'none' }], { duration: MS.zoom, easing: EASE, fill: 'both' });
	const anims: Animation[] = [g];
	if (art) anims.push(art.animate([{ transform: 'none', opacity: 1 }, { transform: shift(to, from), opacity: 0, offset: 0.55 }, { transform: shift(to, from), opacity: 0 }], { duration: MS.zoom, easing: EASE, fill: 'both' }));
	for (const el of others(card)) anims.push(el.animate([{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.97)' }], { duration: MS.recede, easing: EASE, fill: 'both' }));
	face.style.visibility = 'hidden';
	const f: Flight = { group, dir: 'open', ghost, art, anims, grown: false, arrived: false, hidden: face, landed: Promise.resolve(false) };
	f.landed = done(g).then((ok) => {
		if (!ok || flight !== f || f.dir !== 'open') return false;
		f.grown = true;
		f.art?.remove();
		f.art = null;
		return true;
	});
	flight = f;
	return f.landed;
}

/** Turns a zoom that has not landed back into its card (Escape, or a second click elsewhere). Returns true if there was one to turn. */
export function cancelOpen(): boolean {
	const f = flight;
	if (!f || f.dir !== 'open' || f.grown) return false;
	f.dir = 'back';
	for (const a of f.anims) a.reverse();
	void done(f.anims[0]).then(() => {
		if (flight === f) clearFlight();
	});
	return true;
}

/** Called by the page once it is on screen: the sheet clears to show it. */
export function arrive() {
	const f = flight;
	if (!f || f.dir !== 'open') return;
	f.arrived = true;
	f.hidden = null; // That card is gone with the Settings home.
	// Wait for the page's first paint, so the fade never shares a frame with mounting.
	requestAnimationFrame(() => requestAnimationFrame(() => settle(f)));
}

/** Fades a blank sheet over the open page before Back swaps it for the Settings home, so the home mounts unseen and the sheet then shrinks into its card. Resolves when covered (at once where it does not zoom). */
export function coverPage(group: string): Promise<void> {
	clearFlight();
	if (reduced() || phone()) return Promise.resolve();
	const b = stage();
	const ghost = sheet(b, 0);
	const c = ghost.animate([{ opacity: 0 }, { opacity: 1 }], { duration: MS.cover, easing: EASE, fill: 'both' });
	const f: Flight = { group, dir: 'cover', ghost, art: null, anims: [c], grown: true, arrived: true, hidden: null, landed: Promise.resolve(false) };
	flight = f;
	f.landed = done(c);
	// Never hold the page change longer than the fade, even in a background tab.
	return Promise.race([f.landed, new Promise<boolean>((r) => setTimeout(() => r(true), MS.cover + 60))]).then(() => {});
}

/** Shrinks the page's sheet into its card on the Settings home. */
export function zoomBack(card: HTMLElement, group: string): void {
	const face = (card.querySelector('.sx-face') as HTMLElement | null) ?? card;
	const rest = others(card);
	if (reduced() || phone()) {
		clearFlight();
		return;
	}
	const cover = flight && flight.dir === 'cover' && flight.group === group ? flight : null;
	const to = boxOf(face);
	const from = stage();
	let ghost: HTMLElement;
	if (cover) {
		for (const a of cover.anims) a.cancel();
		ghost = cover.ghost;
		ghost.style.opacity = '1';
		place(ghost, from);
		flight = null;
	} else {
		clearFlight();
		ghost = sheet(from, 0);
	}
	ghost.style.borderRadius = `${radiusFor(to, from)}px`;
	const intro = rest.map((el) => el.animate([{ opacity: 0, transform: 'scale(.97)' }, { opacity: 1, transform: 'none' }], { duration: MS.recede + 120, delay: 60, easing: EASE, fill: 'backwards' }));
	const g = ghost.animate([{ transform: 'none', opacity: 1 }, { transform: flip(to, from), opacity: 1, offset: 0.85 }, { transform: flip(to, from), opacity: 0 }], { duration: MS.zoomBack, easing: EASE, fill: 'both' });
	// The sheet lands on the card and fades, uncovering the real card under it.
	const art = null;
	const anims: Animation[] = [g];
	anims.push(...intro);
	const f: Flight = { group, dir: 'back', ghost, art, anims, grown: true, arrived: true, hidden: null, landed: Promise.resolve(true) };
	flight = f;
	void done(g).then(() => {
		if (flight === f) clearFlight();
	});
}

/** Brings a screen in on a phone: a fade with a slight settle (never past the screen edge). */
export function push(el: HTMLElement, back: boolean): Animation | null {
	if (reduced() || !phone()) return null;
	return back
		? el.animate([{ transform: 'scale(0.985)', opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: MS.pop, easing: EASE })
		:
		  el.animate([{ transform: 'scale(0.985)', opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: MS.push, easing: EASE });
}

/** The quiet change between two pages of one group. */
export function swap(el: HTMLElement): Animation | null {
	if (reduced()) return null;
	return el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: MS.swap, easing: EASE });
}

/** The content column below the top bar, inside the screen. */
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

function others(card: HTMLElement): HTMLElement[] {
	return [...document.querySelectorAll<HTMLElement>('.sx-card .sx-face, .sx-head, .sx-attention')].filter((el) => !card.contains(el));
}

function sheet(b: Box, radius: number): HTMLElement {
	const el = make('sx-ghost', b);
	el.style.borderRadius = `${radius}px`;
	return el;
}

function settle(f: Flight) {
	if (flight !== f) return;
	const fade = f.ghost.animate([{ opacity: 1 }, { opacity: 0 }], { duration: MS.reveal, easing: EASE, fill: 'both' });
	f.anims.push(fade);
	void done(fade).then(() => {
		if (flight === f) clearFlight();
	});
}
