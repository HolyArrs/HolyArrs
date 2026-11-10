// Full-page loader, sign-in handoff and reconnect veil driven from one DOM node.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
export interface Rect { left: number; top: number; width: number; height: number }

// How long the full loader stays up, and the wait before the thin nav bar shows.
export const HOLD_MS = 2200;
export const NAV_DELAY_MS = 300;
export const SETUP_HOLD_MS = 1800;

// Remembers the last user so a different one gets a welcome line.
const LAST_USER = 'holyarrs-last-user';

export const VEIL_MS = 560;
export const SIGNIN_HOLD_MS = 520;
export const ARRIVE_MS = 420;
const RING = 104;

// Loader state lives at module level because one loader node serves the whole page.
const el = () => (typeof document === 'undefined' ? null : document.getElementById('hl'));
let shownAt = 0;
let hold = pageHold();
let reconnecting = false;
let full = !el()?.hidden;
let timer: ReturnType<typeof setTimeout> | undefined;
let navTimer: ReturnType<typeof setTimeout> | undefined;

let handoffWait: Promise<void> | null = null;
let settle: () => void = () => {};

// Offset and scale that put the loader ring over the sign-in avatar before it glides to centre.
export function handoffFrom(from: Rect, vw: number, vh: number, ring = RING): { x: number; y: number; s: number } {
	const s = Math.max(0.2, Math.min(1, from.height / ring));
	return { x: from.left + from.width / 2 - vw / 2, y: from.top + from.height / 2 - vh / 2, s };
}

export function isUserSwitch(last: string | null, name: string): boolean {
	return !!last && !!name && last !== name;
}

// Time left to keep the loader up so it never flashes by.
export function remaining(shownAt: number, now: number, hold = HOLD_MS): number {
	return Math.max(0, shownAt + hold - now);
}

// Only a reload replays the full loader, and never on the login page.
export function bootHold(navType: string | undefined, path: string): number {
	if (/\/login\/?$/.test(path)) return 0;
	return navType === 'reload' ? HOLD_MS : 0;
}

// Resolves once the handoff has fully left the screen.
export function afterHandoff(): Promise<void> {
	return handoffWait ?? Promise.resolve();
}

export function loaderReady(): void {
	const node = el();
	if (!node || node.hidden || !full) return;
	clearTimeout(timer);
	timer = setTimeout(() => leave(node), remaining(shownAt, performance.now(), hold));
}

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

// Starts the sign-in handoff and keeps the page waiting until it finishes.
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
	say(isUserSwitch(last, name) ? t('core.loader.welcome', { name }) : '');
	if (!ring || !from) return;
	const f = handoffFrom(from, innerWidth, innerHeight);
	ring.style.transition = 'none';
	ring.style.transform = `translate(${f.x}px, ${f.y}px) scale(${f.s})`;
	ring.style.opacity = '0';
	void ring.offsetWidth;
	requestAnimationFrame(() => requestAnimationFrame(() => {
		ring.style.transition = '';
		ring.style.transform = '';
		ring.style.opacity = '';
		node.classList.add('veil');
	}));
}

export function playSignIn(name: string, holdMs = HOLD_MS): void {
	const last = rememberUser(name);
	playLoader(isUserSwitch(last, name) ? t('core.loader.welcome', { name }) : '', holdMs);
}

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
	say(t('core.loader.reconnecting'));
}

export function hideReconnecting(): void {
	if (!reconnecting) return;
	reconnecting = false;
	const node = el();
	if (node && !node.hidden) leave(node);
}

// Retry wait that doubles each try up to 15 seconds.
export function backoff(n: number): number {
	return Math.min(15000, 1000 * 2 ** n);
}

// Shows the nav bar only if the page stays busy past the delay, so quick moves show nothing.
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

export function navEnded(): void {
	clearTimeout(navTimer);
	const node = el();
	if (!node || full || node.hidden) return;
	clearTimeout(timer);
	leave(node);
}

// Hold is zero on first loads so only reloads replay the full loader.
function pageHold(): number {
	if (typeof performance === 'undefined' || typeof location === 'undefined') return HOLD_MS;
	const nav = performance.getEntriesByType?.('navigation')[0] as PerformanceNavigationTiming | undefined;
	return bootHold(nav?.type, location.pathname);
}

// The 280 ms matches the fade-out in the loader styles.
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

function say(text: string): void {
	const t = typeof document === 'undefined' ? null : document.getElementById('hlt');
	if (t) t.textContent = text;
}

// Adds a brief class so the page eases in after a handoff.
function arrive(): void {
	const root = document.documentElement;
	if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
	root.classList.add('hl-arrive');
	setTimeout(() => root.classList.remove('hl-arrive'), ARRIVE_MS + 80);
}

// Storage can be blocked in private windows, so failures are ignored.
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
