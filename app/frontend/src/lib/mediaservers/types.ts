export interface MediaServerView {
	id: number;
	name: string;
	kind: 'plex' | 'jellyfin' | 'emby' | 'subsonic' | 'kodi';
	url: string;
	token: string;
	// Sign-in name of a Subsonic-style music server or Kodi; only sent, never shown.
	username?: string;
	enabled: boolean;
	on_import: boolean;
	on_rename: boolean;
	on_delete: boolean;
	path_from: string;
	path_to: string;
	allow_local_network: boolean;
	web_url: string;
	// Subsonic only: send the password itself (LDAP servers), over https or a local address.
	plain_password: boolean;
	last_ok_at?: string;
	last_error?: string;
}

// Where a Library title opens on one media server (GET /media-servers/watch-links).
export interface WatchLink {
	server: string;
	kind: 'plex' | 'jellyfin' | 'emby';
	url: string;
}
