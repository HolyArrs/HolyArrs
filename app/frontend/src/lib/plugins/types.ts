// Shapes of the plugin routes (GET /api/v1/plugins).

export interface PluginAccess {
	hosts: string[] | null;
	folders: string[] | null;
	actions: string[] | null;
}

export interface Plugin {
	id: string;
	version: string;
	installed: boolean;
	installed_version?: string;
	running: boolean;
	enabled: boolean;
	disabled_reason?: string;
	name?: string;
	author?: string;
	description?: string;
	icon?: string;
	access: PluginAccess | null;
	reaches_app?: boolean;
}

export interface PluginOverview {
	key_set: boolean;
	blocklist_checked_at: string | null;
	blocklist_stale: boolean;
	/** The list arrived but failed its signature check. */
	list_bad?: boolean;
	can_run: boolean;
	plugins: Plugin[];
}

/** GET /api/v1/plugins/tailscale/status: the plugin's own status file. */
export interface TailscaleStatus {
	state: string;
	login_url?: string;
	login_qr?: string[];
	device_name?: string;
	tsnet_name?: string;
	tailscale_version?: string;
	last_updated?: string;
	login_expiry?: string;
	connection?: string;
}
