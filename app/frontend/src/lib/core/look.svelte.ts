// The app's look: full (big pictures, trailers that play by themselves) or minimal (plain headers, nothing plays by itself).
// Saved in this browser, like the theme. The first paint reads the same key in app.html and sets <html data-look>, so a minimal page never shows the big pictures first.

export type Look = 'full' | 'minimal';

const KEY = 'holyarrs-look';

export const look = $state<{ value: Look }>({ value: typeof document === 'undefined' ? 'full' : current() });

export function setLook(l: Look) {
	look.value = l;
	if (typeof document !== 'undefined' && document.documentElement) {
		if (l === 'minimal') document.documentElement.dataset.look = 'minimal';
		else delete document.documentElement.dataset.look;
	}
	try {
		if (l === 'full') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, l);
	} catch {
		// Private windows may refuse storage; the choice then lasts this visit.
	}
}

/** True when this browser asked for the minimal look. */
export function minimalLook(): boolean {
	return look.value === 'minimal';
}

function current(): Look {
	if (document.documentElement?.dataset?.look === 'minimal') return 'minimal';
	try {
		return localStorage.getItem(KEY) === 'minimal' ? 'minimal' : 'full';
	} catch {
		return 'full';
	}
}
