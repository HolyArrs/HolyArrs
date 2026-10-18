// Loads what Settings status needs, once per visit, from routes the app
// already has. Anything a person may not read is simply left unknown.
import { api } from './api.ts';
import { session } from './session.svelte.ts';
import { settingsStatus, type StatusAnswer } from './settings-status.ts';
import type { BackupInfo, HealthChecks, HealthStates, LibraryFolder, SourceView, UpdateInfo } from './types.ts';
import type { DownloaderView } from './downloaders.ts';

export const settingsNow = $state<{ answer: StatusAnswer | null }>({ answer: null });

const soft = <T>(p: Promise<T>): Promise<T | null> => p.catch(() => null);

export async function refreshSettingsStatus(): Promise<void> {
	if (!session.can('settings')) {
		settingsNow.answer = settingsStatus({});
		return;
	}
	const proof = session.s.via === 'api_key' || session.s.localRecent;
	const [checks, states, folders, sources, downloaders, backups, tmdb, update] = await Promise.all([
		soft(api.get<HealthChecks>('/health/checks')),
		soft(api.get<HealthStates>('/health/states')),
		soft(api.get<{ library_folders: LibraryFolder[] }>('/library-folders')),
		soft(api.get<{ sources: SourceView[] }>('/sources')),
		soft(api.get<{ downloaders: DownloaderView[] }>('/downloaders')),
		soft(api.get<{ backups: BackupInfo[] }>('/backups')),
		proof ? soft(api.get<{ token: string }>('/settings/tmdb-token')) : Promise.resolve(null),
		soft(api.get<UpdateInfo>('/app/update'))
	]);
	const newest = backups ? (backups.backups ?? []).map((b) => b.created_at).sort().at(-1) ?? '' : null;
	settingsNow.answer = settingsStatus({
		checks: checks?.checks,
		states: states?.states,
		folders: folders ? (folders.library_folders ?? []).length : null,
		sources: sources ? (sources.sources ?? []).map((s) => ({ name: s.name, state: s.health?.state ?? '' })) : null,
		downloaders: downloaders ? (downloaders.downloaders ?? []).map((d) => ({ name: d.name, state: d.health?.state ?? '' })) : null,
		newestBackup: newest,
		tmdbKey: tmdb ? !!tmdb.token : null,
		updateNewer: update ? !!update.newer : null
	});
}
