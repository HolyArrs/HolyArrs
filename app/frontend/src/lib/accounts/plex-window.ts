// The Plex sign-in window, shared by every Plex button: open it while the click
// still counts, send it to Plex with our own "done" page as the way back, and
// close it the moment the sign-in ends. Plex's page can cut our hold on the
// window, so the done page also closes itself.
import { to } from '../core/paths.ts';

/** Our own done page on this address. Anything but a plain path falls back to /plex-done. */
export function plexDoneUrl(origin: string, path: string): string {
	const safe = path.startsWith('/') && !path.startsWith('//') ? path : '/plex-done';
	const u = new URL(safe, origin);
	return u.origin === new URL(origin).origin ? u.href : new URL('/plex-done', origin).href;
}

/** Adds forwardUrl to Plex's hash query (app.plex.tv/auth#?...). */
export function withPlexForward(authUrl: string, forward: string): string {
	if (!authUrl.includes('#?')) return authUrl;
	return `${authUrl}&forwardUrl=${encodeURIComponent(forward)}`;
}

/** Opens an empty window now; null when the browser blocked it. */
export function openPlexWindow(name: string): Window | null {
	const win = window.open('about:blank', name, 'width=600,height=720');
	// Keep the opener: Chromium only lets this tab close the window later while it is kept. Plex is another origin and cannot reach this tab's page.
	return win;
}

/** Sends the window (or a new tab when it was blocked) to Plex. */
export function sendToPlex(win: Window | null, authUrl: string): void {
	const url = withPlexForward(authUrl, plexDoneUrl(window.location.origin, to('/plex-done')));
	if (win) win.location.href = url;
	else window.open(url, '_blank', 'noopener');
}

export function closePlexWindow(win: Window | null | undefined): void {
	try {
		win?.close();
	} catch {
		// The window may already be gone.
	}
}
