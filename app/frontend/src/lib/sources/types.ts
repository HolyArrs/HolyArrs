export interface SearchSettings {
	feed_check_minutes: number;
	missing_search_count: number;
	missing_search_better: boolean;
	try_next_downloader: boolean;
	missing_search_shows: boolean;
	/** all: every episode a season pack holds must gain; any: one is enough. */
	season_pack_upgrade: 'all' | 'any';
	/** rank: a season pack wins ties; always: a passing pack comes first. */
	season_pack_prefer: 'rank' | 'always';
	multi_season_packs: boolean;
	/** The nightly and bulk searches cover albums. */
	missing_search_music: boolean;
	/** A Release with a whole discography may fill an album. */
	music_discography_packs: boolean;
	/** Files a Release may lack against the album's tracks; -1 off. */
	album_track_tolerance: number;
	/** Early and fake Releases (4.3.5). */
	refuse_early_releases: boolean;
	early_release_grace_hours: number;
	refuse_tiny_releases: boolean;
	/** Least video and track file placed at import; 0 uses the fixed floors. */
	import_min_video_mb: number;
	import_min_track_kb: number;
	/** The fixed least sizes at import and the judged-size check; off leaves empty files and the sizes set. */
	import_size_floors: boolean;
	/** Automatic searches hold a Release of unknown size or dates for a person; off warns only. */
	hold_unknown_releases: boolean;
	/** Length taken for an episode of unknown length by the size floor; 1 to 240. */
	episode_default_minutes: number;
}

export interface SourceHealth {
	state: 'working' | 'unreachable' | 'auth_failed' | 'disabled';
	text: string;
	until?: string;
	/** Why it waits: slowed, down, blocked or key. */
	reason?: 'slowed' | 'down' | 'blocked' | 'key';
	/** A site's protection stops it: bot_check or site_ban (#106). */
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
	/** With any, the Source serves only titles sharing one (#108). */
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
