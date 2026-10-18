// Shapes the HolyArrs API sends. Records straight from the database
// (downloads, jobs, events, imports) keep their Go field names.

export interface CopyFile {
	ID: number; VersionID: number; RelativePath: string; Size: number; AddedAt: string; State?: string; Protected?: boolean; MediaInfo?: Record<string, unknown> | null; ReleaseName?: string | null; Quality?: Record<string, unknown> | null;
}

export interface SlotView {
	id: number;
	version: string;
	version_id: number;
	state: string;
	state_label: string;
	reason?: string;
	active_download_id?: number;
	/** The active (or last) Download in plain words. */
	download?: import('./downloaders.ts').DownloadView;
	file?: Omit<CopyFile, "ID" | "VersionID"> & Partial<Pick<CopyFile, "ID" | "VersionID">> | null;
}

export interface TitleView {
	id: number;
	kind: string;
	name: string;
	year?: number;
	overview?: string;
	original_language?: string;
	ids: Record<string, string> | null;
	keep_up_to_date: boolean;
	why_added: string;
	release_gate: string;
	gate_delay_days: number;
	gate_opens_at?: string;
	cinema_date?: string;
	digital_date?: string;
	physical_date?: string;
	refreshed_at?: string;
	stage: string;
	gate_open: boolean;
	gate_reason: string;
	library_folder_id?: number;
	folder_path?: string;
	pick_rules_id?: number;
	pick_rules_name?: string;
	artwork: string[];
	/** Wide backdrop art (TMDB w1280), when known. */
	backdrop?: string;
	/** Title logo art (transparent PNG), when known. */
	logo?: string;
	genres?: string[];
	runtime_minutes?: number;
	/** Age rating, e.g. PG-13. */
	rating?: string;
	versions: VersionView[];
	slots: SlotView[];
	/** Wishlist asks that led to it: who asked and who decided. */
	asks?: Ask[];
	/** A show's choices and season summaries (shows only). */
	show?: ShowInfo;
	/** An artist's choices and counts (artists only). */
	artist?: ArtistInfo;
}

/** ArtistInfo is what an artist's page needs besides the shared Title fields. */
export interface ArtistInfo {
	mbid: string;
	type?: string;
	disambiguation?: string;
	status: string;
	album_choice?: string;
	new_albums: string;
	album_types: string[];
	secondary_types: string[];
	release_statuses: string[];
	albums: number;
	included: number;
	tracks: number;
	in_library: number;
	missing: number;
	not_out_yet: number;
	better_wanted: number;
	next_release?: string;
}

/** AlbumSummary is one album of an artist with its track counts. */
export interface AlbumSummary {
	id: number;
	mbid: string;
	name: string;
	type: string;
	secondary_types: string[];
	disambiguation?: string;
	release_date?: string;
	included: boolean;
	keep_up_to_date: boolean;
	dropped?: boolean;
	pressings: number;
	pressing?: string;
	pressing_id?: number;
	tracks: number;
	in_library: number;
	missing: number;
	better_wanted: number;
	not_out_yet: number;
	state: string;
}

export interface PressingTrack {
	id: string;
	recording_id: string;
	disc: number;
	number: number;
	position: number;
	name: string;
	length_ms?: number;
}

/** PressingView is one Pressing (edition) of an album with its tracks. */
export interface PressingView {
	id: number;
	mbid: string;
	name: string;
	status: string;
	country?: string;
	date?: string;
	disambiguation?: string;
	formats: string[];
	track_count: number;
	chosen: boolean;
	chosen_by?: string;
	tracks: PressingTrack[];
}

export interface TrackView {
	id: number;
	disc: number;
	number: number;
	name?: string;
	length_ms?: number;
	recording_id?: string;
	release_date?: string;
	keep_up_to_date: boolean;
	dropped?: boolean;
	ids: Record<string, string>;
	slots: EpisodeSlot[];
}

/** AskableAlbum is an album of a looked-up artist as the ask dialog shows it. */
export interface AskableAlbum {
	mbid: string;
	name: string;
	type: string;
	secondary_types: string[];
	release_date?: string;
	kept: boolean;
	left_out: boolean;
}

export interface AskAlbum {
	mbid: string;
	name: string;
	year?: number;
}

