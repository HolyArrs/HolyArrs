export const settingsFind = $state<{ term: string; first: string }>({ term: '', first: '' });

export function inSettings(path: string): boolean {
	return path === '/settings' || path.startsWith('/settings/');
}
