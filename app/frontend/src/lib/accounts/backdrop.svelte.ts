// The app's background behind every page: plain (the default, as Explore looks) or a wall of this Library's posters. Saved in this browser, like the theme.

export type Backdrop = 'plain' | 'posters';

const KEY = 'holyarrs-backdrop';

export const backdrop = $state<{ value: Backdrop }>({ value: typeof document === 'undefined' ? 'plain' : stored() });

export function setBackdrop(b: Backdrop) {
	backdrop.value = b;
	try {
		if (b === 'plain') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, b);
	} catch {
		// Private windows may refuse storage; the choice then lasts this visit.
	}
}

function stored(): Backdrop {
	try {
		return localStorage.getItem(KEY) === 'posters' ? 'posters' : 'plain';
	} catch {
		return 'plain';
	}
}
