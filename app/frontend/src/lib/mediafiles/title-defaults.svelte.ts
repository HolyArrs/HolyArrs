// Shared loader for the saved add-title defaults, wired to the API.
import { api } from '../core/api.ts';
import { defaultsLoader, rememberSearch, type TitleKind } from './title-defaults.ts';
import type { TitleDefaults } from './types.ts';

export const savedDefaults = defaultsLoader((p) => api.get<TitleDefaults>(p));

// Saves the last Search when added choice as the default for that kind, so the dialog and the Movies and TV defaults stay one setting.
// Users who may not change settings keep the old default; a failed save is not an add error.
export async function saveSearchChoice(kind: TitleKind, searchOnAdd: boolean): Promise<void> {
	const saved = await savedDefaults.load();
	// Without the saved copy a save would put built-in values over the rest of the defaults.
	if (!saved) return;
	const next = rememberSearch(saved, kind, searchOnAdd);
	if (!next) return;
	try {
		await api.put<TitleDefaults>('/settings/title-defaults', next);
	} catch {
		// Kept as it was.
	} finally {
		savedDefaults.forget();
	}
}
