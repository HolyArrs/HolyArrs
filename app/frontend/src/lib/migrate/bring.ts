// After a read: the Downloaders the old apps used come over at once, so no one types them again.
import { api } from '#lib/core/api.ts';
import type { AppKind, Snapshot } from './move.ts';

/** One Downloader brought: its row, the old app, the client kind, and what the old app hid ("key", "password", "token"). */
export type Brought = { id: number; app: string; name: string; kind: string; needs?: string };

/** The snapshots a read gives: an Overseerr or Seerr read also reads the Sonarr and Radarr it keeps. */
export function readSnapshots(r: { snapshot: Snapshot; linked?: { snapshot: Snapshot }[] }): Snapshot[] {
	return [r.snapshot, ...(r.linked ?? []).map((l) => l.snapshot)];
}

/** Adds the Downloaders of the apps a read gave (Sonarr, Radarr or Lidarr, or the ones Overseerr keeps). */
export async function bringDownloaders(r: { snapshot: Snapshot; linked?: { snapshot: Snapshot }[] }, kind: AppKind): Promise<Brought[]> {
	const ids = kind === 'sonarr' || kind === 'radarr' || kind === 'lidarr' ? [r.snapshot.id] : (r.linked ?? []).map((l) => l.snapshot.id);
	if (!ids.length) return [];
	try {
		return (await api.post<{ downloaders: Brought[] }>('/migrate/downloaders', { snapshots: ids })).downloaders?.filter((b) => b.id) ?? [];
	} catch {
		return [];
	}
}
