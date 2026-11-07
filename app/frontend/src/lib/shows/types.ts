import type { AskBrief } from '../core/types.ts';

export interface ShowSettings {
	season_choice: string;
	new_seasons: string;
	episode_order: string;
	details_source: string;
	keep_specials: boolean;
	episode_wait_minutes: number;
}
