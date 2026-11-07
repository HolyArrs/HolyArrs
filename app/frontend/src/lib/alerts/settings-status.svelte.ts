import { api } from '../core/api.ts';
import { session } from '../core/session.svelte.ts';
import { fromServer, type ServerStatus, type StatusAnswer } from './settings-status.ts';

export const settingsNow = $state<{ answer: StatusAnswer | null }>({ answer: null });

// Without the settings right, or on a failed read, everything stays unknown.
export async function refreshSettingsStatus(): Promise<void> {
	if (!session.can('settings')) {
		settingsNow.answer = fromServer(null);
		return;
	}
	settingsNow.answer = fromServer(await api.get<ServerStatus>('/settings/status').catch(() => null));
}
