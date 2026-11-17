export interface MediaServerView {
	id: number;
	name: string;
	kind: 'plex' | 'jellyfin' | 'emby';
	url: string;
	token: string;
	enabled: boolean;
	on_import: boolean;
	on_rename: boolean;
	on_delete: boolean;
	path_from: string;
	path_to: string;
	allow_local_network: boolean;
	web_url: string;
	last_ok_at?: string;
	last_error?: string;
}

// Where a Library title opens on one media server (GET /media-servers/watch-links).
export interface WatchLink {
	server: string;
	kind: 'jellyfin' | 'emby';
	url: string;
}
