// What reaches the bell from outside the page you are on: request updates, failed downloads, downloads waiting for a decision and a new version. All of it rides on data the app already loads (the inbox, the decisions list, live events), plus one update check every few hours for people who manage the app.
import { cut, messageWords } from './bell-words.ts';
import { decisions } from '#lib/downloads/index.ts';
import type { PinnedAlert } from '#lib/alerts/index.ts';
import { goto } from '$app/navigation';
import { api } from '#lib/core/api.ts';
import { live } from '#lib/core/live.svelte.ts';
import { to } from '#lib/core/paths.ts';
import { inbox } from '#lib/core/inbox.svelte.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import type { UpdateInfo } from '#lib/core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** Pinned rows from these feeds (Settings warnings are added by the panel). */
export const feedPins = $state<{ update: PinnedAlert | null }>({ update: null });

/** Downloads waiting for a person, pinned until handled. */
export function decisionPins(): PinnedAlert[] {
	return decisions.items.map((d) => ({
		key: `decision|${d.slot_id}`,
		title: t('frame.feeds.decision', { title: `${d.title}${d.year ? ` (${d.year})` : ''}` }),
		body: d.reason ? cut(d.reason) : undefined,
		tone: 'warn',
		href: '/downloads',
		label: t('frame.feeds.downloads')
	}));
}

// Messages already handled this visit. Kept across restarts of the feeds (permissions load after sign-in and start them again), so nothing lands in the list twice.
const known = new Set<number>();

const TOLD = 'holyarrs.bell.told';

/** Runs while someone is signed in. `canManage` turns on the parts only staff see (failed downloads, a new version). Returns the stop function. */
export function startBellFeeds(canManage: boolean): () => void {
	const stops: (() => void)[] = [];

	// Request updates pop up once each, wherever you are, and stay in the list. Ones this browser already showed (kept in local storage) only go into the list after a reload.
	const told = readTold();
	const stopInbox = $effect.root(() => {
		$effect(() => {
			const unread = inbox.messages.filter((m) => !m.read_at && !known.has(m.id));
			for (const m of unread) {
				known.add(m.id);
				const w = messageWords(m);
				if (told.has(m.id)) toasts.record(w.title, w.tone, { body: w.body });
				else {
					// Ready: open the title. Anything else: the request itself on Requests.
					const open = m.kind === 'ready' && m.title_id ? { label: t('frame.feeds.open'), path: `/library/${m.title_id}` } : { label: t('frame.feeds.requests'), path: m.ask_id ? `/requests?ask=${m.ask_id}` : '/requests' };
					toasts.push(w.title, w.tone, { body: w.body, action: { label: open.label, run: () => void goto(to(open.path)) } });
				}
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
				toasts.push(t('frame.feeds.failed'), 'fail', { body: why ? cut(why) : t('frame.feeds.nextRelease'), action: { label: t('frame.feeds.downloads'), run: () => void goto(to('/downloads')) } });
			})
		);

		// A new version: one pinned row until it is installed.
		const check = () =>
			api
				.get<UpdateInfo>('/app/update')
				.then((u) => {
					feedPins.update = u.on && u.newer && u.latest ? { key: 'update', title: t('frame.feeds.update'), body: t('frame.feeds.updateReady', { version: u.latest }), tone: 'info', href: '/health?tab=updates', label: t('frame.feeds.updateLabel') } : null;
				})
				.catch(() => {});
		void check();
		const timer = setInterval(check, 6 * 3600_000);
		stops.push(() => clearInterval(timer));
	}

	return () => {
		stops.forEach((s) => s());
		feedPins.update = null;
	};
}

/** For sign-out: the next person starts with nothing handled. */
export function forgetFeeds(): void {
	known.clear();
}

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
		// Private window: the next reload may show one again.
	}
}
