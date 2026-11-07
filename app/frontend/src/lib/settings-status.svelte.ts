// Loads Settings status once per visit from the one status route. A person
// who may not change settings, or a failed read, leaves everything unknown.
import { api } from './core/api.ts';
import { session } from './core/session.svelte.ts';
import { fromServer, type ServerStatus, type StatusAnswer } from './settings-status.ts';

export const settingsNow = $state<{ answer: StatusAnswer | null }>({ answer: null });

export async function refreshSettingsStatus(): Promise<void> {
	if (!session.can('settings')) {
		settingsNow.answer = fromServer(null);
		return;
	}
	settingsNow.answer = fromServer(await api.get<ServerStatus>('/settings/status').catch(() => null));
}
