// Small notifications at the top of the screen (Apple TV style), and the
// Recent alerts list that keeps them so they can be read again.

export type ToastTone = 'ok' | 'fail' | 'info';
export interface Toast {
	id: number;
	/** The title: the thing and what happened ("Severance deleted"). */
	text: string;
	/** Optional second line: the next step or key fact, a few words ("Files kept 7 days"). */
	body?: string;
	tone: ToastTone;
	/** An optional button, such as Undo; pressing it runs and closes the toast. */
	action?: ToastAction;
	/** Bumped when the same message is pushed again, so it can be announced again. */
	seen: number;
}
export interface ToastAction {
	label: string;
	run: () => void;
}
/** One row of Recent alerts. */
export interface PastAlert {
	id: number;
	text: string;
	body?: string;
	tone: ToastTone;
	/** When it arrived (ms). */
	at: number;
	/** The toast's button, kept exactly as long as the toast offers it (until `until`). */
	action?: ToastAction;
	until?: number;
	/** Not seen in the list yet. */
	fresh: boolean;
}

/** Shown at once; the rest wait their turn. */
const SHOW = 2;
/** Waiting at most; plain notes drop first, so an Undo is never lost to a burst (all stay in Recent alerts). */
const WAIT = 6;
/** Rows kept in Recent alerts. */
const KEEP = 50;

export function createToasts(ms = 6000) {
	const list = $state<Toast[]>([]);
	const past = $state<PastAlert[]>([]);
	// How many wait behind the two on screen (for the "+N more" chip).
	const counts = $state({ waiting: 0 });
	const waiting: (Toast & { life: number })[] = [];
	const timers = new Map<number, { t?: ReturnType<typeof setTimeout>; left: number; at: number }>();
	let next = 1;

	// Failures stay longer, and longer still when there is more to read.
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

	function dismiss(id: number) {
		const timer = timers.get(id);
		if (timer?.t) clearTimeout(timer.t);
		timers.delete(id);
		const w = waiting.findIndex((t) => t.id === id);
		if (w >= 0) waiting.splice(w, 1);
		counts.waiting = waiting.length;
		const i = list.findIndex((t) => t.id === id);
		if (i < 0) return;
		list.splice(i, 1);
		const up = waiting.shift();
		counts.waiting = waiting.length;
		if (up) {
			const { life: left, ...t } = up;
			show(t, left);
		}
	}

	function remember(t: Toast, left: number) {
		const now = Date.now();
		past.unshift({ id: t.id, text: t.text, body: t.body, tone: t.tone, at: now, action: t.action, until: t.action ? now + left : undefined, fresh: true });
		if (past.length > KEEP) past.length = KEEP;
	}

	function push(text: string, tone: ToastTone = 'info', opts: { action?: ToastAction; ms?: number; body?: string } = {}): number {
		const left = life(tone, text + (opts.body ?? ''), opts.ms);
		const alike = (t: { text: string; body?: string; tone: ToastTone; action?: ToastAction }) => t.text === text && t.body === opts.body && t.tone === tone && !t.action && !opts.action;
		// The same message again refreshes the one on screen instead of stacking.
		const same = list.find(alike);
		if (same) {
			same.seen++;
			const timer = timers.get(same.id);
			// Held under the pointer: refresh its time, keep holding.
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
		if (list.length < SHOW) show(t, left);
		else {
			waiting.push({ ...t, life: left });
			while (waiting.length > WAIT) {
				const plain = waiting.findIndex((x) => !x.action);
				waiting.splice(plain >= 0 ? plain : 0, 1);
			}
			counts.waiting = waiting.length;
		}
		return t.id;
	}

	/** Hover or focus holds a toast; leaving gives back the time it had left. */
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

	/** Opening Recent alerts: everything waiting moves into the list, and nothing is new any more. */
	function seeAll() {
		for (const t of [...waiting]) dismiss(t.id);
		waiting.length = 0;
		counts.waiting = 0;
		for (const p of past) p.fresh = false;
	}
	/** × on a row. */
	function forget(id: number) {
		const i = past.findIndex((p) => p.id === id);
		if (i >= 0) past.splice(i, 1);
	}
	/** Clear, and sign-out. */
	function forgetAll() {
		past.length = 0;
	}
	/** A row's button (Undo) while it still works; it closes the matching toast too. */
	function runPast(id: number) {
		const p = past.find((x) => x.id === id);
		if (!p?.action || (p.until && Date.now() > p.until)) return;
		const act = p.action;
		p.action = undefined;
		dismiss(id);
		act.run();
	}
	const unseen = () => past.filter((p) => p.fresh).length;

	return { list, past, counts, push, dismiss, pause, resume, seeAll, forget, forgetAll, runPast, unseen };
}

export const toasts = createToasts();

/** Whether Recent alerts is open (the bell, the "+N more" chip and the Manage sheet open it). */
export const alertsPanel = $state({ open: false });
