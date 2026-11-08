
// Short pop-up messages plus a kept history for the alerts panel.
export type ToastTone = 'ok' | 'fail' | 'info' | 'warn';
export interface Toast {
	id: number;
	text: string;
	body?: string;
	tone: ToastTone;
	action?: ToastAction;
	seen: number;
}
export interface ToastAction {
	label: string;
	run: () => void;
}
export interface PastAlert {
	id: number;
	text: string;
	body?: string;
	tone: ToastTone;
	at: number;
	action?: ToastAction;
	until?: number;
	fresh: boolean;
}

// At most two on screen, six waiting behind them, fifty kept in the panel.
const SHOW = 2;
const WAIT = 6;
const KEEP = 50;

export function createToasts(ms = 6000) {
	const list = $state<Toast[]>([]);
	const past = $state<PastAlert[]>([]);
	const counts = $state({ waiting: 0 });
	// Count of toasts pushed out of view, shown with the waiting ones.
	let bumped = 0;
	const recount = () => (counts.waiting = waiting.length + bumped);
	const waiting: (Toast & { life: number })[] = [];
	const timers = new Map<number, { t?: ReturnType<typeof setTimeout>; left: number; at: number }>();
	let next = 1;

	// Failures stay longer, scaled by length so the text can be read.
	const life = (tone: ToastTone, text: string, own?: number) =>
		own ?? (tone === 'fail' ? Math.max(8000, 3000 + text.length * 60) : ms);

	function arm(id: number, left: number) {
		const old = timers.get(id);
		if (old?.t) clearTimeout(old.t);
		timers.set(id, { left, at: Date.now(), t: setTimeout(() => dismiss(id), left) });
	}

	function show(t: Toast, left: number) {
		list.push(t);
		arm(t.id, left);
	}

	// Closing one lets the next waiting toast take its place.
	function dismiss(id: number) {
		const timer = timers.get(id);
		if (timer?.t) clearTimeout(timer.t);
		timers.delete(id);
		const w = waiting.findIndex((t) => t.id === id);
		if (w >= 0) waiting.splice(w, 1);
		const i = list.findIndex((t) => t.id === id);
		if (i >= 0) {
			list.splice(i, 1);
			const up = waiting.shift();
			if (up) {
				const { life: left, ...t } = up;
				show(t, left);
			}
		}
		if (!list.length) bumped = 0;
		recount();
	}

	function remember(t: Toast, left: number) {
		const now = Date.now();
		past.unshift({ id: t.id, text: t.text, body: t.body, tone: t.tone, at: now, action: t.action, until: t.action ? now + left : undefined, fresh: true });
		if (past.length > KEEP) past.length = KEEP;
	}

	function push(text: string, tone: ToastTone = 'info', opts: { action?: ToastAction; ms?: number; body?: string } = {}): number {
		const left = life(tone, text + (opts.body ?? ''), opts.ms);
		const alike = (t: { text: string; body?: string; tone: ToastTone; action?: ToastAction }) => t.text === text && t.body === opts.body && t.tone === tone && !t.action && !opts.action;
		// Repeats of a toast on screen only bump its count and restart its timer.
		const same = list.find(alike);
		if (same) {
			same.seen++;
			const timer = timers.get(same.id);
			if (timer && !timer.t) timers.set(same.id, { left, at: Date.now() });
			else arm(same.id, left);
			const row = past.find((p) => p.id === same.id);
			if (row) row.at = Date.now();
			return same.id;
		}
		const queued = waiting.find(alike);
		if (queued) return queued.id;

		const t: Toast = { id: next++, text, body: opts.body, tone, action: opts.action, seen: 0 };
		remember(t, left);
		if (list.length >= SHOW) {
			// When full, drop the oldest plain one; toasts with an action are kept.
			const old = list.find((x) => !x.action);
			if (old) {
				const timer = timers.get(old.id);
				if (timer?.t) clearTimeout(timer.t);
				timers.delete(old.id);
				list.splice(list.indexOf(old), 1);
				bumped++;
			}
		}
		if (list.length < SHOW) show(t, left);
		else {
			waiting.push({ ...t, life: left });
			while (waiting.length > WAIT) {
				const plain = waiting.findIndex((x) => !x.action);
				waiting.splice(plain >= 0 ? plain : 0, 1);
			}
		}
		recount();
		return t.id;
	}

	// Hovering holds the timer, but never leaves less than a moment to react.
	function pause(id: number) {
		const timer = timers.get(id);
		if (!timer?.t) return;
		clearTimeout(timer.t);
		timers.set(id, { left: Math.max(1500, timer.left - (Date.now() - timer.at)), at: Date.now() });
	}
	function resume(id: number) {
		const timer = timers.get(id);
		if (timer && !timer.t) arm(id, timer.left);
	}

	function seeAll() {
		for (const t of [...waiting]) dismiss(t.id);
		waiting.length = 0;
		bumped = 0;
		recount();
		for (const p of past) p.fresh = false;
	}
	function forget(id: number) {
		const i = past.findIndex((p) => p.id === id);
		if (i >= 0) past.splice(i, 1);
	}
	function forgetAll() {
		past.length = 0;
	}
	// An action can run once, and only before it expires.
	function runPast(id: number) {
		const p = past.find((x) => x.id === id);
		if (!p?.action || (p.until && Date.now() > p.until)) return;
		const act = p.action;
		p.action = undefined;
		dismiss(id);
		act.run();
	}
	// History only: the entry is stored but nothing pops up.
	function record(text: string, tone: ToastTone = 'info', opts: { body?: string } = {}): number {
		const t: Toast = { id: next++, text, body: opts.body, tone, seen: 0 };
		remember(t, 0);
		return t.id;
	}
	const unseen = () => past.filter((p) => p.fresh).length;

	return { list, past, counts, push, record, dismiss, pause, resume, seeAll, forget, forgetAll, runPast, unseen };
}

export const toasts = createToasts();

export const alertsPanel = $state({ open: false });
