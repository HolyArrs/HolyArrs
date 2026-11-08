// Shared types for the shows folder.
import type { Component } from 'svelte';
import type { Ask, AskBrief, LookupShow } from '../core/types.ts';

// Per-show options as the settings form edits them.
export interface ShowSettings {
	season_choice: string;
	new_seasons: string;
	episode_order: string;
	details_source: string;
	keep_specials: boolean;
	episode_wait_minutes: number;
}

// Passed in by the page because the shows folder cannot import the requests folder.
export type AskShowSheet = Component<
	{
		onasked: (show: LookupShow, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (show: LookupShow, body: Record<string, unknown>) => void;
	},
	{ open: (s: LookupShow) => void }
>;