/** LookupArtist is a MusicBrainz artist with what the Library knows of it. */
export interface LookupArtist {
	mbid: string;
	name: string;
	sort_name?: string;
	type?: string;
	disambiguation?: string;
	country?: string;
	status: string;
	score?: number;
	title_id?: number;
	never_add?: boolean;
	ask?: { id: number; state: AskState; state_label: string; mine: boolean; people: number; albums?: AskAlbum[] };
	ask_albums?: AskableAlbum[];
}

/** MusicSettings are the add-an-artist defaults and MusicBrainz settings. */
export interface MusicSettings {
	album_choice: string;
	new_albums: string;
	album_types: string[];
	secondary_types: string[];
	release_statuses: string[];
	pressing_rule: string;
	musicbrainz_url: string;
	cache_hours: number;
}

/** SeasonSummary is one season of a show with its episode counts. */
export interface SeasonSummary {
	number: number;
	name: string;
	keep_up_to_date: boolean;
	episodes: number;
	aired: number;
	in_library: number;
	missing: number;
	not_out_yet: number;
	better_wanted: number;
	next_air_date?: string;
	dropped?: boolean;
}

/** ShowInfo is what a show's page needs besides the shared Title fields. */
export interface ShowInfo {
	details_source: string;
	episode_order: string;
	season_choice?: string;
	new_seasons: string;
	status: string;
	network?: string;
	air_time?: string;
	time_zone?: string;
	seasons: SeasonSummary[];
	episodes: number;
	missing: number;
	not_out_yet: number;
	in_library: number;
	better_wanted: number;
	next_air_date?: string;
}

export interface EpisodeSlot {
	id: number;
	version: string;
	version_id: number;
	state: string;
	state_label: string;
	reason?: string;
	file_id?: number;
}

export interface EpisodeView {
	id: number;
	season: number;
	episode: number;
	absolute?: number;
	name?: string;
	air_date?: string;
	air_day?: string;
	runtime?: number;
	runtime_minutes?: number;
	/** A still from the episode (w780), when known. */
	still?: string;
	keep_up_to_date: boolean;
	dropped?: boolean;
	ids: Record<string, string>;
	slots: EpisodeSlot[];
}

/** LookupShow is a looked-up show (without episodes) with what the Library knows. */
export interface LookupShow {
	source: string;
	name: string;
	year?: number;
	overview?: string;
	status: string;
	network?: string;
	runtime?: number;
	ids: Record<string, string>;
	poster?: string;
	seasons?: { number: number; name?: string }[];
	title_id?: number;
	never_add?: boolean;
	/** The open Wishlist ask for it, if someone asked. */
	ask?: AskBrief;
}

export interface ShowSettings {
	season_choice: string;
	new_seasons: string;
	episode_order: string;
	details_source: string;
	keep_specials: boolean;
	episode_wait_minutes: number;
}

/** VersionView is one wanted copy of a title, such as HD or 4K. */
export interface VersionView {
	edition?: string;
	audio_languages?: string[];
	id: number;
	label: string;
	main: boolean;
	pick_rules_id: number;
	pick_rules_name: string;
	library_folder_id: number;
	folder_path: string;
	keep_up_to_date: boolean;
}

export interface Movie {
	name: string;
	year: number;
	overview: string;
	original_language: string;
	runtime?: number;
	ids: Record<string, string>;
	poster?: string;
	cinema_date?: string;
	digital_date?: string;
	physical_date?: string;
}

export interface LookupMovie extends Movie {
	title_id?: number;
	never_add: boolean;
	/** The open Wishlist ask for it, if someone asked. */
	ask?: AskBrief;
}

export type AskState = 'asked' | 'approved' | 'turned_down' | 'available';

export interface AskBrief {
	id: number;
	state: AskState;
	state_label: string;
	mine: boolean;
	people: number;
	/** A show ask's seasons; empty is the whole show. */
	seasons?: number[];
}

export interface Who {
	id: number;
	name: string;
}

