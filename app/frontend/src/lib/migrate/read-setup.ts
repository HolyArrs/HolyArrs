// First-run setup's inline read (ReadSheet): what each app brings, where its key lives, and the finished read's helpers.
import { api } from '#lib/core/api.ts';
import { appName } from './move.ts';

// What each app brings over, shown beside the form in setup.
export const BRINGS: Record<string, string[]> = {
	radarr: ['migrate.brings.movies', 'migrate.brings.folders', 'migrate.brings.quality', 'migrate.brings.rules', 'migrate.brings.naming', 'migrate.brings.sources', 'migrate.brings.lists', 'migrate.brings.alerts'],
	sonarr: ['migrate.brings.shows', 'migrate.brings.folders', 'migrate.brings.quality', 'migrate.brings.rules', 'migrate.brings.naming', 'migrate.brings.sources', 'migrate.brings.lists', 'migrate.brings.alerts'],
	lidarr: ['migrate.brings.artists', 'migrate.brings.folders', 'migrate.brings.quality', 'migrate.brings.rules', 'migrate.brings.naming', 'migrate.brings.sources', 'migrate.brings.lists', 'migrate.brings.alerts'],
	prowlarr: ['migrate.brings.indexers', 'migrate.brings.appLinks', 'migrate.brings.downloadApps', 'migrate.brings.alerts'],
	seerr: ['migrate.brings.people', 'migrate.brings.requests', 'migrate.brings.limits', 'migrate.brings.itsArrs'],
	overseerr: ['migrate.brings.people', 'migrate.brings.requests', 'migrate.brings.limits', 'migrate.brings.itsArrs'],
	bazarr: ['migrate.brings.subLanguages', 'migrate.brings.subProfiles', 'migrate.brings.subSites'],
	sabnzbd: ['migrate.brings.dlFolders', 'migrate.brings.usenetServers'],
	qbittorrent: ['migrate.brings.dlFolders']
};

// The app's own page that shows its API key.
export const KEY_PAGES: Record<string, string> = { radarr: '/settings/general', sonarr: '/settings/general', lidarr: '/settings/general', prowlarr: '/settings/general', bazarr: '/settings/general', seerr: '/settings/main', overseerr: '/settings/main', sabnzbd: '/sabnzbd/config/general/' };

type Done = { snapshot: { id: number }; linked?: { kind: string; snapshot: { id: number; address?: string } }[]; unread?: { kind: string; address: string }[]; skipped?: { kind: string; address: string }[] };

/** A finished read the owner changed their mind about (new address or key): the server forgets it and the apps it read along. */
export function forgetRead(d: Done | { r: Done } | null) {
	const r = d && 'r' in d ? d.r : d;
	if (!r) return;
	for (const id of [r.snapshot.id, ...(r.linked ?? []).map((l) => l.snapshot.id)]) void api.del(`/migrate/snapshots/${id}`).catch(() => undefined);
}

const host = (a: string) => a.replace(/^https?:\/\//, '').replace(/\/$/, '');
/** Seerr's own Radarr and Sonarr from a read, as tags: read or counted (green), not reached (amber). */
export function linkedTags(r: Done | undefined): { label: string; ok: boolean }[] {
	if (!r) return [];
	return [
		...(r.linked ?? []).map((l) => ({ label: `${appName(l.kind)} · ${host(l.snapshot.address ?? '')}`, ok: true })),
		...(r.skipped ?? []).map((l) => ({ label: `${appName(l.kind)} · ${host(l.address)}`, ok: true })),
		...(r.unread ?? []).map((l) => ({ label: `${appName(l.kind)} · ${host(l.address)}`, ok: false }))
	];
}
