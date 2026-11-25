// Downloaders an import brought from Sonarr, Radarr or Lidarr: they arrive paused,
// are resumed once they answer, and ask once for a key the old app hides.
import { api, message } from '#lib/core/api.ts';
import { formBody, formFrom } from './downloaders.ts';
import { reachWords } from '#lib/core/settings.ts';
import type { DownloaderView } from './downloaders.ts';

/** One the import brought: the old app, the client kind, and what the old app hid ("key", "password", "token"). */
export type BroughtInfo = { id: number; app: string; name?: string; kind?: string; needs?: string };

/** Where each client shows the key or password the old app hides (the client's own menu names). */
const where: Record<string, string> = {
	sabnzbd: 'SABnzbd > Config > General',
	nzbget: 'NZBGet > Settings > Security',
	qbittorrent: 'qBittorrent > Tools > Options > Web UI',
	transmission: 'Transmission > Preferences > Remote',
	deluge: 'Deluge > Preferences > Interface',
	aria2: 'aria2 > rpc-secret'
};
export function hiddenWhere(kind: string): string {
	return where[kind] ?? kind;
}

/** Tests a brought Downloader and resumes it when it answers. */
export async function checkBrought(id: number): Promise<{ ok: boolean; text: string }> {
	try {
		const t = await api.post<{ ok: boolean; text: string; health?: { state: string } }>(`/downloaders/${id}/test`);
		if (t.ok && t.health?.state === 'paused') await api.post(`/downloaders/${id}/resume`).catch(() => {});
		return { ok: t.ok, text: t.ok ? t.text : reachWords(t.text) };
	} catch (err) {
		return { ok: false, text: reachWords(message(err)) };
	}
}

/** Saves the key (or password) pasted on a brought row, keeping the rest of the Downloader, then checks it. */
export async function saveHiddenKey(id: number, secret: string): Promise<{ ok: boolean; text: string }> {
	try {
		const list = (await api.get<{ downloaders: DownloaderView[] }>('/downloaders')).downloaders ?? [];
		const d = list.find((x) => x.id === id);
		if (!d) return { ok: false, text: '' };
		await api.put(`/downloaders/${id}`, { ...formBody(formFrom(d)), secret: secret.trim() });
	} catch (err) {
		return { ok: false, text: message(err) };
	}
	return checkBrought(id);
}
