// Shared state for the settings search box.

export const settingsFind = $state<{ term: string; first: string }>({ term: '', first: '' });

// True for the settings page and anything under it.
export function inSettings(path: string): boolean {
	return path === '/settings' || path.startsWith('/settings/');
}
