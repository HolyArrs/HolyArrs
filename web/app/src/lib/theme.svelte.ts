// Light or dark. The first paint reads the same key in app.html, so the
// page never flashes the wrong colours.

export type Theme = 'light' | 'dark';
const KEY = 'holyarrs-theme';

function current(): Theme {
	if (typeof document === 'undefined') return 'dark';
	return document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
}

export const theme = $state<{ value: Theme }>({ value: current() });

export function setTheme(t: Theme) {
	theme.value = t;
	document.documentElement.dataset.theme = t;
	try {
		localStorage.setItem(KEY, t);
	} catch {
		// Private windows may refuse storage; the choice then lasts this visit.
	}
}

export function toggleTheme() {
	setTheme(theme.value === 'dark' ? 'light' : 'dark');
}
