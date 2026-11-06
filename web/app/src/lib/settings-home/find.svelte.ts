// The words typed into "Search settings": the top bar field while in
// Settings, and the field on the Settings home on a phone (no top bar there).
// One shared term, so both fields and the results always agree.
export const settingsFind = $state<{ term: string; first: string }>({ term: '', first: '' });

/** inSettings says whether a path is Settings (the top bar field then searches settings). */
export function inSettings(path: string): boolean {
	return path === '/settings' || path.startsWith('/settings/');
}
