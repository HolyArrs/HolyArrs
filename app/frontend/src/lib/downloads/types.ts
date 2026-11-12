import type { PickRulesStep } from '../core/types.ts';

export interface ImportRecord {
	ID: number;
	State: string;
	/** The download it imported; none for a by-hand import of a folder. */
	DownloadID?: number | null;
	CreatedAt: string;
	Plan?: { source_file: string; target_path?: string; confidence?: number }[] | null;
	Result?: { source_file: string; outcome: string; placed_by?: string; error?: string }[] | null;
}

/** Bounds a Release's size in bytes; with per_minute, per minute of runtime. */
export interface PickRulesSize {
	min: number;
	max: number;
	per_minute: boolean;
}

/** What a set of Pick rules takes. */
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
	/** With any, a search asks only the Sources sharing one of these tags. */
	tags?: string[];
	/** Set when a TRaSH Guides profile was applied. */
	synced?: SyncedProfile;
}

/** One of your own traits: a test on the Release name or release group, named by key in Pick rules (own:<id>). */
export interface OwnTrait {
	id: number;
	key: string;
	name: string;
	match: 'name' | 'group' | 'size';
	/** Plain words (any case) or /pattern/. */
	terms: string[];
	/** "trash" for a trait synced from TRaSH Guides, "import" for one brought from an old app, "arrapi" for one an app sent through the arr API: it tests specs, which only a sync or an import changes. */
	source?: string;
	/** For an imported trait, the old app it came from (sonarr, radarr). */
	app?: string;
	specs?: OwnSpec[];
}

/** One test of a synced trait. */
export interface OwnSpec {
	name: string;
	kind: 'name' | 'group' | 'source' | 'resolution';
	value: string;
	negate?: boolean;
	required?: boolean;
}

/** The TRaSH Guides profile last applied to a set of Pick rules. */
export interface SyncedProfile {
	from: 'trash';
	app: 'sonarr' | 'radarr';
	profile_id: string;
	name: string;
	at: string;
}

/** A TRaSH Guides profile to pick from. */
export interface TrashProfile {
	id: string;
	app: 'sonarr' | 'radarr';
	name: string;
	description: string;
	url: string;
	min_points: number;
	good_enough_points: number;
	upgrade_min_points: number;
}

/** What applying a TRaSH Guides profile did. */
export interface TrashResult {
	pick_rules: PickRulesDetail;
	added: number;
	updated: number;
	scored: { key: string; name: string; points: number }[];
	skipped: { name: string; kinds?: string[]; reason: string }[];
}

/** Counts what uses one set of Pick rules. */
export interface PickRulesUse {
	versions: number;
	library_folders: number;
	outside_lists: number;
	wishlist: number;
	title_defaults: number;
}

/** One set of Pick rules with its body and where it is used. */
export interface PickRulesDetail {
	id: number;
	name: string;
	kinds: string[];
	preset: boolean;
	revision: number;
	body: PickRulesBody;
	in_use: PickRulesUse;
}