/** Ask is one movie or show someone asked for on the Wishlist. */
export interface Ask {
	id: number;
	kind: string;
	tmdb_id: string;
	/** The id it is added by, such as tmdb:693134 or tvmaze:82. */
	ref?: string;
	/** A show ask's seasons; empty is the whole show. */
	seasons?: number[];
	/** For a show: "the whole show", "season 2", "seasons 1 and 2". */
	season_words?: string;
	/** An artist ask's albums; empty is the whole artist. */
	albums?: AskAlbum[];
	/** For an artist: "every album", or the albums' names. */
	album_words?: string;
	name: string;
	year?: number;
	poster?: string;
	state: AskState;
	state_label: string;
	title_id?: number;
	asked_by: Who;
	asked_at: string;
	people: (Who & { asked_at: string })[];
	decided_by?: Who;
	decided_at?: string;
	note?: string;
	auto_approved: boolean;
	available_at?: string;
	mine: boolean;
}

/** The kinds a weekly limit counts: each person has one limit per kind. */
export type LimitKind = 'movies' | 'shows' | 'music';

export interface AskKindLimit {
	/** 0 is no limit. */
	per_week: number;
	used: number;
	left: number;
	/** When the next ask of this kind frees up, if none are left. */
	retry_at?: string;
	no_limit: boolean;
}

export interface AskLimit {
	/** No limit at all (people who may approve). */
	no_limit: boolean;
	movies: AskKindLimit;
	shows: AskKindLimit;
	music: AskKindLimit;
}

/** A person's own weekly limit per kind; null takes that kind's default. */
export interface PersonLimit {
	movies_per_week: number | null;
	shows_per_week: number | null;
	music_per_week: number | null;
}

export interface AskSettings {
	movies_per_week: number;
	shows_per_week: number;
	music_per_week: number;
	library_folder_id: number;
	pick_rules_id: number;
	search_on_approve: boolean;
	show_library_folder_id: number;
	show_pick_rules_id: number;
	music_library_folder_id: number;
	music_pick_rules_id: number;
}

export interface AppMessage {
	id: number;
	created_at: string;
	kind: 'approved' | 'turned_down' | 'ready';
	text: string;
	ask_id?: number;
	title_id?: number;
	read_at?: string;
}

export type Moment =
	| 'ask_waiting'
	| 'ask_approved'
	| 'ask_turned_down'
	| 'ask_ready'
	| 'grab'
	| 'import'
	| 'rename'
	| 'add'
	| 'delete'
	| 'health'
	| 'needs_decision';

export interface AlertView {
	id: number;
	name: string;
	kind: 'discord' | 'email' | 'webhook';
	enabled: boolean;
	moments: Moment[];
	secret?: string;
	webhook_auth?: 'sign' | 'bearer' | 'none';
	url?: string;
	smtp_host?: string;
	smtp_port?: number;
	smtp_security?: 'starttls' | 'tls' | 'none';
	smtp_from?: string;
	smtp_user?: string;
	smtp_pass?: string;
	staff_to?: string;
	allow_local_network?: boolean;
	last_sent_at?: string;
	last_error?: string;
}

export interface LibraryFolder {
	id: number;
	path: string;
	kind: string;
	label: string;
	default_pick_rules_id: number | null;
	free_space: number;
	health: string;
	health_reason: string;
	health_checked_at?: string;
}

/** Reason is one line of a verdict in plain words. */
export interface Reason {
	/** step.rule, stable for the screens */
	code: string;
	/** pass, fail, add, subtract, warn, hold, rank */
	effect: string;
	text: string;
}

/** ReleaseView is a stored Release with its verdict and reasons. */
export interface ReleaseView {
	id: number;
	name: string;
	size: number;
	seeders?: number;
	peers?: number;
	protocol: string;
	/** seen, judged, chosen, held, passed_over, refused, turned_down */
	state: string;
	found_by: string;
	source_id: number;
	source_name: string;
	slot_id?: number;
	version?: string;
	title_id?: number;
	title_name?: string;
	published_at?: string;
	hold_until?: string;
	/** chosen, held, passed_over, refused */
	outcome: string;
	passed: boolean;
	rank: number;
	points: number;
	step?: string;
	good_enough: boolean;
	needs_force: boolean;
	gate_skipped: boolean;
	reasons: Reason[];
	created_at: string;
	/** Shows: the episodes it holds or was picked for ("S01E02", "S01 (10 episodes)"). */
	episode?: string;
	/** Shows: every Slot it was matched or picked for, when more than one. */
	slot_ids?: number[];
	/** Shows: the episodes it was turned down for one by one. */
	turned_down_for?: string;
	/** Artists: the album it is for. */
	album?: string;
}

