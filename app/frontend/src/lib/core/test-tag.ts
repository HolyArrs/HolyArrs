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
	// The version split leaves only these letters of aria2 as a word.
	aria: 'aria2',
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

const connectHelp = 'https://docs.holyarrs.com/start/connect/#if-it-does-not-work';

// The row of the Connect page's "If it does not work" table for a failed test
// line that shows what an address's name turned into; '' for any other line.
// The text fragment scrolls the page to that row.
export function connectHelpUrl(text: string | null | undefined): string {
	const m = (text ?? '').match(/^(\S+) → (not found\.|\S+ → (?:refused|no answer) on port )/);
	if (!m) return '';
	const name = m[1].toLowerCase();
	let row = 'Not answering';
	if (m[2] === 'not found.') {
		if (name.endsWith('.local')) row = 'A .local name like';
		else if (name.endsWith('.ts.net')) row = 'A Tailscale name';
		else if (name.includes('.')) row = 'A Pi-hole or router name';
		else row = 'A container name like';
	}
	return `${connectHelp}:~:text=${encodeURIComponent(row).replace(/-/g, '%2D')}`;
}
