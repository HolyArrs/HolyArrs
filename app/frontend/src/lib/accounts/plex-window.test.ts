import { describe, expect, it, vi } from 'vitest';
import { closePlexWindow, plexDoneUrl, withPlexForward } from './plex-window.ts';

describe('Plex sign-in window', () => {
	it('adds our own done page as forwardUrl', () => {
		const u = withPlexForward('https://app.plex.tv/auth#?clientID=a&code=b', 'https://home.example:6262/plex-done');
		expect(u).toBe('https://app.plex.tv/auth#?clientID=a&code=b&forwardUrl=https%3A%2F%2Fhome.example%3A6262%2Fplex-done');
	});

	it('builds the done page from this page origin and base only', () => {
		expect(plexDoneUrl('http://10.0.0.2:6262', '/arr/plex-done')).toBe('http://10.0.0.2:6262/arr/plex-done');
		// A path that tries to leave our address stays on our address.
		expect(plexDoneUrl('http://10.0.0.2:6262', '//evil.example/x')).toBe('http://10.0.0.2:6262/plex-done');
		expect(plexDoneUrl('http://10.0.0.2:6262', 'https://evil.example/plex-done')).toBe('http://10.0.0.2:6262/plex-done');
	});

	it('leaves an address without a hash query untouched', () => {
		expect(withPlexForward('https://app.plex.tv/auth', 'http://x/plex-done')).toBe('https://app.plex.tv/auth');
	});

	it('closes the window and survives one that is gone', () => {
		const win = { close: vi.fn() } as unknown as Window;
		closePlexWindow(win);
		expect(win.close).toHaveBeenCalled();
		closePlexWindow({ close: () => { throw new Error('gone'); } } as unknown as Window);
		closePlexWindow(null);
	});
});
