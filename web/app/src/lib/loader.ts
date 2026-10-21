// The Orbit loading screen that app.html draws before
// any script runs. This module only hides it, replays it after sign-in and
// shows a small one over slow in-app page changes.

/** Shortest time the full screen stays up on a page load or sign-in. */
export const HOLD_MS = 2200;
/** In-app page changes faster than this never show the loader. */
export const NAV_DELAY_MS = 300;
/** After the setup wizard: a shorter hold before Home. */
export const SETUP_HOLD_MS = 1800;

const LAST_USER = 'holyarrs-last-user';

// Sign-in hand-off (one continuous move, about 1.2–1.6 s): the Orbit grows out
// of the Sign in button and glides to the loader's centre while the panel and
// art dissolve under a fading app-colour veil; the app then opens below it.
/** The ring's glide from the button to the centre. */
export const GLIDE_MS = 560;
/** The veil and the dissolve: the app opens under the Orbit after this. */
export const VEIL_MS = 560;
/** Shortest time the Orbit rests at the centre once the app is opening, so it never blinks. */
export const SIGNIN_HOLD_MS = 520;
/** The Orbit's leave and the page's rise. */
export const ARRIVE_MS = 420;
const RING = 104;

export interface Rect { left: number; top: number; width: number; height: number }

/** FLIP start for the ring: from the centre of `from`, at its height, to the viewport centre at full size. */
export function handoffFrom(from: Rect, vw: number, vh: number, ring = RING): { x: number; y: number; s: number } {
	const s = Math.max(0.2, Math.min(1, from.height / ring));
	return { x: from.left + from.width / 2 - vw / 2, y: from.top + from.height / 2 - vh / 2, s };
}

/** Total sign-in sequence for an app that is ready after `readyMs` (from the press). */
export function handoffTotal(readyMs: number, reduced = false): number {
	if (reduced) return 400;
	return Math.max(VEIL_MS, readyMs) + SIGNIN_HOLD_MS + ARRIVE_MS;
}

/** A sign-in by someone other than the last person on this device. */
export function isUserSwitch(last: string | null, name: string): boolean {
	return !!last && !!name && last !== name;
}

/** How long to keep the screen up: what is left of the hold, never below 0. */
export function remaining(shownAt: number, now: number, hold = HOLD_MS): number {
	return Math.max(0, shownAt + hold - now);
}

/**
 * bootHold is the page-load hold: the full ~2.2 s only for a full refresh
 * (reload) or the sign-in page; any other load (a link to a title, an
 * in-app address opened fresh) lets the screen go on the first content paint.
 */
export function bootHold(navType: string | undefined, path: string): number {
	return navType === 'reload' || /\/login\/?$/.test(path) ? HOLD_MS : 0;
}

function pageHold(): number {
	if (typeof performance === 'undefined' || typeof location === 'undefined') return HOLD_MS;
	const nav = performance.getEntriesByType?.('navigation')[0] as PerformanceNavigationTiming | undefined;
	return bootHold(nav?.type, location.pathname);
}

const el = () => (typeof document === 'undefined' ? null : document.getElementById('hl'));
let shownAt = 0; // page load: time zero of performance.now()
let hold = pageHold();
let reconnecting = false;
let full = true; // the full screen is up (page load or sign-in)
let timer: ReturnType<typeof setTimeout> | undefined;
let navTimer: ReturnType<typeof setTimeout> | undefined;

function leave(node: HTMLElement): void {
	const handoff = node.classList.contains('handoff');
	node.classList.add('out');
	if (handoff) arrive();
	timer = setTimeout(() => {
		node.hidden = true;
		node.classList.remove('out', 'nav', 'handoff', 'veil');
		say('');
		full = false;
		settle();
	}, 280);
}

// While a sign-in hand-off plays, the first page waits with its heavy work.
let handoffWait: Promise<void> | null = null;
let settle: () => void = () => {};

/** Resolves once the sign-in hand-off has fully left (at once when none is playing). */
export function afterHandoff(): Promise<void> {
	return handoffWait ?? Promise.resolve();
}

/** The app is ready: hide the full screen once its hold has passed. */
export function loaderReady(): void {
	const node = el();
	if (!node || node.hidden || !full) return;
	clearTimeout(timer);
	timer = setTimeout(() => leave(node), remaining(shownAt, performance.now(), hold));
}

