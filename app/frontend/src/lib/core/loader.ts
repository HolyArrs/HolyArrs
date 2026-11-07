
export const HOLD_MS = 2200;
export const NAV_DELAY_MS = 300;
export const SETUP_HOLD_MS = 1800;

const LAST_USER = 'holyarrs-last-user';

export const GLIDE_MS = 560;
export const VEIL_MS = 560;
export const SIGNIN_HOLD_MS = 520;
export const ARRIVE_MS = 420;
const RING = 104;

export interface Rect { left: number; top: number; width: number; height: number }

export function handoffFrom(from: Rect, vw: number, vh: number, ring = RING): { x: number; y: number; s: number } {
	const s = Math.max(0.2, Math.min(1, from.height / ring));
	return { x: from.left + from.width / 2 - vw / 2, y: from.top + from.height / 2 - vh / 2, s };
}

export function handoffTotal(readyMs: number, reduced = false): number {
	if (reduced) return 400;
	return Math.max(VEIL_MS, readyMs) + SIGNIN_HOLD_MS + ARRIVE_MS;
}

export function isUserSwitch(last: string | null, name: string): boolean {
	return !!last && !!name && last !== name;
}

export function remaining(shownAt: number, now: number, hold = HOLD_MS): number {
	return Math.max(0, shownAt + hold - now);
}

export function bootHold(navType: string | undefined, path: string): number {
	if (/\/login\/?$/.test(path)) return 0;
	return navType === 'reload' ? HOLD_MS : 0;
}

function pageHold(): number {
	if (typeof performance === 'undefined' || typeof location === 'undefined') return HOLD_MS;
	const nav = performance.getEntriesByType?.('navigation')[0] as PerformanceNavigationTiming | undefined;
	return bootHold(nav?.type, location.pathname);
}

const el = () => (typeof document === 'undefined' ? null : document.getElementById('hl'));
let shownAt = 0;
let hold = pageHold();
let reconnecting = false;
let full = !el()?.hidden;
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

let handoffWait: Promise<void> | null = null;
let settle: () => void = () => {};

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

function say(text: string): void {
	const t = typeof document === 'undefined' ? null : document.getElementById('hlt');
	if (t) t.textContent = text;
}

function arrive(): void {
	const root = document.documentElement;
	if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
	root.classList.add('hl-arrive');
	setTimeout(() => root.classList.remove('hl-arrive'), ARRIVE_MS + 80);
}

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

export function playSignIn(name: string, holdMs = HOLD_MS): void {
	const last = rememberUser(name);
	playLoader(isUserSwitch(last, name) ? `Welcome, ${name}` : '', holdMs);
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
	say('Reconnecting…');
}

export function hideReconnecting(): void {
	if (!reconnecting) return;
	reconnecting = false;
	const node = el();
	if (node && !node.hidden) leave(node);
}

export function backoff(n: number): number {
	return Math.min(15000, 1000 * 2 ** n);
}

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
