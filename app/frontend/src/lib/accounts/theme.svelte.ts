// Light, dark or follow the system. The first paint reads the same key in app.html, so the page never flashes the wrong colours. `value` is always the colours in use; `choice` is what the person picked.

export type Theme = 'light' | 'dark';
export type ThemeChoice = Theme | 'system';

const KEY = 'holyarrs-theme';

export const theme = $state<{ value: Theme; choice: ThemeChoice }>({
	value: current(),
	choice: typeof document === 'undefined' ? 'system' : storedChoice()
});

export function setTheme(t: ThemeChoice) {
	theme.choice = t;
	apply(t === 'system' ? systemTheme() : t);
	try {
		if (t === 'system') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, t);
	} catch {
		// Private windows may refuse storage; the choice then lasts this visit.
	}
}

// Follow the system while the person has not picked one.
if (typeof matchMedia !== 'undefined') {
	try {
		matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
			if (theme.choice === 'system') apply(systemTheme());
		});
	} catch {
		// Old browsers without addEventListener on media queries keep the first answer.
	}
}


function systemTheme(): Theme {
	if (typeof matchMedia === 'undefined') return 'light';
	return matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function storedChoice(): ThemeChoice {
	try {
		const t = localStorage.getItem(KEY);
		return t === 'light' || t === 'dark' ? t : 'system';
	} catch {
		return 'system';
	}
}

function current(): Theme {
	if (typeof document === 'undefined') return 'light';
	return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
}

function apply(t: Theme) {
	theme.value = t;
	if (typeof document !== 'undefined') document.documentElement.dataset.theme = t;
}
