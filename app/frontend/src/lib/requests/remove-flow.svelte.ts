// Removing requests (people who approve): one request, or all the failed or all the finished ones, behind one confirm sheet.
import { api, message } from '#lib/core/api.ts';
import type { Ask } from '#lib/core/types.ts';

export type RemoveWhat = { one: Ask } | { all: 'failed' | 'finished'; n: number };

/** The call that removes it: DELETE for one, the bulk route for all failed or finished. */
export async function removeRequests(what: RemoveWhat): Promise<void> {
	if ('one' in what) await api.del(`/asks/${what.one.id}`);
	else await api.post('/asks/remove', { filter: what.all });
}

export function createRemoveFlow(ctx: { load: (quiet?: boolean) => Promise<void> }) {
	let open = $state(false);
	let what = $state<RemoveWhat | null>(null);
	let error = $state('');
	let busy = $state(false);

	function ask(w: RemoveWhat) {
		what = w;
		error = '';
		open = true;
	}

	// The sheet stays open, busy, while it goes; a failure shows in it.
	async function confirm() {
		if (!what || busy) return;
		busy = true;
		error = '';
		try {
			await removeRequests(what);
			await ctx.load();
			open = false;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}

	return {
		ask,
		confirm,
		get open() { return open; },
		set open(v) { open = v; },
		get what() { return what; },
		get error() { return error; },
		get busy() { return busy; }
	};
}
