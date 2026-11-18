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

/** The group of health checks a fix changes; none means nothing to re-run, only a fresh read. */
export type CheckGroup = 'backups' | 'library_folder' | 'source' | 'downloader';

// After a fix made on a settings page: just that group of checks runs again (which also drops the server's kept answer), then one read, so the warning and the menu dot go out at once instead of at the next timed check.
export async function recheckSettingsStatus(only?: CheckGroup): Promise<void> {
	if (only) await api.post(`/health/checks/run?only=${only}`, {}).catch(() => {});
	await refreshSettingsStatus();
}
