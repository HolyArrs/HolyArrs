// One page of Events for History: the page's filters plus the kinds
// of the picked tab. Newer requests win; "more" appends the next page.
import { api, message } from '../api.ts';
import { live } from '../live.svelte.ts';
import { createLatestOnly, dateRangeProblem, type HistoryFilter } from '../history.ts';
import { eventsQuery, settle } from '../history-views.ts';
import { lookTabs, tabKinds, ALL } from '../history-looks.ts';
import type { EventPage, EventRecord } from '../types.ts';

export class EventFeed {
	events = $state<EventRecord[]>([]);
	total = $state(0);
	/** With an episode filter: the episode, named by the server. */
	episode = $state('');
	offset = $state(0);
	loaded = $state(false);
	busy = $state(false);
	error = $state('');
	#latest = createLatestOnly();

	constructor(readonly size: number) {}

	/** load reads the page at `offset` (append adds it under the rows already shown). */
	async load(filter: HistoryFilter, kinds: string[] | undefined, offset = 0, append = false) {
		const ok = this.#latest();
		if (dateRangeProblem(filter) || (kinds && !kinds.length)) {
			this.events = [];
			this.total = 0;
			this.loaded = true;
			return;
		}
		this.busy = true;
		try {
			const p = await api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit: this.size, offset }));
			if (!ok()) return;
			this.events = append ? [...this.events, ...p.events] : p.events;
			this.total = p.total;
			this.episode = p.episode ?? '';
			this.offset = offset;
			this.error = '';
		} catch (e) {
			if (ok()) this.error = message(e);
		} finally {
			if (ok()) {
				this.loaded = true;
				this.busy = false;
			}
		}
	}

	/** watch reloads (settled) on live news while the first page is shown. */
	watch(reload: () => void): () => void {
		const s = settle(() => this.offset === 0 && reload(), 2000);
		const offs = [live.on('event', s.poke), live.on('resync', s.poke)];
		return () => {
			offs.forEach((o) => o());
			s.stop();
		};
	}
}

/** Counts per tab (and per sub-tab of the open one) for the tab chips, read with limit=1. */
export class TabCounts {
	tabs = $state<Record<string, number>>({});
	subs = $state<Record<string, number>>({});
	#latest = createLatestOnly();

	async load(filter: HistoryFilter, open: string) {
		const ok = this.#latest();
		if (dateRangeProblem(filter)) return;
		const count = async (kinds: string[] | undefined) => {
			try {
				return (await api.get<EventPage>('/events?' + eventsQuery(filter, { kinds, limit: 1, offset: 0 }))).total;
			} catch {
				return -1;
			}
		};
		const tab = lookTabs.find((t) => t.key === open);
		const [tabs, subs] = await Promise.all([
			Promise.all(lookTabs.map(async (t) => [t.key, await count(tabKinds(t.key))] as const)),
			Promise.all((tab && tab.key !== ALL ? tab.subs : []).map(async (s) => [s.key, await count(s.kinds)] as const))
		]);
		if (!ok()) return;
		this.tabs = Object.fromEntries(tabs.filter(([, n]) => n >= 0));
		this.subs = Object.fromEntries(subs.filter(([, n]) => n >= 0));
	}
}
