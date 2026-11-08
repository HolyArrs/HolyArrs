import { cleanDisplayName, shownName } from './profile.ts';
import { api } from '#lib/core/api.ts';

// My display name, shared by the settings side menu and the Profile card. Loaded once per visit; saving updates both at once.
export const profile = $state<{ displayName: string; loaded: boolean }>({ displayName: '', loaded: false });

let loading: Promise<void> | null = null;
export function loadProfile(): Promise<void> {
	loading ??= api
		.get<{ display_name: string }>('/my/profile')
		.then((r) => {
			profile.displayName = r.display_name ?? '';
			profile.loaded = true;
		})
		.catch(() => {
			loading = null;
		});
	return loading;
}

export async function saveDisplayName(typed: string): Promise<string> {
	const r = await api.put<{ display_name: string }>('/my/profile', { display_name: cleanDisplayName(typed) });
	profile.displayName = r.display_name ?? '';
	profile.loaded = true;
	return profile.displayName;
}

export { shownName };
