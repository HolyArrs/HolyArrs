// Approve all (people who approve): every waiting request the filters show, also past the loaded cards, behind one confirm sheet that says how many.
import { api, message } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import type { Ask } from '#lib/core/types.ts';
import { allWaiting, type ListPage } from './ask-pages.ts';

export function createApproveAll(ctx: { capped: () => boolean; approve: (list: Ask[]) => void }) {
	let open = $state(false);
	let list = $state<Ask[]>([]);
	let busy = $state(false);

	/** shown: the waiting cards on screen; match: the page's filters. A cut list reads every waiting request first, so the count is the real one. */
	async function ask(shown: Ask[], match: (a: Ask) => boolean) {
		if (busy) return;
		if (!ctx.capped()) {
			list = shown;
			open = true;
			return;
		}
		busy = true;
		try {
			list = await allWaiting((p) => api.get<ListPage>(p), match);
			open = true;
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = false;
		}
	}

	function confirm() {
		open = false;
		ctx.approve(list);
	}

	return {
		ask,
		confirm,
		get open() { return open; },
		set open(v) { open = v; },
		get n() { return list.length; },
		get busy() { return busy; }
	};
}
