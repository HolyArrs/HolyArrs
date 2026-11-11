// Shapes of the API answers the frontend reads.

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
	download?: DownloadView;
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
	tags?: string[];
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
	backdrop?: string;
	logo?: string;
	genres?: string[];
	runtime_minutes?: number;
	rating?: string;
	// Members' average out of 10 and its vote count (not the age rating); missing when not known.
	score?: number;
	votes?: number;
	added_at?: string;
	size_bytes?: number;
	copy?: TitleCopy;
	last_activity?: string;
	versions: VersionView[];
	slots: SlotView[];
	asks?: Ask[];
	show?: ShowInfo;
	artist?: ArtistInfo;
}

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
	downloading?: number;
	failed?: number;
	next_release?: string;
	bio?: string;
}

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

export interface AskAlbum {
	mbid: string;
	name: string;
	year?: number;
	songs?: string[];
}

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
	downloading?: number;
	failed?: number;
	next_air_date?: string;
	dropped?: boolean;
}

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
	downloading?: number;
	failed?: number;
	next_air_date?: string;
	season_count?: number;
	last_air_date?: string;
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
	still?: string;
	keep_up_to_date: boolean;
	dropped?: boolean;
	ids: Record<string, string>;
	slots: EpisodeSlot[];
}

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
	ask?: AskBrief;
	ask_4k?: AskBrief;
	has_4k?: boolean;
}

export type AskState = 'asked' | 'approved' | 'turned_down' | 'available';

export interface AskBrief {
	id: number;
	state: AskState;
	state_label: string;
	mine: boolean;
	people: number;
	seasons?: number[];
}

export interface Who {
	id: number;
	name: string;
	display_name?: string;
}

export interface AskStep {
	kind: 'searching' | 'downloading' | 'importing' | 'waiting_on_admin' | 'waiting' | 'not_out';
	text?: string;
	searched_at?: string;
	next_search?: string;
}

export interface Ask {
	id: number;
	kind: string;
	tmdb_id: string;
	ref?: string;
	seasons?: number[];
	season_words?: string;
	episodes?: { season: number; episodes: number[] }[];
	future_only?: boolean;
	four_k?: boolean;
	albums?: AskAlbum[];
	album_words?: string;
	artist_id?: string;
	name: string;
	year?: number;
	poster?: string;
	state: AskState;
	state_label: string;
	title_id?: number;
	asked_by: Who;
	asked_at: string;
	people: (Who & { asked_at: string; from_watchlist?: boolean })[];
	decided_by?: Who;
	also_declined?: Who[];
	step?: AskStep;
	decided_at?: string;
	note?: string;
	auto_approved: boolean;
	available_at?: string;
	problem?: string;
	mine: boolean;
}

export interface AppMessage {
	id: number;
	created_at: string;
	kind: 'approved' | 'turned_down' | 'ready' | 'fixed';
	text: string;
	ask_id?: number;
	title_id?: number;
	read_at?: string;
	report_id?: number;
}

export type Moment =
	| 'ask_waiting'
	| 'ask_approved'
	| 'ask_turned_down'
	| 'ask_ready'
	| 'new_episodes'
	| 'new_albums'
	| 'report_made'
	| 'report_fixed'
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
	kind: 'discord' | 'email' | 'webhook' | 'telegram' | 'ntfy' | 'gotify' | 'pushover';
	enabled: boolean;
	moments: Moment[];
	secret?: string;
	bot_token?: string;
	chat_id?: string;
	server_url?: string;
	topic?: string;
	user_key?: string;
	app_token?: string;
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
	copies_expected?: boolean;
}

export interface Reason {
	code: string;
	effect: string;
	text: string;
}

export interface ReleaseView {
	id: number;
	name: string;
	size: number;
	seeders?: number;
	peers?: number;
	protocol: string;
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
	episode?: string;
	slot_ids?: number[];
	turned_down_for?: string;
	album?: string;
}

export interface ReleasePage {
	releases: ReleaseView[];
	total: number;
	limit: number;
	offset: number;
	running_job_ids?: number[];
}

export interface Choice {
	release_id: number;
	slot_id: number;
	slot_ids?: number[];
	forced: boolean;
	warnings: string[];
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
	actor_name?: string;
	title_name?: string;
	episode?: string;
	version?: string;
	artwork?: string;
}

export interface EventPage {
	events: EventRecord[];
	episode?: string;
	total: number;
	limit: number;
	offset: number;
}

