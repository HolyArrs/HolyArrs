
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

export function testVersion(text: string): string {
	const m = (text ?? '').match(/(?<![\w./:@-])v?(\d+(?:\.\d+)+)/);
	return m ? `v${m[1].split('.').slice(0, 3).join('.')}` : '';
}

export function testApp(text: string): string {
	for (const w of (text ?? '').toLowerCase().match(/[a-z]+/g) ?? []) if (apps[w]) return apps[w];
	return '';
}

export function testLabel(text: string, app = '', extra = ''): string {
	const head = [app || testApp(text), testVersion(text)].filter(Boolean).join(' ') || 'Connected';
	return [head, extra].filter(Boolean).join(' · ');
}

export function testCount(text: string): string {
	const m = (text ?? '').match(/\b(\d+) (Releases?|subtitles?|indexers?|sites?)\b/i);
	return m ? `${m[1]} ${m[2]}` : '';
}
