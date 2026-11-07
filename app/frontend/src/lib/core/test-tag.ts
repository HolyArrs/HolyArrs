// The words of a connection test's tag: "Radarr v6.0.4", "qBittorrent v5.1.2 · 3 indexers".

const apps: Record<string, string> = {
	qbittorrent: 'qBittorrent',
	sabnzbd: 'SABnzbd',
	transmission: 'Transmission',
	nzbget: 'NZBGet',
	deluge: 'Deluge',
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

/** testVersion picks the version out of a test's answer ("... qbittorrent 5.1.2.",
 * "Radarr 6.0.4.10291") as "v5.1.2": at most three parts; empty when none. */
export function testVersion(text: string): string {
	const m = (text ?? '').match(/(?<![\w./:@-])v?(\d+(?:\.\d+)+)/);
	return m ? `v${m[1].split('.').slice(0, 3).join('.')}` : '';
}

/** testApp names the app a test's answer speaks of ("qbittorrent" → "qBittorrent"); empty when none. */
export function testApp(text: string): string {
	for (const w of (text ?? '').toLowerCase().match(/[a-z]+/g) ?? []) if (apps[w]) return apps[w];
	return '';
}

/** testLabel is the tag's words: app and version, then any extra ("3 indexers"); "Connected" when there is neither. */
export function testLabel(text: string, app = '', extra = ''): string {
	const head = [app || testApp(text), testVersion(text)].filter(Boolean).join(' ') || 'Connected';
	return [head, extra].filter(Boolean).join(' · ');
}

/** testCount picks a count a test's answer gives ("... answered with 2 Releases.",
 * "... 14 subtitles for a test title.") as the tag's extra: "2 Releases"; empty when none. */
export function testCount(text: string): string {
	const m = (text ?? '').match(/\b(\d+) (Releases?|subtitles?|indexers?|sites?)\b/i);
	return m ? `${m[1]} ${m[2]}` : '';
}