export interface ReleasePage {
	releases: ReleaseView[];
	total: number;
	limit: number;
	offset: number;
	running_job_ids?: number[];
}

/** Choice is what choosing a Release by hand answered. */
export interface Choice {
	release_id: number;
	slot_id: number;
	/** A season pack or multi-episode Release: every Slot it was chosen for. */
	slot_ids?: number[];
	forced: boolean;
	warnings: string[];
}

/** SearchView is one search for Home: how far it is and what it found. */
export interface SearchView {
	job_id: number;
	kind: 'search_slot' | 'search_missing' | 'feed_check' | 'search_season' | 'search_album';
	state: string;
	by_hand: boolean;
	only_show: boolean;
	title_id?: number;
	title_name?: string;
	version?: string;
	/** Shows: the episode (S01E02) or season (S01) searched. */
	episode?: string;
	/** Artists: the album searched. */
	album?: string;
	progress: number;
	note?: string;
	started_at?: string;
	ended_at?: string;
	found: number;
	refused: number;
	outcome?: 'chosen' | 'held' | 'shown' | 'none' | 'failed' | 'episodes' | 'album';
	picked?: string;
	error?: string;
}

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

export interface Download {
	ID: number;
	DownloaderID: number;
	ExternalID: string;
	Progress: number;
	Speed: number;
	ETA: number | null;
	State: string;
	StateReason: string | null;
	FilesSeen: { name?: string; size?: number }[] | null;
	SavePath: string;
	StartedAt: string | null;
	FinishedAt: string | null;
}

export interface ImportRecord {
	ID: number;
	State: string;
	CreatedAt: string;
	Plan?: { source_file: string; target_path?: string; confidence?: number }[] | null;
	Result?: { source_file: string; outcome: string; placed_by?: string; error?: string }[] | null;
}

export interface EventRecord {
	ID: number;
	At: string;
	Kind: string;
	Summary: string;
	TitleID: number | null;
	SlotID?: number | null;
	DownloadID?: number | null;
	Data?: Record<string, unknown>;
	Actor?: { kind: string; id?: number };
	title_name?: string;
	/** A show Event: the episodes it is about ("S01E02 The Mill"). */
	episode?: string;
	/** The Version (such as 4K) it is about, when the title has more than one. */
	version?: string;
	/** Optional title artwork supplied with an Event row. */
	artwork?: string;
}

export interface EventPage {
	events: EventRecord[];
	/** With an episode filter: the episode, named ("S01E02 The Mill"). */
	episode?: string;
	total: number;
	limit: number;
	offset: number;
}

export interface HealthCheck {
	id: number;
	key: string;
	kind: 'library_folder' | 'disk_space' | 'source' | 'downloader' | 'backups' | 'update';
	subject: string;
	state: 'ok' | 'warning' | 'error';
	message: string;
	since: string;
	checked_at: string;
}

/** One stage or linked app and its state (GET /health/states). */
export interface StageState {
	key: string;
	kind: 'stage' | 'source' | 'downloader' | 'subtitle_site' | 'media_server' | 'part';
	name: string;
	state: 'off' | 'starting' | 'running' | 'retrying' | 'failed';
	reason: string;
	since: string;
	stuck: boolean;
	restarts: number;
}

export interface HealthStates {
	safe_mode: boolean;
	states: StageState[];
}

export interface HealthChecks {
	checks: HealthCheck[];
	worst: 'ok' | 'warning' | 'error';
}

export interface HealthSettings {
	every_minutes: number;
	flap_guard: number;
	low_space_mb: number;
	update_check_url: string;
	update_check?: boolean;
	update_check_local?: boolean;
	update_prerelease?: boolean;
	update_check_default?: string;
}

export interface UpdateInfo {
	on: boolean;
	feed: string;
	checked_at?: string;
	latest?: string;
	newer: boolean;
	link?: string;
	no_release?: boolean;
	problem?: string;
	version: string;
}

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
	last_ok_at?: string;
	last_error?: string;
}

