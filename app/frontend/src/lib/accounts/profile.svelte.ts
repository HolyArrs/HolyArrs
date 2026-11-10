import { cleanDisplayName, shownName } from './profile.ts';
import { api } from '#lib/core/api.ts';
import { isLocaleChoice, lang, setLang, type LocaleChoice } from '#lib/core/i18n/i18n.svelte.ts';

// My display name, shared by the settings side menu and the Profile card. Loaded once per visit; saving updates both at once.
export const profile = $state<{ displayName: string; loaded: boolean }>({ displayName: '', loaded: false });

let loading: Promise<void> | null = null;
export function loadProfile(): Promise<void> {
	loading ??= api
		.get<{ display_name: string; language?: string }>('/my/profile')
		.then((r) => {
			profile.displayName = r.display_name ?? '';
			profile.loaded = true;
			// A language saved on the account wins over this browser's.
			if (r.language && isLocaleChoice(r.language) && r.language !== lang.choice) void setLang(r.language);
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

/** Shows the app in `choice` now and keeps it on the account ('auto' clears it, so the browser decides). */
export async function saveLanguage(choice: LocaleChoice): Promise<void> {
	await setLang(choice);
	await api.put('/my/language', { language: choice === 'auto' ? '' : choice });
}

/** Forgets the loaded profile, so the next person to sign in gets theirs. */
export function forgetProfile(): void {
	loading = null;
	profile.displayName = '';
	profile.loaded = false;
}

export { shownName };
