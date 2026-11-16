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
	access: PluginAccess | null;
}

export interface ActiveLimits {
	Cgroup: boolean;
	Pids: boolean;
	Memory: boolean;
	CPU: boolean;
	GroupKill: boolean;
}

export interface PluginOverview {
	key_set: boolean;
	blocklist_checked_at: string | null;
	blocklist_stale: boolean;
	can_run: boolean;
	limits: ActiveLimits;
	plugins: Plugin[];
}