export interface CalendarEntry {
	title_id: number;
	name: string;
	year?: number;
	kind: 'cinema' | 'digital' | 'physical' | 'episode' | 'album';
	date: string;
	/** An album: name is the artist; the album, its kind and how many tracks are in the Library. */
	part_id?: number;
	album?: string;
	album_type?: string;
	tracks?: number;
	have?: number;
	/** An episode: date is its exact air time when timed, else air_day is the day. */
	item_id?: number;
	season?: number;
	episode?: number;
	episode_name?: string;
	air_day?: string;
	timed?: boolean;
	runtime?: number;
	network?: string;
	state?: string;
	keep_up_to_date?: boolean;
	/** A show with more than one Version: each Version's state, main first. */
	versions?: { label: string; state: string }[];
}

export interface Health {
	ok: boolean;
	version: string;
	schema: number;
	uptime_s: number;
	write_wait_p95_ms: number;
	write_wait_samples: number;
	writes: number;
	sources: { name: string; state: string }[] | null;
	downloaders: { name: string; state: string }[] | null;
	live_clients: number;
	url_base?: string;
}

export interface SourceHealth {
	state: 'working' | 'unreachable' | 'auth_failed' | 'disabled';
	text: string;
	until?: string;
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

export interface BackupInfo {
	name: string;
	kind: string;
	created_at: string;
	size: number;
}

export interface ApiKey {
	id: number;
	label: string;
	hint: string;
	created_at: string;
	last_used_at?: string;
}

export interface PickRulesView {
	id: number;
	name: string;
	kinds: string[];
	preset: boolean;
	/** How many places use these rules, all kinds together. */
	in_use?: number;
}

/** PickRulesStep is one rung of a quality ladder, on or off. */
export interface PickRulesStep {
	quality: string;
	allowed: boolean;
}

/** PickRulesSize bounds a Release's size in bytes; with per_minute, per minute of runtime. */
export interface PickRulesSize {
	min: number;
	max: number;
	per_minute: boolean;
}

/** PickRulesBody is what a set of Pick rules takes (internal/domain/pickrules.go). */
export interface PickRulesBody {
	steps: PickRulesStep[];
	good_enough_at: string;
	traits: { trait: string; points: number }[];
	must_have: string[];
	never: string[];
	min_points: number;
	good_enough_points?: number;
	/** By step; "*" is every step. */
	size_limits: Record<string, PickRulesSize>;
	protocol: { prefer: 'torrent' | 'usenet' | 'either'; wait_minutes_torrent: number; wait_minutes_usenet: number; skip_wait_at_good_enough: boolean };
	language: { wanted: string[]; original_ok: boolean };
	order: 'quality_then_points' | 'points_then_quality';
	upgrade_min_points: number;
	proper_policy: { mode: 'take_any' | 'same_group_only' | 'prefer_only' | 'ignore'; max_age_days?: number };
}

/** PickRulesUse counts what uses one set of Pick rules. */
export interface PickRulesUse {
	versions: number;
	library_folders: number;
	outside_lists: number;
	wishlist: number;
	title_defaults: number;
}

/** PickRulesDetail is one set of Pick rules with its body and where it is used. */
export interface PickRulesDetail {
	id: number;
	name: string;
	kinds: string[];
	preset: boolean;
	revision: number;
	body: PickRulesBody;
	in_use: PickRulesUse;
}

/** KindDefaults is what a new movie or show gets; 0 means the first folder and the folder's own Pick rules. */
export interface KindDefaults {
	library_folder_id: number;
	pick_rules_id: number;
	keep_up_to_date: boolean;
	search_on_add: boolean;
}

/** TitleDefaults are the Movies and TV defaults (GET /settings/title-defaults). */
export interface TitleDefaults {
	movie: KindDefaults;
	show: KindDefaults;
}

export interface GateDefaults {
	release_gate: string;
	gate_delay_days: number;
	bulk_search_respects_gate: boolean;
}

/** OutsideList is a TMDB or Trakt list whose movies are added on a schedule. */
export interface OutsideList {
	id: number;
	name: string;
	kind: 'tmdb_list' | 'trakt_list';
	ref: string;
	library_folder_id: number;
	pick_rules_id: number | null;
	keep_up_to_date: boolean;
	search_on_add: boolean;
	release_gate: string;
	enabled: boolean;
	sync_every_hours: number;
	last_synced_at?: string;
	last_result: string;
	last_error: string;
}

/** NeverAddEntry is a title outside lists must never add. */
export interface NeverAddEntry {
	id: number;
	kind: string;
	ids: Record<string, string>;
	name: string;
	reason: string;
	created_at: string;
}
