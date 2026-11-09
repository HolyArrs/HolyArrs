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
}

/** One of your own traits: a test on the Release name or release group, named by key in Pick rules (own:<id>). */
export interface OwnTrait {
	id: number;
	key: string;
	name: string;
	match: 'name' | 'group';
	/** Plain words (any case) or /pattern/. */
	terms: string[];
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
