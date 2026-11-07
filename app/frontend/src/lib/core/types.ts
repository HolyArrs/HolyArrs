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
	/** Tags route the title's grabs and searches. */
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
	/** Wide backdrop art (TMDB w1280), when known. */
	backdrop?: string;
	/** Title logo art (transparent PNG), when known. */
	logo?: string;
	genres?: string[];
	runtime_minutes?: number;
	/** Age rating, e.g. PG-13. */
	rating?: string;
	/** When it joined the Library. */
	added_at?: string;
	/** Every kept file together, in bytes. */
	size_bytes?: number;
	/** A movie's main file (Library list, people who manage the Library only). */
	copy?: TitleCopy;
	/** The newest grab or import. */
	last_activity?: string;
	versions: VersionView[];
	slots: SlotView[];
	/** Requests that led to it: who asked and who decided. */
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
	/** Tracks on the way, and tracks whose download failed or needs a decision. */
	downloading?: number;
	failed?: number;
	next_release?: string;
	/** The start of the artist's kept Wikipedia summary (Library list only). */
	bio?: string;
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

export interface AskAlbum {
	mbid: string;
	name: string;
	year?: number;
	/** The songs (recording ids) an approver kept; absent: every song. */
	songs?: string[];
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
	downloading?: number;
	failed?: number;
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
	/** Episodes on the way, and episodes whose download failed or needs a decision. */
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
	/** A still from the episode (w780), when known. */
	still?: string;
	keep_up_to_date: boolean;
	dropped?: boolean;
	ids: Record<string, string>;
	slots: EpisodeSlot[];
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
	/** The open request for it, if someone asked. */
	ask?: AskBrief;
	/** The open request for a 4K copy, if someone asked. */
	ask_4k?: AskBrief;
	/** The Library copy has a 4K Version already. */
	has_4k?: boolean;
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
	/** The person's chosen name, when set (the server sends it for the decider). */
	display_name?: string;
}

/** AskStep says where an approved ask is: searching, downloading, importing, waiting_on_admin, waiting (nothing found yet) or not_out. */
export interface AskStep {
	kind: 'searching' | 'downloading' | 'importing' | 'waiting_on_admin' | 'waiting' | 'not_out';
	/** Why it waits, in the search's words (people who approve only). */
	text?: string;
	/** When it was last looked for. */
	searched_at?: string;
	/** Not looked for yet: when the search for missing looks next. */
	next_search?: string;
}

/** Ask is one movie or show someone asked for on the Requests page. */
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
	/** A show ask's single episodes, by season (seasons not asked whole). */
	episodes?: { season: number; episodes: number[] }[];
	future_only?: boolean;
	/** A movie ask for a 4K copy (approved apart from the regular one). */
	four_k?: boolean;
	/** An artist ask's albums; empty is the whole artist. */
	albums?: AskAlbum[];
	/** For an artist: "every album", or the albums' names. */
	album_words?: string;
	/** An artist ask's MusicBrainz id, for its page and photo. */
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
	/** Others who declined the same title on earlier requests, newest first. */
	also_declined?: Who[];
	/** Where an approved ask is now, from the server's records. */
	step?: AskStep;
	decided_at?: string;
	note?: string;
	auto_approved: boolean;
	available_at?: string;
	/** Why an approved ask will not reach the Library, when the server knows. */
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
	/** A reported problem that was fixed (kind fixed). */
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
	/** A Telegram Alert's bot token: masked when stored, never the real one. */
	bot_token?: string;
	chat_id?: string;
	/** An ntfy or Gotify server address. */
	server_url?: string;
	/** An ntfy Alert's topic. */
	topic?: string;
	/** A Pushover Alert's user key. */
	user_key?: string;
	/** The ntfy, Gotify or Pushover token: masked when stored, never the real one. */
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
	/** The member who did it, named by the server. */
	actor_name?: string;
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
	kind: 'library_folder' | 'disk_space' | 'source' | 'downloader' | 'backups' | 'update' | 'config_disk' | 'config_folder' | 'waiting_downloads';
	subject: string;
	state: 'ok' | 'warning' | 'error';
	message: string;
	since: string;
	checked_at: string;
	/** The state the flap guard has confirmed (what Alerts and Home go by); older servers leave it out. */
	confirmed?: 'ok' | 'warning' | 'error';
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
	/** The show's TMDB id, for reading the episode's description. */
	tmdb?: string;
	air_day?: string;
	timed?: boolean;
	runtime?: number;
	network?: string;
	state?: string;
	keep_up_to_date?: boolean;
	/** A show with more than one Version: each Version's state, main first. */
	versions?: { label: string; state: string }[];
	/** Artwork from the title row; still is the episode's own picture when known. */
	poster?: string;
	backdrop?: string;
	still?: string;
	/** A movie: genres, length (runtime), directors, top-billed cast, the series it belongs to, its cinema day and first day at home. */
	genres?: string[];
	directors?: string[];
	cast?: string[];
	collection?: string;
	/** A movie's first production company. */
	studio?: string;
	cinema_date?: string;
	home_date?: string;
	/** An album: length in minutes and its place among the artist's releases of its kind (1 = the first). */
	length_min?: number;
	nth?: number;
	/** Whether the reader follows the title, and who follows it (the people the reader may see; followers_hidden counts the rest). */
	following?: boolean;
	followers?: CalendarFollower[];
	followers_hidden?: number;
}

