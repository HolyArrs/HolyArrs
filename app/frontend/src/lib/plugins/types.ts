// Shapes of the plugin routes (GET /api/v1/plugins).

export interface PluginAccess {
	hosts: string[] | null;
	folders: FolderReq[] | null;
	actions: string[] | null;
	/** Moments the plugin is told as they happen, such as "library.add"; missing on older plugins. */
	events?: string[] | null;
}

/** One folder a plugin asks for; the owner picks the real folder. */
export interface FolderReq {
	name: string;
	mode: 'read' | 'read_write';
	why?: string;
	required?: boolean;
}

/** A folder the owner chose for a plugin. */
export interface FolderGrant {
	path: string;
	mode: 'read' | 'read_write';
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
	/** The robot's copy of the logo on the plugin site: images/<id>/icon-256.webp. Wins over icon. */
	icon_url?: string;
	access: PluginAccess | null;
	reaches_app?: boolean;
	/** Optional page fields from the signed manifest. */
	long_description?: string;
	how_it_works?: string[];
	whats_new?: string;
	/** GitHub logins. */
	developers?: string[];
	/** YYYY-MM-DD. */
	updated?: string;
	licence?: string;
	repo?: string;
	category?: string;
	/** CPU types, such as linux-amd64. */
	platforms?: string[];
	/** How people reach the author for help. */
	support_email?: string;
	support_url?: string;
	/** Public numbers from stats.json; any part may be missing. */
	/** listed: when the plugin was first listed (ISO time); drives the New tag. */
	stats?: { downloads?: number; week?: number; stars?: number; updated?: string; listed?: string };
	/** The plugin file's size in bytes for this CPU type. */
	size?: number;
	/** An install under way: downloading, checking or starting. */
	install_step?: 'downloading' | 'checking' | 'starting';
	/** Why the last install failed, until the next try. */
	install_error?: string;
	/** What the installed version may reach, when a newer version is listed. */
	installed_access?: PluginAccess;
	/** The app version the listed version needs, when newer than this app. */
	needs_app?: string;
	/** Fields the plugin declares for its settings form. */
	settings?: PluginSetting[];
	/** What the installed version asks for beyond what the owner approved: "host:x", "event:x", "folder:x" or "choose:x". */
	needs_approval?: string[];
	/** The folders the owner chose, by folder name. */
	grants?: Record<string, FolderGrant>;
	/** Hosts the owner added (a self-hosted Headscale), "name" or "name:port". */
	owner_hosts?: string[];
	/** How often it tried to reach a blocked address since it started. */
	blocked_count?: number;
	/** A plugin from the developer folder (unsigned); dev_error says why its files can't be read. */
	dev?: boolean;
	dev_error?: string;
	/** An installed test version; test_version is a newer test version; test_on is the owner's choice to get them. */
	test?: boolean;
	test_version?: string;
	test_on?: boolean;
	/** The API scopes it asks for, as install lines: one per group, with red warnings. */
	scope_groups?: ScopeGroup[];
	/** The installed version's asked caps, and the owner's own caps. */
	resources?: PluginResources;
	owner_limits?: PluginResources;
}

/** One install line: an area, read or read and change, the exact scopes and the red warnings. */
export interface ScopeGroup {
	id: string;
	change: boolean;
	scopes: string[];
	warnings?: string[];
}

/** Resource caps; a missing field keeps the default. */
export interface PluginResources {
	memory_mib?: number;
	cpus?: number;
	processes?: number;
	open_files?: number;
}

/** Why the system can't give plugins the file lock. */
export type FileLockReason = 'old_kernel' | 'blocked' | 'turned_off';

export interface PluginOverview {
	key_set: boolean;
	blocklist_checked_at: string | null;
	blocklist_stale: boolean;
	/** The list arrived but failed its signature check. */
	list_bad?: boolean;
	can_run: boolean;
	/** The system can't give plugins the file lock (Landlock); plugins start only when the owner allows each one. */
	no_file_lock?: boolean;
	file_lock_reason?: FileLockReason;
	/** The owner's one choice for the whole install: signed plugins may run without the file lock. */
	allow_no_file_lock?: boolean;
	/** The owner answered the file lock question. */
	no_file_lock_asked?: boolean;
	/** The system can't keep plugins to the websites they list (network lock); the same choice as the file lock. */
	no_net_lock?: boolean;
	net_lock_reason?: FileLockReason;
	allow_no_net_lock?: boolean;
	no_net_lock_asked?: boolean;
	/** The owner runs unsigned plugins from dev_dir. */
	dev_mode?: boolean;
	dev_dir?: string;
	plugins: Plugin[];
}

/** GET /api/v1/plugins/{id}/remote/status: a forwarding plugin's own status file. */
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

/** One declared settings field. Values travel as text: toggles are "true"/"false". */
export interface PluginSetting {
	key: string;
	label: string;
	type: 'text' | 'number' | 'toggle' | 'select' | 'secret';
	default?: string;
	options?: string[];
	help?: string;
}

/** GET/PUT /api/v1/plugins/{id}/settings: secrets show only as set. */
export interface SettingsView {
	fields: PluginSetting[];
	values: Record<string, string>;
	secrets_set: Record<string, boolean>;
	/** Saved, but the running plugin did not start again with it. */
	start_error?: string;
}
