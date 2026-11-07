import type { Component } from 'svelte';
import type { Ask, AskBrief, LookupShow } from '../core/types.ts';

export interface ShowSettings {
	season_choice: string;
	new_seasons: string;
	episode_order: string;
	details_source: string;
	keep_specials: boolean;
	episode_wait_minutes: number;
}

/** AskShowSheet is the ask dialog, handed in by the page because a shows part cannot import requests. */
export type AskShowSheet = Component<
	{
		onasked: (show: LookupShow, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (show: LookupShow, body: Record<string, unknown>) => void;
	},
	{ open: (s: LookupShow) => void }
>;
