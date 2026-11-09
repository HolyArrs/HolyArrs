// Taking a request back: the confirm dialog and what follows.
import { api, message } from '#lib/core/api.ts';
import type { Ask } from '#lib/core/types.ts';

export function createTakeBack(ctx: { takingBack: Set<number>; shown: Ask[]; load: (quiet?: boolean) => Promise<void> }) {
	let takeBackOpen = $state(false);
	let takeBackAsk = $state<Ask | null>(null);
	let takeBackError = $state('');
	// Where focus goes once the sheet has closed after a Take back.
	let focusAfterTakeBack: (() => void) | null = null;
	function askTakeBack(a: Ask) {
		takeBackAsk = a;
		takeBackError = '';
		takeBackOpen = true;
	}

	// The sheet stays open, busy, while the ask is taken back; a failure shows in it.
	async function takeBack(a: Ask) {
		if (ctx.takingBack.has(a.id)) return;
		takeBackError = '';
		const at = ctx.shown.findIndex((x) => x.id === a.id);
		ctx.takingBack = new Set([...ctx.takingBack, a.id]);
		try {
			await api.del(`/asks/${a.id}`);
			await ctx.load();
			focusAfterTakeBack = () => {
				const rows = document.querySelectorAll<HTMLElement>('[data-testid="ask"]');
				const row = rows.length ? rows[Math.min(Math.max(at, 0), rows.length - 1)] : undefined;
				const heading = document.querySelector<HTMLElement>('main h1');
				const target = row?.querySelector<HTMLElement>('button, a') ?? heading;
				if (target === heading) heading?.setAttribute('tabindex', '-1');
				target?.focus();
			};
			takeBackOpen = false;
		} catch (err) {
			takeBackError = message(err);
		} finally {
			ctx.takingBack = new Set([...ctx.takingBack].filter((id) => id !== a.id));
		}
	}

	function takeBackClosed() {
		const f = focusAfterTakeBack;
		focusAfterTakeBack = null;
		// After the browser has put focus back on the (now gone) opener.
		if (f) setTimeout(f, 0);
	}

	// Requests news: no inbox card. Unread news about an ask marks its card New for this visit; other news (a fixed report, new albums, a followed title ready) shows once as a toast. Opening the page reads it all, so the badge clears.

	return {
		askTakeBack,
		get takeBackOpen() { return takeBackOpen; },
		set takeBackOpen(v) { takeBackOpen = v; },
		takeBackClosed,
		get takeBackAsk() { return takeBackAsk; },
		set takeBackAsk(v) { takeBackAsk = v; },
		get takeBackError() { return takeBackError; },
		set takeBackError(v) { takeBackError = v; },
		takeBack,
	};
}
