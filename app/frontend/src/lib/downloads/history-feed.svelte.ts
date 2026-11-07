// One page of Events for History: the page's filters plus the kinds
// of the picked tab. Newer requests win; "more" appends the next page.
import { api, message } from '../core/api.ts';
import { live } from '../core/live.svelte.ts';
import { createLatestOnly, dateRangeProblem, type HistoryFilter } from './history.ts';
import { eventsQuery, settle } from './history-kinds.ts';
import type { EventPage, EventRecord } from '../core/types.ts';

export class EventFeed {
	events = $state<EventRecord[]>([]);
	total = $state(0);
	/** With an episode filter: the episode, named by the server. */
	episode = $state('');
	offset = $state(0);
	/** Where the next read starts (Show more and the screen fill read from here). */
	next = $state(0);
	loaded = $state(false);
	busy = $state(false);
	/** A person pressed Show more: live news no longer reloads (it would pull the rows away). */
	manual = $state(false);
	error = $state('');
	#latest = createLatestOnly();

	constructor(readonly size: number) {}

	/** load reads the page at `offset` (append adds it under the rows already shown); `limit` reads several pages at once. */
	async load(filter: HistoryFilter, kinds: string[] | undefined, offset = 0, append = false, limit = this.size) {
		const ok = this.#latest();
		if (dateRangeProblem(filter) || (kinds && !kinds.length)) {
			this.events = [];
			this.total = 0;
			this.error = '';
			this.loaded = true;
			return;
		}
		this.busy = true;
		try {
			const p = await api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit, offset }));
			if (!ok()) return;
			this.events = append ? [...this.events, ...p.events] : p.events;
			this.total = p.total;
			this.episode = p.episode ?? '';
			this.offset = offset;
			this.next = offset + limit;
			if (!append) this.manual = false;
			this.error = '';
		} catch (e) {
			if (!ok()) return;
			this.error = message(e);
			// A failed reload must not leave the rows of the last filter on screen.
			if (!append) {
				this.events = [];
				this.total = 0;
			}
		} finally {
			if (ok()) {
				this.loaded = true;
				this.busy = false;
			}
		}
	}

	/** watch reloads (settled) on live news unless a person pressed Show more. */
	watch(reload: () => void): () => void {
		const s = settle(() => !this.manual && reload(), 2000);
		const offs = [live.on('event', s.poke), live.on('resync', s.poke)];
		return () => {
			offs.forEach((o) => o());
			s.stop();
		};
	}
}
