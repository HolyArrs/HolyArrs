import type { AskBrief } from '../core/types.ts';

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

export interface ShowSettings {
	season_choice: string;
	new_seasons: string;
	episode_order: string;
	details_source: string;
	keep_specials: boolean;
	episode_wait_minutes: number;
}
