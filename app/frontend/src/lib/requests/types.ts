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
	/** Which kinds can take a 4K request (4K quality set up). */
	four_k?: { movie: boolean; show: boolean };
	/** The lines of what people can ask for that this person may use, per kind, in order. */
	qualities?: { movie: QualityChoice[]; show: QualityChoice[] };
	/** People may pick lines kept as their own copy (4K) on top of the one they pick. */
	pick_many?: boolean;
}

/** One Quality button: a line of what people can ask for. */
export interface QualityChoice {
	id: string;
	/** The owner's name for it; '' shows the built-in name (Standard, 4K). */
	name: string;
	/** Kept as its own copy (4K), not the title's main copy. */
	extra: boolean;
	pick_rules_id: number;
}

/** A line of what people can ask for, as Request rules edits it. */
export interface QualityLine extends QualityChoice {
	/** The owner's own name for it, seen only where lines are set up; '' shows `name`. */
	label: string;
	hidden: boolean;
	library_folder_id: number;
	/** It can be asked for now (the 4K line needs 4K quality set up). */
	ready: boolean;
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
	/** Where approved 4K requests go and how they are picked (0: the plain folder, the quality named 4K). */
	four_k_library_folder_id: number;
	four_k_pick_rules_id: number;
	show_four_k_library_folder_id: number;
	show_four_k_pick_rules_id: number;
	/** Where approved requests for anime go (0: the same folder as other requests). */
	anime_library_folder_id: number;
	show_anime_library_folder_id: number;
	/** People may have their Plex watchlist asked for as them. */
	plex_watchlists?: boolean;
	plex_watchlists_everyone?: boolean;
	/** Also read the watchlists of people imported from Plex, with the owner's Plex link. */
	plex_friends?: boolean;
	/** The owner's own requests, Plex watchlist ones too, are approved straight away. */
	owner_auto_approve?: boolean;
	/** What people can ask for: the Quality buttons, per kind. Only names and hidden are saved. */
	qualities?: { movie: QualityLine[]; show: QualityLine[] };
	/** Let people pick more than one: a line kept as its own copy (4K) on top of the one picked. Off by default. */
	pick_many?: boolean;
	/** My streaming services: the country (two letters, '' for none) and the picked services there, in order. */
	streaming_region?: string;
	streaming_services?: number[];
	/** Requests for movies and shows on one of those services are turned down instead of downloaded. */
	skip_on_services?: boolean;
}

/** A TMDB, Trakt or MDBList list whose movies or shows are added on a schedule. */
export interface OutsideList {
	id: number;
	name: string;
	kind: 'tmdb_list' | 'trakt_list' | 'mdblist_list';
	/** What the list adds: movie or show. */
	title_kind: 'movie' | 'show';
	ref: string;
	library_folder_id: number;
	pick_rules_id: number | null;
	keep_up_to_date: boolean;
	search_on_add: boolean;
	release_gate: string;
	/** Show lists: which seasons a new show keeps up to date, and new seasons (all, none). */
	seasons: string;
	new_seasons: string;
	enabled: boolean;
	sync_every_hours: number;
	last_synced_at?: string;
	last_result: string;
	last_error: string;
}

/** A title outside lists must never add. */
export interface NeverAddEntry {
	id: number;
	kind: string;
	ids: Record<string, string>;
	name: string;
	reason: string;
	created_at: string;
}

/** The lines one person may pick, per kind; null is every line that is shown. */
export interface PersonQualities {
	movie: string[] | null;
	show: string[] | null;
}