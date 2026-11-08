// Plex sign-in: opens Plex in a window and polls until the person approves.
import { ApiError } from '../core/api.ts';
import { session, type SignInResult } from '../core/session.svelte.ts';

export interface PlexHooks {
	setup: boolean;
	waiting: (on: boolean) => void;
	done: (r: SignInResult) => void;
	failed: (err: unknown) => void;
	timedOut: () => void;
}

const TIMEOUT_MS = 9 * 60 * 1000;

/** Starts a Plex sign-in. The returned stop() ends the polling. */
export function plexSignIn(h: PlexHooks): { stop: () => void } {
	let timer: ReturnType<typeof setInterval> | undefined;
	const stop = () => clearInterval(timer);
	void run();
	return { stop };

	async function run() {
		const setup = h.setup;
		// Open the window while the click still counts, then point it at Plex.
		const win = window.open('', 'plex-sign-in', 'width=600,height=720');
		try {
			const p = await (setup ? session.setupPlexStart() : session.plexStart());
			if (win) {
				win.opener = null; // The Plex page cannot reach back into HolyArrs.
				win.location.href = p.auth_url;
			}
			else window.open(p.auth_url, '_blank', 'noopener');
			h.waiting(true);
			const started = Date.now();
			clearInterval(timer);
			timer = setInterval(async () => {
				if (Date.now() - started > TIMEOUT_MS) {
					clearInterval(timer);
					h.waiting(false);
					h.timedOut();
					return;
				}
				try {
					const r = await (setup ? session.setupPlexFinish(p.id) : session.plexFinish(p.id));
					if (r === 'waiting') return;
					clearInterval(timer);
					h.waiting(false);
					win?.close();
					h.done(r);
				} catch (err) {
					clearInterval(timer);
					h.waiting(false);
					win?.close();
					if (setup && err instanceof ApiError && err.status === 409) void session.load();
					h.failed(err);
				}
			}, 2000);
		} catch (err) {
			win?.close();
			if (setup && err instanceof ApiError && err.status === 409) void session.load();
			h.failed(err);
		}
	}
}
