// Builds the short result line shown after a connection test, like "Sonarr v4.0.1".

// Known apps by the lowercase word that appears in their test reply.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
const apps: Record<string, string> = {
	qbittorrent: 'qBittorrent',
	sabnzbd: 'SABnzbd',
	transmission: 'Transmission',
	nzbget: 'NZBGet',
	deluge: 'Deluge',
	rtorrent: 'rTorrent',
	prowlarr: 'Prowlarr',
	jackett: 'Jackett',
	sonarr: 'Sonarr',
	radarr: 'Radarr',
	lidarr: 'Lidarr',
	readarr: 'Readarr',
	bazarr: 'Bazarr',
	whisparr: 'Whisparr',
	overseerr: 'Overseerr',
	jellyseerr: 'Jellyseerr',
	jellyfin: 'Jellyfin',
	emby: 'Emby',
	plex: 'Plex'
};

// Keeps up to three version parts; ignores numbers inside paths, hosts and addresses.
export function testVersion(text: string): string {
	const m = (text ?? '').match(/(?<![\w./:@-])v?(\d+(?:\.\d+)+)/);
	return m ? `v${m[1].split('.').slice(0, 3).join('.')}` : '';
}

export function testApp(text: string): string {
	for (const w of (text ?? '').toLowerCase().match(/[a-z]+/g) ?? []) if (apps[w]) return apps[w];
	return '';
}

export function testLabel(text: string, app = '', extra = ''): string {
	const head = [app || testApp(text), testVersion(text)].filter(Boolean).join(' ') || t('core.testTag.connected');
	return [head, extra].filter(Boolean).join(' · ');
}

// Picks out counts like "12 indexers" from the reply.
export function testCount(text: string): string {
	const m = (text ?? '').match(/\b(\d+) (Releases?|subtitles?|indexers?|sites?)\b/i);
	return m ? `${m[1]} ${m[2]}` : '';
}