/** Replay the full screen (right after signing in); call loaderReady when the app is there. */
export function playLoader(line = '', holdMs = HOLD_MS): void {
	const node = el();
	if (!node) return;
	say(line);
	if (full && !node.hidden) return;
	hold = holdMs;
	clearTimeout(timer);
	clearTimeout(navTimer);
	node.classList.remove('out', 'nav', 'handoff', 'veil');
	node.hidden = false;
	full = true;
	shownAt = performance.now();
}

function say(text: string): void {
	const t = typeof document === 'undefined' ? null : document.getElementById('hlt');
	if (t) t.textContent = text; // plain text only
}

/** The first page fades and rises in as the Orbit leaves (CSS in app.html). */
function arrive(): void {
	const root = document.documentElement;
	if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
	root.classList.add('hl-arrive');
	setTimeout(() => root.classList.remove('hl-arrive'), ARRIVE_MS + 80);
}

/**
 * Start the sign-in hand-off: the Orbit grows from `from` (the Sign in
 * button) and glides to the centre over a transparent veil that fades to
 * the app colour. The same #hl element then serves as the boot loader.
 */
export function playHandoff(from: Rect | null, name: string): void {
	const node = el();
	if (!node) return;
	const last = rememberUser(name);
	const ring = node.querySelector<HTMLElement>('.hw');
	clearTimeout(timer);
	clearTimeout(navTimer);
	node.classList.remove('out', 'nav');
	node.classList.add('handoff');
	node.hidden = false;
	if (!handoffWait)
		handoffWait = new Promise<void>((done) => {
			settle = () => {
				handoffWait = null;
				settle = () => {};
				done();
			};
		});
	full = true;
	hold = SIGNIN_HOLD_MS;
	shownAt = performance.now() + VEIL_MS;
	say(isUserSwitch(last, name) ? `Welcome, ${name}` : '');
	if (!ring || !from) return;
	const f = handoffFrom(from, innerWidth, innerHeight);
	ring.style.transition = 'none';
	ring.style.transform = `translate(${f.x}px, ${f.y}px) scale(${f.s})`;
	ring.style.opacity = '0';
	void ring.offsetWidth;
	// Two frames: the start state must paint once, or nothing transitions out of display:none.
	requestAnimationFrame(() => requestAnimationFrame(() => {
		ring.style.transition = '';
		ring.style.transform = '';
		ring.style.opacity = '';
		node.classList.add('veil');
	}));
}

function rememberUser(name: string): string | null {
	let last: string | null = null;
	try {
		last = localStorage.getItem(LAST_USER);
		localStorage.setItem(LAST_USER, name);
	} catch {
		/* private window */
	}
	return last;
}

/** Play after a sign-in; says welcome when a different person signs in on this device. */
export function playSignIn(name: string, holdMs = HOLD_MS): void {
	const last = rememberUser(name);
	playLoader(isUserSwitch(last, name) ? `Welcome, ${name}` : '', holdMs);
}

/** The server stopped answering: cover the app until it is back. */
export function showReconnecting(): void {
	const node = el();
	if (!node || reconnecting) return;
	reconnecting = true;
	clearTimeout(timer);
	clearTimeout(navTimer);
	node.classList.remove('out', 'nav');
	node.hidden = false;
	full = true;
	hold = 0;
	say('Reconnecting…');
}

/** The server answers again. */
export function hideReconnecting(): void {
	if (!reconnecting) return;
	reconnecting = false;
	const node = el();
	if (node && !node.hidden) leave(node);
}

/** Wait before retry number n (0-based): 1 s, 2 s, 4 s… at most 15 s. */
export function backoff(n: number): number {
	return Math.min(15000, 1000 * 2 ** n);
}

/** An in-app page change began: show the small loader only if it is slow. */
export function navStarted(): void {
	clearTimeout(navTimer);
	if (full) return;
	navTimer = setTimeout(() => {
		const node = el();
		if (!node || full) return;
		clearTimeout(timer);
		node.classList.remove('out');
		node.classList.add('nav');
		node.hidden = false;
	}, NAV_DELAY_MS);
}

/** The page change finished (or failed): drop the small loader straight away. */
export function navEnded(): void {
	clearTimeout(navTimer);
	const node = el();
	if (!node || full || node.hidden) return;
	clearTimeout(timer);
	leave(node);
}
