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

/** A sign-in by someone other than the last person on this device. */
export function isUserSwitch(last: string | null, name: string): boolean {
	return !!last && !!name && last !== name;
}

/** How long to keep the screen up: what is left of the hold, never below 0. */
export function remaining(shownAt: number, now: number, hold = HOLD_MS): number {
	return Math.max(0, shownAt + hold - now);
}

const el = () => (typeof document === 'undefined' ? null : document.getElementById('hl'));
let shownAt = 0; // page load: time zero of performance.now()
let hold = HOLD_MS;
let reconnecting = false;
let full = true; // the full screen is up (page load or sign-in)
let timer: ReturnType<typeof setTimeout> | undefined;
let navTimer: ReturnType<typeof setTimeout> | undefined;

function leave(node: HTMLElement): void {
	node.classList.add('out');
	timer = setTimeout(() => {
		node.hidden = true;
		node.classList.remove('out', 'nav');
		say('');
		full = false;
	}, 280);
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
	node.classList.remove('out', 'nav');
	node.hidden = false;
	full = true;
	shownAt = performance.now();
}

function say(text: string): void {
	const t = typeof document === 'undefined' ? null : document.getElementById('hlt');
	if (t) t.textContent = text; // plain text only
}

/** Play after a sign-in; says welcome when a different person signs in on this device. */
export function playSignIn(name: string): void {
	let last: string | null = null;
	try {
		last = localStorage.getItem(LAST_USER);
		localStorage.setItem(LAST_USER, name);
	} catch {
		/* private window */
	}
	playLoader(isUserSwitch(last, name) ? `Welcome, ${name}` : '');
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