/** One person who follows a calendar title. */
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
	/** How many places use these rules, all kinds together. */
	in_use?: number;
}

/** PickRulesStep is one rung of a quality ladder, on or off. */
export interface PickRulesStep {
	quality: string;
	allowed: boolean;
}

/** TitleCopy is a movie's main file as the Library list shows it. */
export interface TitleCopy {
	file_id: number;
	version_id: number;
	/** h264, hevc, av1, vp9, mpeg2, xvid, vc1 or unknown. */
	codec: string;
	container: string;
	audio_languages: string[];
	protected: boolean;
}

/** DownloadView is a Download in plain words (the movie page). */
export interface DownloadView {
	id: number;
	state: string;
	label: string;
	text: string;
	progress: number;
	speed: number;
	/** Upload speed, seeds and peers (torrents; older servers omit them). */
	up_speed?: number;
	seeds?: number;
	peers?: number;
	seeds_total?: number;
	peers_total?: number;
	eta?: number;
	active: boolean;
}

/** DownloadRow is one row of the Downloads screen. */
export interface DownloadRow extends DownloadView {
	phase: string;
	name: string;
	title_id?: number;
	title: string;
	year?: number;
	version: string;
	downloader: string;
	/** The folder or file the Downloader holds it in, as it sees it. */
	path?: string;
	/** The Downloader's adapter (qbittorrent, sabnzbd...) and its protocol. */
	downloader_kind?: string;
	protocol?: string;
	size: number;
	started_at?: string;
	finished_at?: string;
	updated_at: string;
	slot_state?: string;
	can_act: boolean;
	/** HolyArrs may not change this row's Downloader: only its own records (Remove, Mark failed). */
	client_locked?: boolean;
	/** A failed copy HolyArrs turned down and moved on from (searching again or another copy on its way). */
	replaced?: boolean;
	kind?: 'movie' | 'show' | 'artist';
	episodes?: EpisodeLine[];
	covers?: string;
	album?: string;
	/** Optional artwork supplied with an activity row; Library metadata remains the fallback. */
	artwork?: string;
	/** A seeding torrent's share ratio, when the Downloader reports one. */
	ratio?: number;
	/** A client torrent's own seeding rules (-2 the Downloader's rule, -1 none). */
	/** A client torrent's finer state: checking, moving, errored, missing (files gone) or idle_seed (finished, nobody asking now). */
	/** A client torrent's place in the Downloader's queue (absent: not queued). */
	queue_pos?: number;
	/** The host of the tracker a client torrent works with now. */
	tracker?: string;
	/** Who asked for the row's title ("Added in <client>" when nobody did; absent with no title). */
	requested_by?: string;
	/** The Source the release came from (downloads HolyArrs sent). */
	source_name?: string;
	source_id?: number;
	/** When the Usenet post was made (RFC3339). */
	posted_at?: string;
	/** Trackers not all well: error, warning, or announce (none answering). */
	tracker_problem?: 'error' | 'warning' | 'announce';
	/** A downloading torrent's full copies in the swarm (under 1: nobody holds every piece). */
	availability?: number;
	/** A client torrent's own speed caps in bytes per second (absent: none). */
	limit_down?: number;
	limit_up?: number;
	client_hint?: 'checking' | 'moving' | 'errored' | 'missing' | 'idle_seed' | 'verifying' | 'repairing' | 'unpacking' | 'finishing' | 'script';
	limits?: { ratio: number; seeding_minutes: number; inactive_minutes: number; action: '' | 'stop' | 'remove' | 'remove_files' | 'super_seed' };
	/** Read straight from the client's own list: no HolyArrs download id; addressed by downloader_id and hash. */
	client_item?: boolean;
	/** HolyArrs did not send it (a torrent added in the client by hand or by another app). */
	outside?: boolean;
	/** An outside torrent matched to a Library title: title, kind and covers come from that match. */
	linked?: boolean;
	downloader_id?: number;
	hash?: string;
	/** The client's own category for it. */
	category?: string;
	/** The row's key in push updates: "<downloader id>:<external id>". */
	live_key?: string;
}

/** EpisodeLine is one episode (or track) a download covers, with where its
 *  Slot is; a track carries its disc in season and its number in episode. */
export interface EpisodeLine {
	slot_id: number;
	season: number;
	episode: number;
	label: string;
	name?: string;
	state: string;
	track?: boolean;
}

/** Decision is one movie on Needs a decision. */
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
	/** Optional artwork supplied with a decision; Library metadata remains the fallback. */
	artwork?: string;
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
	/** The open request for it, if someone asked. */
	ask?: AskBrief;
}
