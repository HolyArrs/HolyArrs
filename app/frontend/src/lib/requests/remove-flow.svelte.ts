// Removing requests (people who approve): one request, or all the failed or all the finished ones, behind one confirm sheet.
import { api, message } from '#lib/core/api.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import type { Ask } from '#lib/core/types.ts';

// ids: the cards counted; byDownload: the Failed ones among them whose download failed (only the page knows that). The server keeps only those still failed or finished. shared: one of them was asked for by more than one person.
// current: the cards now shown in that state, read again when the owner confirms; filtered: a type, person or status filter is on, so only those cards go.
export type RemoveWhat = { one: Ask } | { all: 'failed' | 'finished'; n: number; ids?: number[]; byDownload?: number[]; shared?: boolean; current?: () => Ask[]; filtered?: boolean };

/** What to send for the shown cards. With no filter on, no ids: the server clears everything in that state, past the page's loaded cards. With a filter on, the shown ids. A Failed card with a title still in the Library failed by its download, which only the page knows and the server cannot re-check, so those ids are sent only for the cards passed in. */
export function clearParts(which: 'failed' | 'finished', list: Ask[], filtered: boolean): { ids?: number[]; byDownload?: number[] } {
	if (which === 'finished') return filtered ? { ids: list.map((a) => a.id) } : {};
	const dl = list.filter((a) => a.title_id && !a.problem).map((a) => a.id);
	return filtered ? { ids: list.map((a) => a.id), byDownload: dl } : { byDownload: dl };
}

/** What a Clear button counts and takes. No status picked: every request in that state (`total`, the server's count when known). A status picked: only the cards it shows, by id, like a type or person filter. */
export function clearScope<A extends Ask>(inState: A[], wordOf: (a: A) => string, status: string, total: number): { asks: A[]; n: number; narrowed: boolean } {
	if (!status) return { asks: inState, n: total, narrowed: false };
	const asks = inState.filter((a) => wordOf(a) === status);
	return { asks, n: asks.length, narrowed: true };
}

/** Whether any of these requests was asked for by more than one person (removing takes it away for all of them). */
export const anyShared = (asks: Pick<Ask, 'people'>[]): boolean => asks.some((a) => (a.people?.length ?? 0) > 1);

/** The call that removes it: DELETE for one, the bulk route for the shown cards or all failed or finished; answers how many went. A waiting one goes by the bulk route too: DELETE on a waiting ask is a take-back, which leaves it to the others who asked. */
export async function removeRequests(what: RemoveWhat): Promise<number> {
	if ('one' in what) {
		if (what.one.state !== 'asked') return (await api.del(`/asks/${what.one.id}`), 1);
		return (await api.post<{ removed: number }>('/asks/remove', { ids: [what.one.id] }))?.removed ?? 0;
	}
	const dl = what.byDownload?.length ? { failed_downloads: what.byDownload } : {};
	// The shown ids go with `only` and no filter: a server that does not know `only` (1.1.4) then removes exactly those ids, where a filter would make it take every failed or finished request. With nothing shown the filter alone clears every one.
	const body = what.ids ? { ids: what.ids, only: what.all, ...dl } : what.byDownload ? { filter: what.all, ids: [], ...dl } : { filter: what.all };
	return (await api.post<{ removed: number }>('/asks/remove', body))?.removed ?? 0;
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

	// The sheet stays open, busy, while it goes; a failure shows in it. A bulk clear says how many really went (some may have changed meanwhile).
	async function confirm() {
		if (!what || busy) return;
		busy = true;
		error = '';
		try {
			let w = what;
			let none = false;
			if ('all' in w && w.current) {
				const now = w.current();
				// A filter is on and nothing it shows is left: sending no ids would mean everything.
				none = !!w.filtered && !now.length;
				w = { ...w, ...clearParts(w.all, now, !!w.filtered) };
			}
			const n = none ? 0 : await removeRequests(w);
			toasts.push(n ? t(`requests.board.removedN.${n === 1 ? 'one' : 'other'}`, { n }) : t('requests.board.removedNone'), n ? 'ok' : 'info');
			// Closed as soon as the server has answered; the list reads again in the background.
			open = false;
			void ctx.load();
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
