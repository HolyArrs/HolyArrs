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
	/** People may have their Plex watchlist asked for as them. */
	plex_watchlists?: boolean;
	plex_watchlists_everyone?: boolean;
	/** Also read the watchlists of people imported from Plex, with the owner's Plex link. */
	plex_friends?: boolean;
}

/** A TMDB or Trakt list whose movies are added on a schedule. */
export interface OutsideList {
	id: number;
	name: string;
	kind: 'tmdb_list' | 'trakt_list';
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