export interface HealthCheck {
	id: number;
	key: string;
	kind: 'library_folder' | 'disk_space' | 'source' | 'downloader' | 'backups' | 'update' | 'config_disk' | 'config_folder' | 'waiting_downloads';
	subject: string;
	state: 'ok' | 'warning' | 'error';
	message: string;
	since: string;
	checked_at: string;
	confirmed?: 'ok' | 'warning' | 'error';
}

export interface CalendarEntry {
	title_id: number;
	name: string;
	year?: number;
	kind: 'cinema' | 'digital' | 'physical' | 'episode' | 'album';
	date: string;
	part_id?: number;
	album?: string;
	album_type?: string;
	tracks?: number;
	have?: number;
	item_id?: number;
	season?: number;
	episode?: number;
	episode_name?: string;
	tmdb?: string;
	air_day?: string;
	timed?: boolean;
	runtime?: number;
	network?: string;
	state?: string;
	keep_up_to_date?: boolean;
	versions?: { label: string; state: string }[];
	poster?: string;
	backdrop?: string;
	still?: string;
	genres?: string[];
	directors?: string[];
	cast?: string[];
	collection?: string;
	studio?: string;
	cinema_date?: string;
	home_date?: string;
	length_min?: number;
	nth?: number;
	following?: boolean;
	followers?: CalendarFollower[];
	followers_hidden?: number;
}

export interface CalendarFollower {
	id: number;
	name: string;
	avatar?: string;
	me?: boolean;
}

export interface PickRulesView {
	id: number;
	name: string;
	kinds: string[];
	preset: boolean;
	in_use?: number;
}

export interface PickRulesStep {
	quality: string;
	allowed: boolean;
}

export interface TitleCopy {
	file_id: number;
	version_id: number;
	codec: string;
	container: string;
	audio_languages: string[];
	protected: boolean;
}

export interface DownloadView {
	id: number;
	state: string;
	label: string;
	text: string;
	progress: number;
	speed: number;
	up_speed?: number;
	seeds?: number;
	peers?: number;
	seeds_total?: number;
	peers_total?: number;
	eta?: number;
	active: boolean;
}

export interface DownloadRow extends DownloadView {
	phase: string;
	name: string;
	title_id?: number;
	title: string;
	year?: number;
	version: string;
	downloader: string;
	path?: string;
	downloader_kind?: string;
	protocol?: string;
	size: number;
	started_at?: string;
	finished_at?: string;
	updated_at: string;
	slot_state?: string;
	can_act: boolean;
	client_locked?: boolean;
	replaced?: boolean;
	kind?: 'movie' | 'show' | 'artist';
	episodes?: EpisodeLine[];
	covers?: string;
	album?: string;
	artwork?: string;
	ratio?: number;
	queue_pos?: number;
	tracker?: string;
	requested_by?: string;
	source_name?: string;
	source_id?: number;
	posted_at?: string;
	tracker_problem?: 'error' | 'warning' | 'announce';
	availability?: number;
	limit_down?: number;
	limit_up?: number;
	client_hint?: 'checking' | 'moving' | 'errored' | 'missing' | 'idle_seed' | 'verifying' | 'repairing' | 'unpacking' | 'finishing' | 'script';
	limits?: { ratio: number; seeding_minutes: number; inactive_minutes: number; action: '' | 'stop' | 'remove' | 'remove_files' | 'super_seed' };
	client_item?: boolean;
	outside?: boolean;
	linked?: boolean;
	downloader_id?: number;
	hash?: string;
	category?: string;
	live_key?: string;
}

export interface EpisodeLine {
	slot_id: number;
	season: number;
	episode: number;
	label: string;
	name?: string;
	state: string;
	track?: boolean;
}

export interface Decision {
	slot_id: number;
	title_id: number;
	title: string;
	year?: number;
	version: string;
	versions: number;
	why: 'ran_out' | 'conflict' | 'not_replaced' | 'not_sent';
	reason: string;
	release_id?: number;
	release_name?: string;
	has_copy: boolean;
	since: string;
	kind?: 'movie' | 'show' | 'artist';
	episode?: EpisodeLine;
	album?: string;
	download_id?: number;
	pack_slot_ids?: number[];
	pack?: EpisodeLine[];
	artwork?: string;
}

export interface SearchView {
	job_id: number;
	kind: 'search_slot' | 'search_missing' | 'feed_check' | 'search_season' | 'search_album';
	state: string;
	by_hand: boolean;
	only_show: boolean;
	title_id?: number;
	title_name?: string;
	version?: string;
	episode?: string;
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
	ask?: AskBrief;
}

export interface ApiKey {
	id: number;
	label: string;
	hint: string;
	created_at: string;
	last_used_at?: string;
	scopes?: string[];
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
