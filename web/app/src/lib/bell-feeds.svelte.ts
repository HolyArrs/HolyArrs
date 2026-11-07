// What reaches the bell from outside the page you are on: request updates, failed
// downloads, downloads waiting for a decision and a new version. All of it rides on
// data the app already loads (the inbox, the decisions list, live events), plus one
// update check every few hours for people who manage the app.
import { goto } from '$app/navigation';
import { api } from './api.ts';
import { decisions } from './decisions.svelte.ts';
import { live } from './live.svelte.ts';
import { to } from './paths.ts';
import { cut, messageWords } from './bell-words.ts';
import { inbox } from './requests.svelte.ts';
import { toasts } from './toasts.svelte';
import type { UpdateInfo } from './types.ts';
import type { PinnedAlert } from './attention-alerts.ts';

/** Pinned rows from these feeds (Settings warnings are added by the panel). */
export const feedPins = $state<{ update: PinnedAlert | null }>({ update: null });

/** decisionPins lists downloads waiting for a person, pinned until handled. */
export function decisionPins(): PinnedAlert[] {
	return decisions.items.map((d) => ({
		key: `decision|${d.slot_id}`,
		title: `${d.title}${d.year ? ` (${d.year})` : ''} needs a decision`,
		body: d.reason ? cut(d.reason) : undefined,
		tone: 'warn',
		href: '/downloads',
		label: 'Downloads'
	}));
}

// Messages already handled this visit; kept across restarts of the feeds (permissions
// load after sign-in and start them again), so nothing lands in the list twice.
const known = new Set<number>();

const TOLD = 'holyarrs.bell.told';
function readTold(): Set<number> {
	try {
		return new Set((JSON.parse(localStorage.getItem(TOLD) ?? '[]') as number[]).slice(-200));
	} catch {
		return new Set();
	}
}
function saveTold(ids: Set<number>) {
	try {
		localStorage.setItem(TOLD, JSON.stringify([...ids].slice(-200)));
	} catch {
		// private window: the next reload may show one again
	}
}

/**
 * startBellFeeds runs while someone is signed in. `canManage` turns on the parts only
 * staff see (failed downloads, a new version). It returns the stop function.
 */
export function startBellFeeds(canManage: boolean): () => void {
	const stops: (() => void)[] = [];

	// Request updates pop up once each, wherever you are, and stay in the list. Ones this
	// browser already showed (kept in local storage) only go into the list after a reload.
	const told = readTold();
	const stopInbox = $effect.root(() => {
		$effect(() => {
			const unread = inbox.messages.filter((m) => !m.read_at && !known.has(m.id));
			for (const m of unread) {
				known.add(m.id);
				const w = messageWords(m);
				if (told.has(m.id)) toasts.record(w.title, w.tone, { body: w.body });
				else toasts.push(w.title, w.tone, { body: w.body, action: { label: 'Requests', run: () => void goto(to('/requests')) } });
				told.add(m.id);
			}
			if (unread.length) saveTold(told);
		});
	});
	stops.push(stopInbox);

	if (canManage) {
		// A download that fails pops up with its reason and stays in the list.
		stops.push(
			live.on('download', (x) => {
				const d = x as { State?: string; StateReason?: string | null; state?: string; state_reason?: string | null };
				if ((d.State ?? d.state) !== 'failed') return;
				const why = d.StateReason ?? d.state_reason ?? '';
				// Someone marked it failed by hand: they already know.
				if (/^marked failed/i.test(why)) return;
				toasts.push('Download failed', 'fail', { body: why ? cut(why) : 'Searching for the next Release', action: { label: 'Downloads', run: () => void goto(to('/downloads')) } });
			})
		);

		// A new version: one pinned row until it is installed.
		const check = () =>
			api
				.get<UpdateInfo>('/app/update')
				.then((u) => {
					feedPins.update = u.on && u.newer && u.latest ? { key: 'update', title: 'Update available', body: `Version ${u.latest} is ready`, tone: 'info', href: '/status?tab=updates', label: 'Update' } : null;
				})
				.catch(() => {});
		void check();
		const t = setInterval(check, 6 * 3600_000);
		stops.push(() => clearInterval(t));
	}

	return () => {
		stops.forEach((s) => s());
		feedPins.update = null;
	};
}

/** forgetFeeds is for sign-out: the next person starts with nothing handled. */
export function forgetFeeds(): void {
	known.clear();
}
