/** KindDefaults is what a new movie or show gets; 0 means the first folder and the folder's own Pick rules. */
export interface KindDefaults {
	library_folder_id: number;
	pick_rules_id: number;
	keep_up_to_date: boolean;
	search_on_add: boolean;
}

/** TitleDefaults are the Movies and TV defaults (GET /settings/title-defaults). */
export interface TitleDefaults {
	movie: KindDefaults;
	show: KindDefaults;
}

export interface GateDefaults {
	release_gate: string;
	gate_delay_days: number;
	bulk_search_respects_gate: boolean;
}
