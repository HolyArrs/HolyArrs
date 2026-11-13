// The view each person picked last on a page (the Library view per kind, the Import a library view, the Calendar's view and follows), kept on their account so a phone, another browser or the installed app opens on the same view. The picks come with the session, so a page never opens on the wrong view first.

import { api as defaultClient, type Client } from './api';

export type ViewName = `library.view.${'all' | 'movie' | 'show' | 'artist'}` | 'library.import.view' | 'calendar.view' | 'calendar.follows';

/** How long the move of this browser's older picks waits, so the first page loads before it. */
export const VIEW_SEED_WAIT = 3000;

type OldStore = Pick<Storage, 'getItem' | 'removeItem'>;

// Library views this browser kept before they lived on the account: each is read once to seed the account, then dropped.
const OLD_KEYS: [ViewName, string][] = (['all', 'movie', 'show', 'artist'] as const).map((k) => [`library.view.${k}`, `holyarrs.library-view.${k}`]);
// The old page opened on Rows for any other value, so only these are moved.
const OLD_VIEWS = new Set(['posters', 'overview', 'table']);

export function createViews(client: Client = defaultClient, oldStore: () => OldStore | undefined = () => globalThis.localStorage) {
	const s = $state<{ saved: Record<string, unknown> }>({ saved: {} });
	let signedIn = false;
	// Bumped on sign-out, so a save still on its way never hands its picks to the next person.
	let round = 0;
	let sending = false;
	let waiting: Record<string, unknown> = {};
	// Old browser keys to drop once the account has their picks.
	let moving: string[] = [];
	let timer: ReturnType<typeof setTimeout> | undefined;

	function oldValue(key: string): string | null {
		try {
			return oldStore()?.getItem(key) ?? null;
		} catch {
			return null;
		}
	}
	function dropOld(key: string) {
		try {
			oldStore()?.removeItem(key);
		} catch {
			// Read again next time; the account copy wins anyway.
		}
	}

	// One save at a time; picks made meanwhile go in the next one, so the last pick is the one kept.
	async function send() {
		clearTimeout(timer);
		timer = undefined;
		if (sending || !signedIn || Object.keys(waiting).length === 0) return;
		const body = waiting;
		const drop = moving;
		const was = round;
		waiting = {};
		moving = [];
		sending = true;
		let saved = false;
		try {
			await client.request('/my/views', { method: 'PUT', body, keepalive: true });
			drop.forEach(dropOld);
			saved = true;
		} catch {
			// Not saved: sent again with the next pick (a newer pick of the same view wins).
			if (was === round) {
				waiting = { ...body, ...waiting };
				moving = [...drop, ...moving];
			}
		} finally {
			sending = false;
		}
		if (saved || was !== round) void send();
	}

	return {
		/** Takes the picks that came with the session, and moves this browser's older Library picks onto the account. */
		take(views: Record<string, unknown> | null | undefined) {
			signedIn = true;
			s.saved = { ...(views ?? {}), ...waiting };
			for (const [name, key] of OLD_KEYS) {
				const old = oldValue(key);
				if (old === null) continue;
				if (s.saved[name] !== undefined || !OLD_VIEWS.has(old)) {
					dropOld(key);
					continue;
				}
				s.saved[name] = old;
				waiting[name] = old;
				moving.push(key);
			}
			if (Object.keys(waiting).length > 0 && !timer) timer = setTimeout(() => void send(), VIEW_SEED_WAIT);
		},
		/** The saved pick, undefined when there is none; each page checks the value itself. */
		get(name: ViewName): unknown {
			return s.saved[name];
		},
		/** A new pick: used at once and saved to the account straight away. */
		set(name: ViewName, value: string) {
			if (s.saved[name] === value) return;
			s.saved[name] = value;
			if (!signedIn) return;
			waiting[name] = value;
			void send();
		},
		/** Signed out: nothing is kept or sent for the next person. */
		forget() {
			signedIn = false;
			round++;
			clearTimeout(timer);
			timer = undefined;
			waiting = {};
			moving = [];
			s.saved = {};
		}
	};
}

export const views = createViews();
