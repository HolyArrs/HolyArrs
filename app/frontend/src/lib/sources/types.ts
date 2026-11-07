export interface SearchSettings {
	feed_check_minutes: number;
	missing_search_count: number;
	missing_search_better: boolean;
	try_next_downloader: boolean;
	missing_search_shows: boolean;
	season_pack_upgrade: 'all' | 'any';
	season_pack_prefer: 'rank' | 'always';
	multi_season_packs: boolean;
	missing_search_music: boolean;
	music_discography_packs: boolean;
	album_track_tolerance: number;
	refuse_early_releases: boolean;
	early_release_grace_hours: number;
	refuse_tiny_releases: boolean;
	import_min_video_mb: number;
	import_min_track_kb: number;
	import_size_floors: boolean;
	hold_unknown_releases: boolean;
	episode_default_minutes: number;
}

export interface SourceHealth {
	state: 'working' | 'unreachable' | 'auth_failed' | 'disabled';
	text: string;
	until?: string;
	reason?: 'slowed' | 'down' | 'blocked' | 'key';
	block?: string;
}

export interface SourceUse {
	feed: boolean;
	auto_search: boolean;
	hand_search: boolean;
}

export interface SourceLimits {
	searches_per_window?: number;
	grabs_per_window?: number;
	window_seconds?: number;
	gap_ms?: number;
	timeout_ms?: number;
}

export interface SourceSummary {
	last_success: string | null;
	last_failure: string | null;
	calls_24h: number;
	failures_24h: number;
	avg_ms_24h: number;
}

export interface SourceView {
	id: number;
	name: string;
	protocol: 'torrent' | 'usenet';
	format: 'torznab' | 'newznab' | 'rss' | 'bridge';
	url: string;
	api_key: string;
	use_for: SourceUse;
	kinds: string[] | null;
	categories: Record<string, number[]> | null;
	min_seeders: number | null;
	limits: SourceLimits;
	tags?: string[];
	state: string;
	health: SourceHealth;
	priority: number;
	allow_local_network: boolean;
	proxy: string;
	proxy_password: string;
	proxy_allow_local_network: boolean;
	summary: SourceSummary;
}

export interface SourceDay {
	day: string;
	calls: number;
	failures: number;
	grabs: number;
	avg_ms: number;
}

export interface SourceCall {
	at: string;
	op: 'caps' | 'search' | 'feed' | 'test';
	ok: boolean;
	ms: number;
	error_kind: string;
}
